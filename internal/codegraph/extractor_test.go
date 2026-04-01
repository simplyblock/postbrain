package codegraph_test

import (
	"context"
	"slices"
	"testing"

	"github.com/simplyblock/postbrain/internal/codegraph"
)

// ─── Go ──────────────────────────────────────────────────────────────────────

const goSrc = `package auth

import (
	"context"
	"fmt"

	"github.com/simplyblock/postbrain/internal/db"
)

// Token holds session data.
type Token struct {
	ID    string
	Value string
}

// Storer is implemented by token stores.
type Storer interface {
	Lookup(ctx context.Context, id string) (*Token, error)
}

// TokenStore backs token lookup.
type TokenStore struct {
	pool *db.Pool
}

// Lookup finds a token by ID.
func (s *TokenStore) Lookup(ctx context.Context, id string) (*Token, error) {
	fmt.Println(id)
	return nil, nil
}

// VerifyToken parses and validates a raw token string.
func VerifyToken(ctx context.Context, raw string) (*Token, error) {
	store := &TokenStore{}
	return store.Lookup(ctx, raw)
}

var cookieName = "session"
`

func TestExtractGo_Symbols(t *testing.T) {
	syms, _, err := codegraph.ExtractGo(context.Background(), []byte(goSrc), "internal/auth/token.go")
	if err != nil {
		t.Fatalf("ExtractGo: %v", err)
	}

	wantNames := []string{
		"internal/auth/token.go",
		"auth.Token",
		"auth.Storer",
		"auth.TokenStore",
		"auth.VerifyToken",
		"auth.cookieName",
	}
	got := make(map[string]string)
	for _, s := range syms {
		got[s.Name] = string(s.Kind)
	}
	for _, name := range wantNames {
		if _, ok := got[name]; !ok {
			t.Errorf("missing symbol %q; got: %v", name, got)
		}
	}

	foundMethod := false
	for name := range got {
		if slices.Contains([]string{
			"auth.(*TokenStore).Lookup",
			"auth.(TokenStore).Lookup",
			"auth.(*auth.TokenStore).Lookup",
		}, name) {
			foundMethod = true
		}
	}
	if !foundMethod {
		t.Errorf("expected a method symbol for TokenStore.Lookup; got: %v", got)
	}
}

func TestExtractGo_Edges(t *testing.T) {
	_, edges, err := codegraph.ExtractGo(context.Background(), []byte(goSrc), "internal/auth/token.go")
	if err != nil {
		t.Fatalf("ExtractGo: %v", err)
	}

	type kv struct{ pred, obj string }
	bySubject := make(map[string][]kv)
	for _, ed := range edges {
		bySubject[ed.SubjectName] = append(bySubject[ed.SubjectName], kv{ed.Predicate, ed.ObjectName})
	}

	fileEdges := bySubject["internal/auth/token.go"]

	for _, pkg := range []string{"context", "fmt", "github.com/simplyblock/postbrain/internal/db"} {
		if !slices.ContainsFunc(fileEdges, func(e kv) bool { return e.pred == "imports" && e.obj == pkg }) {
			t.Errorf("expected file → imports → %q; got: %v", pkg, fileEdges)
		}
	}

	for _, name := range []string{"auth.Token", "auth.Storer", "auth.TokenStore", "auth.VerifyToken", "auth.cookieName"} {
		if !slices.ContainsFunc(fileEdges, func(e kv) bool { return e.pred == "defines" && e.obj == name }) {
			t.Errorf("expected file → defines → %q; got: %v", name, fileEdges)
		}
	}

	verifyEdges := bySubject["auth.VerifyToken"]
	if !slices.ContainsFunc(verifyEdges, func(e kv) bool { return e.pred == "calls" && e.obj == "Lookup" }) {
		t.Errorf("expected auth.VerifyToken → calls → Lookup; got: %v", verifyEdges)
	}
}

func TestExtractGo_KindClassification(t *testing.T) {
	syms, _, err := codegraph.ExtractGo(context.Background(), []byte(goSrc), "auth.go")
	if err != nil {
		t.Fatalf("ExtractGo: %v", err)
	}
	kindOf := make(map[string]string)
	for _, s := range syms {
		kindOf[s.Name] = string(s.Kind)
	}
	checks := map[string]string{
		"auth.Token":       "struct",
		"auth.Storer":      "interface",
		"auth.TokenStore":  "struct",
		"auth.VerifyToken": "function",
		"auth.cookieName":  "variable",
	}
	for name, want := range checks {
		if got := kindOf[name]; got != want {
			t.Errorf("symbol %q: want kind %q, got %q", name, want, got)
		}
	}
}

// ─── Python ──────────────────────────────────────────────────────────────────

const pySrc = `import os
import os.path
from collections import OrderedDict

class Animal:
    def speak(self):
        pass

class Dog(Animal):
    def speak(self):
        print("woof")

def helper():
    d = Dog()
    d.speak()
`

func TestExtractPython_Symbols(t *testing.T) {
	syms, edges, err := codegraph.ExtractPython(context.Background(), []byte(pySrc), "animals.py")
	if err != nil {
		t.Fatalf("ExtractPython: %v", err)
	}

	kindOf := make(map[string]string)
	for _, s := range syms {
		kindOf[s.Name] = string(s.Kind)
	}

	if kindOf["animals.Animal"] != "class" {
		t.Errorf("expected animals.Animal → class, got %q", kindOf["animals.Animal"])
	}
	if kindOf["animals.Dog"] != "class" {
		t.Errorf("expected animals.Dog → class, got %q", kindOf["animals.Dog"])
	}
	if kindOf["animals.helper"] != "function" {
		t.Errorf("expected animals.helper → function, got %q", kindOf["animals.helper"])
	}
	if kindOf["animals.Dog.speak"] != "method" {
		t.Errorf("expected animals.Dog.speak → method, got %q", kindOf["animals.Dog.speak"])
	}

	type kv struct{ pred, obj string }
	bySubj := make(map[string][]kv)
	for _, e := range edges {
		bySubj[e.SubjectName] = append(bySubj[e.SubjectName], kv{e.Predicate, e.ObjectName})
	}

	fileEdges := bySubj["animals.py"]
	if !slices.ContainsFunc(fileEdges, func(e kv) bool { return e.pred == "imports" && e.obj == "os" }) {
		t.Errorf("expected animals.py → imports → os; edges: %v", fileEdges)
	}
	if !slices.ContainsFunc(fileEdges, func(e kv) bool { return e.pred == "imports" && e.obj == "collections" }) {
		t.Errorf("expected animals.py → imports → collections; edges: %v", fileEdges)
	}

	dogEdges := bySubj["animals.Dog"]
	if !slices.ContainsFunc(dogEdges, func(e kv) bool { return e.pred == "extends" && e.obj == "Animal" }) {
		t.Errorf("expected animals.Dog → extends → Animal; edges: %v", dogEdges)
	}
}

// ─── TypeScript ──────────────────────────────────────────────────────────────

const tsSrc = `import { Pool } from "pg";
import fs from "fs";

export interface Repository {
  find(id: string): Promise<string>;
}

export class UserRepo implements Repository {
  private pool: Pool;

  constructor(pool: Pool) {
    this.pool = pool;
  }

  find(id: string): Promise<string> {
    return this.pool.query(id);
  }
}

export function createRepo(pool: Pool): UserRepo {
  return new UserRepo(pool);
}
`

func TestExtractTypeScript_Symbols(t *testing.T) {
	syms, edges, err := codegraph.ExtractTypeScript(context.Background(), []byte(tsSrc), "src/repo.ts")
	if err != nil {
		t.Fatalf("ExtractTypeScript: %v", err)
	}

	kindOf := make(map[string]string)
	for _, s := range syms {
		kindOf[s.Name] = string(s.Kind)
	}

	if kindOf["repo.Repository"] != "interface" {
		t.Errorf("expected repo.Repository → interface, got %q", kindOf["repo.Repository"])
	}
	if kindOf["repo.UserRepo"] != "class" {
		t.Errorf("expected repo.UserRepo → class, got %q", kindOf["repo.UserRepo"])
	}
	if kindOf["repo.createRepo"] != "function" {
		t.Errorf("expected repo.createRepo → function, got %q", kindOf["repo.createRepo"])
	}

	type kv struct{ pred, obj string }
	bySubj := make(map[string][]kv)
	for _, e := range edges {
		bySubj[e.SubjectName] = append(bySubj[e.SubjectName], kv{e.Predicate, e.ObjectName})
	}

	fileEdges := bySubj["src/repo.ts"]
	if !slices.ContainsFunc(fileEdges, func(e kv) bool { return e.pred == "imports" && e.obj == "pg" }) {
		t.Errorf("expected src/repo.ts → imports → pg; edges: %v", fileEdges)
	}
}

// ─── Rust ────────────────────────────────────────────────────────────────────

const rsSrc = `use std::collections::HashMap;
use std::io::{Read, Write};

pub struct Cache {
    data: HashMap<String, String>,
}

pub trait Store {
    fn get(&self, key: &str) -> Option<String>;
}

impl Store for Cache {
    fn get(&self, key: &str) -> Option<String> {
        self.data.get(key).cloned()
    }
}

pub fn new_cache() -> Cache {
    Cache { data: HashMap::new() }
}
`

func TestExtractRust_Symbols(t *testing.T) {
	syms, edges, err := codegraph.ExtractRust(context.Background(), []byte(rsSrc), "src/cache.rs")
	if err != nil {
		t.Fatalf("ExtractRust: %v", err)
	}

	kindOf := make(map[string]string)
	for _, s := range syms {
		kindOf[s.Name] = string(s.Kind)
	}

	if kindOf["cache::Cache"] != "struct" {
		t.Errorf("expected cache::Cache → struct, got %q", kindOf["cache::Cache"])
	}
	if kindOf["cache::Store"] != "interface" {
		t.Errorf("expected cache::Store → interface, got %q", kindOf["cache::Store"])
	}
	if kindOf["cache::new_cache"] != "function" {
		t.Errorf("expected cache::new_cache → function, got %q", kindOf["cache::new_cache"])
	}

	type kv struct{ pred, obj string }
	bySubj := make(map[string][]kv)
	for _, e := range edges {
		bySubj[e.SubjectName] = append(bySubj[e.SubjectName], kv{e.Predicate, e.ObjectName})
	}

	fileEdges := bySubj["src/cache.rs"]
	if !slices.ContainsFunc(fileEdges, func(e kv) bool {
		return e.pred == "imports" && slices.Contains([]string{
			"std::collections::HashMap",
			"std::collections",
		}, e.obj)
	}) {
		t.Errorf("expected src/cache.rs → imports → std::collections::HashMap; edges: %v", fileEdges)
	}

	// Cache implements Store
	cacheEdges := bySubj["cache::Cache"]
	if !slices.ContainsFunc(cacheEdges, func(e kv) bool { return e.pred == "implements" && e.obj == "Store" }) {
		t.Errorf("expected cache::Cache → implements → Store; edges: %v", cacheEdges)
	}
}

// ─── Dispatch ─────────────────────────────────────────────────────────────────

func TestExtract_Dispatch(t *testing.T) {
	cases := []struct {
		filename string
		src      string
	}{
		{"main.go", goSrc},
		{"script.py", pySrc},
		{"app.ts", tsSrc},
		{"lib.rs", rsSrc},
	}
	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			syms, edges, err := codegraph.Extract(context.Background(), []byte(tc.src), tc.filename)
			if err != nil {
				t.Fatalf("Extract(%q): %v", tc.filename, err)
			}
			if len(syms) == 0 {
				t.Errorf("Extract(%q): no symbols returned", tc.filename)
			}
			if len(edges) == 0 {
				t.Errorf("Extract(%q): no edges returned", tc.filename)
			}
		})
	}
}

// ─── Go edge cases ────────────────────────────────────────────────────────────

const goGenericReceiverSrc = `package repo

type Repo[T any] struct{}

func (r *Repo[T]) Method() {}
`

// TestExtractGo_GenericPointerReceiver verifies that a method on a generic
// pointer receiver (*Repo[T]) is emitted with kind "method" and a name that
// includes both the receiver type and the method name.
func TestExtractGo_GenericPointerReceiver(t *testing.T) {
	syms, _, err := codegraph.ExtractGo(context.Background(), []byte(goGenericReceiverSrc), "repo/generic.go")
	if err != nil {
		t.Fatalf("ExtractGo: %v", err)
	}

	var found bool
	for _, s := range syms {
		if string(s.Kind) == "method" && slices.ContainsFunc([]string{
			"repo.(*Repo).Method",
			"repo.(Repo).Method",
		}, func(want string) bool { return s.Name == want }) {
			found = true
		}
	}
	if !found {
		names := make([]string, len(syms))
		for i, s := range syms {
			names[i] = s.Name + "(" + string(s.Kind) + ")"
		}
		t.Errorf("expected a method symbol with receiver Repo and name Method; got: %v", names)
	}
}

const goNamedReturnSrc = `package math

func Divide(a, b int) (result int, err error) {
	result = a / b
	return
}
`

// TestExtractGo_NamedReturnValues_NoDuplicates verifies that named return
// value identifiers (result, err) are not emitted as separate symbols.
func TestExtractGo_NamedReturnValues_NoDuplicates(t *testing.T) {
	syms, _, err := codegraph.ExtractGo(context.Background(), []byte(goNamedReturnSrc), "math/ops.go")
	if err != nil {
		t.Fatalf("ExtractGo: %v", err)
	}

	for _, s := range syms {
		if s.Name == "math.result" || s.Name == "math.err" {
			t.Errorf("unexpected symbol %q — named return values must not be emitted as symbols", s.Name)
		}
	}

	// The function itself must still be present.
	var foundFn bool
	for _, s := range syms {
		if s.Name == "math.Divide" && string(s.Kind) == "function" {
			foundFn = true
		}
	}
	if !foundFn {
		t.Error("expected symbol math.Divide with kind function")
	}
}

const goIotaSrc = `package color

const (
	Red   = iota
	Green
	Blue
)
`

// TestExtractGo_ConstIota_AllVariable verifies that all names in a const/iota
// block are emitted with kind "variable".
func TestExtractGo_ConstIota_AllVariable(t *testing.T) {
	syms, _, err := codegraph.ExtractGo(context.Background(), []byte(goIotaSrc), "color/color.go")
	if err != nil {
		t.Fatalf("ExtractGo: %v", err)
	}

	kindOf := make(map[string]string)
	for _, s := range syms {
		kindOf[s.Name] = string(s.Kind)
	}

	for _, name := range []string{"color.Red", "color.Green", "color.Blue"} {
		if kindOf[name] != "variable" {
			t.Errorf("symbol %q: want kind %q, got %q", name, "variable", kindOf[name])
		}
	}
}

// ─── TypeScript edge cases ────────────────────────────────────────────────────

const tsExportDefaultFnSrc = `export default function greet(name: string): string {
  return "hello " + name;
}
`

// TestExtractTypeScript_ExportDefaultFunction verifies that an
// `export default function` declaration is extracted as a function symbol.
func TestExtractTypeScript_ExportDefaultFunction(t *testing.T) {
	syms, _, err := codegraph.ExtractTypeScript(context.Background(), []byte(tsExportDefaultFnSrc), "src/greet.ts")
	if err != nil {
		t.Fatalf("ExtractTypeScript: %v", err)
	}

	var found bool
	for _, s := range syms {
		if s.Name == "greet.greet" && string(s.Kind) == "function" {
			found = true
		}
	}
	if !found {
		kindOf := make(map[string]string)
		for _, s := range syms {
			kindOf[s.Name] = string(s.Kind)
		}
		t.Errorf("expected symbol greet.greet with kind function; got: %v", kindOf)
	}
}

const tsExtendsImplementsSrc = `class Animal {}
interface Serializable {}

class Dog extends Animal implements Serializable {
  bark(): void {}
}
`

// TestExtractTypeScript_ClassExtendsImplements verifies that both the extends
// and implements edges are emitted for a class with both clauses.
func TestExtractTypeScript_ClassExtendsImplements(t *testing.T) {
	_, edges, err := codegraph.ExtractTypeScript(context.Background(), []byte(tsExtendsImplementsSrc), "src/dog.ts")
	if err != nil {
		t.Fatalf("ExtractTypeScript: %v", err)
	}

	type kv struct{ pred, obj string }
	bySubj := make(map[string][]kv)
	for _, e := range edges {
		bySubj[e.SubjectName] = append(bySubj[e.SubjectName], kv{e.Predicate, e.ObjectName})
	}

	dogEdges := bySubj["dog.Dog"]
	if !slices.ContainsFunc(dogEdges, func(e kv) bool { return e.pred == "extends" && e.obj == "Animal" }) {
		t.Errorf("expected dog.Dog → extends → Animal; got: %v", dogEdges)
	}
	if !slices.ContainsFunc(dogEdges, func(e kv) bool { return e.pred == "implements" && e.obj == "Serializable" }) {
		t.Errorf("expected dog.Dog → implements → Serializable; got: %v", dogEdges)
	}
}

// ─── Rust edge cases ─────────────────────────────────────────────────────────

const rsImplTraitSrc = `pub trait Greeter {
    fn greet(&self) -> String;
}

pub struct Hello {}

impl Greeter for Hello {
    fn greet(&self) -> String {
        String::from("hello")
    }
}
`

// TestExtractRust_ImplTraitFor verifies that `impl Trait for Type` emits an
// implements edge from the type to the trait, and that methods inside the
// impl block have kind "method".
func TestExtractRust_ImplTraitFor(t *testing.T) {
	syms, edges, err := codegraph.ExtractRust(context.Background(), []byte(rsImplTraitSrc), "src/hello.rs")
	if err != nil {
		t.Fatalf("ExtractRust: %v", err)
	}

	type kv struct{ pred, obj string }
	bySubj := make(map[string][]kv)
	for _, e := range edges {
		bySubj[e.SubjectName] = append(bySubj[e.SubjectName], kv{e.Predicate, e.ObjectName})
	}

	helloEdges := bySubj["hello::Hello"]
	if !slices.ContainsFunc(helloEdges, func(e kv) bool { return e.pred == "implements" && e.obj == "Greeter" }) {
		t.Errorf("expected hello::Hello → implements → Greeter; got: %v", helloEdges)
	}

	kindOf := make(map[string]string)
	for _, s := range syms {
		kindOf[s.Name] = string(s.Kind)
	}
	if kindOf["hello::Hello::greet"] != "method" {
		t.Errorf("expected hello::Hello::greet → method, got %q", kindOf["hello::Hello::greet"])
	}
}

// ─── Python edge cases ────────────────────────────────────────────────────────

const pyDecoratedFnSrc = `def my_decorator(func):
    return func

@my_decorator
def foo():
    pass
`

// TestExtractPython_DecoratedFunction verifies that a function wrapped in a
// decorator is still extracted with kind "function".
func TestExtractPython_DecoratedFunction(t *testing.T) {
	syms, _, err := codegraph.ExtractPython(context.Background(), []byte(pyDecoratedFnSrc), "src/decorators.py")
	if err != nil {
		t.Fatalf("ExtractPython: %v", err)
	}

	kindOf := make(map[string]string)
	for _, s := range syms {
		kindOf[s.Name] = string(s.Kind)
	}

	if kindOf["decorators.foo"] != "function" {
		t.Errorf("expected decorators.foo → function, got %q; all symbols: %v", kindOf["decorators.foo"], kindOf)
	}
}

// ─── Java edge cases ─────────────────────────────────────────────────────────

const javaAnonClassSrc = `package com.example;

public class Outer {
    public void doWork() {
        Runnable r = new Runnable() {
            public void run() {}
        };
    }
}
`

// TestExtractJava_AnonymousInnerClass verifies that the presence of an
// anonymous inner class does not prevent the outer class symbol from being
// extracted.
func TestExtractJava_AnonymousInnerClass(t *testing.T) {
	syms, _, err := codegraph.ExtractJava(context.Background(), []byte(javaAnonClassSrc), "src/Outer.java")
	if err != nil {
		t.Fatalf("ExtractJava: %v", err)
	}

	kindOf := make(map[string]string)
	for _, s := range syms {
		kindOf[s.Name] = string(s.Kind)
	}

	if kindOf["com.example.Outer"] != "class" {
		t.Errorf("expected com.example.Outer → class, got %q; all symbols: %v", kindOf["com.example.Outer"], kindOf)
	}
}

func TestExtract_UnsupportedExtension(t *testing.T) {
	_, _, err := codegraph.Extract(context.Background(), []byte(""), "file.unknown")
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
	var unsup codegraph.ErrUnsupportedLanguage
	if _, ok := err.(codegraph.ErrUnsupportedLanguage); !ok {
		_ = unsup
		t.Errorf("expected ErrUnsupportedLanguage, got %T: %v", err, err)
	}
}
