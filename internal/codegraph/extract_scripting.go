// extract_scripting.go contains extractors for Bash, Lua, PHP, and Ruby.
package codegraph

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/lua"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/ruby"
)

// ─── Bash ────────────────────────────────────────────────────────────────────

// ExtractBash parses Bash/shell source (.sh / .bash) and returns symbols and edges.
func ExtractBash(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, bash.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &bashExtractor{src: src, filename: filename}
	e.run(root)
	return e.symbols, e.edges, nil
}

type bashExtractor struct {
	src      []byte
	filename string
	symbols  []Symbol
	edges    []Edge
}

func (e *bashExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *bashExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, File: e.filename})
}

func (e *bashExtractor) addSymbolNode(name string, kind SymbolKind, n *sitter.Node) {
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

func (e *bashExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

func (e *bashExtractor) scriptName() string {
	base := filepath.Base(e.filename)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (e *bashExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)
	e.walkStatements(fileCanon, root)
}

func (e *bashExtractor) walkStatements(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "function_definition":
			e.handleFunction(fileCanon, child)
		case "command":
			// Detect: source ./lib.sh  or  . ./lib.sh
			e.handleCommand(fileCanon, child)
		}
	}
}

func (e *bashExtractor) handleCommand(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	cmdName := e.text(nameNode)
	if cmdName != "source" && cmdName != "." {
		return
	}
	// first argument is the path
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil || n.FieldNameForChild(i) != "argument" {
			continue
		}
		path := strings.Trim(e.text(child), `'"`)
		if path != "" {
			e.addEdge(fileCanon, "imports", path)
		}
		break
	}
}

func (e *bashExtractor) handleFunction(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.scriptName() + "." + e.text(nameNode)
	e.addSymbolNode(name, KindFunction, n)
	e.addEdge(fileCanon, "defines", name)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(name, body)
	}
}

func (e *bashExtractor) collectCalls(callerName string, n *sitter.Node) {
	if n == nil {
		return
	}
	if n.Type() == "command" {
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			callee := e.text(nameNode)
			if callee != "" && !strings.HasPrefix(callee, "-") {
				e.addEdge(callerName, "calls", callee)
			}
		}
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		e.collectCalls(callerName, n.Child(i))
	}
}

// ─── Lua ─────────────────────────────────────────────────────────────────────

// ExtractLua parses Lua source (.lua) and returns symbols and edges.
func ExtractLua(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, lua.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &luaExtractor{src: src, filename: filename}
	e.run(root)
	return e.symbols, e.edges, nil
}

type luaExtractor struct {
	src      []byte
	filename string
	symbols  []Symbol
	edges    []Edge
}

func (e *luaExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *luaExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, File: e.filename})
}

func (e *luaExtractor) addSymbolNode(name string, kind SymbolKind, n *sitter.Node) {
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

func (e *luaExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

func (e *luaExtractor) moduleName() string {
	base := filepath.Base(e.filename)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (e *luaExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)
	e.walkBlock(fileCanon, root)
}

func (e *luaExtractor) walkBlock(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "function_statement":
			e.handleFunction(fileCanon, child)
		case "local_function":
			e.handleFunction(fileCanon, child)
		case "assignment_statement":
			e.handleAssignment(fileCanon, child)
		}
	}
}

func (e *luaExtractor) handleFunction(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	rawName := e.text(nameNode)
	name := e.moduleName() + "." + rawName
	e.addSymbolNode(name, KindFunction, n)
	e.addEdge(fileCanon, "defines", name)

	body := n.ChildByFieldName("body")
	if body == nil {
		body = n // fall back to entire node for call scanning
	}
	e.collectCalls(name, body)
}

func (e *luaExtractor) handleAssignment(fileCanon string, n *sitter.Node) {
	// Handle require() calls: local x = require("module")
	vars := n.ChildByFieldName("variables")
	vals := n.ChildByFieldName("values")
	if vars == nil || vals == nil {
		return
	}
	count := int(vals.ChildCount())
	for i := 0; i < count; i++ {
		child := vals.Child(i)
		if child == nil || child.Type() != "function_call" {
			continue
		}
		fn := child.ChildByFieldName("name")
		if fn != nil && e.text(fn) == "require" {
			args := child.ChildByFieldName("args")
			if args != nil {
				ac := int(args.ChildCount())
				for j := 0; j < ac; j++ {
					arg := args.Child(j)
					if arg != nil && arg.Type() == "string" {
						modPath := strings.Trim(e.text(arg), `'"`)
						e.addEdge(fileCanon, "imports", modPath)
					}
				}
			}
		}
	}
}

func (e *luaExtractor) collectCalls(callerName string, n *sitter.Node) {
	if n == nil {
		return
	}
	if n.Type() == "function_call" {
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			e.addEdge(callerName, "calls", e.text(nameNode))
		}
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		e.collectCalls(callerName, n.Child(i))
	}
}

// ─── PHP ─────────────────────────────────────────────────────────────────────

// ExtractPHP parses PHP source (.php) and returns symbols and edges.
func ExtractPHP(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, php.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &phpExtractor{src: src, filename: filename}
	e.run(root)
	return e.symbols, e.edges, nil
}

type phpExtractor struct {
	src      []byte
	filename string
	symbols  []Symbol
	edges    []Edge
}

func (e *phpExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *phpExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, File: e.filename})
}

func (e *phpExtractor) addSymbolNode(name string, kind SymbolKind, n *sitter.Node) {
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

func (e *phpExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

func (e *phpExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)
	e.walkStatements(fileCanon, root, "")
}

func (e *phpExtractor) walkStatements(fileCanon string, n *sitter.Node, classScope string) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "namespace_use_declaration":
			e.handleUse(fileCanon, child)
		case "function_definition":
			e.handleFunction(fileCanon, child, classScope)
		case "class_declaration":
			e.handleClass(fileCanon, child)
		case "interface_declaration":
			e.handleInterface(fileCanon, child)
		case "trait_declaration":
			e.handleTrait(fileCanon, child)
		case "enum_declaration":
			e.handleEnum(fileCanon, child)
		case "program":
			e.walkStatements(fileCanon, child, classScope)
		}
	}
}

func (e *phpExtractor) handleUse(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == "namespace_use_clause" {
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				nameNode = child.NamedChild(0)
			}
			if nameNode != nil {
				e.addEdge(fileCanon, "imports", e.text(nameNode))
			}
		}
	}
}

func (e *phpExtractor) handleFunction(fileCanon string, n *sitter.Node, classScope string) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	rawName := e.text(nameNode)
	name := rawName
	if classScope != "" {
		name = classScope + "::" + rawName
	}
	kind := KindFunction
	if classScope != "" {
		kind = KindMethod
	}
	e.addSymbolNode(name, kind, n)
	e.addEdge(fileCanon, "defines", name)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(name, body)
	}
}

func (e *phpExtractor) handleClass(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	e.addSymbolNode(name, KindClass, n)
	e.addEdge(fileCanon, "defines", name)

	base := n.ChildByFieldName("base_clause")
	if base != nil {
		count := int(base.ChildCount())
		for i := 0; i < count; i++ {
			child := base.Child(i)
			if child != nil && child.Type() == "qualified_name" {
				e.addEdge(name, "extends", e.text(child))
			}
		}
	}

	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkClassBody(fileCanon, name, body)
	}
}

func (e *phpExtractor) handleInterface(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	e.addSymbol(name, KindInterface)
	e.addEdge(fileCanon, "defines", name)
}

func (e *phpExtractor) handleTrait(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	e.addSymbol(name, KindType)
	e.addEdge(fileCanon, "defines", name)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkClassBody(fileCanon, name, body)
	}
}

func (e *phpExtractor) handleEnum(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	e.addSymbol(name, KindType)
	e.addEdge(fileCanon, "defines", name)
}

func (e *phpExtractor) walkClassBody(fileCanon, className string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == "method_declaration" {
			e.handleFunction(fileCanon, child, className)
		}
	}
}

func (e *phpExtractor) collectCalls(callerName string, n *sitter.Node) {
	if n == nil {
		return
	}
	if n.Type() == "function_call_expression" {
		fn := n.ChildByFieldName("function")
		if fn != nil {
			e.addEdge(callerName, "calls", e.text(fn))
		}
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		e.collectCalls(callerName, n.Child(i))
	}
}

// ─── Ruby ────────────────────────────────────────────────────────────────────

// ExtractRuby parses Ruby source (.rb) and returns symbols and edges.
func ExtractRuby(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, ruby.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &rubyExtractor{src: src, filename: filename}
	e.run(root)
	return e.symbols, e.edges, nil
}

type rubyExtractor struct {
	src      []byte
	filename string
	symbols  []Symbol
	edges    []Edge
}

func (e *rubyExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *rubyExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, File: e.filename})
}

func (e *rubyExtractor) addSymbolNode(name string, kind SymbolKind, n *sitter.Node) {
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

func (e *rubyExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

func (e *rubyExtractor) moduleName() string {
	base := filepath.Base(e.filename)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (e *rubyExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)
	e.walkBody(fileCanon, root, "")
}

func (e *rubyExtractor) walkBody(fileCanon string, n *sitter.Node, classScope string) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "method":
			e.handleMethod(fileCanon, child, classScope)
		case "singleton_method":
			e.handleMethod(fileCanon, child, classScope)
		case "class":
			e.handleClass(fileCanon, child)
		case "module":
			e.handleModule(fileCanon, child)
		case "call":
			// require 'lib' / require_relative 'lib'
			e.handleRequire(fileCanon, child)
		}
	}
}

func (e *rubyExtractor) handleMethod(fileCanon string, n *sitter.Node, classScope string) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	rawName := e.text(nameNode)
	name := rawName
	if classScope != "" {
		name = classScope + "#" + rawName
	} else {
		mod := e.moduleName()
		if mod != "" {
			name = mod + "." + rawName
		}
	}
	kind := KindMethod
	if classScope == "" {
		kind = KindFunction
	}
	e.addSymbolNode(name, kind, n)
	e.addEdge(fileCanon, "defines", name)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(name, body)
	}
}

func (e *rubyExtractor) handleClass(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	e.addSymbolNode(name, KindClass, n)
	e.addEdge(fileCanon, "defines", name)

	// Superclass
	superclass := n.ChildByFieldName("superclass")
	if superclass != nil {
		count := int(superclass.ChildCount())
		for i := 0; i < count; i++ {
			child := superclass.Child(i)
			if child != nil && (child.Type() == "constant" || child.Type() == "scope_resolution") {
				e.addEdge(name, "extends", e.text(child))
			}
		}
	}

	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkBody(fileCanon, body, name)
	}
}

func (e *rubyExtractor) handleModule(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	e.addSymbol(name, KindModule)
	e.addEdge(fileCanon, "defines", name)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkBody(fileCanon, body, name)
	}
}

func (e *rubyExtractor) handleRequire(fileCanon string, n *sitter.Node) {
	// call node: method = require/require_relative, arguments contain the path
	method := n.ChildByFieldName("method")
	if method == nil {
		return
	}
	methodName := e.text(method)
	if methodName != "require" && methodName != "require_relative" {
		return
	}
	args := n.ChildByFieldName("arguments")
	if args == nil {
		return
	}
	count := int(args.ChildCount())
	for i := 0; i < count; i++ {
		child := args.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == "string" {
			path := strings.Trim(e.text(child), `'"`)
			e.addEdge(fileCanon, "imports", path)
		}
	}
}

func (e *rubyExtractor) collectCalls(callerName string, n *sitter.Node) {
	if n == nil {
		return
	}
	if n.Type() == "call" {
		method := n.ChildByFieldName("method")
		if method != nil {
			e.addEdge(callerName, "calls", e.text(method))
		}
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		e.collectCalls(callerName, n.Child(i))
	}
}