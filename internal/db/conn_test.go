package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
)

// TestNewPool_InvalidURL verifies that an invalid database URL returns an error
// rather than panicking or returning a nil pool without error.
func TestNewPool_InvalidURL(t *testing.T) {
	cfg := &config.DatabaseConfig{
		URL:            "not-a-valid-url",
		MaxOpen:        5,
		MaxIdle:        2,
		ConnectTimeout: 2 * time.Second,
	}
	pool, err := db.NewPool(context.Background(), cfg)
	if err == nil {
		if pool != nil {
			pool.Close()
		}
		t.Fatal("expected error for invalid URL, got nil")
	}
}

// TestNewPool_EmptyURL verifies that an empty database URL returns an error.
func TestNewPool_EmptyURL(t *testing.T) {
	cfg := &config.DatabaseConfig{
		URL:            "",
		MaxOpen:        5,
		MaxIdle:        2,
		ConnectTimeout: 2 * time.Second,
	}
	pool, err := db.NewPool(context.Background(), cfg)
	if err == nil {
		if pool != nil {
			pool.Close()
		}
		t.Fatal("expected error for empty URL, got nil")
	}
}
