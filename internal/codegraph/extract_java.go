package codegraph

import (
	"context"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/kotlin"
)

// ExtractJava parses Java source (.java) and returns symbols and edges.
func ExtractJava(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, java.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &javaExtractor{src: src, filename: filename}
	e.run(root)
	return e.symbols, e.edges, nil
}

// ExtractKotlin parses Kotlin source (.kt / .kts) and returns symbols and edges.
func ExtractKotlin(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, kotlin.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &javaExtractor{src: src, filename: filename, isKotlin: true}
	e.run(root)
	return e.symbols, e.edges, nil
}

// ─── extractor (shared Java/Kotlin) ─────────────────────────────────────────

type javaExtractor struct {
	src      []byte
	filename string
	isKotlin bool
	pkg      string
	symbols  []Symbol
	edges    []Edge
}

func (e *javaExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *javaExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, Package: e.pkg, File: e.filename})
}

func (e *javaExtractor) addSymbolNode(name string, kind SymbolKind, n *sitter.Node) {
	e.symbols = append(e.symbols, Symbol{
		Name:      name,
		Kind:      kind,
		Package:   e.pkg,
		File:      e.filename,
		StartLine: n.StartPoint().Row,
		EndLine:   n.EndPoint().Row,
		StartByte: n.StartByte(),
		EndByte:   n.EndByte(),
	})
}

func (e *javaExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

func (e *javaExtractor) qual(name string) string {
	if e.pkg == "" {
		return name
	}
	return e.pkg + "." + name
}

func (e *javaExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)

	count := int(root.ChildCount())
	for i := 0; i < count; i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "package_declaration":
			e.pkg = e.packageName(child)
		case "import_declaration":
			e.handleImport(fileCanon, child)
		case "class_declaration", "record_declaration":
			e.handleClass(fileCanon, child)
		case "interface_declaration":
			e.handleInterface(fileCanon, child)
		case "enum_declaration":
			e.handleEnum(fileCanon, child)
		case "annotation_type_declaration":
			e.handleAnnotationType(fileCanon, child)
		// Kotlin top-level
		case "function_declaration":
			e.handleFunction(fileCanon, child, "")
		case "property_declaration":
			e.handleProperty(fileCanon, child)
		case "object_declaration":
			e.handleClass(fileCanon, child)
		}
	}
}

func (e *javaExtractor) packageName(n *sitter.Node) string {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "scoped_identifier", "identifier":
			return e.text(child)
		}
	}
	return ""
}

func (e *javaExtractor) handleImport(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "scoped_identifier", "identifier":
			e.addEdge(fileCanon, "imports", e.text(child))
		}
	}
}

func (e *javaExtractor) handleClass(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.qual(e.text(nameNode))
	e.addSymbolNode(name, KindClass, n)
	e.addEdge(fileCanon, "defines", name)

	// Superclass → extends
	superclass := n.ChildByFieldName("superclass")
	if superclass == nil {
		// Kotlin: primary_constructor or delegationSpecifiers
		superclass = n.ChildByFieldName("delegationSpecifiers")
	}
	if superclass != nil {
		e.walkTypeRefs(name, "extends", superclass)
	}

	// Interfaces → implements
	interfaces := n.ChildByFieldName("interfaces")
	if interfaces != nil {
		e.walkTypeRefs(name, "implements", interfaces)
	}

	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkClassBody(fileCanon, name, body)
	}
}

func (e *javaExtractor) handleInterface(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.qual(e.text(nameNode))
	e.addSymbolNode(name, KindInterface, n)
	e.addEdge(fileCanon, "defines", name)

	extends := n.ChildByFieldName("extends_interfaces")
	if extends != nil {
		e.walkTypeRefs(name, "extends", extends)
	}

	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkClassBody(fileCanon, name, body)
	}
}

func (e *javaExtractor) handleEnum(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.qual(e.text(nameNode))
	e.addSymbol(name, KindType)
	e.addEdge(fileCanon, "defines", name)
}

func (e *javaExtractor) handleAnnotationType(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.qual(e.text(nameNode))
	e.addSymbol(name, KindType)
	e.addEdge(fileCanon, "defines", name)
}

func (e *javaExtractor) walkClassBody(fileCanon, className string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "method_declaration", "constructor_declaration":
			e.handleMethod(fileCanon, className, child)
		case "class_declaration", "record_declaration":
			e.handleClass(fileCanon, child)
		case "interface_declaration":
			e.handleInterface(fileCanon, child)
		case "enum_declaration":
			e.handleEnum(fileCanon, child)
		// Kotlin
		case "function_declaration":
			e.handleFunction(fileCanon, child, className)
		case "property_declaration":
			e.handleProperty(fileCanon, child)
		}
	}
}

func (e *javaExtractor) handleMethod(fileCanon, className string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := className + "." + e.text(nameNode)
	kind := KindMethod
	if n.Type() == "constructor_declaration" {
		kind = KindFunction
	}
	e.addSymbolNode(name, kind, n)
	e.addEdge(fileCanon, "defines", name)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(name, body)
	}
}

func (e *javaExtractor) handleFunction(fileCanon string, n *sitter.Node, className string) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	rawName := e.text(nameNode)
	var name string
	if className != "" {
		name = className + "." + rawName
	} else {
		name = e.qual(rawName)
	}
	kind := KindFunction
	if className != "" {
		kind = KindMethod
	}
	e.addSymbolNode(name, kind, n)
	e.addEdge(fileCanon, "defines", name)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(name, body)
	}
}

func (e *javaExtractor) handleProperty(fileCanon string, n *sitter.Node) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := e.qual(e.text(nameNode))
	e.addSymbol(name, KindVariable)
	e.addEdge(fileCanon, "defines", name)
}

func (e *javaExtractor) walkTypeRefs(subject, predicate string, n *sitter.Node) {
	if n == nil {
		return
	}
	switch n.Type() {
	case "type_identifier", "identifier":
		typeName := e.text(n)
		if typeName != "" {
			e.addEdge(subject, predicate, typeName)
		}
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		e.walkTypeRefs(subject, predicate, n.Child(i))
	}
}

func (e *javaExtractor) collectCalls(callerName string, n *sitter.Node) {
	if n == nil {
		return
	}
	if n.Type() == "method_invocation" {
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
