package publisher

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/javadebadi/instagram-publisher/secret"
)

// fakeInstagram answers the few calls publishing makes and records them.
type fakeInstagram struct {
	mu       sync.Mutex
	calls    []string
	userID   string
	username string
	failMe   bool
	// dropPublish cuts the connection during media_publish: the post may or
	// may not have gone out.
	dropPublish bool
}

func (f *fakeInstagram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/me"):
		if f.failMe {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"Error validating access token","type":"OAuthException","code":190}}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"user_id": f.userID, "username": f.username})
	case strings.HasSuffix(r.URL.Path, "/media"):
		w.Write([]byte(`{"id":"container1"}`))
	case strings.HasSuffix(r.URL.Path, "/media_publish"):
		if f.dropPublish {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		w.Write([]byte(`{"id":"media99"}`))
	case strings.HasSuffix(r.URL.Path, "/media99"):
		w.Write([]byte(`{"permalink":"https://www.instagram.com/p/abc/"}`))
	case strings.HasSuffix(r.URL.Path, "/refresh_access_token"):
		w.Write([]byte(`{"access_token":"NEW-TOKEN","expires_in":5184000}`))
	case strings.HasSuffix(r.URL.Path, "/container1"):
		w.Write([]byte(`{"status_code":"FINISHED"}`))
	default:
		w.WriteHeader(404)
	}
}

func (f *fakeInstagram) posted() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.HasSuffix(c, "/media") || strings.HasSuffix(c, "/media_publish") {
			return true
		}
	}
	return false
}

// slideHost serves one JPEG the way a public bucket would, and refuses HEAD
// like a signed URL does.
func slideHost(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing.jpg" {
			w.WriteHeader(404)
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(403)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("jpeg"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func publisher(t *testing.T, ig *fakeInstagram) (http.Handler, string) {
	t.Helper()
	igServer := httptest.NewServer(ig)
	t.Cleanup(igServer.Close)
	t.Setenv("IGPUB_TEST_TOKEN", "OLD-TOKEN")
	return New(Config{
		Secrets:          secret.NewResolver(),
		InstagramBaseURL: igServer.URL,
	}), "env:IGPUB_TEST_TOKEN"
}

func doJSON(h http.Handler, path string, body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", path, strings.NewReader(string(data))))
	return rec
}

func TestPublishPostsACarousel(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001", username: "acct"}
	h, ref := publisher(t, ig)
	host := slideHost(t)
	rec := doJSON(h, "/publish/carousel", CarouselRequest{
		TokenRef: ref, UserID: ig.userID, Caption: "hello",
		Images: []string{host.URL + "/1.jpg?sig=secret", host.URL + "/2.jpg?sig=secret"},
	})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var resp CarouselResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.MediaID != "media99" || resp.Permalink == "" || resp.Account != "acct" || resp.DryRun {
		t.Errorf("unexpected answer: %+v", resp)
	}
}

func TestPublishDryRunPostsNothing(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001", username: "acct"}
	h, ref := publisher(t, ig)
	host := slideHost(t)
	rec := doJSON(h, "/publish/carousel", CarouselRequest{TokenRef: ref, UserID: ig.userID, DryRun: true, Images: []string{host.URL + "/1.jpg"}})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var resp CarouselResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.DryRun || resp.MediaID != "" || resp.Account != "acct" {
		t.Errorf("unexpected answer: %+v", resp)
	}
	if ig.posted() {
		t.Errorf("a dry run called the posting endpoints: %v", ig.calls)
	}
}

func TestPublishRefusesTheWrongAccountBeforePosting(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001", username: "someone-else"}
	h, ref := publisher(t, ig)
	host := slideHost(t)
	rec := doJSON(h, "/publish/carousel", CarouselRequest{TokenRef: ref, UserID: "17841499999999999", Images: []string{host.URL + "/1.jpg"}})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409: %s", rec.Code, rec.Body)
	}
	if ig.posted() {
		t.Error("posted to the wrong account")
	}
}

func TestPublishChecksEverySlideIsReachableBeforePosting(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001", username: "acct"}
	h, ref := publisher(t, ig)
	host := slideHost(t)
	rec := doJSON(h, "/publish/carousel", CarouselRequest{TokenRef: ref, UserID: ig.userID, Images: []string{host.URL + "/1.jpg", host.URL + "/missing.jpg?X-Goog-Signature=abc123"}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "abc123") {
		t.Errorf("a signature leaked into the error: %s", rec.Body)
	}
	if ig.posted() {
		t.Error("posted although a slide was missing")
	}
}

func TestPublishMarksATokenInstagramRejects(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001", failMe: true}
	h, ref := publisher(t, ig)
	host := slideHost(t)
	rec := doJSON(h, "/publish/carousel", CarouselRequest{TokenRef: ref, UserID: ig.userID, Images: []string{host.URL + "/1.jpg"}})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", rec.Code)
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["token_error"] != true {
		t.Errorf("the token problem should be marked: %v", body)
	}
	if strings.Contains(rec.Body.String(), "OLD-TOKEN") {
		t.Error("the token is in the error")
	}
}

func TestPublishSaysWhenTheOutcomeIsUnknown(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001", username: "acct", dropPublish: true}
	h, ref := publisher(t, ig)
	host := slideHost(t)
	rec := doJSON(h, "/publish/carousel", CarouselRequest{TokenRef: ref, UserID: ig.userID, Images: []string{host.URL + "/1.jpg"}})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502: %s", rec.Code, rec.Body)
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["outcome_unknown"] != true {
		t.Errorf("an unknown outcome should be marked: %v", body)
	}
}

func TestPublishRejectsBadRequests(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001"}
	h, ref := publisher(t, ig)
	ok := CarouselRequest{TokenRef: ref, UserID: "17841400000000001", Images: []string{"https://x.example/1.jpg"}}
	for name, mutate := range map[string]func(*CarouselRequest){
		"no token ref":     func(r *CarouselRequest) { r.TokenRef = "" },
		"bad user id":      func(r *CarouselRequest) { r.UserID = "abc" },
		"no images":        func(r *CarouselRequest) { r.Images = nil },
		"eleven images":    func(r *CarouselRequest) { r.Images = make([]string, 11) },
		"not a url":        func(r *CarouselRequest) { r.Images = []string{"file:///etc/passwd"} },
		"caption too long": func(r *CarouselRequest) { r.Caption = strings.Repeat("x", 2201) },
	} {
		req := ok
		mutate(&req)
		if rec := doJSON(h, "/publish/carousel", req); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", name, rec.Code, rec.Body)
		}
	}
	if ig.posted() {
		t.Error("a bad request reached Instagram")
	}
}

func TestRefreshStoresTheNewTokenAndReturnsOnlyDates(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001"}
	igServer := httptest.NewServer(ig)
	defer igServer.Close()
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("IG=OLD-TOKEN\n"), 0o600)
	h := New(Config{Secrets: secret.NewResolver(), InstagramBaseURL: igServer.URL})

	rec := doJSON(h, "/token/refresh", RefreshRequest{TokenRef: "envfile:" + path + "#IG"})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(path); string(data) != "IG=NEW-TOKEN\n" {
		t.Errorf("file is now %q", data)
	}
	if strings.Contains(rec.Body.String(), "NEW-TOKEN") || strings.Contains(rec.Body.String(), "OLD-TOKEN") {
		t.Errorf("a token is in the answer: %s", rec.Body)
	}
	var resp RefreshResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.ExpiresAt.Before(resp.RefreshedAt) {
		t.Errorf("bad dates: %+v", resp)
	}
}

func TestRefreshOfAStoreThatCannotBeUpdatedFails(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001"}
	h, ref := publisher(t, ig) // env: cannot be updated
	rec := doJSON(h, "/token/refresh", RefreshRequest{TokenRef: ref})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502: %s", rec.Code, rec.Body)
	}
}

// Callers from before posts had a route per kind still send POST /publish.
func TestPublishKeepsTheOldRoute(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001", username: "acct"}
	h, ref := publisher(t, ig)
	host := slideHost(t)
	rec := doJSON(h, "/publish", CarouselRequest{TokenRef: ref, UserID: ig.userID, DryRun: true, Images: []string{host.URL + "/1.jpg"}})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
}

func TestUnknownKindIsNotFound(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001"}
	h, ref := publisher(t, ig)
	if rec := doJSON(h, "/publish/reel", CarouselRequest{TokenRef: ref}); rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404 until reels exist", rec.Code)
	}
}

func TestAccountCheck(t *testing.T) {
	ig := &fakeInstagram{userID: "17841400000000001", username: "acct"}
	h, ref := publisher(t, ig)
	rec := doJSON(h, "/account/check", AccountRequest{TokenRef: ref, UserID: ig.userID})
	var resp AccountResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if rec.Code != 200 || resp.Account != "acct" {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if ig.posted() {
		t.Error("checking an account must post nothing")
	}
	if rec := doJSON(h, "/account/check", AccountRequest{TokenRef: ref, UserID: "17841499999999999"}); rec.Code != http.StatusConflict {
		t.Errorf("another account's token: status %d, want 409", rec.Code)
	}
	if rec := doJSON(h, "/account/check", AccountRequest{TokenRef: ref, UserID: "abc"}); rec.Code != http.StatusBadRequest {
		t.Errorf("bad user id: status %d, want 400", rec.Code)
	}
}
