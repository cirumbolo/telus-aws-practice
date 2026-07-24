package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// memStore is an in-memory Store used to exercise the handlers without touching
// the filesystem. That it satisfies the interface also proves the interface is
// honest.
type memStore struct{ m map[string]string }

func newMemStore() *memStore { return &memStore{m: map[string]string{}} }

func (s *memStore) Get(_ context.Context, slug string) (string, error) {
	v, ok := s.m[slug]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *memStore) Put(_ context.Context, slug, text string) error {
	s.m[slug] = text
	return nil
}

func newTestAPI() *API {
	return &API{store: newMemStore(), webDir: "../web"}
}

func TestGetFreshSlugReturnsEmpty200(t *testing.T) {
	srv := httptest.NewServer(newTestAPI().routes())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/notes/brandnew")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct{ Text string }
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Text != "" {
		t.Fatalf("text = %q, want empty", body.Text)
	}
}

func TestPutThenGetRoundtrip(t *testing.T) {
	srv := httptest.NewServer(newTestAPI().routes())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/notes/abc12",
		strings.NewReader(`{"text":"hello world"}`))
	req.Header.Set("Content-Type", "application/json")
	putRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	putRes.Body.Close()
	if putRes.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204", putRes.StatusCode)
	}

	getRes, err := http.Get(srv.URL + "/notes/abc12")
	if err != nil {
		t.Fatal(err)
	}
	defer getRes.Body.Close()
	var body struct{ Text string }
	json.NewDecoder(getRes.Body).Decode(&body)
	if body.Text != "hello world" {
		t.Fatalf("text = %q, want %q", body.Text, "hello world")
	}
}

func TestInvalidSlugRejected(t *testing.T) {
	srv := httptest.NewServer(newTestAPI().routes())
	defer srv.Close()

	// A space in the slug is invalid; the mux still routes it to getNote,
	// which rejects it with 400.
	res, err := http.Get(srv.URL + "/notes/has%20space")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	srv := httptest.NewServer(newTestAPI().routes())
	defer srv.Close()

	big := strings.Repeat("x", maxBody+1)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/notes/big",
		strings.NewReader(`{"text":"`+big+`"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNoContent {
		t.Fatalf("oversized body accepted (204); want rejection")
	}
	if res.StatusCode != http.StatusRequestEntityTooLarge && res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 413 or 400", res.StatusCode)
	}
}

func TestRootRedirectsToSlug(t *testing.T) {
	api := newTestAPI()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/") || !validSlug(strings.TrimPrefix(loc, "/")) {
		t.Fatalf("Location = %q, want redirect to a valid slug", loc)
	}
}
