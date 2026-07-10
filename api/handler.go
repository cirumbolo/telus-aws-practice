package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// maxBody caps a PUT request body at 100 KiB, per the note-size convention.
const maxBody = 100 << 10

// slugRe is the single source of truth for valid slugs. Anything else is
// rejected, which prevents key injection / path traversal via the slug.
var slugRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func validSlug(s string) bool { return slugRe.MatchString(s) }

// API holds the request handlers and their dependencies.
type API struct {
	store       Store
	webDir      string // directory holding index.html + static assets
	allowOrigin string // CORS allow-origin; empty disables CORS (local, same-origin)
}

// routes builds the mux and wraps it with CORS. Go 1.22+ ServeMux method+path
// patterns handle routing; no external router.
func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /notes/{slug}", a.getNote)
	mux.HandleFunc("PUT /notes/{slug}", a.putNote)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir(a.webDir))))
	mux.HandleFunc("GET /", a.serveApp)
	return a.withCORS(mux)
}

// getNote returns the note text. A missing note is a normal case: it yields
// 200 with empty text rather than a 404.
func (a *API) getNote(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !validSlug(slug) {
		http.Error(w, "invalid slug", http.StatusBadRequest)
		return
	}
	text, err := a.store.Get(r.Context(), slug)
	if err != nil && !errors.Is(err, ErrNotFound) {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	// ErrNotFound leaves text as "" — the empty-note-as-200 rule, in one place.
	writeJSON(w, map[string]string{"text": text})
}

// putNote upserts the note text.
func (a *API) putNote(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !validSlug(slug) {
		http.Error(w, "invalid slug", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Covers malformed JSON and the MaxBytesReader overflow.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "note too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := a.store.Put(r.Context(), slug, req.Text); err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// serveApp handles everything not matched by the API routes: the root redirect,
// the pad page for a slug, and 404s. Static assets are served by the /static/
// file server registered in routes. This is a local convenience; on S3 the
// static site owns these responsibilities.
func (a *API) serveApp(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/"+randomSlug(5), http.StatusFound)
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/")
	if !validSlug(slug) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path.Join(a.webDir, "index.html"))
}

// withCORS is a no-op when allowOrigin is empty (local, same-origin). When set
// (the eventual S3 two-origin deployment, via ALLOW_ORIGIN), it adds the CORS
// headers and short-circuits preflight requests.
func (a *API) withCORS(next http.Handler) http.Handler {
	if a.allowOrigin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", a.allowOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
