package codegraph

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/csharp"
)

// ExtractCSharp parses C# source (.cs) and returns symbols and edges.
func ExtractCSharp(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, csharp.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &csExtractor{src: src, filename: filename}
	e.run(root)
	return e.symbols, e.edges, nil
}

// ─── extractor ───────────────────────────────────────────────────────────────

type csExtractor struct {
	src       []byte
	filename  string
	namespace string
	symbols   []Symbol
	edges     []Edge
}

func (e *csExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *csExtractor) fieldText(n *sitter.Node, field string) string {
	c := n.ChildByFieldName(field)
	if c == nil {
		return ""
	}
	return e.text(c)
}

func (e *csExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, Package: e.namespace, File: e.filename})
}

func (e *csExtractor) addSymbolNode(name string, kind SymbolKind, n *sitter.Node) {
	e.symbols = append(e.symbols, Symbol{
		Name:      name,
		Kind:      kind,
		Package:   e.namespace,
		File:      e.filename,
		StartLine: n.StartPoint().Row,
		EndLine:   n.EndPoint().Row,
		StartByte: n.StartByte(),
		EndByte:   n.EndByte(),
	})
}

func (e *csExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

func (e *csExtractor) qual(name string) string {
	if e.namespace == "" {
		return name
	}
	return e.namespace + "." + name
}

func (e *csExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)
	e.walkMembers(fileCanon, root, "")
}

func (e *csExtractor) walkMembers(fileCanon string, n *sitter.Node, classScope string) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "using_directive":
			e.handleUsing(fileCanon, child)
		case "namespace_declaration", "file_scoped_namespace_declaration":
			e.handleNamespace(fileCanon, child)
		case "class_declaration", "record_declaration", "struct_declaration":
			e.handleClass(fileCanon, child)
		case "interface_declaration":
			e.handleInterface(fileCanon, child)
		case "enum_declaration":
			e.handleEnum(fileCanon, child)
		case "method_declaration", "constructor_declaration":
			e.handleMethod(fileCanon, child, classScope)
		case "property_declaration":
			e.handleProperty(fileCanon, child, classScope)
		case "field_declaration":
			e.handleField(fileCanon, child, classScope)
		}
	}
}

func (e *csExtractor) handleUsing(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "qualified_name", "identifier":
			e.addEdge(fileCanon, "imports", e.text(child))
		}
	}
}

func (e *csExtractor) handleNamespace(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode != nil {
		e.namespace = e.text(nameNode)
	}
	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkMembers(fileCanon, body, "")
	} else {
		// file-scoped namespace: rest of file is the body
		e.walkMembers(fileCanon, n, "")
	}
}

func (e *csExtractor) handleClass(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.qual(e.text(nameNode))

	kind := KindClass
	if n.Type() == "struct_declaration" {
		kind = KindStruct
	}
	e.addSymbolNode(name, kind, n)
	e.addEdge(fileCanon, "defines", name)

	// Base types → extends / implements
	bases := n.ChildByFieldName("bases")
	if bases != nil {
		bc := int(bases.ChildCount())
		for i := 0; i < bc; i++ {
			child := bases.Child(i)
			if child == nil || !child.IsNamed() {
				continue
			}
			switch child.Type() {
			case "identifier", "qualified_name", "generic_name":
				e.addEdge(name, "extends", e.text(child))
			}
		}
	}

	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkMembers(fileCanon, body, name)
	}
}

func (e *csExtractor) handleInterface(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.qual(e.text(nameNode))
	e.addSymbolNode(name, KindInterface, n)
	e.addEdge(fileCanon, "defines", name)

	bases := n.ChildByFieldName("bases")
	if bases != nil {
		e.walkTypeRefs(name, "extends", bases)
	}

	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkMembers(fileCanon, body, name)
	}
}

func (e *csExtractor) handleEnum(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.qual(e.text(nameNode))
	e.addSymbol(name, KindType)
	e.addEdge(fileCanon, "defines", name)
}

func (e *csExtractor) handleMethod(fileCanon string, n *sitter.Node, classScope string) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	rawName := e.text(nameNode)
	var name string
	if classScope != "" {
		name = classScope + "." + rawName
	} else {
		name = e.qual(rawName)
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

func (e *csExtractor) handleProperty(fileCanon string, n *sitter.Node, classScope string) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	rawName := e.text(nameNode)
	var name string
	if classScope != "" {
		name = classScope + "." + rawName
	} else {
		name = e.qual(rawName)
	}
	e.addSymbol(name, KindVariable)
	e.addEdge(fileCanon, "defines", name)
}

func (e *csExtractor) handleField(fileCanon string, n *sitter.Node, classScope string) {
	// field_declaration → variable_declaration → variable_declarator
	varDecl := n.ChildByFieldName("declaration")
	if varDecl == nil {
		return
	}
	count := int(varDecl.ChildCount())
	for i := 0; i < count; i++ {
		child := varDecl.Child(i)
		if child == nil || child.Type() != "variable_declarator" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		rawName := e.text(nameNode)
		var name string
		if classScope != "" {
			name = classScope + "." + rawName
		} else {
			name = e.qual(rawName)
		}
		e.addSymbol(name, KindVariable)
		e.addEdge(fileCanon, "defines", name)
	}
}

func (e *csExtractor) walkTypeRefs(subject, predicate string, n *sitter.Node) {
	if n == nil {
		return
	}
	switch n.Type() {
	case "identifier", "qualified_name", "generic_name":
		e.addEdge(subject, predicate, e.text(n))
		return
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		e.walkTypeRefs(subject, predicate, n.Child(i))
	}
}

func (e *csExtractor) collectCalls(callerName string, n *sitter.Node) {
	if n == nil {
		return
	}
	if n.Type() == "invocation_expression" {
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

func (e *csExtractor) calleeText(n *sitter.Node) string {
	if n == nil {
		return ""
	}
	switch n.Type() {
	case "identifier":
		return e.text(n)
	case "member_access_expression":
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			return e.text(nameNode)
		}
	case "qualified_name":
		// keep rightmost component
		count := int(n.NamedChildCount())
		if count > 0 {
			return e.text(n.NamedChild(count - 1))
		}
	}
	return strings.TrimSpace(e.text(n))
}