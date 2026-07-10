package main

import (
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

// newStore selects the persistence backend from the environment — the only
// place backend selection happens. The S3 branch is a documented seam: it is
// activated later by implementing NewS3Store and returning it here, with no
// changes to the handlers.
func newStore() (Store, error) {
	backend := getenv("STORE", "fs")
	if backend == "s3" || os.Getenv("NOTES_BUCKET") != "" {
		return nil, fmt.Errorf("s3 store not yet implemented (set STORE=fs for local dev)")
	}
	return NewFSStore(getenv("NOTES_DIR", "./data"))
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
	store, err := newStore()
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

	log.Printf("listening on %s (store=%s)", addr, getenv("STORE", "fs"))
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
