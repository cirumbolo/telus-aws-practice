package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFSStoreRoundtrip(t *testing.T) {
	s, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	ctx := context.Background()

	if err := s.Put(ctx, "hello", "world"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, "hello")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "world" {
		t.Fatalf("Get = %q, want %q", got, "world")
	}
}

func TestFSStoreGetMissing(t *testing.T) {
	s, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	_, err = s.Get(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing err = %v, want ErrNotFound", err)
	}
}

func TestFSStoreOverwrite(t *testing.T) {
	s, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "k", "first"); err != nil {
		t.Fatalf("Put first: %v", err)
	}
	if err := s.Put(ctx, "k", "second"); err != nil {
		t.Fatalf("Put second: %v", err)
	}
	got, _ := s.Get(ctx, "k")
	if got != "second" {
		t.Fatalf("Get after overwrite = %q, want %q", got, "second")
	}
}

// A regular file can't have children, so os.MkdirAll on a path beneath it
// fails — this is how we reach NewFSStore's error branch without needing
// permission tricks.
func TestNewFSStoreMkdirFailure(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup WriteFile: %v", err)
	}

	_, err := NewFSStore(filepath.Join(blocker, "data"))
	if err == nil {
		t.Fatal("NewFSStore err = nil, want an error")
	}
}

func TestFSStoreRenameMovesContent(t *testing.T) {
	s, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "old", "hello"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := s.Rename(ctx, "old", "new"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	got, err := s.Get(ctx, "new")
	if err != nil {
		t.Fatalf("Get(new): %v", err)
	}
	if got != "hello" {
		t.Fatalf("Get(new) = %q, want %q", got, "hello")
	}
	if _, err := s.Get(ctx, "old"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(old) err = %v, want ErrNotFound", err)
	}
}

func TestFSStoreRenameMissingSourceErrors(t *testing.T) {
	s, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	if err := s.Rename(context.Background(), "nope", "new"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Rename err = %v, want ErrNotFound", err)
	}
}

func TestFSStoreRenameExistingDestinationErrors(t *testing.T) {
	s, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "old", "hello"); err != nil {
		t.Fatalf("Put(old): %v", err)
	}
	if err := s.Put(ctx, "new", "already here"); err != nil {
		t.Fatalf("Put(new): %v", err)
	}

	if err := s.Rename(ctx, "old", "new"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("Rename err = %v, want ErrAlreadyExists", err)
	}
	// Neither note should have been touched by the rejected rename.
	got, _ := s.Get(ctx, "new")
	if got != "already here" {
		t.Fatalf("Get(new) after rejected rename = %q, want unchanged %q", got, "already here")
	}
}
