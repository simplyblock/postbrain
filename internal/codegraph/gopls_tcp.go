package codegraph

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultLSPTimeout = 5 * time.Second

// GoplsTCPResolver resolves Go symbols via an external gopls endpoint over TCP.
type GoplsTCPResolver struct {
	conn        net.Conn
	reader      *bufio.Reader
	timeout     time.Duration
	rootURI     string
	rootPath    string
	mu          sync.Mutex
	nextID      int64
	initialized bool
	closed      bool
}

// NewGoplsTCPResolver connects to a running gopls TCP endpoint.
func NewGoplsTCPResolver(addr string, timeout time.Duration, rootURI string) (*GoplsTCPResolver, error) {
	if timeout <= 0 {
		timeout = defaultLSPTimeout
	}
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	return newGoplsTCPResolverWithConn(conn, timeout, rootURI), nil
}

func newGoplsTCPResolverWithConn(conn net.Conn, timeout time.Duration, rootURI string) *GoplsTCPResolver {
	if timeout <= 0 {
		timeout = defaultLSPTimeout
	}
	rootPath := ""
	if rootURI != "" {
		if u, err := url.Parse(rootURI); err == nil && u.Scheme == "file" {
			rootPath = u.Path
		}
	}
	return &GoplsTCPResolver{
		conn:     conn,
		reader:   bufio.NewReader(conn),
		timeout:  timeout,
		rootURI:  rootURI,
		rootPath: rootPath,
	}
}

func (r *GoplsTCPResolver) Language() string { return ".go" }

func (r *GoplsTCPResolver) Resolve(ctx context.Context, file, symbol string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return "", fmt.Errorf("gopls resolver is closed")
	}
	if err := r.ensureInitializedLocked(ctx); err != nil {
		return "", err
	}

	if canonical, err := r.resolveViaDefinitionLocked(ctx, file, symbol); err == nil && canonical != "" {
		return canonical, nil
	}

	result, err := r.requestLocked(ctx, "workspace/symbol", map[string]any{"query": symbol})
	if err != nil {
		return "", err
	}

	items, ok := result.([]any)
	if !ok || len(items) == 0 {
		return "", nil
	}

	first, ok := items[0].(map[string]any)
	if !ok {
		return "", nil
	}
	name, _ := first["name"].(string)
	container, _ := first["containerName"].(string)
	if name == "" {
		return "", nil
	}
	if container != "" {
		return container + "." + name, nil
	}
	return name, nil
}

func (r *GoplsTCPResolver) resolveViaDefinitionLocked(ctx context.Context, file, symbol string) (string, error) {
	fileURI, line, char, err := r.callsiteURIAndPosition(file, symbol)
	if err != nil {
		return "", err
	}

	result, err := r.requestLocked(ctx, "textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": fileURI},
		"position":     map[string]any{"line": line, "character": char},
	})
	if err != nil {
		return "", err
	}

	locURI := firstDefinitionURI(result)
	if locURI == "" {
		return "", fmt.Errorf("empty definition result")
	}

	defPath, err := filePathFromURI(locURI)
	if err != nil {
		return "", err
	}
	pkg, err := packageNameFromFile(defPath)
	if err != nil || pkg == "" {
		return "", fmt.Errorf("package name not found for %s", defPath)
	}
	return pkg + "." + symbol, nil
}

func (r *GoplsTCPResolver) callsiteURIAndPosition(file, symbol string) (string, int, int, error) {
	path := file
	if !filepath.IsAbs(path) && r.rootPath != "" {
		path = filepath.Join(r.rootPath, filepath.FromSlash(file))
	}
	if !filepath.IsAbs(path) {
		return "", 0, 0, fmt.Errorf("no absolute path for %q", file)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, 0, err
	}
	needle := symbol + "("
	off := strings.Index(string(data), needle)
	if off < 0 {
		off = strings.Index(string(data), symbol)
	}
	if off < 0 {
		return "", 0, 0, fmt.Errorf("symbol %q not found in %s", symbol, path)
	}
	line, char := offsetToLineChar(data, off)
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
	return uri, line, char, nil
}

func offsetToLineChar(data []byte, off int) (line int, char int) {
	for i := 0; i < off && i < len(data); i++ {
		if data[i] == '\n' {
			line++
			char = 0
			continue
		}
		char++
	}
	return line, char
}

func firstDefinitionURI(result any) string {
	switch v := result.(type) {
	case []any:
		if len(v) == 0 {
			return ""
		}
		if m, ok := v[0].(map[string]any); ok {
			if uri, _ := m["uri"].(string); uri != "" {
				return uri
			}
			if targetURI, _ := m["targetUri"].(string); targetURI != "" {
				return targetURI
			}
		}
	case map[string]any:
		if uri, _ := v["uri"].(string); uri != "" {
			return uri
		}
	}
	return ""
}

func filePathFromURI(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("unsupported uri scheme %q", u.Scheme)
	}
	return u.Path, nil
}

var packageLineRE = regexp.MustCompile(`(?m)^\s*package\s+([A-Za-z_][A-Za-z0-9_]*)\b`)

func packageNameFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	m := packageLineRE.FindSubmatch(data)
	if len(m) < 2 {
		return "", nil
	}
	return string(m[1]), nil
}

func (r *GoplsTCPResolver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true

	if r.initialized {
		_, _ = r.requestLocked(context.Background(), "shutdown", nil)
		_ = r.notifyLocked(context.Background(), "exit", nil)
	}
	return r.conn.Close()
}

func (r *GoplsTCPResolver) ensureInitializedLocked(ctx context.Context) error {
	if r.initialized {
		return nil
	}

	_, err := r.requestLocked(ctx, "initialize", map[string]any{
		"processId": nil,
		"rootUri":   r.rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{},
			"workspace":    map[string]any{},
		},
		"clientInfo": map[string]any{
			"name":    "postbrain",
			"version": "0",
		},
	})
	if err != nil {
		return err
	}
	if err := r.notifyLocked(ctx, "initialized", map[string]any{}); err != nil {
		return err
	}
	r.initialized = true
	return nil
}

func (r *GoplsTCPResolver) requestLocked(ctx context.Context, method string, params any) (any, error) {
	id := atomic.AddInt64(&r.nextID, 1)
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	if err := r.writeFrameLocked(ctx, msg); err != nil {
		return nil, err
	}

	for {
		resp, err := r.readMessageLocked(ctx)
		if err != nil {
			return nil, err
		}
		respID, hasID := resp["id"]
		if !hasID {
			continue
		}
		if !sameJSONRPCID(respID, id) {
			continue
		}
		if e, hasErr := resp["error"]; hasErr && e != nil {
			return nil, fmt.Errorf("lsp error: %v", e)
		}
		return resp["result"], nil
	}
}

func (r *GoplsTCPResolver) notifyLocked(ctx context.Context, method string, params any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	return r.writeFrameLocked(ctx, msg)
}

func (r *GoplsTCPResolver) writeFrameLocked(ctx context.Context, msg map[string]any) error {
	if err := r.setDeadlineLocked(ctx); err != nil {
		return err
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)
	_, err = r.conn.Write([]byte(frame))
	return err
}

func (r *GoplsTCPResolver) readMessageLocked(ctx context.Context) (map[string]any, error) {
	if err := r.setDeadlineLocked(ctx); err != nil {
		return nil, err
	}
	headers := map[string]string{}
	for {
		line, err := r.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	var n int
	if _, err := fmt.Sscanf(headers["Content-Length"], "%d", &n); err != nil {
		return nil, fmt.Errorf("invalid content-length: %w", err)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r.reader, body); err != nil {
		return nil, err
	}
	var msg map[string]any
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (r *GoplsTCPResolver) setDeadlineLocked(ctx context.Context) error {
	d := time.Now().Add(r.timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(d) {
		d = dl
	}
	return r.conn.SetDeadline(d)
}

func sameJSONRPCID(v any, id int64) bool {
	switch t := v.(type) {
	case float64:
		return int64(t) == id
	case int64:
		return t == id
	case int:
		return int64(t) == id
	default:
		return false
	}
}
