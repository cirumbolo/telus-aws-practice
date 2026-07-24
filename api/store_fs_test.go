package main

import (
	"context"
	"errors"
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
