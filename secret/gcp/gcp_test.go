package gcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/javadebadi/instagram-publisher/secret"
)

func TestGCPGetAndSet(t *testing.T) {
	var added string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == "GET" && req.URL.Path == "/projects/p/secrets/tok/versions/latest:access":
			data := base64.StdEncoding.EncodeToString([]byte("from-gcp\n"))
			json.NewEncoder(w).Encode(map[string]any{"payload": map[string]string{"data": data}})
		case req.Method == "POST" && req.URL.Path == "/projects/p/secrets/tok:addVersion":
			body, _ := io.ReadAll(req.Body)
			var in struct{ Payload struct{ Data string } }
			json.Unmarshal(body, &in)
			raw, _ := base64.StdEncoding.DecodeString(in.Payload.Data)
			added = string(raw)
			w.Write([]byte("{}"))
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error":{"message":"nope"}}`))
		}
	}))
	defer srv.Close()
	r := secret.NewResolver()
	r.Register("gcp", NewAt(srv.URL, srv.Client()))

	got, err := r.Get(context.Background(), "gcp:projects/p/secrets/tok")
	if err != nil || got != "from-gcp" {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := r.Set(context.Background(), "gcp:projects/p/secrets/tok", "fresh"); err != nil || added != "fresh" {
		t.Fatalf("set: added=%q err=%v", added, err)
	}
	_, err = r.Get(context.Background(), "gcp:projects/p/secrets/other")
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("Google's message should come through, got %v", err)
	}
	for _, bad := range []string{"gcp:secrets/tok", "gcp:projects/p/secrets/a/b"} {
		if _, err := r.Get(context.Background(), bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if err := r.Set(context.Background(), "gcp:projects/p/secrets/tok/versions/3", "x"); err == nil {
		t.Error("setting a specific version should be rejected")
	}
}

// A failed update must not echo the value it was sending.
func TestGCPSetErrorsDoNotLeakTheValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":{"message":"permission denied"}}`))
	}))
	defer srv.Close()
	r := secret.NewResolver()
	r.Register("gcp", NewAt(srv.URL, srv.Client()))
	err := r.Set(context.Background(), "gcp:projects/p/secrets/tok", "SUPER-SECRET-VALUE")
	if err == nil || strings.Contains(err.Error(), "SUPER-SECRET-VALUE") {
		t.Fatalf("want an error without the value, got %v", err)
	}
}
