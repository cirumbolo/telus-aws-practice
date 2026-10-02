package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// memStore is an in-memory Store used to exercise the handlers without touching
// the filesystem. That it satisfies the interface also proves the interface is
// honest. getErr/putErr, when set, let tests reach the handlers' store-error
// branches, which an always-succeeding fake never could.
type memStore struct {
	m         map[string]string
	getErr    error
	putErr    error
	renameErr error
}

func newMemStore() *memStore { return &memStore{m: map[string]string{}} }

func (s *memStore) Get(_ context.Context, slug string) (string, error) {
	if s.getErr != nil {
		return "", s.getErr
	}
	v, ok := s.m[slug]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *memStore) Put(_ context.Context, slug, text string) error {
	if s.putErr != nil {
		return s.putErr
	}
	s.m[slug] = text
	return nil
}

func (s *memStore) Rename(_ context.Context, oldSlug, newSlug string) error {
	if s.renameErr != nil {
		return s.renameErr
	}
	v, ok := s.m[oldSlug]
	if !ok {
		return ErrNotFound
	}
	if _, ok := s.m[newSlug]; ok {
		return ErrAlreadyExists
	}
	delete(s.m, oldSlug)
	s.m[newSlug] = v
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

func TestGetStoreErrorReturns500(t *testing.T) {
	api := &API{store: &memStore{getErr: errors.New("boom")}, webDir: "../web"}
	req := httptest.NewRequest(http.MethodGet, "/notes/abc12", nil)
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestPutStoreErrorReturns500(t *testing.T) {
	api := &API{store: &memStore{putErr: errors.New("boom")}, webDir: "../web"}
	req := httptest.NewRequest(http.MethodPut, "/notes/abc12",
		strings.NewReader(`{"text":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestPutMalformedJSONRejected(t *testing.T) {
	srv := httptest.NewServer(newTestAPI().routes())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/notes/abc12",
		strings.NewReader(`not json at all`))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestServeAppServesIndexForValidSlug(t *testing.T) {
	srv := httptest.NewServer(newTestAPI().routes())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/somevalidslug")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestServeAppInvalidPathReturns404(t *testing.T) {
	srv := httptest.NewServer(newTestAPI().routes())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/has%20space")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
}

func TestCORSHeadersSetWhenOriginConfigured(t *testing.T) {
	api := &API{store: newMemStore(), webDir: "../web", allowOrigin: "http://example.com"}
	req := httptest.NewRequest(http.MethodGet, "/notes/abc12", nil)
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "http://example.com")
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, PUT, PATCH, POST, OPTIONS" {
		t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, "GET, PUT, PATCH, POST, OPTIONS")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type" {
		t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, "Content-Type")
	}
}

func TestCORSPreflightOptionsShortCircuits(t *testing.T) {
	store := newMemStore()
	api := &API{store: store, webDir: "../web", allowOrigin: "http://example.com"}
	req := httptest.NewRequest(http.MethodOptions, "/notes/abc12", nil)
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if _, ok := store.m["abc12"]; ok {
		t.Fatal("preflight reached the store; want short-circuit before routing")
	}
}

func TestRenameNoteSuccess(t *testing.T) {
	store := newMemStore()
	store.m["old"] = "hello"
	api := &API{store: store, webDir: "../web"}
	req := httptest.NewRequest(http.MethodPatch, "/notes/old",
		strings.NewReader(`{"slug":"new"}`))
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if store.m["new"] != "hello" {
		t.Fatalf("m[new] = %q, want %q", store.m["new"], "hello")
	}
	if _, ok := store.m["old"]; ok {
		t.Fatal("m[old] still present after rename")
	}
}

func TestRenameNoteInvalidNewSlugRejected(t *testing.T) {
	store := newMemStore()
	store.m["old"] = "hello"
	api := &API{store: store, webDir: "../web"}
	req := httptest.NewRequest(http.MethodPatch, "/notes/old",
		strings.NewReader(`{"slug":"has space"}`))
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRenameNoteMissingSourceReturns404(t *testing.T) {
	api := newTestAPI()
	req := httptest.NewRequest(http.MethodPatch, "/notes/nope",
		strings.NewReader(`{"slug":"new"}`))
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRenameNoteExistingDestinationReturns409(t *testing.T) {
	store := newMemStore()
	store.m["old"] = "hello"
	store.m["new"] = "already here"
	api := &API{store: store, webDir: "../web"}
	req := httptest.NewRequest(http.MethodPatch, "/notes/old",
		strings.NewReader(`{"slug":"new"}`))
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestRenameNoteStoreErrorReturns500(t *testing.T) {
	api := &API{store: &memStore{m: map[string]string{}, renameErr: errors.New("boom")}, webDir: "../web"}
	req := httptest.NewRequest(http.MethodPatch, "/notes/old",
		strings.NewReader(`{"slug":"new"}`))
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestRenameNoteMalformedJSONRejected(t *testing.T) {
	api := newTestAPI()
	req := httptest.NewRequest(http.MethodPatch, "/notes/old",
		strings.NewReader(`not json at all`))
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRenameNoteInvalidOldSlugRejected(t *testing.T) {
	api := newTestAPI()
	req := httptest.NewRequest(http.MethodPatch, "/notes/has%20space",
		strings.NewReader(`{"slug":"new"}`))
	rec := httptest.NewRecorder()
	api.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

type fakeSummarizer struct {
	out string
	err error
	got string
}

func (f *fakeSummarizer) Summarize(_ context.Context, text string) (string, error) {
	f.got = text
	return f.out, f.err
}

func postSummary(t *testing.T, a *API, slug string) *http.Response {
	t.Helper()
	srv := httptest.NewServer(a.routes())
	t.Cleanup(srv.Close)
	res, err := http.Post(srv.URL+"/notes/"+slug+"/summary", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func TestSummarySuccess(t *testing.T) {
	a := newTestAPI()
	f := &fakeSummarizer{out: "about cats"}
	a.summarizer = f
	a.store.(*memStore).m["pad"] = "cats are great"

	res := postSummary(t, a, "pad")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["summary"] != "about cats" || f.got != "cats are great" {
		t.Errorf("body=%v got=%q", body, f.got)
	}
}

func TestSummaryStatusCodes(t *testing.T) {
	t.Run("not configured", func(t *testing.T) {
		if res := postSummary(t, newTestAPI(), "pad"); res.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("status = %d", res.StatusCode)
		}
	})
	t.Run("invalid slug", func(t *testing.T) {
		a := newTestAPI()
		a.summarizer = &fakeSummarizer{}
		if res := postSummary(t, a, "bad.slug"); res.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d", res.StatusCode)
		}
	})
	t.Run("empty note", func(t *testing.T) {
		a := newTestAPI()
		a.summarizer = &fakeSummarizer{}
		if res := postSummary(t, a, "fresh"); res.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d", res.StatusCode)
		}
	})
	t.Run("upstream error", func(t *testing.T) {
		a := newTestAPI()
		a.summarizer = &fakeSummarizer{err: errors.New("boom")}
		a.store.(*memStore).m["pad"] = "text"
		if res := postSummary(t, a, "pad"); res.StatusCode != http.StatusBadGateway {
			t.Errorf("status = %d", res.StatusCode)
		}
	})
	t.Run("store error", func(t *testing.T) {
		a := newTestAPI()
		a.summarizer = &fakeSummarizer{}
		a.store.(*memStore).getErr = errors.New("boom")
		if res := postSummary(t, a, "pad"); res.StatusCode != http.StatusInternalServerError {
			t.Errorf("status = %d", res.StatusCode)
		}
	})
}
