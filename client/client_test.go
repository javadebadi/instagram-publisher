package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/javadebadi/instagram-publisher/publisher"
)

// The tests use a fake service, or the real handler on input it rejects
// before reaching Instagram; nothing here reaches Instagram.
func fake(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL + "/", TokenRef: "env:T", UserID: "17841400000000001"}
}

func TestCarouselSendsWhatItShould(t *testing.T) {
	var got publisher.CarouselRequest
	c := fake(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/publish/carousel" {
			t.Errorf("path %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"account":"a","media_id":"m1","permalink":"https://www.instagram.com/p/X/"}`))
	}))
	p, err := c.Carousel(context.Background(), []string{"https://x/1.jpg"}, "Hi")
	if err != nil || p.MediaID != "m1" || p.Permalink == "" {
		t.Fatalf("got %+v, %v", p, err)
	}
	if got.TokenRef != "env:T" || got.UserID != "17841400000000001" || got.Caption != "Hi" || len(got.Images) != 1 {
		t.Errorf("request body: %+v", got)
	}
}

func TestCheck(t *testing.T) {
	c := fake(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"account":"tester"}`))
	}))
	if name, err := c.Check(context.Background()); err != nil || name != "tester" {
		t.Fatalf("%q, %v", name, err)
	}
}

func TestRefresh(t *testing.T) {
	c := fake(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token/refresh" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Write([]byte(`{"refreshed_at":"2026-01-01T00:00:00Z","expires_at":"2026-03-02T00:00:00Z"}`))
	}))
	r, err := c.Refresh(context.Background())
	if err != nil || r.ExpiresAt.Month() != 3 {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestErrorsCarryTheServicesFlags(t *testing.T) {
	c := fake(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		w.Write([]byte(`{"error":"posting to Instagram: boom","token_error":false,"outcome_unknown":true}`))
	}))
	_, err := c.Carousel(context.Background(), []string{"https://x/1.jpg"}, "")
	var e *Error
	if !errors.As(err, &e) || !e.OutcomeUnknown || e.Status != 502 || !strings.Contains(err.Error(), "check the Instagram account") {
		t.Fatalf("got %v", err)
	}
}

func TestImageNotFetchable(t *testing.T) {
	c := fake(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"error":"not an image"}`))
	}))
	_, err := c.Carousel(context.Background(), []string{"https://x/1.jpg"}, "")
	if !ImageNotFetchable(err) {
		t.Errorf("422 should read as not fetchable: %v", err)
	}
}

// Against the real handler: what the client sends is what the service reads.
func TestRealServiceReadsTheRequest(t *testing.T) {
	c := fake(t, publisher.New(publisher.Config{}))
	c.UserID = "not-a-number"
	_, err := c.Carousel(context.Background(), []string{"https://x/1.jpg"}, "")
	var e *Error
	if !errors.As(err, &e) || e.Status != 400 || !strings.Contains(e.Message, "user_id") {
		t.Fatalf("got %v", err)
	}
}

func TestUnreachable(t *testing.T) {
	c := &Client{BaseURL: "http://127.0.0.1:1"}
	if _, err := c.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Errorf("got %v", err)
	}
}
