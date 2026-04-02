package mcp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestScopeTakingHandlersCallAuthorizeRequestedScope(t *testing.T) {
	t.Parallel()

	required := map[string][]string{
		"remember.go": {
			"handleRemember",
		},
		"publish.go": {
			"handlePublish",
		},
		"recall.go": {
			"handleRecall",
		},
		"context.go": {
			"handleContext",
		},
		"skill_search.go": {
			"handleSkillSearch",
		},
		"promote.go": {
			"handlePromote",
		},
		"collect.go": {
			"collectAddToCollection",
			"collectCreate",
			"collectList",
		},
		"session.go": {
			"handleSessionBegin",
		},
		"summarize.go": {
			"handleSummarize",
		},
		"synthesize.go": {
			"handleSynthesizeTopic",
		},
		"skill_install.go": {
			"handleSkillInstall",
		},
		"skill_invoke.go": {
			"handleSkillInvoke",
		},
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	baseDir := filepath.Dir(currentFile)

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
