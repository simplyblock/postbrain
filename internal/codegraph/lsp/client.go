package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

const defaultStdioTimeout = 10 * time.Second

// ── Wire types ────────────────────────────────────────────────────────────────

type lspMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

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

// ── stdioClient ───────────────────────────────────────────────────────────────

type stdioClient struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser

	writeMu sync.Mutex
	nextID  atomic.Int64

	rootURI    string
	timeout    time.Duration
	languages  map[string]int
	languageID string

	initOnce sync.Once
	initErr  error

	mu          sync.Mutex
	closed      bool
	openedFiles map[string]struct{}

	pendingMu sync.Mutex
	pending   map[int64]chan lspMessage

	diagMu    sync.RWMutex
	diagCache map[string][]Diagnostic
}

func newStdioClient(command string, args []string, languages map[string]int, languageID, rootDir string, timeout time.Duration) (*stdioClient, error) {
	if timeout <= 0 {
		timeout = defaultStdioTimeout
	}
	abs, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve root dir: %w", err)
	}

	cmd := exec.Command(command, args...)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	c := &stdioClient{
		cmd:         cmd,
		stdin:       stdinPipe,
		rootURI:     pathToFileURI(abs),
		timeout:     timeout,
		languages:   normalizeSupportedLanguages(languages),
		languageID:  languageID,
		openedFiles: make(map[string]struct{}),
		pending:     make(map[int64]chan lspMessage),
		diagCache:   make(map[string][]Diagnostic),
	}
	go c.readLoop(stdoutPipe)
	return c, nil
}

// ── Client interface methods ──────────────────────────────────────────────────

func (c *stdioClient) SupportedLanguages() map[string]int {
	out := make(map[string]int, len(c.languages))
	for ext, prio := range c.languages {
		out[ext] = prio
	}
	return out
}

func (c *stdioClient) Definition(ctx context.Context, file string, pos Position) ([]Location, error) {
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

func (c *stdioClient) References(ctx context.Context, file string, pos Position, includeDecl bool) ([]Location, error) {
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

func (c *stdioClient) IncomingCalls(ctx context.Context, file string, pos Position) ([]CallSite, error) {
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
		return nil, fmt.Errorf("lsp: decode incomingCalls: %w", err)
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

func (c *stdioClient) OutgoingCalls(ctx context.Context, file string, pos Position) ([]CallSite, error) {
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
		return nil, fmt.Errorf("lsp: decode outgoingCalls: %w", err)
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

func (c *stdioClient) DocumentSymbols(ctx context.Context, file string) ([]Symbol, error) {
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

func (c *stdioClient) WorkspaceSymbols(ctx context.Context, query string) ([]Symbol, error) {
	if err := c.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	raw, err := c.request(ctx, "workspace/symbol", map[string]any{"query": query})
	if err != nil {
		return nil, err
	}
	return decodeSymbolInformation(raw, "")
}

func (c *stdioClient) Hover(ctx context.Context, file string, pos Position) (string, error) {
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
		return "", fmt.Errorf("lsp: decode hover: %w", err)
	}
	return extractHoverText(resp.Contents), nil
}

func (c *stdioClient) CanonicalName(ctx context.Context, file string, pos Position) (string, error) {
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

func (c *stdioClient) Diagnostics(ctx context.Context, file string) ([]Diagnostic, error) {
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

	c.diagMu.RLock()
	cached := c.diagCache[uri]
	c.diagMu.RUnlock()
	return cached, nil
}

func (c *stdioClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = c.doRequest(ctx, "shutdown", nil)
	_ = c.doNotify("exit", nil)

	_ = c.stdin.Close()
	return c.cmd.Wait()
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (c *stdioClient) ensureInitialized(ctx context.Context) error {
	c.initOnce.Do(func() { c.initErr = c.doInitialize(ctx) })
	return c.initErr
}

func (c *stdioClient) doInitialize(ctx context.Context) error {
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
		return fmt.Errorf("lsp: initialize: %w", err)
	}
	return c.doNotify("initialized", map[string]any{})
}

func (c *stdioClient) prepareFile(ctx context.Context, file string) error {
	if err := c.ensureInitialized(ctx); err != nil {
		return err
	}
	return c.openFile(file)
}

func (c *stdioClient) openFile(file string) error {
	uri := pathToFileURI(file)

	c.mu.Lock()
	if _, ok := c.openedFiles[uri]; ok {
		c.mu.Unlock()
		return nil
	}
	c.openedFiles[uri] = struct{}{}
	c.mu.Unlock()

	data, err := os.ReadFile(file)
	if err != nil {
		c.mu.Lock()
		delete(c.openedFiles, uri)
		c.mu.Unlock()
		return fmt.Errorf("lsp: read file %q: %w", file, err)
	}
	return c.doNotify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": c.languageID,
			"version":    1,
			"text":       string(data),
		},
	})
}

func (c *stdioClient) prepareCallHierarchy(ctx context.Context, file string, pos Position) (json.RawMessage, string, error) {
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

func (c *stdioClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil, fmt.Errorf("lsp: client is closed")
	}
	if err := c.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	return c.doRequest(ctx, method, params)
}

func (c *stdioClient) doRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
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
			return nil, fmt.Errorf("lsp: %s: %s", method, msg.Error)
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
		return nil, fmt.Errorf("lsp: %s timed out after %v", method, timeout)
	}
}

func (c *stdioClient) doNotify(method string, params any) error {
	return c.writeFrame(method, nil, params)
}

func (c *stdioClient) writeFrame(method string, id any, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if id != nil {
		msg["id"] = id
	}
	if params != nil {
		msg["params"] = params
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("lsp: marshal %s: %w", method, err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return fmt.Errorf("lsp: write header: %w", err)
	}
	if _, err := c.stdin.Write(payload); err != nil {
		return fmt.Errorf("lsp: write body: %w", err)
	}
	return nil
}

func normalizeSupportedLanguages(in map[string]int) map[string]int {
	if len(in) == 0 {
		return map[string]int{}
	}
	out := make(map[string]int, len(in))
	for ext, prio := range in {
		normalizedExt := strings.ToLower(strings.TrimSpace(ext))
		if normalizedExt == "" {
			continue
		}
		if !strings.HasPrefix(normalizedExt, ".") {
			normalizedExt = "." + normalizedExt
		}
		if prio < 1 {
			prio = 1
		}
		if prio > 100 {
			prio = 100
		}
		out[normalizedExt] = prio
	}
	return out
}

func (c *stdioClient) readLoop(r io.Reader) {
	reader := bufio.NewReader(r)
	for {
		msg, err := readLSPFrame(reader)
		if err != nil {
			closeErr := json.RawMessage(fmt.Sprintf("%q", "connection closed: "+err.Error()))
			c.pendingMu.Lock()
			for id, ch := range c.pending {
				ch <- lspMessage{Error: closeErr}
				delete(c.pending, id)
			}
			c.pendingMu.Unlock()
			return
		}

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

		if msg.Method == "textDocument/publishDiagnostics" && len(msg.Params) > 0 {
			c.handlePublishDiagnostics(msg.Params)
		}
	}
}

func (c *stdioClient) handlePublishDiagnostics(raw json.RawMessage) {
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
		return lspMessage{}, fmt.Errorf("lsp: missing Content-Length header")
	}
	var n int
	if _, err := fmt.Sscanf(clStr, "%d", &n); err != nil {
		return lspMessage{}, fmt.Errorf("lsp: invalid Content-Length %q: %w", clStr, err)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(reader, body); err != nil {
		return lspMessage{}, fmt.Errorf("lsp: read body: %w", err)
	}
	var msg lspMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return lspMessage{}, fmt.Errorf("lsp: unmarshal: %w", err)
	}
	return msg, nil
}

// ── Decode helpers ────────────────────────────────────────────────────────────

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

func decodeSymbolInformation(raw json.RawMessage, defaultURI string) ([]Symbol, error) {
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}
	var infoItems []struct {
		Name          string `json:"name"`
		Kind          int    `json:"kind"`
		ContainerName string `json:"containerName"`
		Location      lspLoc `json:"location"`
	}
	if err := json.Unmarshal(raw, &infoItems); err == nil {
		hasSymbolInfoLocation := false
		for _, it := range infoItems {
			if it.Location.URI != "" || it.Location.Range != (lspRng{}) {
				hasSymbolInfoLocation = true
				break
			}
		}
		if hasSymbolInfoLocation {
			out := make([]Symbol, 0, len(infoItems))
			for _, it := range infoItems {
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
	}

	// Some servers (including gopls) return []DocumentSymbol where the
	// identifier position is in selectionRange, not range/location.
	var docItems []struct {
		Name           string `json:"name"`
		Kind           int    `json:"kind"`
		Range          lspRng `json:"range"`
		SelectionRange lspRng `json:"selectionRange"`
		Children       []struct {
			Name           string `json:"name"`
			Kind           int    `json:"kind"`
			Range          lspRng `json:"range"`
			SelectionRange lspRng `json:"selectionRange"`
			Children       []struct {
				Name           string `json:"name"`
				Kind           int    `json:"kind"`
				Range          lspRng `json:"range"`
				SelectionRange lspRng `json:"selectionRange"`
			} `json:"children"`
		} `json:"children"`
	}
	if err := json.Unmarshal(raw, &docItems); err != nil {
		return nil, fmt.Errorf("lsp: decode symbols: %w", err)
	}
	if len(docItems) == 0 {
		return nil, nil
	}

	out := make([]Symbol, 0, len(docItems))
	appendDocSymbols := func(prefix string, items []struct {
		Name           string `json:"name"`
		Kind           int    `json:"kind"`
		Range          lspRng `json:"range"`
		SelectionRange lspRng `json:"selectionRange"`
		Children       []struct {
			Name           string `json:"name"`
			Kind           int    `json:"kind"`
			Range          lspRng `json:"range"`
			SelectionRange lspRng `json:"selectionRange"`
			Children       []struct {
				Name           string `json:"name"`
				Kind           int    `json:"kind"`
				Range          lspRng `json:"range"`
				SelectionRange lspRng `json:"selectionRange"`
			} `json:"children"`
		} `json:"children"`
	}) {
		for _, it := range items {
			canonical := it.Name
			if prefix != "" {
				canonical = prefix + "." + it.Name
			}
			rng := it.SelectionRange
			if rng == (lspRng{}) {
				rng = it.Range
			}
			out = append(out, Symbol{
				Name:      it.Name,
				Canonical: canonical,
				Kind:      lspKindToSymbolKind(it.Kind),
				Location:  wireLoc(lspLoc{URI: defaultURI, Range: rng}),
			})
			if len(it.Children) > 0 {
				// Flatten immediate children using parent canonical as container.
				for _, child := range it.Children {
					childCanonical := child.Name
					if canonical != "" {
						childCanonical = canonical + "." + child.Name
					}
					childRange := child.SelectionRange
					if childRange == (lspRng{}) {
						childRange = child.Range
					}
					out = append(out, Symbol{
						Name:      child.Name,
						Canonical: childCanonical,
						Kind:      lspKindToSymbolKind(child.Kind),
						Location:  wireLoc(lspLoc{URI: defaultURI, Range: childRange}),
					})
					for _, grand := range child.Children {
						grandCanonical := grand.Name
						if childCanonical != "" {
							grandCanonical = childCanonical + "." + grand.Name
						}
						grandRange := grand.SelectionRange
						if grandRange == (lspRng{}) {
							grandRange = grand.Range
						}
						out = append(out, Symbol{
							Name:      grand.Name,
							Canonical: grandCanonical,
							Kind:      lspKindToSymbolKind(grand.Kind),
							Location:  wireLoc(lspLoc{URI: defaultURI, Range: grandRange}),
						})
					}
				}
			}
		}
	}
	appendDocSymbols("", docItems)
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

func extractHoverText(raw json.RawMessage) string {
	if raw == nil || string(raw) == "null" {
		return ""
	}
	var mc struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &mc) == nil && mc.Value != "" {
		return mc.Value
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
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

func lspKindToSymbolKind(k int) SymbolKind {
	switch k {
	case 1:
		return KindFile
	case 2, 3, 4:
		return KindModule
	case 5, 23:
		return KindClass
	case 6:
		return KindMethod
	case 9:
		return KindConstructor
	case 10, 26:
		return KindType
	case 11:
		return KindInterface
	case 12:
		return KindFunction
	case 7, 8, 13, 14:
		return KindVariable
	default:
		return KindUnknown
	}
}

// ── Coordinate / URI helpers ──────────────────────────────────────────────────

func posToWire(p Position) lspPos { return lspPos(p) }

func wireRangeToRange(r lspRng) Range {
	return Range{
		Start: Position{Line: r.Start.Line, Character: r.Start.Character},
		End:   Position{Line: r.End.Line, Character: r.End.Character},
	}
}

func wireLoc(l lspLoc) Location {
	return Location{URI: l.URI, Range: wireRangeToRange(l.Range)}
}

func rangeContains(r Range, p Position) bool {
	if p.Line < r.Start.Line || p.Line > r.End.Line {
		return false
	}
	if p.Line == r.Start.Line && p.Character < r.Start.Character {
		return false
	}
	if p.Line == r.End.Line && p.Character >= r.End.Character {
		return false
	}
	return true
}

func pathToFileURI(path string) string {
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func fileURIToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("lsp: parse URI %q: %w", uri, err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("lsp: non-file URI scheme %q", u.Scheme)
	}
	return filepath.FromSlash(u.Path), nil
}
