package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// FSStore persists notes as {slug}.txt files under dir. The layout deliberately
// mirrors the eventual S3 layout (notes/{slug}.txt) so the mental model is the
// same across backends.
type FSStore struct {
	dir string
}

// NewFSStore creates the data directory if needed and returns a store rooted at
// it.
func NewFSStore(dir string) (*FSStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FSStore{dir: dir}, nil
}

func (s *FSStore) path(slug string) string {
	// The caller validates slug against ^[a-zA-Z0-9_-]{1,64}$ before reaching
	// the store, so it contains no path separators or "..": filepath.Join
	// cannot escape s.dir. Defense in depth against key injection.
	return filepath.Join(s.dir, slug+".txt")
}

func (s *FSStore) Get(_ context.Context, slug string) (string, error) {
	b, err := os.ReadFile(s.path(slug))
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *FSStore) Put(_ context.Context, slug, text string) error {
	return os.WriteFile(s.path(slug), []byte(text), 0o644)
}
