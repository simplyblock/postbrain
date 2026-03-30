package codegraph

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/rust"
)

// ExtractRust parses Rust source with tree-sitter and returns symbols and edges.
func ExtractRust(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, rust.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &rustExtractor{src: src, filename: filename}
	e.run(root)
	return e.symbols, e.edges, nil
}

// ─── extractor ───────────────────────────────────────────────────────────────

type rustExtractor struct {
	src      []byte
	filename string
	// crateName is the crate/module inferred from the filename (best-effort).
	crateName string
	symbols   []Symbol
	edges     []Edge
}

func (e *rustExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *rustExtractor) fieldText(n *sitter.Node, field string) string {
	c := n.ChildByFieldName(field)
	if c == nil {
		return ""
	}
	return e.text(c)
}

func (e *rustExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, Package: e.crateName, File: e.filename})
}

func (e *rustExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

// moduleName derives a module identifier from the filename.
func (e *rustExtractor) moduleName() string {
	base := filepath.Base(e.filename)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	// lib.rs and main.rs are root modules; use parent dir name instead.
	if name == "lib" || name == "main" {
		parent := filepath.Base(filepath.Dir(e.filename))
		if parent != "." && parent != "/" {
			return parent
		}
	}
	return name
}

func (e *rustExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.crateName = e.moduleName()
	e.addSymbol(fileCanon, KindFile)
	e.walkItems(fileCanon, root, "")
}

// walkItems iterates over items in a source_file, mod block, or impl block.
// implType is non-empty when inside an impl block.
func (e *rustExtractor) walkItems(fileCanon string, n *sitter.Node, implType string) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "use_declaration":
			e.handleUse(fileCanon, child)
		case "function_item":
			e.handleFunction(fileCanon, child, implType)
		case "struct_item":
			e.handleStruct(fileCanon, child)
		case "enum_item":
			e.handleEnum(fileCanon, child)
		case "trait_item":
			e.handleTrait(fileCanon, child)
		case "impl_item":
			e.handleImpl(fileCanon, child)
		case "mod_item":
			e.handleMod(fileCanon, child)
		case "type_item":
			e.handleTypeAlias(fileCanon, child)
		case "const_item", "static_item":
			e.handleConst(fileCanon, child)
		}
	}
}

// handleUse processes use declarations and emits file → imports → path edges.
func (e *rustExtractor) handleUse(fileCanon string, n *sitter.Node) {
	arg := n.ChildByFieldName("argument")
	if arg == nil {
		return
	}
	paths := e.collectUsePaths(arg)
	for _, p := range paths {
		e.addEdge(fileCanon, "imports", p)
	}
}

// collectUsePaths recursively collects all use paths from a use_tree node.
func (e *rustExtractor) collectUsePaths(n *sitter.Node) []string {
	if n == nil {
		return nil
	}
	switch n.Type() {
	case "scoped_identifier", "identifier":
		return []string{e.text(n)}
	case "use_wildcard":
		// e.g. std::io::* — report the prefix
		count := int(n.ChildCount())
		for i := 0; i < count; i++ {
			child := n.Child(i)
			if child != nil && (child.Type() == "scoped_identifier" || child.Type() == "identifier") {
				return []string{e.text(child)}
			}
		}
	case "use_as_clause":
		path := n.ChildByFieldName("path")
		if path != nil {
			return []string{e.text(path)}
		}
	case "use_list":
		var out []string
		count := int(n.ChildCount())
		for i := 0; i < count; i++ {
			child := n.Child(i)
			if child == nil || !child.IsNamed() {
				continue
			}
			out = append(out, e.collectUsePaths(child)...)
		}
		return out
	case "scoped_use_list":
		// e.g. std::io::{Read, Write}
		path := n.ChildByFieldName("path")
		list := n.ChildByFieldName("list")
		prefix := ""
		if path != nil {
			prefix = e.text(path)
		}
		if list != nil {
			sub := e.collectUsePaths(list)
			for i, s := range sub {
				if prefix != "" {
					sub[i] = prefix + "::" + s
				} else {
					sub[i] = s
				}
			}
			return sub
		}
		if prefix != "" {
			return []string{prefix}
		}
	}
	return nil
}

func (e *rustExtractor) handleFunction(fileCanon string, n *sitter.Node, implType string) {
	name := e.fieldText(n, "name")
	if name == "" {
		return
	}
	var qualified string
	if implType != "" {
		qualified = implType + "::" + name
	} else {
		mod := e.moduleName()
		if mod != "" {
			qualified = mod + "::" + name
		} else {
			qualified = name
		}
	}

	kind := KindFunction
	if implType != "" {
		kind = KindMethod
	}
	e.addSymbol(qualified, kind)
	e.addEdge(fileCanon, "defines", qualified)

	// Walk body for calls.
	body := n.ChildByFieldName("body")
	if body != nil {
		e.collectCalls(qualified, body)
	}
}

func (e *rustExtractor) handleStruct(fileCanon string, n *sitter.Node) {
	name := e.fieldText(n, "name")
	if name == "" {
		return
	}
	mod := e.moduleName()
	qualified := name
	if mod != "" {
		qualified = mod + "::" + name
	}
	e.addSymbol(qualified, KindStruct)
	e.addEdge(fileCanon, "defines", qualified)
}

func (e *rustExtractor) handleEnum(fileCanon string, n *sitter.Node) {
	name := e.fieldText(n, "name")
	if name == "" {
		return
	}
	mod := e.moduleName()
	qualified := name
	if mod != "" {
		qualified = mod + "::" + name
	}
	e.addSymbol(qualified, KindType)
	e.addEdge(fileCanon, "defines", qualified)
}

func (e *rustExtractor) handleTrait(fileCanon string, n *sitter.Node) {
	name := e.fieldText(n, "name")
	if name == "" {
		return
	}
	mod := e.moduleName()
	qualified := name
	if mod != "" {
		qualified = mod + "::" + name
	}
	e.addSymbol(qualified, KindInterface)
	e.addEdge(fileCanon, "defines", qualified)

	// Walk body for default methods.
	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkItems(fileCanon, body, qualified)
	}
}

func (e *rustExtractor) handleImpl(fileCanon string, n *sitter.Node) {
	// impl Type — or impl Trait for Type
	typeNode := n.ChildByFieldName("type")
	traitNode := n.ChildByFieldName("trait")

	typeName := ""
	if typeNode != nil {
		typeName = e.text(typeNode)
	}
	// Qualify with module.
	mod := e.moduleName()
	implType := typeName
	if mod != "" && typeName != "" {
		implType = mod + "::" + typeName
	}

	// If this is `impl Trait for Type`, emit struct → implements → trait.
	if traitNode != nil && typeName != "" {
		e.addEdge(implType, "implements", e.text(traitNode))
	}

	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkItems(fileCanon, body, implType)
	}
}

func (e *rustExtractor) handleMod(fileCanon string, n *sitter.Node) {
	name := e.fieldText(n, "name")
	if name == "" {
		return
	}
	mod := e.moduleName()
	qualified := name
	if mod != "" {
		qualified = mod + "::" + name
	}
	e.addSymbol(qualified, KindModule)
	e.addEdge(fileCanon, "defines", qualified)

	body := n.ChildByFieldName("body")
	if body != nil {
		e.walkItems(fileCanon, body, "")
	}
}

func (e *rustExtractor) handleTypeAlias(fileCanon string, n *sitter.Node) {
	name := e.fieldText(n, "name")
	if name == "" {
		return
	}
	mod := e.moduleName()
	qualified := name
	if mod != "" {
		qualified = mod + "::" + name
	}
	e.addSymbol(qualified, KindType)
	e.addEdge(fileCanon, "defines", qualified)
}

func (e *rustExtractor) handleConst(fileCanon string, n *sitter.Node) {
	name := e.fieldText(n, "name")
	if name == "" {
		return
	}
	mod := e.moduleName()
	qualified := name
	if mod != "" {
		qualified = mod + "::" + name
	}
	e.addSymbol(qualified, KindVariable)
	e.addEdge(fileCanon, "defines", qualified)
}

func (e *rustExtractor) collectCalls(callerName string, n *sitter.Node) {
	if n == nil {
		return
	}
	switch n.Type() {
	case "call_expression":
		fn := n.ChildByFieldName("function")
		if fn != nil {
			callee := e.calleeText(fn)
			if callee != "" {
				e.addEdge(callerName, "calls", callee)
			}
		}
	case "macro_invocation":
		// e.g. println!(...) — treat macro name as a call
		macro := n.ChildByFieldName("macro")
		if macro == nil {
			macro = n.Child(0)
		}
		if macro != nil {
			e.addEdge(callerName, "calls", e.text(macro)+"!")
		}
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		e.collectCalls(callerName, n.Child(i))
	}
}

func (e *rustExtractor) calleeText(n *sitter.Node) string {
	if n == nil {
		return ""
	}
	switch n.Type() {
	case "identifier":
		return e.text(n)
	case "scoped_identifier":
		// e.g. std::mem::drop → "drop"
		name := n.ChildByFieldName("name")
		if name != nil {
			return e.text(name)
		}
		return e.text(n)
	case "field_expression":
		// e.g. self.method → "method"
		field := n.ChildByFieldName("field")
		if field != nil {
			return e.text(field)
		}
	}
	return ""
}