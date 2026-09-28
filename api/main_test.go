package main

import (
	"context"
	"strings"
	"testing"
)

func TestResolveBackendNotesBucketImpliesS3(t *testing.T) {
	t.Setenv("NOTES_BUCKET", "some-bucket")
	t.Setenv("STORE", "")

	if got := resolveBackend(); got != "s3" {
		t.Fatalf("resolveBackend() = %q, want %q", got, "s3")
	}
}

func TestResolveBackendExplicitStoreEnv(t *testing.T) {
	t.Setenv("NOTES_BUCKET", "")
	t.Setenv("STORE", "s3")

	if got := resolveBackend(); got != "s3" {
		t.Fatalf("resolveBackend() = %q, want %q", got, "s3")
	}
}

func TestResolveBackendDefaultsToFS(t *testing.T) {
	t.Setenv("NOTES_BUCKET", "")
	t.Setenv("STORE", "")

	if got := resolveBackend(); got != "fs" {
		t.Fatalf("resolveBackend() = %q, want %q", got, "fs")
	}
}

func TestNewStoreFSBackend(t *testing.T) {
	t.Setenv("NOTES_DIR", t.TempDir())

	store, err := newStore(context.Background(), "fs")
	if err != nil {
		t.Fatalf("newStore(fs): %v", err)
	}
	if _, ok := store.(*FSStore); !ok {
		t.Fatalf("newStore(fs) = %T, want *FSStore", store)
	}
}

func TestNewStoreUnknownBackendErrors(t *testing.T) {
	_, err := newStore(context.Background(), "bogus")
	if err == nil {
		t.Fatal("newStore(bogus) err = nil, want an error")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("newStore(bogus) err = %v, want it to mention the bad backend name", err)
	}
}
