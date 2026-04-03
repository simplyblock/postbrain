//go:build !cgo

package codegraph

import (
	"context"
	"strings"
)

func unsupportedByNoCGO(ext string) error {
	return ErrUnsupportedLanguage{Ext: strings.ToLower(ext)}
}

func ExtractPython(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".py")
}

func ExtractTypeScript(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".ts")
}

func ExtractJavaScript(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".js")
}

func ExtractRust(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".rs")
}

func ExtractBash(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".sh")
}

func ExtractLua(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".lua")
}

func ExtractPHP(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".php")
}

func ExtractRuby(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".rb")
}

func ExtractC(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".c")
}

func ExtractCPP(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".cpp")
}

func ExtractCSharp(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".cs")
}

func ExtractJava(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".java")
}

func ExtractKotlin(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".kt")
}

func ExtractCSS(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".css")
}

func ExtractHTML(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".html")
}

func ExtractDockerfile(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO("dockerfile")
}

func ExtractHCL(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".hcl")
}

func ExtractProtobuf(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".proto")
}

func ExtractSQL(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".sql")
}

func ExtractTOML(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".toml")
}

func ExtractYAML(context.Context, []byte, string) ([]Symbol, []Edge, error) {
	return nil, nil, unsupportedByNoCGO(".yaml")
}
