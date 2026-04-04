package mcp

import "testing"

func TestHandleGraphQuery_MissingCypher_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"scope": "project:acme/api",
	}, s.handleGraphQuery)
	assertToolError(t, result)
}

func TestHandleGraphQuery_MissingScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"cypher": "RETURN n",
	}, s.handleGraphQuery)
	assertToolError(t, result)
}

func TestHandleGraphQuery_NilPool_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"cypher": "RETURN n",
		"scope":  "project:acme/api",
	}, s.handleGraphQuery)
	assertToolError(t, result)
}
