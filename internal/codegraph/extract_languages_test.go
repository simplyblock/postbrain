package codegraph_test

// Smoke tests for all new language extractors.
// Each test verifies that:
//  1. Parsing succeeds.
//  2. A file symbol is emitted.
//  3. At least one expected declaration symbol is present.
//  4. At least one expected edge is present.

import (
	"context"
	"slices"
	"testing"

	"github.com/simplyblock/postbrain/internal/codegraph"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

type symMap map[string]string // name → kind
type edgeList []codegraph.Edge

func (el edgeList) hasEdge(subj, pred, obj string) bool {
	return slices.ContainsFunc([]codegraph.Edge(el), func(e codegraph.Edge) bool {
		return e.SubjectName == subj && e.Predicate == pred && e.ObjectName == obj
	})
}

func parseAll(t *testing.T, fn func() ([]codegraph.Symbol, []codegraph.Edge, error)) (symMap, edgeList) {
	t.Helper()
	syms, edges, err := fn()
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	sm := make(symMap)
	for _, s := range syms {
		sm[s.Name] = string(s.Kind)
	}
	return sm, edgeList(edges)
}

// ─── Bash ────────────────────────────────────────────────────────────────────

const bashSrc = `#!/bin/bash
source ./lib.sh

greet() {
  echo "hello $1"
  cleanup
}

cleanup() {
  rm -f /tmp/tmpfile
}
`

func TestExtractBash(t *testing.T) {
	sm, el := parseAll(t, func() ([]codegraph.Symbol, []codegraph.Edge, error) {
		return codegraph.ExtractBash(context.Background(), []byte(bashSrc), "scripts/greet.sh")
	})
	if sm["scripts/greet.sh"] != "file" {
		t.Errorf("missing file symbol")
	}
	if sm["greet.greet"] != "function" {
		t.Errorf("expected greet.greet → function; got %v", sm)
	}
	if !el.hasEdge("scripts/greet.sh", "imports", "./lib.sh") {
		t.Errorf("expected imports edge for lib.sh; edges: %v", el)
	}
}

// ─── C ───────────────────────────────────────────────────────────────────────

const cSrc = `#include <stdio.h>
#include "utils.h"

typedef struct {
    int x;
    int y;
} Point;

void print_point(Point p) {
    printf("%d %d\n", p.x, p.y);
}
`

func TestExtractC(t *testing.T) {
	sm, el := parseAll(t, func() ([]codegraph.Symbol, []codegraph.Edge, error) {
		return codegraph.ExtractC(context.Background(), []byte(cSrc), "src/point.c")
	})
	if sm["src/point.c"] != "file" {
		t.Errorf("missing file symbol")
	}
	if sm["print_point"] != "function" {
		t.Errorf("expected print_point → function; got %v", sm)
	}
	if !el.hasEdge("src/point.c", "imports", "stdio.h") {
		t.Errorf("expected imports edge for stdio.h; edges: %v", el)
	}
	if !el.hasEdge("src/point.c", "imports", "utils.h") {
		t.Errorf("expected imports edge for utils.h; edges: %v", el)
	}
}

// ─── C++ ─────────────────────────────────────────────────────────────────────

const cppSrc = `#include <vector>

class Animal {
public:
    virtual void speak() = 0;
};

class Dog : public Animal {
public:
    void speak() override {
        printf("woof\n");
    }
};
`

func TestExtractCPP(t *testing.T) {
	sm, el := parseAll(t, func() ([]codegraph.Symbol, []codegraph.Edge, error) {
		return codegraph.ExtractCPP(context.Background(), []byte(cppSrc), "src/animal.cpp")
	})
	if sm["Animal"] != "class" {
		t.Errorf("expected Animal → class; got %v", sm)
	}
	if sm["Dog"] != "class" {
		t.Errorf("expected Dog → class; got %v", sm)
	}
	if !el.hasEdge("src/animal.cpp", "imports", "vector") {
		t.Errorf("expected imports → vector; edges: %v", el)
	}
}

// ─── C# ──────────────────────────────────────────────────────────────────────

const csSrc = `using System;
using System.Collections.Generic;

namespace MyApp.Services {
    public interface IRepository {
        string Find(string id);
    }

    public class UserRepository : IRepository {
        public string Find(string id) {
            return id;
        }
    }
}
`

func TestExtractCSharp(t *testing.T) {
	sm, el := parseAll(t, func() ([]codegraph.Symbol, []codegraph.Edge, error) {
		return codegraph.ExtractCSharp(context.Background(), []byte(csSrc), "Services/UserRepo.cs")
	})
	if sm["Services/UserRepo.cs"] != "file" {
		t.Errorf("missing file symbol")
	}
	if sm["MyApp.Services.IRepository"] != "interface" {
		t.Errorf("expected IRepository → interface; got %v", sm)
	}
	if sm["MyApp.Services.UserRepository"] != "class" {
		t.Errorf("expected UserRepository → class; got %v", sm)
	}
	if !el.hasEdge("Services/UserRepo.cs", "imports", "System") {
		t.Errorf("expected imports → System; edges: %v", el)
	}
}

// ─── Java ────────────────────────────────────────────────────────────────────

const javaSrc = `package com.example;

import java.util.List;

public interface Greeter {
    String greet(String name);
}

public class HelloGreeter implements Greeter {
    public String greet(String name) {
        return "Hello " + name;
    }
}
`

func TestExtractJava(t *testing.T) {
	sm, el := parseAll(t, func() ([]codegraph.Symbol, []codegraph.Edge, error) {
		return codegraph.ExtractJava(context.Background(), []byte(javaSrc), "src/Hello.java")
	})
	if sm["com.example.Greeter"] != "interface" {
		t.Errorf("expected com.example.Greeter → interface; got %v", sm)
	}
	if sm["com.example.HelloGreeter"] != "class" {
		t.Errorf("expected com.example.HelloGreeter → class; got %v", sm)
	}
	if !el.hasEdge("src/Hello.java", "imports", "java.util.List") {
		t.Errorf("expected imports → java.util.List; edges: %v", el)
	}
}

// ─── CSS ─────────────────────────────────────────────────────────────────────

const cssSrc = `@import "reset.css";
@import url("fonts.css");

.container { display: flex; }
#header { background: blue; }
`

func TestExtractCSS(t *testing.T) {
	sm, el := parseAll(t, func() ([]codegraph.Symbol, []codegraph.Edge, error) {
		return codegraph.ExtractCSS(context.Background(), []byte(cssSrc), "styles/main.css")
	})
	if sm["styles/main.css"] != "file" {
		t.Errorf("missing file symbol")
	}
	if !el.hasEdge("styles/main.css", "imports", "reset.css") {
		t.Errorf("expected imports → reset.css; edges: %v", el)
	}
}

// ─── Dockerfile ──────────────────────────────────────────────────────────────

const dockerfileSrc = `FROM golang:1.22 AS builder
RUN go build -o app .

FROM gcr.io/distroless/static AS runtime
COPY --from=builder /app /app
`

func TestExtractDockerfile(t *testing.T) {
	sm, el := parseAll(t, func() ([]codegraph.Symbol, []codegraph.Edge, error) {
		return codegraph.ExtractDockerfile(context.Background(), []byte(dockerfileSrc), "Dockerfile")
	})
	if sm["Dockerfile"] != "file" {
		t.Errorf("missing file symbol")
	}
	if !el.hasEdge("Dockerfile", "imports", "golang:1.22") {
		t.Errorf("expected imports → golang:1.22; edges: %v", el)
	}
}

// ─── HCL ─────────────────────────────────────────────────────────────────────

const hclSrc = `
variable "region" {}

resource "aws_s3_bucket" "my_bucket" {
  bucket = "my-bucket"
}

module "vpc" {
  source = "./modules/vpc"
}
`

func TestExtractHCL(t *testing.T) {
	sm, el := parseAll(t, func() ([]codegraph.Symbol, []codegraph.Edge, error) {
		return codegraph.ExtractHCL(context.Background(), []byte(hclSrc), "main.tf")
	})
	if sm["main.tf"] != "file" {
		t.Errorf("missing file symbol")
	}
	if sm["resource.aws_s3_bucket.my_bucket"] != "variable" {
		t.Errorf("expected resource symbol; got %v", sm)
	}
	if sm["module.vpc"] != "module" {
		t.Errorf("expected module.vpc symbol; got %v", sm)
	}
	if !el.hasEdge("module.vpc", "imports", "./modules/vpc") {
		t.Errorf("expected module.vpc → imports → ./modules/vpc; edges: %v", el)
	}
}

// ─── Protobuf ────────────────────────────────────────────────────────────────

const protoSrc = `syntax = "proto3";
package payments.v1;

import "google/protobuf/timestamp.proto";

message Payment {
    string id = 1;
    int64 amount = 2;
}

service PaymentService {
    rpc CreatePayment (Payment) returns (Payment);
}
`

func TestExtractProtobuf(t *testing.T) {
	sm, el := parseAll(t, func() ([]codegraph.Symbol, []codegraph.Edge, error) {
		return codegraph.ExtractProtobuf(context.Background(), []byte(protoSrc), "payments/v1/payments.proto")
	})
	if sm["payments.v1.Payment"] != "struct" {
		t.Errorf("expected payments.v1.Payment → struct; got %v", sm)
	}
	if sm["payments.v1.PaymentService"] != "interface" {
		t.Errorf("expected payments.v1.PaymentService → interface; got %v", sm)
	}
	if sm["payments.v1.PaymentService.CreatePayment"] != "method" {
		t.Errorf("expected CreatePayment → method; got %v", sm)
	}
	if !el.hasEdge("payments/v1/payments.proto", "imports", "google/protobuf/timestamp.proto") {
		t.Errorf("expected imports → timestamp.proto; edges: %v", el)
	}
}

// ─── TOML ────────────────────────────────────────────────────────────────────

const tomlSrc = `[package]
name = "my-app"
version = "1.0.0"

[dependencies]
serde = "1.0"
`

func TestExtractTOML(t *testing.T) {
	sm, _ := parseAll(t, func() ([]codegraph.Symbol, []codegraph.Edge, error) {
		return codegraph.ExtractTOML(context.Background(), []byte(tomlSrc), "Cargo.toml")
	})
	if sm["Cargo.toml"] != "file" {
		t.Errorf("missing file symbol")
	}
	if sm["package"] != "variable" {
		t.Errorf("expected package → variable; got %v", sm)
	}
	if sm["dependencies"] != "variable" {
		t.Errorf("expected dependencies → variable; got %v", sm)
	}
}

// ─── YAML ────────────────────────────────────────────────────────────────────

const yamlSrc = `name: my-service
version: "2.0"
dependencies:
  - postgres
  - redis
`

func TestExtractYAML(t *testing.T) {
	sm, _ := parseAll(t, func() ([]codegraph.Symbol, []codegraph.Edge, error) {
		return codegraph.ExtractYAML(context.Background(), []byte(yamlSrc), "service.yaml")
	})
	if sm["service.yaml"] != "file" {
		t.Errorf("missing file symbol")
	}
	if sm["name"] != "variable" {
		t.Errorf("expected name → variable; got %v", sm)
	}
}

// ─── Dispatch: Dockerfile by basename ────────────────────────────────────────

func TestExtract_DockerfileBasename(t *testing.T) {
	syms, _, err := codegraph.Extract(context.Background(), []byte(dockerfileSrc), "Dockerfile")
	if err != nil {
		t.Fatalf("Extract(Dockerfile): %v", err)
	}
	if len(syms) == 0 {
		t.Error("expected symbols from Dockerfile")
	}
}