package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFuelIXSummarizeSuccess(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody struct {
		Model    string              `json:"model"`
		Messages []map[string]string `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"  A short summary.  "}}]}`))
	}))
	defer srv.Close()

	s := NewFuelIXSummarizer(srv.URL+"/v1/", "k3y", "some-model")
	got, err := s.Summarize(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if got != "A short summary." {
		t.Errorf("summary = %q", got)
	}
	if gotAuth != "Bearer k3y" || gotPath != "/v1/chat/completions" {
		t.Errorf("auth=%q path=%q", gotAuth, gotPath)
	}
	if gotBody.Model != "some-model" || len(gotBody.Messages) != 2 || gotBody.Messages[1]["content"] != "hello world" {
		t.Errorf("unexpected request body: %+v", gotBody)
	}
}

func TestFuelIXSummarizeErrors(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"non-2xx": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad key", http.StatusUnauthorized)
		},
		"no choices": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[]}`))
		},
		"bad json": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			s := NewFuelIXSummarizer(srv.URL, "k", "m")
			if _, err := s.Summarize(context.Background(), "x"); err == nil {
				t.Error("expected error")
			}
		})
	}
}
