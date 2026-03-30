package codegraph

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
)

// ExtractC parses C source (.c / .h) and returns symbols and edges.
func ExtractC(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, c.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &cExtractor{src: src, filename: filename}
	e.run(root)
	return e.symbols, e.edges, nil
}

// ExtractCPP parses C++ source (.cpp / .cc / .cxx / .hpp / .hxx) and returns symbols and edges.
func ExtractCPP(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, cpp.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &cExtractor{src: src, filename: filename, isCPP: true}
	e.run(root)
	return e.symbols, e.edges, nil
}

// ─── extractor ───────────────────────────────────────────────────────────────

type cExtractor struct {
	src      []byte
	filename string
	isCPP    bool
	symbols  []Symbol
	edges    []Edge
}

func (e *cExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *cExtractor) fieldText(n *sitter.Node, field string) string {
	c := n.ChildByFieldName(field)
	if c == nil {
		return ""
	}
	return e.text(c)
}

func (e *cExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, File: e.filename})
}

func (e *cExtractor) addSymbolNode(name string, kind SymbolKind, n *sitter.Node) {
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

func (e *cExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

func (e *cExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)
	e.walkItems(fileCanon, root, "")
}

func (e *cExtractor) walkItems(fileCanon string, n *sitter.Node, classScope string) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "preproc_include":
			e.handleInclude(fileCanon, child)
		case "function_definition":
			e.handleFunction(fileCanon, child, classScope)
		case "declaration":
			e.handleDeclaration(fileCanon, child)
		case "struct_specifier", "union_specifier":
			e.handleStruct(fileCanon, child)
		case "enum_specifier":
			e.handleEnum(fileCanon, child)
		case "type_definition":
			e.handleTypedef(fileCanon, child)
		// C++ additions
		case "class_specifier":
			e.handleClass(fileCanon, child)
		case "namespace_definition":
			e.handleNamespace(fileCanon, child)
		}
	}
}

func (e *cExtractor) handleInclude(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "string_literal", "system_lib_string":
			path := strings.Trim(e.text(child), `"<>`)
			e.addEdge(fileCanon, "imports", path)
		}
	}
}

func (e *cExtractor) handleFunction(fileCanon string, n *sitter.Node, classScope string) {
	declarator := n.ChildByFieldName("declarator")
	if declarator == nil {
		return
	}
	name := e.extractDeclaratorName(declarator)
	if name == "" {
		return
	}
	qualified := name
	if classScope != "" {
		qualified = classScope + "::" + name
	}
	e.addSymbolNode(qualified, KindFunction, n)
	e.addEdge(fileCanon, "defines", qualified)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(qualified, body)
	}
}

// extractDeclaratorName digs into nested declarator nodes to find the function name.
func (e *cExtractor) extractDeclaratorName(n *sitter.Node) string {
	if n == nil {
		return ""
	}
	switch n.Type() {
	case "identifier":
		return e.text(n)
	case "function_declarator":
		inner := n.ChildByFieldName("declarator")
		return e.extractDeclaratorName(inner)
	case "pointer_declarator":
		inner := n.ChildByFieldName("declarator")
		return e.extractDeclaratorName(inner)
	case "reference_declarator":
		// C++
		count := int(n.ChildCount())
		for i := 0; i < count; i++ {
			child := n.Child(i)
			if child != nil && child.IsNamed() {
				return e.extractDeclaratorName(child)
			}
		}
	case "qualified_identifier":
		// C++: ClassName::method
		name := n.ChildByFieldName("name")
		scope := n.ChildByFieldName("scope")
		if name != nil && scope != nil {
			return e.text(scope) + "::" + e.text(name)
		}
		if name != nil {
			return e.text(name)
		}
	case "destructor_name":
		return e.text(n)
	case "operator_name":
		return e.text(n)
	}
	// Fallback: first named child
	count := int(n.NamedChildCount())
	if count > 0 {
		return e.extractDeclaratorName(n.NamedChild(0))
	}
	return ""
}

func (e *cExtractor) handleDeclaration(fileCanon string, n *sitter.Node) {
	// Top-level declarations may be function prototypes or variable declarations.
	// Only emit for simple named variables.
	declarator := n.ChildByFieldName("declarator")
	if declarator == nil {
		return
	}
	name := e.extractDeclaratorName(declarator)
	if name == "" {
		return
	}
	// Heuristic: if it looks like a function pointer skip it (name contains parens in raw text).
	if strings.ContainsAny(name, "()") {
		return
	}
	e.addSymbol(name, KindVariable)
	e.addEdge(fileCanon, "defines", name)
}

func (e *cExtractor) handleStruct(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	e.addSymbolNode(name, KindStruct, n)
	e.addEdge(fileCanon, "defines", name)
}

func (e *cExtractor) handleEnum(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	e.addSymbol(name, KindType)
	e.addEdge(fileCanon, "defines", name)
}

func (e *cExtractor) handleTypedef(fileCanon string, n *sitter.Node) {
	// type_definition has a declarator child with the alias name.
	count := int(n.NamedChildCount())
	for i := 0; i < count; i++ {
		child := n.NamedChild(i)
		if child == nil {
			continue
		}
		if child.Type() == "type_identifier" {
			name := e.text(child)
			e.addSymbol(name, KindType)
			e.addEdge(fileCanon, "defines", name)
		}
	}
}

// C++ class
func (e *cExtractor) handleClass(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.text(nameNode)
	e.addSymbolNode(name, KindClass, n)
	e.addEdge(fileCanon, "defines", name)

	// Base classes → extends
	basesNode := n.ChildByFieldName("bases")
	if basesNode != nil {
		bc := int(basesNode.ChildCount())
		for i := 0; i < bc; i++ {
			child := basesNode.Child(i)
			if child == nil {
				continue
			}
			if child.Type() == "type_identifier" || child.Type() == "qualified_identifier" {
				e.addEdge(name, "extends", e.text(child))
			}
		}
	}

	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkItems(fileCanon, body, name)
	}
}

func (e *cExtractor) handleNamespace(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = e.text(nameNode)
		e.addSymbol(name, KindModule)
	}
	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkItems(fileCanon, body, name)
	}
}

func (e *cExtractor) collectCalls(callerName string, n *sitter.Node) {
	if n == nil {
		return
	}
	if n.Type() == "call_expression" {
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

func (e *cExtractor) calleeText(n *sitter.Node) string {
	if n == nil {
		return ""
	}
	switch n.Type() {
	case "identifier":
		return e.text(n)
	case "field_expression":
		field := n.ChildByFieldName("field")
		if field != nil {
			return e.text(field)
		}
	case "qualified_identifier":
		name := n.ChildByFieldName("name")
		if name != nil {
			return e.text(name)
		}
	}
	return ""
}