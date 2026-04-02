package rest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

type restScopeRouteInventoryItem struct {
	File      string
	Handler   string
	Route     string
	Operation string
}

// restScopeRouteInventory is the explicit inventory of REST scope-taking operations.
var restScopeRouteInventory = []restScopeRouteInventoryItem{
	{File: "memories.go", Handler: "createMemory", Route: "POST /v1/memories", Operation: "create memory"},
	{File: "memories.go", Handler: "recallMemories", Route: "GET /v1/memories/recall", Operation: "recall memories"},
	{File: "memories.go", Handler: "promoteMemory", Route: "POST /v1/memories/{id}/promote", Operation: "promote memory"},
	{File: "memories.go", Handler: "handleSummarizeMemories", Route: "POST /v1/memories/summarize", Operation: "summarize memories"},
	{File: "knowledge.go", Handler: "createArtifact", Route: "POST /v1/knowledge", Operation: "create artifact"},
	{File: "knowledge.go", Handler: "searchArtifacts", Route: "GET /v1/knowledge/search", Operation: "search artifacts"},
	{File: "skills.go", Handler: "createSkill", Route: "POST /v1/skills", Operation: "create skill"},
	{File: "skills.go", Handler: "searchSkills", Route: "GET /v1/skills/search", Operation: "search skills"},
	{File: "collections.go", Handler: "createCollection", Route: "POST /v1/collections", Operation: "create collection"},
	{File: "collections.go", Handler: "listCollections", Route: "GET /v1/collections", Operation: "list collections"},
	{File: "collections.go", Handler: "getCollection", Route: "GET /v1/collections/{slug|id}", Operation: "get collection"},
	{File: "context.go", Handler: "getContext", Route: "GET /v1/context", Operation: "get context"},
	{File: "upload.go", Handler: "uploadKnowledge", Route: "POST /v1/knowledge/upload", Operation: "upload knowledge"},
	{File: "sessions.go", Handler: "createSession", Route: "POST /v1/sessions", Operation: "create session"},
	{File: "promotions.go", Handler: "listPromotions", Route: "GET /v1/promotions", Operation: "list promotions"},
}

func TestScopeTakingHandlersCallAuthorizeRequestedScope(t *testing.T) {
	t.Parallel()

	if len(restScopeRouteInventory) == 0 {
		t.Fatal("rest scope route inventory must not be empty")
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	baseDir := filepath.Dir(currentFile)

	required := map[string][]string{}
	for _, item := range restScopeRouteInventory {
		required[item.File] = append(required[item.File], item.Handler)
	}

	for fileName, fnNames := range required {
		filePath := filepath.Join(baseDir, fileName)
		fileSet := token.NewFileSet()
		fileAST, err := parser.ParseFile(fileSet, filePath, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", fileName, err)
		}

		functions := map[string]*ast.FuncDecl{}
		for _, decl := range fileAST.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			functions[fn.Name.Name] = fn
		}

		for _, fnName := range fnNames {
			fn := functions[fnName]
			if fn == nil {
				t.Fatalf("%s missing function %s", fileName, fnName)
			}
			if !containsAuthorizeRequestedScopeCall(fn) {
				t.Fatalf("%s in %s must call authorizeRequestedScope", fnName, fileName)
			}
		}
	}
}

func containsAuthorizeRequestedScopeCall(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch c := call.Fun.(type) {
		case *ast.SelectorExpr:
			if c.Sel != nil && c.Sel.Name == "authorizeRequestedScope" {
				found = true
				return false
			}
		case *ast.Ident:
			if c.Name == "authorizeRequestedScope" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
