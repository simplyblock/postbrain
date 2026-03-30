package codegraph

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	ts "github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
)

// ExtractTypeScript parses TypeScript (.ts / .tsx) source and returns symbols and edges.
func ExtractTypeScript(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	lang := ts.GetLanguage()
	if strings.ToLower(filepath.Ext(filename)) == ".tsx" {
		lang = tsx.GetLanguage()
	}
	root, err := sitter.ParseCtx(ctx, src, lang)
	if err != nil {
		return nil, nil, err
	}
	e := &jsExtractor{src: src, filename: filename}
	e.run(root)
	return e.symbols, e.edges, nil
}

// ExtractJavaScript parses JavaScript (.js / .jsx / .mjs / .cjs) source and returns symbols and edges.
func ExtractJavaScript(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, javascript.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &jsExtractor{src: src, filename: filename}
	e.run(root)
	return e.symbols, e.edges, nil
}

// ─── shared JS/TS extractor ──────────────────────────────────────────────────

type jsExtractor struct {
	src      []byte
	filename string
	symbols  []Symbol
	edges    []Edge
}

func (e *jsExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *jsExtractor) fieldText(n *sitter.Node, field string) string {
	c := n.ChildByFieldName(field)
	if c == nil {
		return ""
	}
	return e.text(c)
}

func (e *jsExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, File: e.filename})
}

func (e *jsExtractor) addSymbolNode(name string, kind SymbolKind, n *sitter.Node) {
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

func (e *jsExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

// moduleName derives a module identifier from the filename.
func (e *jsExtractor) moduleName() string {
	base := filepath.Base(e.filename)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (e *jsExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)
	e.walkProgram(fileCanon, root)
}

func (e *jsExtractor) walkProgram(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		e.handleStatement(fileCanon, child, "")
	}
}

func (e *jsExtractor) handleStatement(fileCanon string, n *sitter.Node, className string) {
	switch n.Type() {
	case "import_statement":
		e.handleImport(fileCanon, n)
	case "export_statement":
		e.handleExport(fileCanon, n, className)
	case "function_declaration", "function":
		e.handleFunction(fileCanon, n, className)
	case "class_declaration", "class":
		e.handleClass(fileCanon, n)
	case "lexical_declaration", "variable_declaration":
		e.handleVarDecl(fileCanon, n)

	// TypeScript-specific
	case "interface_declaration":
		e.handleInterface(fileCanon, n)
	case "type_alias_declaration":
		e.handleTypeAlias(fileCanon, n)
	case "enum_declaration":
		e.handleEnum(fileCanon, n)
	case "abstract_class_declaration":
		e.handleClass(fileCanon, n)
	}
}

// handleImport: import ... from "module"
func (e *jsExtractor) handleImport(fileCanon string, n *sitter.Node) {
	src := n.ChildByFieldName("source")
	if src == nil {
		// walk children for string literal
		count := int(n.ChildCount())
		for i := 0; i < count; i++ {
			child := n.Child(i)
			if child != nil && child.Type() == "string" {
				modPath := strings.Trim(e.text(child), `'"`)
				e.addEdge(fileCanon, "imports", modPath)
				return
			}
		}
		return
	}
	modPath := strings.Trim(e.text(src), `'"`)
	e.addEdge(fileCanon, "imports", modPath)
}

// handleExport unwraps export statements to their declarations.
func (e *jsExtractor) handleExport(fileCanon string, n *sitter.Node, className string) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "function_declaration", "function":
			e.handleFunction(fileCanon, child, className)
		case "class_declaration", "class", "abstract_class_declaration":
			e.handleClass(fileCanon, child)
		case "lexical_declaration", "variable_declaration":
			e.handleVarDecl(fileCanon, child)
		case "interface_declaration":
			e.handleInterface(fileCanon, child)
		case "type_alias_declaration":
			e.handleTypeAlias(fileCanon, child)
		case "enum_declaration":
			e.handleEnum(fileCanon, child)
		}
	}
}

func (e *jsExtractor) handleFunction(fileCanon string, n *sitter.Node, className string) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
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

	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(qualified, body)
	}
}

func (e *jsExtractor) handleClass(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	if name == "" {
		return
	}
	mod := e.moduleName()
	qualified := name
	if mod != "" {
		qualified = mod + "." + name
	}
	e.addSymbolNode(qualified, KindClass, n)
	e.addEdge(fileCanon, "defines", qualified)

	// Superclass → extends edge.
	heritage := n.ChildByFieldName("heritage")
	if heritage == nil {
		// TypeScript: class_heritage
		count := int(n.ChildCount())
		for i := 0; i < count; i++ {
			child := n.Child(i)
			if child != nil && (child.Type() == "class_heritage" || child.Type() == "extends_clause") {
				heritage = child
				break
			}
		}
	}
	if heritage != nil {
		hcount := int(heritage.ChildCount())
		for i := 0; i < hcount; i++ {
			child := heritage.Child(i)
			if child == nil {
				continue
			}
			switch child.Type() {
			case "identifier", "member_expression", "type_identifier":
				e.addEdge(qualified, "extends", e.text(child))
			}
		}
	}

	// Walk class body for method definitions.
	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkClassBody(fileCanon, qualified, body)
	}
}

func (e *jsExtractor) walkClassBody(fileCanon, className string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "method_definition", "public_field_definition":
			e.handleMethodDef(fileCanon, className, child)
		}
	}
}

func (e *jsExtractor) handleMethodDef(fileCanon, className string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	if name == "" {
		return
	}
	qualified := className + "." + name
	e.addSymbolNode(qualified, KindMethod, n)
	e.addEdge(fileCanon, "defines", qualified)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(qualified, body)
	}
}

func (e *jsExtractor) handleVarDecl(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil || child.Type() != "variable_declarator" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := e.text(nameNode)
		if name == "" {
			continue
		}
		mod := e.moduleName()
		qualified := name
		if mod != "" {
			qualified = mod + "." + name
		}

		// If the value is an arrow function or function expression, treat as function.
		val := child.ChildByFieldName("value")
		if val != nil && (val.Type() == "arrow_function" || val.Type() == "function") {
			e.addSymbol(qualified, KindFunction)
			e.addEdge(fileCanon, "defines", qualified)
			body := val.ChildByFieldName("body")
			if body != nil {
				e.collectCalls(qualified, body)
			}
		} else {
			e.addSymbol(qualified, KindVariable)
			e.addEdge(fileCanon, "defines", qualified)
		}
	}
}

func (e *jsExtractor) handleInterface(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	mod := e.moduleName()
	qualified := name
	if mod != "" {
		qualified = mod + "." + name
	}
	e.addSymbol(qualified, KindInterface)
	e.addEdge(fileCanon, "defines", qualified)
}

func (e *jsExtractor) handleTypeAlias(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	mod := e.moduleName()
	qualified := name
	if mod != "" {
		qualified = mod + "." + name
	}
	e.addSymbol(qualified, KindType)
	e.addEdge(fileCanon, "defines", qualified)
}

func (e *jsExtractor) handleEnum(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	mod := e.moduleName()
	qualified := name
	if mod != "" {
		qualified = mod + "." + name
	}
	e.addSymbol(qualified, KindType)
	e.addEdge(fileCanon, "defines", qualified)
}

func (e *jsExtractor) collectCalls(callerName string, n *sitter.Node) {
	if n == nil {
		return
	}
	if n.Type() == "call_expression" {
		fn := n.ChildByFieldName("function")
		if fn != nil {
			callee := e.jsCalleeText(fn)
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

func (e *jsExtractor) jsCalleeText(n *sitter.Node) string {
	if n == nil {
		return ""
	}
	switch n.Type() {
	case "identifier":
		return e.text(n)
	case "member_expression":
		// e.g. db.create → "create"
		prop := n.ChildByFieldName("property")
		if prop != nil {
			return e.text(prop)
		}
		return e.text(n)
	default:
		return ""
	}
}