package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRunWithRetry_RetriesSerializationFailures(t *testing.T) {
	t.Parallel()

	attempts := 0
	err := runWithRetry(context.Background(), 3, func() error {
		attempts++
		if attempts < 3 {
			return &pgconn.PgError{Code: "40001"}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("runWithRetry returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestRunWithRetry_DoesNotRetryNonRetryableErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	attempts := 0
	err := runWithRetry(context.Background(), 3, func() error {
		attempts++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRunWithRetry_StopsAtMaxAttempts(t *testing.T) {
	t.Parallel()

	attempts := 0
	err := runWithRetry(context.Background(), 2, func() error {
		attempts++
		return &pgconn.PgError{Code: "40P01"}
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestIsRetryableTxError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "serialization",
			err:  &pgconn.PgError{Code: "40001"},
			want: true,
		},
		{
			name: "deadlock",
			err:  &pgconn.PgError{Code: "40P01"},
			want: true,
		},
		{
			name: "wrapped serialization",
			err:  errors.Join(errors.New("wrapper"), &pgconn.PgError{Code: "40001"}),
			want: true,
		},
		{
			name: "other pg error",
			err:  &pgconn.PgError{Code: "23505"},
			want: false,
		},
		{
			name: "plain",
			err:  errors.New("plain"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isRetryableTxError(tt.err); got != tt.want {
				t.Fatalf("isRetryableTxError() = %v, want %v", got, tt.want)
			}
		})
	}
}
