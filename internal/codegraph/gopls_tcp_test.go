package codegraph

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/closeutil"
)

func TestGoplsTCPResolver_Language(t *testing.T) {
	t.Parallel()
	r := &GoplsTCPResolver{}
	if got := r.Language(); got != ".go" {
		t.Fatalf("Language() = %q, want %q", got, ".go")
	}
}

func TestGoplsTCPResolver_Resolve_UsesWorkspaceSymbol(t *testing.T) {
	client, server := net.Pipe()
	defer closeutil.Log(client, "gopls test client")

	go serveFakeLSP(t, server)

	r := newGoplsTCPResolverWithConn(client, 2*time.Second, "")
	got, err := r.Resolve(context.Background(), "main.go", "Println")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got != "fmt.Println" {
		t.Fatalf("Resolve canonical = %q, want %q", got, "fmt.Println")
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func serveFakeLSP(t *testing.T, conn net.Conn) {
	t.Helper()
	defer closeutil.Log(conn, "gopls test server")

	reader := bufio.NewReader(conn)
	for {
		req, id, method, params, err := readLSPMessage(reader)
		if err != nil {
			if err == io.EOF || strings.Contains(err.Error(), "closed") {
				return
			}
			t.Errorf("readLSPMessage: %v", err)
			return
		}
		_ = req

		switch method {
		case "initialize":
			writeLSPResult(t, conn, id, map[string]any{
				"capabilities": map[string]any{},
			})
		case "initialized":
			// notification, no response
		case "workspace/symbol":
			query, _ := params["query"].(string)
			if query != "Println" {
				t.Errorf("workspace/symbol query = %q, want %q", query, "Println")
			}
			writeLSPResult(t, conn, id, []map[string]any{
				{
					"name":          "Println",
					"containerName": "fmt",
				},
			})
		case "textDocument/definition":
			// Make definition path fail for this unit test so fallback path is exercised.
			writeLSPResult(t, conn, id, []map[string]any{})
		case "shutdown":
			writeLSPResult(t, conn, id, nil)
		case "exit":
			return
		default:
			t.Errorf("unexpected method: %s", method)
		}
	}
}

func readLSPMessage(reader *bufio.Reader) (map[string]any, any, string, map[string]any, error) {
	headers := map[string]string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, nil, "", nil, err
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
	_, err := fmt.Sscanf(headers["Content-Length"], "%d", &n)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("invalid Content-Length: %w", err)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, nil, "", nil, err
	}

	var msg map[string]any
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, nil, "", nil, err
	}
	method, _ := msg["method"].(string)
	params, _ := msg["params"].(map[string]any)
	return msg, msg["id"], method, params, nil
}

func writeLSPResult(t *testing.T, conn net.Conn, id any, result any) {
	t.Helper()
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)
	if _, err := conn.Write([]byte(frame)); err != nil {
		t.Fatalf("write response: %v", err)
	}
}
