//go:build !cgo

package codegraph

import (
	"context"
	"errors"
	"testing"
)

func TestExtract_NoCGOFallbackForTreeSitterLanguages(t *testing.T) {
	t.Parallel()

	_, _, err := Extract(context.Background(), []byte("def hello(): pass"), "example.py")
	if err == nil {
		t.Fatal("expected unsupported-language error when cgo is disabled")
	}

	var unsupported ErrUnsupportedLanguage
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected ErrUnsupportedLanguage, got %T: %v", err, err)
	}
	if unsupported.Ext != ".py" {
		t.Fatalf("expected unsupported extension .py, got %q", unsupported.Ext)
	}
}
