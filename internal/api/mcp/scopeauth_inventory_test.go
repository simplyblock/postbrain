package mcp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

type mcpScopeToolInventoryItem struct {
	File      string
	Handler   string
	Tool      string
	Operation string
}

// mcpScopeToolInventory is the explicit inventory of MCP scope-taking operations.
var mcpScopeToolInventory = []mcpScopeToolInventoryItem{
	{File: "remember.go", Handler: "handleRemember", Tool: "remember", Operation: "store/update memory"},
	{File: "publish.go", Handler: "handlePublish", Tool: "publish", Operation: "create/update artifact"},
	{File: "recall.go", Handler: "handleRecall", Tool: "recall", Operation: "recall across layers"},
	{File: "context.go", Handler: "handleContext", Tool: "context", Operation: "retrieve context bundle"},
	{File: "skill_search.go", Handler: "handleSkillSearch", Tool: "skill_search", Operation: "search skills"},
	{File: "promote.go", Handler: "handlePromote", Tool: "promote", Operation: "create promotion request"},
	{File: "collect.go", Handler: "collectAddToCollection", Tool: "collect", Operation: "add_to_collection"},
	{File: "collect.go", Handler: "collectCreate", Tool: "collect", Operation: "create_collection"},
	{File: "collect.go", Handler: "collectList", Tool: "collect", Operation: "list_collections"},
	{File: "session.go", Handler: "handleSessionBegin", Tool: "session_begin", Operation: "create session"},
	{File: "summarize.go", Handler: "handleSummarize", Tool: "summarize", Operation: "summarize memories"},
	{File: "synthesize.go", Handler: "handleSynthesizeTopic", Tool: "synthesize_topic", Operation: "synthesize digest"},
	{File: "skill_install.go", Handler: "handleSkillInstall", Tool: "skill_install", Operation: "install skill"},
	{File: "skill_invoke.go", Handler: "handleSkillInvoke", Tool: "skill_invoke", Operation: "invoke skill"},
}

func TestScopeTakingHandlersCallAuthorizeRequestedScope(t *testing.T) {
	t.Parallel()

	if len(mcpScopeToolInventory) == 0 {
		t.Fatal("mcp scope tool inventory must not be empty")
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	baseDir := filepath.Dir(currentFile)

	required := map[string][]string{}
	for _, item := range mcpScopeToolInventory {
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

// containsAuthorizeScopeCall reports true when fn contains a direct call to
// authorizeRequestedScope OR to resolveScope (which calls authorizeRequestedScope
// internally and is the canonical scope-resolution helper for MCP handlers).
func containsAuthorizeScopeCall(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel != nil {
			switch sel.Sel.Name {
			case "authorizeRequestedScope", "resolveScope":
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// Keep the old name as an alias so it remains callable from the test.
func containsAuthorizeRequestedScopeCall(fn *ast.FuncDecl) bool {
	return containsAuthorizeScopeCall(fn)
}
