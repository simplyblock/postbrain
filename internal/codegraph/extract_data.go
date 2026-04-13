//go:build cgo

// extract_data.go contains extractors for CSS, HTML, Dockerfile, HCL,
// Protobuf, SQL, TOML, and YAML.
// These languages are not procedural; the graph focuses on named declarations
// and structural references rather than call edges.
package codegraph

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/dockerfile"
	"github.com/smacker/go-tree-sitter/hcl"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/protobuf"
	"github.com/smacker/go-tree-sitter/sql"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/yaml"
)

// ─── CSS ─────────────────────────────────────────────────────────────────────

// ExtractCSS parses CSS source (.css / .scss / .sass / .less) and returns symbols and edges.
// Symbols: selectors (class / id / element). Edges: @import → imports.
func ExtractCSS(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, css.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &cssExtractor{baseExtractor: baseExtractor{src: src, filename: filename}}
	e.run(root)
	return e.symbols, e.edges, nil
}

// baseExtractor holds the common state and helpers shared by all data-format
// extractors (CSS, HTML, Dockerfile, HCL, Proto, SQL, TOML, YAML).
type baseExtractor struct {
	src      []byte
	filename string
	symbols  []Symbol
	edges    []Edge
}

func (e *baseExtractor) text(n *sitter.Node) string {
	return string(e.src[n.StartByte():n.EndByte()])
}

func (e *baseExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, File: e.filename})
}

func (e *baseExtractor) addEdge(subject, predicate, object string) {
	if subject == "" || object == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subject, Predicate: predicate, ObjectName: object})
}

type cssExtractor struct {
	baseExtractor
}

func (e *cssExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)

	count := int(root.ChildCount())
	for i := 0; i < count; i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "rule_set":
			e.handleRuleSet(fileCanon, child)
		case "at_rule":
			e.handleAtRule(fileCanon, child)
		case "import_statement":
			// grammar emits import_statement directly for @import
			e.handleImportStatement(fileCanon, child)
		}
	}
}

func (e *cssExtractor) handleRuleSet(fileCanon string, n *sitter.Node) {
	selectors := n.ChildByFieldName("selectors")
	if selectors == nil {
		return
	}
	sel := strings.TrimSpace(e.text(selectors))
	if sel != "" {
		e.addSymbol(sel, KindVariable)
		e.addEdge(fileCanon, "defines", sel)
	}
}

// handleImportStatement handles the tree-sitter `import_statement` node for @import.
func (e *cssExtractor) handleImportStatement(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "string_value":
			// text includes surrounding quotes in some grammars; strip them
			raw := e.text(child)
			// string_value child may itself be "quote content quote"
			path := e.cssStringValue(child, raw)
			if path != "" {
				e.addEdge(fileCanon, "imports", path)
			}
		case "call_expression": // url(...)
			arg := child.ChildByFieldName("arguments")
			if arg == nil {
				arg = child
			}
			ac := int(arg.ChildCount())
			for j := 0; j < ac; j++ {
				a := arg.Child(j)
				if a != nil && (a.Type() == "string_value" || a.Type() == "plain_value") {
					e.addEdge(fileCanon, "imports", strings.Trim(e.text(a), `'"`))
				}
			}
		}
	}
}

// cssStringValue extracts the unquoted string from a string_value node.
// The grammar represents "foo.css" as [string_value ["] [content] ["]], so we
// collect the text of the non-quote named children.
func (e *cssExtractor) cssStringValue(n *sitter.Node, fallback string) string {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		t := child.Type()
		if t != "\"" && t != "'" {
			raw := e.text(child)
			if raw != "" {
				return raw
			}
		}
	}
	// fallback: strip surrounding quotes from the raw text
	return strings.Trim(fallback, `'"`)
}

func (e *cssExtractor) handleAtRule(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	isImport := false
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "at_keyword":
			if strings.EqualFold(e.text(child), "@import") {
				isImport = true
			}
		case "string_value", "url":
			if isImport {
				path := strings.Trim(e.text(child), `'"`)
				e.addEdge(fileCanon, "imports", path)
			}
		}
	}
}

// ─── HTML ────────────────────────────────────────────────────────────────────

// ExtractHTML parses HTML source (.html / .htm) and returns symbols and edges.
// Symbols: id attributes. Edges: <link href> and <script src> → imports.
func ExtractHTML(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, html.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &htmlExtractor{baseExtractor: baseExtractor{src: src, filename: filename}}
	e.run(root)
	return e.symbols, e.edges, nil
}

type htmlExtractor struct {
	baseExtractor
}

func (e *htmlExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)
	e.walkNode(fileCanon, root)
}

func (e *htmlExtractor) walkNode(fileCanon string, n *sitter.Node) {
	if n == nil {
		return
	}
	if n.Type() == "element" || n.Type() == "script_element" || n.Type() == "style_element" {
		e.handleElement(fileCanon, n)
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		e.walkNode(fileCanon, n.Child(i))
	}
}

func (e *htmlExtractor) handleElement(fileCanon string, n *sitter.Node) {
	// Find start_tag to read tag name and attributes.
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil || child.Type() != "start_tag" {
			continue
		}
		tagName := ""
		tnNode := child.ChildByFieldName("name")
		if tnNode != nil {
			tagName = strings.ToLower(e.text(tnNode))
		}

		// Scan attributes.
		ac := int(child.ChildCount())
		for j := 0; j < ac; j++ {
			attr := child.Child(j)
			if attr == nil || attr.Type() != "attribute" {
				continue
			}
			attrName := ""
			anNode := attr.ChildByFieldName("name")
			if anNode != nil {
				attrName = strings.ToLower(e.text(anNode))
			}
			attrVal := ""
			avNode := attr.ChildByFieldName("value")
			if avNode != nil {
				attrVal = strings.Trim(e.text(avNode), `'"`)
			}

			switch attrName {
			case "id":
				if attrVal != "" {
					e.addSymbol("#"+attrVal, KindVariable)
				}
			case "href":
				if (tagName == "link" || tagName == "a") && attrVal != "" {
					e.addEdge(fileCanon, "imports", attrVal)
				}
			case "src":
				if (tagName == "script" || tagName == "img") && attrVal != "" {
					e.addEdge(fileCanon, "imports", attrVal)
				}
			}
		}
	}
}

// ─── Dockerfile ──────────────────────────────────────────────────────────────

// ExtractDockerfile parses a Dockerfile and returns symbols and edges.
// Symbols: stage names (FROM … AS name). Edges: FROM → imports (base image).
func ExtractDockerfile(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, dockerfile.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &dockerfileExtractor{baseExtractor: baseExtractor{src: src, filename: filename}}
	e.run(root)
	return e.symbols, e.edges, nil
}

type dockerfileExtractor struct {
	baseExtractor
}

func (e *dockerfileExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)

	count := int(root.ChildCount())
	for i := 0; i < count; i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == "from_instruction" {
			e.handleFrom(fileCanon, child)
		}
	}
}

func (e *dockerfileExtractor) handleFrom(fileCanon string, n *sitter.Node) {
	// image reference is inside image_spec: name[:tag][@digest]
	// stage alias is the `as` field (image_alias node)
	imgRef := e.dockerfileImageRef(n)
	if imgRef != "" {
		e.addEdge(fileCanon, "imports", imgRef)
	}

	stageNode := n.ChildByFieldName("as")
	if stageNode != nil {
		name := e.text(stageNode)
		e.addSymbol(name, KindVariable)
		e.addEdge(fileCanon, "defines", name)
	}
}

func (e *dockerfileExtractor) dockerfileImageRef(n *sitter.Node) string {
	// Look for image_spec child.
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil || child.Type() != "image_spec" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		tagNode := child.ChildByFieldName("tag")
		if nameNode == nil {
			return ""
		}
		ref := e.text(nameNode)
		if tagNode != nil {
			// image_tag node: first child is ":", rest is the tag text
			tagText := e.text(tagNode)
			tagText = strings.TrimPrefix(tagText, ":")
			if tagText != "" {
				ref += ":" + tagText
			}
		}
		return ref
	}
	return ""
}

// ─── HCL (Terraform/OpenTofu) ────────────────────────────────────────────────

// ExtractHCL parses HCL source (.tf / .hcl) and returns symbols and edges.
// Symbols: resource/data/module/variable/output blocks. Edges: module source → imports.
func ExtractHCL(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, hcl.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &hclExtractor{baseExtractor: baseExtractor{src: src, filename: filename}}
	e.run(root)
	return e.symbols, e.edges, nil
}

type hclExtractor struct {
	baseExtractor
}

func (e *hclExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)

	// config_file → body → block*
	e.walkHCLNode(fileCanon, root)
}

func (e *hclExtractor) walkHCLNode(fileCanon string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "block":
			e.handleBlock(fileCanon, child)
		case "body":
			e.walkHCLNode(fileCanon, child)
		}
	}
}

func (e *hclExtractor) handleBlock(fileCanon string, n *sitter.Node) {
	blockType := ""
	labels := []string{}

	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "identifier":
			if blockType == "" {
				blockType = e.text(child)
			}
		case "string_lit":
			labels = append(labels, e.hclStringLit(child))
		}
	}

	switch blockType {
	case "resource", "data":
		if len(labels) >= 2 {
			name := blockType + "." + labels[0] + "." + labels[1]
			e.addSymbol(name, KindVariable)
			e.addEdge(fileCanon, "defines", name)
		}
	case "module":
		if len(labels) >= 1 {
			name := "module." + labels[0]
			e.addSymbol(name, KindModule)
			e.addEdge(fileCanon, "defines", name)
			// Find source attribute inside the block's body
			e.hclFindSourceAttr(name, n)
		}
	case "variable", "output", "locals":
		if len(labels) >= 1 {
			name := blockType + "." + labels[0]
			e.addSymbol(name, KindVariable)
			e.addEdge(fileCanon, "defines", name)
		}
	case "provider":
		if len(labels) >= 1 {
			name := "provider." + labels[0]
			e.addSymbol(name, KindVariable)
			e.addEdge(fileCanon, "defines", name)
		}
	}
}

// hclStringLit extracts the unquoted text from a string_lit node.
// The grammar wraps the value in quoted_template_start/end with a template_literal inside.
func (e *hclExtractor) hclStringLit(n *sitter.Node) string {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child != nil && child.Type() == "template_literal" {
			return e.text(child)
		}
	}
	return strings.Trim(e.text(n), `"`)
}

// hclFindSourceAttr walks a block looking for `source = "..."` and emits an imports edge.
func (e *hclExtractor) hclFindSourceAttr(moduleName string, block *sitter.Node) {
	count := int(block.ChildCount())
	for i := 0; i < count; i++ {
		child := block.Child(i)
		if child == nil || child.Type() != "body" {
			continue
		}
		bc := int(child.ChildCount())
		for j := 0; j < bc; j++ {
			attr := child.Child(j)
			if attr == nil || attr.Type() != "attribute" {
				continue
			}
			// attribute: identifier "=" expression
			attrName := ""
			attrVal := ""
			ac := int(attr.ChildCount())
			for k := 0; k < ac; k++ {
				a := attr.Child(k)
				if a == nil {
					continue
				}
				switch a.Type() {
				case "identifier":
					attrName = e.text(a)
				case "expression":
					attrVal = e.hclExpressionString(a)
				}
			}
			if attrName == "source" && attrVal != "" {
				e.addEdge(moduleName, "imports", attrVal)
			}
		}
	}
}

// hclExpressionString extracts a string value from an expression node.
func (e *hclExtractor) hclExpressionString(n *sitter.Node) string {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "string_lit":
			return e.hclStringLit(child)
		case "literal_value":
			return e.hclExpressionString(child)
		case "template_literal":
			return e.text(child)
		}
	}
	return strings.Trim(e.text(n), `"`)
}

// ─── Protobuf ────────────────────────────────────────────────────────────────

// ExtractProtobuf parses Protocol Buffer source (.proto) and returns symbols and edges.
// Symbols: message, enum, service, rpc definitions.
func ExtractProtobuf(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, protobuf.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &protoExtractor{baseExtractor: baseExtractor{src: src, filename: filename}}
	e.run(root)
	return e.symbols, e.edges, nil
}

type protoExtractor struct {
	baseExtractor
	pkg string
}

func (e *protoExtractor) addSymbol(name string, kind SymbolKind) {
	e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, Package: e.pkg, File: e.filename})
}

func (e *protoExtractor) qual(name string) string {
	if e.pkg == "" {
		return name
	}
	return e.pkg + "." + name
}

func (e *protoExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)

	count := int(root.ChildCount())
	for i := 0; i < count; i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "package":
			e.pkg = e.packageName(child)
		case "import":
			e.handleImport(fileCanon, child)
		case "message":
			e.handleMessage(fileCanon, child)
		case "enum":
			e.handleEnum(fileCanon, child)
		case "service":
			e.handleService(fileCanon, child)
		}
	}
}

func (e *protoExtractor) packageName(n *sitter.Node) string {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child != nil && child.Type() == "full_ident" {
			return e.text(child)
		}
	}
	return ""
}

func (e *protoExtractor) handleImport(fileCanon string, n *sitter.Node) {
	// import node has a `path` field (string node)
	pathNode := n.ChildByFieldName("path")
	if pathNode != nil {
		e.addEdge(fileCanon, "imports", strings.Trim(e.text(pathNode), `"`))
		return
	}
	// fallback: find any string child
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child != nil && child.Type() == "string" {
			path := strings.Trim(e.text(child), `"`)
			e.addEdge(fileCanon, "imports", path)
		}
	}
}

// protoName returns the name text from a node that contains a *_name child.
// e.g. message → message_name → identifier
func (e *protoExtractor) protoName(n *sitter.Node) string {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "message_name", "enum_name", "service_name", "rpc_name":
			// these contain an identifier child
			ic := int(child.ChildCount())
			for j := 0; j < ic; j++ {
				id := child.Child(j)
				if id != nil && id.Type() == "identifier" {
					return e.text(id)
				}
			}
			return e.text(child)
		}
	}
	// fallback: field name
	nameNode := n.ChildByFieldName("name")
	if nameNode != nil {
		return e.text(nameNode)
	}
	return ""
}

func (e *protoExtractor) handleMessage(fileCanon string, n *sitter.Node) {
	name := e.protoName(n)
	if name == "" {
		return
	}
	qualified := e.qual(name)
	e.addSymbol(qualified, KindStruct)
	e.addEdge(fileCanon, "defines", qualified)
}

func (e *protoExtractor) handleEnum(fileCanon string, n *sitter.Node) {
	name := e.protoName(n)
	if name == "" {
		return
	}
	qualified := e.qual(name)
	e.addSymbol(qualified, KindType)
	e.addEdge(fileCanon, "defines", qualified)
}

func (e *protoExtractor) handleService(fileCanon string, n *sitter.Node) {
	name := e.protoName(n)
	if name == "" {
		return
	}
	svcName := e.qual(name)
	e.addSymbol(svcName, KindInterface)
	e.addEdge(fileCanon, "defines", svcName)

	// Walk children looking for rpc nodes (may be inside a block or directly)
	e.walkServiceRPCs(fileCanon, svcName, n)
}

func (e *protoExtractor) walkServiceRPCs(fileCanon, svcName string, n *sitter.Node) {
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == "rpc" {
			rpcName := e.protoName(child)
			if rpcName != "" {
				rpc := svcName + "." + rpcName
				e.addSymbol(rpc, KindMethod)
				e.addEdge(fileCanon, "defines", rpc)
			}
		} else {
			e.walkServiceRPCs(fileCanon, svcName, child)
		}
	}
}

// ─── SQL ─────────────────────────────────────────────────────────────────────

// ExtractSQL parses SQL source (.sql) and returns symbols and edges.
// Symbols: table/view/function/procedure definitions.
func ExtractSQL(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, sql.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &sqlExtractor{baseExtractor: baseExtractor{src: src, filename: filename}}
	e.run(root)
	return e.symbols, e.edges, nil
}

type sqlExtractor struct {
	baseExtractor
}

func (e *sqlExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)

	count := int(root.ChildCount())
	for i := 0; i < count; i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		e.handleStatement(fileCanon, child)
	}
}

func (e *sqlExtractor) handleStatement(fileCanon string, n *sitter.Node) {
	switch n.Type() {
	case "create_table_statement":
		e.handleCreateTable(fileCanon, n)
	case "create_view_statement":
		e.handleCreateView(fileCanon, n)
	case "create_function_statement", "create_procedure_statement":
		e.handleCreateFunction(fileCanon, n)
	case "statement":
		// recurse into wrapper nodes
		count := int(n.ChildCount())
		for i := 0; i < count; i++ {
			child := n.Child(i)
			if child != nil {
				e.handleStatement(fileCanon, child)
			}
		}
	}
}

func (e *sqlExtractor) handleCreateTable(fileCanon string, n *sitter.Node) {
	name := e.findObjectName(n)
	if name == "" {
		return
	}
	e.addSymbol(name, KindStruct)
	e.addEdge(fileCanon, "defines", name)
}

func (e *sqlExtractor) handleCreateView(fileCanon string, n *sitter.Node) {
	name := e.findObjectName(n)
	if name == "" {
		return
	}
	e.addSymbol(name, KindVariable)
	e.addEdge(fileCanon, "defines", name)
}

func (e *sqlExtractor) handleCreateFunction(fileCanon string, n *sitter.Node) {
	name := e.findObjectName(n)
	if name == "" {
		return
	}
	e.addSymbol(name, KindFunction)
	e.addEdge(fileCanon, "defines", name)
}

// findObjectName walks the node looking for a name / identifier after CREATE.
func (e *sqlExtractor) findObjectName(n *sitter.Node) string {
	name := n.ChildByFieldName("name")
	if name != nil {
		return e.text(name)
	}
	// Some grammars use object_reference or relation
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "object_reference", "function_name", "table_name":
			return e.text(child)
		}
	}
	return ""
}

// ─── TOML ────────────────────────────────────────────────────────────────────

// ExtractTOML parses TOML source (.toml) and returns symbols and edges.
// Symbols: top-level tables and array-of-table headers as named entities.
func ExtractTOML(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, toml.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &tomlExtractor{baseExtractor: baseExtractor{src: src, filename: filename}}
	e.run(root)
	return e.symbols, e.edges, nil
}

type tomlExtractor struct {
	baseExtractor
}

func (e *tomlExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)

	count := int(root.ChildCount())
	for i := 0; i < count; i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "table", "table_array_element":
			name := e.tomlTableName(child)
			if name != "" {
				e.addSymbol(name, KindVariable)
				e.addEdge(fileCanon, "defines", name)
			}
		}
	}
}

// tomlTableName extracts the key text from a table or table_array_element node.
// The grammar: [table [ bare_key ] pair* ]  — bare_key is a direct child, not a field.
func (e *tomlExtractor) tomlTableName(n *sitter.Node) string {
	// Try named field first.
	for _, field := range []string{"name", "key"} {
		if k := n.ChildByFieldName(field); k != nil {
			return e.text(k)
		}
	}
	// Walk children for bare_key or quoted_key.
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "bare_key", "quoted_key", "dotted_key":
			return e.text(child)
		}
	}
	return ""
}

// ─── YAML ────────────────────────────────────────────────────────────────────

// ExtractYAML parses YAML source (.yaml / .yml) and returns symbols and edges.
// Symbols: top-level mapping keys as named entities.
func ExtractYAML(ctx context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	root, err := sitter.ParseCtx(ctx, src, yaml.GetLanguage())
	if err != nil {
		return nil, nil, err
	}
	e := &yamlExtractor{baseExtractor: baseExtractor{src: src, filename: filename}}
	e.run(root)
	return e.symbols, e.edges, nil
}

type yamlExtractor struct {
	baseExtractor
}

func (e *yamlExtractor) run(root *sitter.Node) {
	fileCanon := filepath.ToSlash(e.filename)
	e.addSymbol(fileCanon, KindFile)

	// Walk top-level: stream → document → block_node → block_mapping
	e.walkTopLevel(fileCanon, root, 0)
}

func (e *yamlExtractor) walkTopLevel(fileCanon string, n *sitter.Node, depth int) {
	if n == nil || depth > 4 {
		return
	}
	if n.Type() == "block_mapping_pair" {
		key := n.ChildByFieldName("key")
		if key != nil {
			name := strings.TrimSpace(e.text(key))
			if name != "" {
				e.addSymbol(name, KindVariable)
				e.addEdge(fileCanon, "defines", name)
			}
		}
		return // don't recurse into values
	}
	count := int(n.ChildCount())
	for i := 0; i < count; i++ {
		e.walkTopLevel(fileCanon, n.Child(i), depth+1)
	}
}
