package codegraph

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// ExtractGo parses Go source using the standard go/ast package and returns
// the symbols and structural edges defined or referenced in the file.
// It uses parser.AllErrors so partially-valid files still yield symbols.
func ExtractGo(_ context.Context, src []byte, filename string) ([]Symbol, []Edge, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.AllErrors)
	if f == nil {
		return nil, nil, err
	}

	e := &goASTExtractor{
		filename: filepath.ToSlash(filename),
		pkg:      f.Name.Name,
		tf:       fset.File(f.Pos()),
	}
	e.extract(f)
	return e.symbols, e.edges, nil
}

// ─── extractor ───────────────────────────────────────────────────────────────

type goASTExtractor struct {
	filename string
	pkg      string
	tf       *token.File
	symbols  []Symbol
	edges    []Edge
}

func (e *goASTExtractor) qual(name string) string {
	if e.pkg == "" {
		return name
	}
	return e.pkg + "." + name
}

func (e *goASTExtractor) pos2line(p token.Pos) uint32 {
	if !p.IsValid() {
		return 0
	}
	line := e.tf.Position(p).Line
	if line > 0 {
		return uint32(line - 1) // 0-based
	}
	return 0
}

func (e *goASTExtractor) pos2off(p token.Pos) uint32 {
	if !p.IsValid() {
		return 0
	}
	off := e.tf.Offset(p)
	if off < 0 {
		return 0
	}
	return uint32(off)
}

func (e *goASTExtractor) addSym(name string, kind SymbolKind, start, end token.Pos) {
	e.symbols = append(e.symbols, Symbol{
		Name:      name,
		Kind:      kind,
		Package:   e.pkg,
		File:      e.filename,
		StartLine: e.pos2line(start),
		EndLine:   e.pos2line(end),
		StartByte: e.pos2off(start),
		EndByte:   e.pos2off(end),
	})
}

func (e *goASTExtractor) addEdge(subj, pred, obj string) {
	if subj == "" || obj == "" {
		return
	}
	e.edges = append(e.edges, Edge{SubjectName: subj, Predicate: pred, ObjectName: obj})
}

func (e *goASTExtractor) extract(f *ast.File) {
	e.symbols = append(e.symbols, Symbol{Name: e.filename, Kind: KindFile, File: e.filename})

	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path != "" {
			e.addEdge(e.filename, "imports", path)
		}
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			e.handleFunc(d)
		case *ast.GenDecl:
			e.handleGenDecl(d)
		}
	}
}

func (e *goASTExtractor) handleFunc(d *ast.FuncDecl) {
	if d.Name == nil {
		return
	}
	name := d.Name.Name
	var qualified string
	var kind SymbolKind

	if d.Recv != nil && len(d.Recv.List) > 0 {
		recv := goRecvTypeName(d.Recv.List[0])
		if e.pkg != "" {
			qualified = e.pkg + ".(" + recv + ")." + name
		} else {
			qualified = "(" + recv + ")." + name
		}
		kind = KindMethod
	} else {
		qualified = e.qual(name)
		kind = KindFunction
	}

	e.addSym(qualified, kind, d.Pos(), d.End())
	e.addEdge(e.filename, "defines", qualified)

	if d.Type != nil {
		e.walkFieldListTypeRefs(qualified, "uses", d.Type.Params)
		e.walkFieldListTypeRefs(qualified, "uses", d.Type.Results)
	}

	if d.Body != nil {
		ast.Inspect(d.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if callee := goCalleeIdent(call.Fun); callee != "" {
				e.addEdge(qualified, "calls", callee)
			}
			return true
		})
	}
}

func (e *goASTExtractor) handleGenDecl(d *ast.GenDecl) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			e.handleTypeSpec(s)
		case *ast.ValueSpec:
			for _, ident := range s.Names {
				qualified := e.qual(ident.Name)
				e.addSym(qualified, KindVariable, s.Pos(), s.End())
				e.addEdge(e.filename, "defines", qualified)
			}
		}
	}
}

func (e *goASTExtractor) handleTypeSpec(s *ast.TypeSpec) {
	if s.Name == nil {
		return
	}
	qualified := e.qual(s.Name.Name)

	switch t := s.Type.(type) {
	case *ast.StructType:
		e.addSym(qualified, KindStruct, s.Pos(), s.End())
		e.addEdge(e.filename, "defines", qualified)
		if t.Fields != nil {
			e.walkFieldListTypeRefs(qualified, "uses", t.Fields)
		}
	case *ast.InterfaceType:
		e.addSym(qualified, KindInterface, s.Pos(), s.End())
		e.addEdge(e.filename, "defines", qualified)
		if t.Methods != nil {
			for _, m := range t.Methods.List {
				if len(m.Names) == 0 { // embedded interface
					for _, name := range goTypeIdentNames(m.Type) {
						e.addEdge(qualified, "extends", name)
					}
				}
			}
		}
	default:
		e.addSym(qualified, KindType, s.Pos(), s.End())
		e.addEdge(e.filename, "defines", qualified)
	}
}

func (e *goASTExtractor) walkFieldListTypeRefs(subj, pred string, fields *ast.FieldList) {
	if fields == nil {
		return
	}
	for _, f := range fields.List {
		for _, name := range goTypeIdentNames(f.Type) {
			e.addEdge(subj, pred, name)
		}
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// goRecvTypeName returns the receiver type as written, e.g. "*TokenStore".
func goRecvTypeName(f *ast.Field) string {
	switch t := f.Type.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
		// Handle generic pointer receiver: *Repo[T]
		if idx, ok := t.X.(*ast.IndexExpr); ok {
			if id, ok := idx.X.(*ast.Ident); ok {
				return "*" + id.Name
			}
		}
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr: // generic: T[X]
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

// goCalleeIdent returns the simple name of the called function.
func goCalleeIdent(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}

// goTypeIdentNames extracts named type identifiers from a type expression.
func goTypeIdentNames(expr ast.Expr) []string {
	if expr == nil {
		return nil
	}
	switch x := expr.(type) {
	case *ast.Ident:
		return []string{x.Name}
	case *ast.StarExpr:
		return goTypeIdentNames(x.X)
	case *ast.SelectorExpr:
		return []string{x.Sel.Name}
	case *ast.ArrayType:
		return goTypeIdentNames(x.Elt)
	case *ast.MapType:
		return append(goTypeIdentNames(x.Key), goTypeIdentNames(x.Value)...)
	case *ast.ChanType:
		return goTypeIdentNames(x.Value)
	case *ast.Ellipsis:
		return goTypeIdentNames(x.Elt)
	}
	return nil
}
