package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultGoplsTimeout = 10 * time.Second

// ── Wire types ────────────────────────────────────────────────────────────────

// lspMessage is a single JSON-RPC 2.0 frame decoded from the gopls stream.
type lspMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// lspPos / lspRng / lspLoc are the JSON wire representations used by gopls.
// They are kept separate from the exported Position/Range/Location to avoid
// confusion between "what the client speaks" and "what the interface exposes".
type lspPos struct {
	Line      uint32 `json:"line"`
	Character uint32 `json:"character"`
}

type lspRng struct {
	Start lspPos `json:"start"`
	End   lspPos `json:"end"`
}

type lspLoc struct {
	URI   string `json:"uri"`
	Range lspRng `json:"range"`
}

type lspDiagItem struct {
	Range    lspRng          `json:"range"`
	Severity *int            `json:"severity"`
	Code     json.RawMessage `json:"code"`
	Message  string          `json:"message"`
	Source   string          `json:"source"`
}

// ── GoplsClient ───────────────────────────────────────────────────────────────

// GoplsClient implements Client by spawning a gopls subprocess and
// communicating with it over its stdin/stdout stdio transport.
//
// The LSP initialize handshake is deferred to the first method call so the
// constructor returns quickly.  Create a client with NewGoplsClient.
type GoplsClient struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser

	writeMu sync.Mutex // serializes writes to stdin
	nextID  atomic.Int64

	rootURI string
	timeout time.Duration

	initOnce sync.Once
	initErr  error

	mu          sync.Mutex // protects closed, openedFiles
	closed      bool
	openedFiles map[string]struct{} // URI → already sent didOpen

	pendingMu sync.Mutex
	pending   map[int64]chan lspMessage

	diagMu    sync.RWMutex
	diagCache map[string][]Diagnostic // URI → diagnostics from publishDiagnostics
}

// NewGoplsClient starts gopls in stdio mode rooted at rootDir and returns a
// ready-to-use Client.  gopls must be on $PATH.
func NewGoplsClient(rootDir string, timeout time.Duration) (*GoplsClient, error) {
	if timeout <= 0 {
		timeout = defaultGoplsTimeout
	}
	abs, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("gopls: resolve root dir: %w", err)
	}

	cmd := exec.Command("gopls", "-mode=stdio")
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("gopls: stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("gopls: stdout pipe: %w", err)
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("gopls: start process: %w", err)
	}

	c := &GoplsClient{
		cmd:         cmd,
		stdin:       stdinPipe,
		rootURI:     pathToFileURI(abs),
		timeout:     timeout,
		openedFiles: make(map[string]struct{}),
		pending:     make(map[int64]chan lspMessage),
		diagCache:   make(map[string][]Diagnostic),
	}
	go c.readLoop(stdoutPipe)
	return c, nil
}

// ── Client interface ──────────────────────────────────────────────────────────

// Language implements Client.
func (c *GoplsClient) Language() string { return ".go" }

// Definition implements Client.
func (c *GoplsClient) Definition(ctx context.Context, file string, pos Position) ([]Location, error) {
	if err := c.prepareFile(ctx, file); err != nil {
		return nil, err
	}
	raw, err := c.request(ctx, "textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": pathToFileURI(file)},
		"position":     posToWire(pos),
	})
	if err != nil {
		return nil, err
	}
	return decodeLocations(raw)
}

// References implements Client.
func (c *GoplsClient) References(ctx context.Context, file string, pos Position, includeDecl bool) ([]Location, error) {
	if err := c.prepareFile(ctx, file); err != nil {
		return nil, err
	}
	raw, err := c.request(ctx, "textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": pathToFileURI(file)},
		"position":     posToWire(pos),
		"context":      map[string]any{"includeDeclaration": includeDecl},
	})
	if err != nil {
		return nil, err
	}
	return decodeLocations(raw)
}

// IncomingCalls implements Client.
func (c *GoplsClient) IncomingCalls(ctx context.Context, file string, pos Position) ([]CallSite, error) {
	item, calleeSymbol, err := c.prepareCallHierarchy(ctx, file, pos)
	if err != nil || item == nil {
		return nil, err
	}
	raw, err := c.request(ctx, "callHierarchy/incomingCalls", map[string]any{"item": item})
	if err != nil || raw == nil || string(raw) == "null" {
		return nil, err
	}

	var calls []struct {
		From struct {
			Name   string `json:"name"`
			Detail string `json:"detail"`
			URI    string `json:"uri"`
			Range  lspRng `json:"selectionRange"`
		} `json:"from"`
		FromRanges []lspRng `json:"fromRanges"`
	}
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil, fmt.Errorf("gopls: decode incomingCalls: %w", err)
	}

	out := make([]CallSite, 0, len(calls))
	for _, it := range calls {
		caller := it.From.Name
		if it.From.Detail != "" {
			caller = it.From.Detail + "." + it.From.Name
		}
		loc := Location{URI: it.From.URI}
		if len(it.FromRanges) > 0 {
			loc.Range = wireRangeToRange(it.FromRanges[0])
		} else {
			loc.Range = wireRangeToRange(it.From.Range)
		}
		out = append(out, CallSite{CallerSymbol: caller, CalleeSymbol: calleeSymbol, Location: loc})
	}
	return out, nil
}

// OutgoingCalls implements Client.
func (c *GoplsClient) OutgoingCalls(ctx context.Context, file string, pos Position) ([]CallSite, error) {
	item, callerSymbol, err := c.prepareCallHierarchy(ctx, file, pos)
	if err != nil || item == nil {
		return nil, err
	}
	raw, err := c.request(ctx, "callHierarchy/outgoingCalls", map[string]any{"item": item})
	if err != nil || raw == nil || string(raw) == "null" {
		return nil, err
	}

	var calls []struct {
		To struct {
			Name   string `json:"name"`
			Detail string `json:"detail"`
		} `json:"to"`
		FromRanges []lspRng `json:"fromRanges"`
	}
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil, fmt.Errorf("gopls: decode outgoingCalls: %w", err)
	}

	fileURI := pathToFileURI(file)
	out := make([]CallSite, 0, len(calls))
	for _, it := range calls {
		callee := it.To.Name
		if it.To.Detail != "" {
			callee = it.To.Detail + "." + it.To.Name
		}
		loc := Location{URI: fileURI}
		if len(it.FromRanges) > 0 {
			loc.Range = wireRangeToRange(it.FromRanges[0])
		}
		out = append(out, CallSite{CallerSymbol: callerSymbol, CalleeSymbol: callee, Location: loc})
	}
	return out, nil
}

// DocumentSymbols implements Client.
func (c *GoplsClient) DocumentSymbols(ctx context.Context, file string) ([]Symbol, error) {
	if err := c.prepareFile(ctx, file); err != nil {
		return nil, err
	}
	raw, err := c.request(ctx, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]any{"uri": pathToFileURI(file)},
	})
	if err != nil {
		return nil, err
	}
	return decodeSymbolInformation(raw, pathToFileURI(file))
}

// WorkspaceSymbols implements Client.
func (c *GoplsClient) WorkspaceSymbols(ctx context.Context, query string) ([]Symbol, error) {
	if err := c.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	raw, err := c.request(ctx, "workspace/symbol", map[string]any{"query": query})
	if err != nil {
		return nil, err
	}
	return decodeSymbolInformation(raw, "")
}

// Imports parses import statements directly from the Go source file using
// go/parser.  gopls does not expose a dedicated LSP method for imports, so
// AST parsing is more reliable than inferring from document symbols.
func (c *GoplsClient) Imports(_ context.Context, file string) ([]Import, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("gopls: parse imports %q: %w", file, err)
	}
	out := make([]Import, 0, len(f.Imports))
	for _, spec := range f.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		alias := ""
		if spec.Name != nil && spec.Name.Name != "_" && spec.Name.Name != "." {
			alias = spec.Name.Name
		}
		out = append(out, Import{Path: path, Alias: alias, IsStdlib: isGoStdlib(path)})
	}
	return out, nil
}

// Hover implements Client.
func (c *GoplsClient) Hover(ctx context.Context, file string, pos Position) (string, error) {
	if err := c.prepareFile(ctx, file); err != nil {
		return "", err
	}
	raw, err := c.request(ctx, "textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": pathToFileURI(file)},
		"position":     posToWire(pos),
	})
	if err != nil || raw == nil || string(raw) == "null" {
		return "", err
	}
	var resp struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("gopls: decode hover: %w", err)
	}
	return extractHoverText(resp.Contents), nil
}

// CanonicalName implements Client by combining Definition + DocumentSymbols.
func (c *GoplsClient) CanonicalName(ctx context.Context, file string, pos Position) (string, error) {
	locs, err := c.Definition(ctx, file, pos)
	if err != nil || len(locs) == 0 {
		return "", err
	}
	defLoc := locs[0]
	defPath, err := fileURIToPath(defLoc.URI)
	if err != nil {
		return "", err
	}
	syms, err := c.DocumentSymbols(ctx, defPath)
	if err != nil {
		return "", err
	}
	for _, s := range syms {
		if s.Location.URI == defLoc.URI && rangeContains(s.Location.Range, defLoc.Range.Start) {
			if s.Canonical != "" {
				return s.Canonical, nil
			}
			return s.Name, nil
		}
	}
	return "", nil
}

// Diagnostics implements Client.  It first tries the LSP 3.17 pull model
// (textDocument/diagnostic, supported by gopls 0.11+) and falls back to
// diagnostics received via publishDiagnostics push notifications.
func (c *GoplsClient) Diagnostics(ctx context.Context, file string) ([]Diagnostic, error) {
	if err := c.prepareFile(ctx, file); err != nil {
		return nil, err
	}
	uri := pathToFileURI(file)

	raw, err := c.request(ctx, "textDocument/diagnostic", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
	if err == nil && raw != nil && string(raw) != "null" {
		var report struct {
			Kind  string        `json:"kind"`
			Items []lspDiagItem `json:"items"`
		}
		if json.Unmarshal(raw, &report) == nil && report.Kind == "full" {
			return decodeDiagItems(report.Items), nil
		}
	}

	// Fall back to cached push diagnostics.
	c.diagMu.RLock()
	cached := c.diagCache[uri]
	c.diagMu.RUnlock()
	return cached, nil
}

// Close implements Client.
func (c *GoplsClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Best-effort: gopls may not respond if it was never initialized.
	_, _ = c.doRequest(ctx, "shutdown", nil)
	_ = c.doNotify("exit", nil)

	_ = c.stdin.Close()
	return c.cmd.Wait()
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// ensureInitialized sends the LSP initialize handshake exactly once.
func (c *GoplsClient) ensureInitialized(ctx context.Context) error {
	c.initOnce.Do(func() { c.initErr = c.doInitialize(ctx) })
	return c.initErr
}

func (c *GoplsClient) doInitialize(ctx context.Context) error {
	_, err := c.doRequest(ctx, "initialize", map[string]any{
		"processId": os.Getpid(),
		"rootUri":   c.rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"callHierarchy":  map[string]any{"dynamicRegistration": false},
				"documentSymbol": map[string]any{"hierarchicalDocumentSymbolSupport": false},
				"hover":          map[string]any{"contentFormat": []string{"plaintext", "markdown"}},
				"diagnostic":     map[string]any{"dynamicRegistration": false},
			},
			"workspace": map[string]any{
				"symbol": map[string]any{"dynamicRegistration": false},
			},
		},
		"clientInfo": map[string]any{"name": "postbrain", "version": "0"},
	})
	if err != nil {
		return fmt.Errorf("gopls: initialize: %w", err)
	}
	return c.doNotify("initialized", map[string]any{})
}

// prepareFile ensures the LSP is initialized and the file is open.
func (c *GoplsClient) prepareFile(ctx context.Context, file string) error {
	if err := c.ensureInitialized(ctx); err != nil {
		return err
	}
	return c.openFile(file)
}

// openFile sends textDocument/didOpen the first time a file is accessed.
func (c *GoplsClient) openFile(file string) error {
	uri := pathToFileURI(file)

	c.mu.Lock()
	if _, ok := c.openedFiles[uri]; ok {
		c.mu.Unlock()
		return nil
	}
	c.openedFiles[uri] = struct{}{} // mark before releasing lock to prevent double-open
	c.mu.Unlock()

	data, err := os.ReadFile(file)
	if err != nil {
		c.mu.Lock()
		delete(c.openedFiles, uri)
		c.mu.Unlock()
		return fmt.Errorf("gopls: read file %q: %w", file, err)
	}
	return c.doNotify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": "go",
			"version":    1,
			"text":       string(data),
		},
	})
}

// prepareCallHierarchy calls textDocument/prepareCallHierarchy and returns the
// raw CallHierarchyItem JSON (for re-use in incomingCalls/outgoingCalls),
// and the qualified name of the target symbol.
func (c *GoplsClient) prepareCallHierarchy(ctx context.Context, file string, pos Position) (json.RawMessage, string, error) {
	if err := c.prepareFile(ctx, file); err != nil {
		return nil, "", err
	}
	raw, err := c.request(ctx, "textDocument/prepareCallHierarchy", map[string]any{
		"textDocument": map[string]any{"uri": pathToFileURI(file)},
		"position":     posToWire(pos),
	})
	if err != nil || raw == nil || string(raw) == "null" {
		return nil, "", err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return nil, "", err
	}
	var first struct {
		Name   string `json:"name"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(items[0], &first)
	sym := first.Name
	if first.Detail != "" {
		sym = first.Detail + "." + first.Name
	}
	return items[0], sym, nil
}

// ── JSON-RPC transport ────────────────────────────────────────────────────────

// request sends a JSON-RPC request after ensuring the client is initialized,
// and waits for the matching response.
func (c *GoplsClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil, fmt.Errorf("gopls: client is closed")
	}
	if err := c.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	return c.doRequest(ctx, method, params)
}

// doRequest sends a JSON-RPC request without checking initialized state.
func (c *GoplsClient) doRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan lspMessage, 1)

	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.writeFrame(method, id, params); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	timeout := c.timeout
	if dl, ok := ctx.Deadline(); ok {
		if r := time.Until(dl); r > 0 && r < timeout {
			timeout = r
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case msg := <-ch:
		if len(msg.Error) > 0 && string(msg.Error) != "null" {
			return nil, fmt.Errorf("gopls: %s: %s", method, msg.Error)
		}
		return msg.Result, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-timer.C:
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("gopls: %s timed out after %v", method, timeout)
	}
}

// doNotify sends a JSON-RPC notification (no response expected).
func (c *GoplsClient) doNotify(method string, params any) error {
	return c.writeFrame(method, nil, params)
}

// writeFrame marshals a JSON-RPC frame and writes it to gopls stdin.
func (c *GoplsClient) writeFrame(method string, id any, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if id != nil {
		msg["id"] = id
	}
	if params != nil {
		msg["params"] = params
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("gopls: marshal %s: %w", method, err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return fmt.Errorf("gopls: write header: %w", err)
	}
	if _, err := c.stdin.Write(payload); err != nil {
		return fmt.Errorf("gopls: write body: %w", err)
	}
	return nil
}

// readLoop is the background goroutine that reads all frames from gopls stdout
// and dispatches responses to waiting callers or handles server notifications.
func (c *GoplsClient) readLoop(r io.Reader) {
	reader := bufio.NewReader(r)
	for {
		msg, err := readLSPFrame(reader)
		if err != nil {
			// EOF or connection closed: unblock all pending requests.
			closeErr := json.RawMessage(fmt.Sprintf("%q", "connection closed: "+err.Error()))
			c.pendingMu.Lock()
			for id, ch := range c.pending {
				ch <- lspMessage{Error: closeErr}
				delete(c.pending, id)
			}
			c.pendingMu.Unlock()
			return
		}

		// A non-null ID means it's a response to one of our requests.
		if len(msg.ID) > 0 && string(msg.ID) != "null" {
			var id int64
			if json.Unmarshal(msg.ID, &id) == nil {
				c.pendingMu.Lock()
				ch, ok := c.pending[id]
				if ok {
					delete(c.pending, id)
				}
				c.pendingMu.Unlock()
				if ok {
					ch <- msg
				}
			}
			continue
		}

		// Server-to-client notification.
		if msg.Method == "textDocument/publishDiagnostics" && len(msg.Params) > 0 {
			c.handlePublishDiagnostics(msg.Params)
		}
	}
}

func (c *GoplsClient) handlePublishDiagnostics(raw json.RawMessage) {
	var p struct {
		URI   string        `json:"uri"`
		Items []lspDiagItem `json:"diagnostics"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	diags := decodeDiagItems(p.Items)
	c.diagMu.Lock()
	c.diagCache[p.URI] = diags
	c.diagMu.Unlock()
}

// readLSPFrame reads one Content-Length-framed JSON-RPC message.
func readLSPFrame(reader *bufio.Reader) (lspMessage, error) {
	headers := make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return lspMessage{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}

	clStr, ok := headers["Content-Length"]
	if !ok {
		return lspMessage{}, fmt.Errorf("gopls: missing Content-Length header")
	}
	var n int
	if _, err := fmt.Sscanf(clStr, "%d", &n); err != nil {
		return lspMessage{}, fmt.Errorf("gopls: invalid Content-Length %q: %w", clStr, err)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(reader, body); err != nil {
		return lspMessage{}, fmt.Errorf("gopls: read body: %w", err)
	}
	var msg lspMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return lspMessage{}, fmt.Errorf("gopls: unmarshal: %w", err)
	}
	return msg, nil
}

// ── Decode helpers ────────────────────────────────────────────────────────────

// decodeLocations handles both a single Location object and a Location array.
func decodeLocations(raw json.RawMessage) ([]Location, error) {
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}
	var arr []lspLoc
	if err := json.Unmarshal(raw, &arr); err == nil {
		out := make([]Location, len(arr))
		for i, l := range arr {
			out[i] = wireLoc(l)
		}
		return out, nil
	}
	var single lspLoc
	if err := json.Unmarshal(raw, &single); err == nil && single.URI != "" {
		return []Location{wireLoc(single)}, nil
	}
	return nil, nil
}

// decodeSymbolInformation handles flat SymbolInformation[] returned by gopls.
func decodeSymbolInformation(raw json.RawMessage, defaultURI string) ([]Symbol, error) {
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}
	var items []struct {
		Name          string `json:"name"`
		Kind          int    `json:"kind"`
		ContainerName string `json:"containerName"`
		Location      lspLoc `json:"location"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("gopls: decode symbols: %w", err)
	}
	out := make([]Symbol, 0, len(items))
	for _, it := range items {
		uri := it.Location.URI
		if uri == "" {
			uri = defaultURI
		}
		canonical := it.Name
		if it.ContainerName != "" {
			canonical = it.ContainerName + "." + it.Name
		}
		out = append(out, Symbol{
			Name:      it.Name,
			Canonical: canonical,
			Kind:      lspKindToSymbolKind(it.Kind),
			Location:  wireLoc(lspLoc{URI: uri, Range: it.Location.Range}),
		})
	}
	return out, nil
}

func decodeDiagItems(items []lspDiagItem) []Diagnostic {
	out := make([]Diagnostic, 0, len(items))
	for _, it := range items {
		d := Diagnostic{Range: wireRangeToRange(it.Range), Message: it.Message, Source: it.Source}
		if it.Severity != nil {
			d.Severity = DiagnosticSeverity(*it.Severity)
		}
		if len(it.Code) > 0 && string(it.Code) != "null" {
			var s string
			if json.Unmarshal(it.Code, &s) == nil {
				d.Code = s
			} else {
				var n int
				if json.Unmarshal(it.Code, &n) == nil {
					d.Code = fmt.Sprintf("%d", n)
				}
			}
		}
		out = append(out, d)
	}
	return out
}

// extractHoverText handles the three forms gopls may return for hover contents:
// plain string, MarkupContent {kind, value}, or []MarkedString.
func extractHoverText(raw json.RawMessage) string {
	if raw == nil || string(raw) == "null" {
		return ""
	}
	// MarkupContent: {"kind": "...", "value": "..."}
	var mc struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &mc) == nil && mc.Value != "" {
		return mc.Value
	}
	// Plain string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// Array of MarkedString (deprecated but some servers still use it)
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			var plain string
			if json.Unmarshal(item, &plain) == nil {
				parts = append(parts, plain)
				continue
			}
			var ms struct {
				Value string `json:"value"`
			}
			if json.Unmarshal(item, &ms) == nil {
				parts = append(parts, ms.Value)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// lspKindToSymbolKind maps LSP SymbolKind integers to our SymbolKind constants.
func lspKindToSymbolKind(k int) SymbolKind {
	switch k {
	case 1:
		return KindFile
	case 2, 3, 4: // Module, Namespace, Package
		return KindModule
	case 5, 23: // Class, Struct
		return KindClass
	case 6:
		return KindMethod
	case 9:
		return KindConstructor
	case 10, 26: // Enum, TypeParameter
		return KindType
	case 11: // Interface
		return KindInterface
	case 12: // Function
		return KindFunction
	case 7, 8, 13, 14: // Property, Field, Variable, Constant
		return KindVariable
	default:
		return KindUnknown
	}
}

// ── Coordinate / URI helpers ─────────────────────────────────────────────────

func posToWire(p Position) lspPos { return lspPos{Line: p.Line, Character: p.Character} }

func wireRangeToRange(r lspRng) Range {
	return Range{
		Start: Position{Line: r.Start.Line, Character: r.Start.Character},
		End:   Position{Line: r.End.Line, Character: r.End.Character},
	}
}

func wireLoc(l lspLoc) Location {
	return Location{URI: l.URI, Range: wireRangeToRange(l.Range)}
}

// rangeContains reports whether Range r contains Position p (inclusive).
func rangeContains(r Range, p Position) bool {
	if p.Line < r.Start.Line || p.Line > r.End.Line {
		return false
	}
	if p.Line == r.Start.Line && p.Character < r.Start.Character {
		return false
	}
	if p.Line == r.End.Line && p.Character > r.End.Character {
		return false
	}
	return true
}

// pathToFileURI converts an absolute OS path to a file:// URI.
func pathToFileURI(path string) string {
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

// fileURIToPath converts a file:// URI back to an OS path.
func fileURIToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("gopls: parse URI %q: %w", uri, err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("gopls: non-file URI scheme %q", u.Scheme)
	}
	return filepath.FromSlash(u.Path), nil
}

// isGoStdlib reports whether a Go import path is from the standard library.
// Standard-library paths have no dot in the first path component.
func isGoStdlib(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}
