package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"time"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// resolveBackend reports which persistence backend the environment selects.
// NOTES_BUCKET implies s3 even if STORE is unset, so a bucket in the
// environment is never silently ignored.
func resolveBackend() string {
	if os.Getenv("NOTES_BUCKET") != "" {
		return "s3"
	}
	return getenv("STORE", "fs")
}

// newStore builds the backend named by resolveBackend — the only place backend
// selection happens. Handlers depend on the Store interface, so swapping
// filesystem for S3 changes nothing above this function.
func newStore(ctx context.Context, backend string) (Store, error) {
	switch backend {
	case "fs":
		return NewFSStore(getenv("NOTES_DIR", "./data"))
	case "s3":
		return NewS3Store(ctx, os.Getenv("NOTES_BUCKET"))
	default:
		return nil, fmt.Errorf("unknown STORE %q (want fs or s3)", backend)
	}
}

const slugChars = "abcdefghijklmnopqrstuvwxyz0123456789"

// randomSlug returns an n-character lowercase-alphanumeric slug. The charset is
// a subset of the validation regex, so generated slugs always validate. Slug
// unpredictability is not a security property — notes are public-by-URL by
// design, exactly like note.ms — so math/rand/v2 is fine.
func randomSlug(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = slugChars[rand.IntN(len(slugChars))]
	}
	return string(b)
}

func main() {
	backend := resolveBackend()
	store, err := newStore(context.Background(), backend)
	if err != nil {
		log.Fatalf("store init: %v", err)
	}

	api := &API{
		store:       store,
		webDir:      getenv("WEB_DIR", "../web"),
		allowOrigin: os.Getenv("ALLOW_ORIGIN"),
	}

	addr := ":" + getenv("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("listening on %s (store=%s)", addr, backend)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
