//go:build cgo

package codegraph

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
)

// ExtractPython parses Python source with tree-sitter and returns symbols and edges.
func ExtractPython(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, python.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &pyExtractor{src: src, filename: filename}
	e.run(root)
	return e.symbols, e.edges, nil
}

// ─── extractor ───────────────────────────────────────────────────────────────

type pyExtractor struct {
	src      []byte
	filename string
	symbols  []Symbol
	edges    []Edge
}

func (e *pyExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *pyExtractor) fieldText(n *sitter.Node, field string) string {
	c := n.ChildByFieldName(field)
	if c == nil {
		return ""
	}
	return e.text(c)
}

func (e *pyExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, File: e.filename})
}

func (e *pyExtractor) addSymbolNode(name string, kind SymbolKind, n *sitter.Node) {
	e.symbols = append(e.symbols, Symbol{
		Name:      name,
		Kind:      kind,
		File:      e.filename,
		StartLine: n.StartPoint().Row,
		EndLine:   n.EndPoint().Row,
		StartByte: n.StartByte(),
		EndByte:   n.EndByte(),
	})
}

func (e *pyExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

// moduleName derives a module name from the filename (last path component without extension).
func (e *pyExtractor) moduleName() string {
	base := filepath.Base(e.filename)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (e *pyExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)

	e.walkBody(fileCanon, root)
}

// walkBody iterates direct children of a block/module node and dispatches.
func (e *pyExtractor) walkBody(parent string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "import_statement":
			e.handleImport(parent, child)
		case "import_from_statement":
			e.handleImportFrom(parent, child)
		case "function_definition":
			e.handleFunction(parent, child, "")
		case "class_definition":
			e.handleClass(parent, child)
		case "decorated_definition":
			e.handleDecorated(parent, child)
		case "expression_statement":
			// may contain assignments at module level
		}
	}
}

// handleImport processes: import os, import os.path
func (e *pyExtractor) handleImport(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "dotted_name", "aliased_import":
			name := child.ChildByFieldName("name")
			if name == nil {
				// dotted_name directly
				e.addEdge(fileCanon, "imports", e.text(child))
			} else {
				e.addEdge(fileCanon, "imports", e.text(name))
			}
		}
	}
}

// handleImportFrom processes: from os.path import join, exists
func (e *pyExtractor) handleImportFrom(fileCanon string, n *sitter.Node) {
	moduleName := n.ChildByFieldName("module_name")
	if moduleName == nil {
		return
	}
	modText := e.text(moduleName)
	e.addEdge(fileCanon, "imports", modText)
}

// handleFunction processes function_definition nodes.
// className is non-empty when the function is a method inside a class.
func (e *pyExtractor) handleFunction(fileCanon string, n *sitter.Node, className string) {
	name := e.fieldText(n, "name")
	if name == "" {
		return
	}
	var qualified string
	if className != "" {
		qualified = className + "." + name
	} else {
		mod := e.moduleName()
		if mod != "" {
			qualified = mod + "." + name
		} else {
			qualified = name
		}
	}

	kind := KindFunction
	if className != "" {
		kind = KindMethod
	}
	e.addSymbolNode(qualified, kind, n)
	e.addEdge(fileCanon, "defines", qualified)

	// Walk body for calls.
	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(qualified, body)
	}
}

// handleClass processes class_definition nodes.
func (e *pyExtractor) handleClass(fileCanon string, n *sitter.Node) {
	name := e.fieldText(n, "name")
	if name == "" {
		return
	}
	mod := e.moduleName()
	var qualified string
	if mod != "" {
		qualified = mod + "." + name
	} else {
		qualified = name
	}
	e.addSymbolNode(qualified, KindClass, n)
	e.addEdge(fileCanon, "defines", qualified)

	// Superclasses → extends edges.
	superclasses := n.ChildByFieldName("superclasses")
	if superclasses != nil {
		sc := int(superclasses.ChildCount())
		for i := 0; i < sc; i++ {
			child := superclasses.Child(i)
			if child == nil || !child.IsNamed() {
				continue
			}
			switch child.Type() {
			case "identifier", "attribute":
				e.addEdge(qualified, "extends", e.text(child))
			}
		}
	}

	// Walk body for methods.
	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkClassBody(fileCanon, qualified, body)
	}
}

func (e *pyExtractor) walkClassBody(fileCanon, className string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "function_definition":
			e.handleFunction(fileCanon, child, className)
		case "decorated_definition":
			e.handleDecorated(fileCanon, child)
		}
	}
}

// handleDecorated unwraps a decorated_definition to its inner function or class.
func (e *pyExtractor) handleDecorated(parent string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "function_definition":
			e.handleFunction(parent, child, "")
		case "class_definition":
			e.handleClass(parent, child)
		}
	}
}

// collectCalls walks a subtree for call nodes and emits caller → calls → callee.
func (e *pyExtractor) collectCalls(callerName string, n *sitter.Node) {
	if n == nil {
		return
	}
	if n.Type() == "call" {
		fn := n.ChildByFieldName("function")
		if fn != nil {
			callee := e.calleeText(fn)
			if callee != "" {
				e.addEdge(callerName, "calls", callee)
			}
		}
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		e.collectCalls(callerName, n.Child(i))
	}
}

func (e *pyExtractor) calleeText(n *sitter.Node) string {
	if n == nil {
		return ""
	}
	switch n.Type() {
	case "identifier":
		return e.text(n)
	case "attribute":
		// e.g. self.method or module.func — keep the attribute name
		attr := n.ChildByFieldName("attribute")
		if attr != nil {
			return e.text(attr)
		}
		return e.text(n)
	default:
		return ""
	}
}
