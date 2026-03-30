package codegraph

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
)

// ExtractGo parses Go source code with tree-sitter and returns the symbols and
// structural edges defined or referenced in the file.
func ExtractGo(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, golang.GetLanguage())
	if err != nil {
		return nil, nil, err
	}

	e := &goExtractor{src: src, filename: filename}
	e.pkg = goPackageName(root, src)
	e.run(root)
	return e.symbols, e.edges, nil
}

// ─── extractor ───────────────────────────────────────────────────────────────

type goExtractor struct {
	src      []byte
	filename string
	pkg      string
	symbols  []Symbol
	edges    []Edge
}

func (e *goExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *goExtractor) fieldText(n *sitter.Node, field string) string {
	c := n.ChildByFieldName(field)
	if c == nil {
		return ""
	}
	return e.text(c)
}

func (e *goExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, Package: e.pkg, File: e.filename})
}

func (e *goExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

func (e *goExtractor) qual(name string) string {
	if e.pkg == "" || name == "" {
		return name
	}
	return e.pkg + "." + name
}

func (e *goExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)

	count := int(root.ChildCount())
	for i := 0; i < count; i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "import_declaration":
			e.handleImport(fileCanon, child)
		case "function_declaration":
			e.handleFunction(fileCanon, child)
		case "method_declaration":
			e.handleMethod(fileCanon, child)
		case "type_declaration":
			e.handleTypeDecl(fileCanon, child)
		case "var_declaration", "const_declaration":
			e.handleVarConst(fileCanon, child)
		}
	}
}

func (e *goExtractor) handleImport(fileCanon string, n *sitter.Node) {
	e.walkImportSpecs(fileCanon, n)
}

func (e *goExtractor) walkImportSpecs(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "import_spec":
			path := e.fieldText(child, "path")
			path = strings.Trim(path, `"`)
			if path != "" {
				e.addEdge(fileCanon, "imports", path)
			}
		case "import_spec_list":
			e.walkImportSpecs(fileCanon, child)
		}
	}
}

func (e *goExtractor) handleFunction(fileCanon string, n *sitter.Node) {
	name := e.fieldText(n, "name")
	if name == "" {
		return
	}
	qualified := e.qual(name)
	e.addSymbol(qualified, KindFunction)
	e.addEdge(fileCanon, "defines", qualified)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(qualified, body)
		e.collectTypeUses(qualified, n)
	}
}

func (e *goExtractor) handleMethod(fileCanon string, n *sitter.Node) {
	name := e.fieldText(n, "name")
	recv := e.receiverType(n)
	if name == "" {
		return
	}
	var qualified string
	if recv != "" {
		if e.pkg != "" {
			qualified = e.pkg + ".(" + recv + ")." + name
		} else {
			qualified = "(" + recv + ")." + name
		}
	} else {
		qualified = e.qual(name)
	}
	e.addSymbol(qualified, KindMethod)
	e.addEdge(fileCanon, "defines", qualified)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(qualified, body)
		e.collectTypeUses(qualified, n)
	}
}

func (e *goExtractor) receiverType(n *sitter.Node) string {
	params := n.ChildByFieldName("receiver")
	if params == nil {
		return ""
	}
	count := int(params.ChildCount())
	for i := 0; i < count; i++ {
		child := params.Child(i)
		if child == nil || child.Type() != "parameter_declaration" {
			continue
		}
		typeNode := child.ChildByFieldName("type")
		if typeNode == nil {
			typeNode = child.NamedChild(int(child.NamedChildCount()) - 1)
		}
		if typeNode != nil {
			return e.text(typeNode)
		}
	}
	return ""
}

func (e *goExtractor) handleTypeDecl(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == "type_spec" || child.Type() == "type_alias" {
			e.handleTypeSpec(fileCanon, child)
		}
	}
}

func (e *goExtractor) handleTypeSpec(fileCanon string, n *sitter.Node) {
	name := e.fieldText(n, "name")
	if name == "" {
		return
	}
	qualified := e.qual(name)

	typeVal := n.ChildByFieldName("type")
	kind := KindType
	if typeVal != nil {
		switch typeVal.Type() {
		case "struct_type":
			kind = KindStruct
		case "interface_type":
			kind = KindInterface
		}
	}
	e.addSymbol(qualified, kind)
	e.addEdge(fileCanon, "defines", qualified)

	if kind == KindStruct && typeVal != nil {
		e.collectFieldTypes(qualified, typeVal)
	}
	if kind == KindInterface && typeVal != nil {
		e.collectInterfaceEmbeds(qualified, typeVal)
	}
}

func (e *goExtractor) handleVarConst(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		if child.Type() != "var_spec" && child.Type() != "const_spec" {
			continue
		}
		name := e.fieldText(child, "name")
		if name == "" {
			nc := int(child.NamedChildCount())
			for j := 0; j < nc; j++ {
				nc2 := child.NamedChild(j)
				if nc2 != nil && nc2.Type() == "identifier" {
					qualified := e.qual(e.text(nc2))
					e.addSymbol(qualified, KindVariable)
					e.addEdge(fileCanon, "defines", qualified)
				}
			}
		} else {
			qualified := e.qual(name)
			e.addSymbol(qualified, KindVariable)
			e.addEdge(fileCanon, "defines", qualified)
		}
	}
}

func (e *goExtractor) collectCalls(callerName string, n *sitter.Node) {
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

func (e *goExtractor) calleeText(n *sitter.Node) string {
	if n == nil {
		return ""
	}
	switch n.Type() {
	case "identifier":
		return e.text(n)
	case "selector_expression":
		sel := n.ChildByFieldName("field")
		if sel != nil {
			return e.text(sel)
		}
		return e.text(n)
	default:
		return e.text(n)
	}
}

func (e *goExtractor) collectTypeUses(callerName string, n *sitter.Node) {
	if n == nil {
		return
	}
	params := n.ChildByFieldName("parameters")
	if params != nil {
		e.walkTypeRefs(callerName, "uses", params)
	}
	result := n.ChildByFieldName("result")
	if result != nil {
		e.walkTypeRefs(callerName, "uses", result)
	}
}

func (e *goExtractor) collectFieldTypes(structName string, structNode *sitter.Node) {
	count := int(structNode.ChildCount())
	for i := 0; i < count; i++ {
		child := structNode.Child(i)
		if child == nil || child.Type() != "field_declaration_list" {
			continue
		}
		e.walkTypeRefs(structName, "uses", child)
	}
}

func (e *goExtractor) collectInterfaceEmbeds(ifaceName string, ifaceNode *sitter.Node) {
	count := int(ifaceNode.ChildCount())
	for i := 0; i < count; i++ {
		child := ifaceNode.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == "type_elem" || child.Type() == "interface_type_name" {
			typeName := e.text(child)
			if typeName != "" && !strings.ContainsAny(typeName, "{}()") {
				e.addEdge(ifaceName, "extends", typeName)
			}
		}
	}
}

func (e *goExtractor) walkTypeRefs(subject, predicate string, n *sitter.Node) {
	if n == nil {
		return
	}
	switch n.Type() {
	case "type_identifier":
		typeName := e.text(n)
		if typeName != "" {
			e.addEdge(subject, predicate, typeName)
		}
	case "qualified_type":
		sel := n.ChildByFieldName("name")
		if sel != nil {
			e.addEdge(subject, predicate, e.text(sel))
		}
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		e.walkTypeRefs(subject, predicate, n.Child(i))
	}
}

// goPackageName returns the Go package name from a source_file root node.
func goPackageName(root *sitter.Node, src []byte) string {
	count := int(root.ChildCount())
	for i := 0; i < count; i++ {
		child := root.Child(i)
		if child == nil || child.Type() != "package_clause" {
			continue
		}
		nc := int(child.NamedChildCount())
		for j := 0; j < nc; j++ {
			n := child.NamedChild(j)
			if n != nil && n.Type() == "package_identifier" {
				return string(src[n.StartByte():n.EndByte()])
			}
		}
	}
	return ""
}