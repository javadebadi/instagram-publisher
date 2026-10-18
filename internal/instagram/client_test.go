package instagram

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMeta imitates the endpoints a carousel post uses. Each container
// reports IN_PROGRESS once before FINISHED, like a slow real one.
type fakeMeta struct {
	mu        sync.Mutex
	next      int
	polled    map[string]int
	children  []string // image_url of carousel items, in order
	parent    string   // children= of the CAROUSEL container
	caption   string
	published string
	tokens    map[string]bool
}

func (f *fakeMeta) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		f.tokens[r.Form.Get("access_token")] = true
		path := strings.TrimPrefix(r.URL.Path, "/v23.0/")
		switch {
		case r.Method == http.MethodPost && path == "42/media":
			f.next++
			id := fmt.Sprintf("c%d", f.next)
			if r.Form.Get("media_type") == "CAROUSEL" {
				f.parent = r.Form.Get("children")
				f.caption = r.Form.Get("caption")
			} else {
				if r.Form.Get("is_carousel_item") != "true" || r.Form.Get("caption") != "" {
					t.Errorf("carousel item sent without is_carousel_item or with a caption: %v", r.Form)
				}
				f.children = append(f.children, r.Form.Get("image_url"))
			}
			fmt.Fprintf(w, `{"id":%q}`, id)
		case r.Method == http.MethodPost && path == "42/media_publish":
			f.published = r.Form.Get("creation_id")
			fmt.Fprint(w, `{"id":"m1"}`)
		case r.Method == http.MethodGet && strings.HasPrefix(path, "c"):
			f.polled[path]++
			status := "IN_PROGRESS"
			if f.polled[path] > 1 {
				status = "FINISHED"
			}
			fmt.Fprintf(w, `{"status_code":%q}`, status)
		case r.Method == http.MethodGet && path == "m1":
			fmt.Fprint(w, `{"permalink":"https://www.instagram.com/p/X/"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	})
}

func testClient(ts *httptest.Server) *Client {
	c := New("42", "secret-token")
	c.BaseURL = ts.URL + "/v23.0"
	c.PollInterval = time.Millisecond
	return c
}

func TestPostCarousel(t *testing.T) {
	f := &fakeMeta{polled: map[string]int{}, tokens: map[string]bool{}}
	ts := httptest.NewServer(f.handler(t))
	defer ts.Close()
	c := testClient(ts)

	urls := []string{"https://x/1.jpg", "https://x/2.jpg", "https://x/3.jpg"}
	id, err := c.Post(context.Background(), urls, "Hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	if id != "m1" {
		t.Errorf("media id = %q", id)
	}
	if strings.Join(f.children, " ") != strings.Join(urls, " ") {
		t.Errorf("children images = %v, want %v in order", f.children, urls)
	}
	if f.parent != "c1,c2,c3" || f.caption != "Hello" {
		t.Errorf("carousel children=%q caption=%q", f.parent, f.caption)
	}
	if f.published != "c4" {
		t.Errorf("published container %q, want the parent c4", f.published)
	}
	for _, id := range []string{"c1", "c2", "c3", "c4"} {
		if f.polled[id] < 2 {
			t.Errorf("container %s was not polled until FINISHED", id)
		}
	}
	if !f.tokens["secret-token"] {
		t.Error("token was not sent")
	}
	if link, _ := c.Permalink(context.Background(), id); link != "https://www.instagram.com/p/X/" {
		t.Errorf("permalink = %q", link)
	}
}

func TestAPIErrorHint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"Error validating access token","type":"OAuthException","code":190,"fbtrace_id":"abc"}}`)
	}))
	defer ts.Close()
	_, err := testClient(ts).Me(context.Background())
	if !IsTokenError(err) {
		t.Fatalf("want a token error, got %v", err)
	}
	if !strings.Contains(err.Error(), "POST /token/refresh") {
		t.Errorf("error should say what to do: %v", err)
	}
}

func TestTransportErrorHidesToken(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	ts.Close() // nothing listening: the request fails in transport
	_, err := testClient(ts).Me(context.Background())
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("token leaked into error: %v", err)
	}
}

func TestContainerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status_code":"ERROR","status":"Error: media could not be fetched"}`)
	}))
	defer ts.Close()
	err := testClient(ts).WaitReady(context.Background(), "c1")
	if err == nil || !strings.Contains(err.Error(), "could not be fetched") {
		t.Fatalf("want the container's error status, got %v", err)
	}
}

// A connection that dies during the final publish call may or may not have
// posted: that must be said, and nothing before that point may say it.
func TestFailureOfTheFinalPublishIsAnUnknownOutcome(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/media"):
			fmt.Fprint(w, `{"id":"c1"}`)
		case strings.HasSuffix(r.URL.Path, "/c1"):
			fmt.Fprint(w, `{"status_code":"FINISHED"}`)
		case strings.HasSuffix(r.URL.Path, "/media_publish"):
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close() // the answer never arrives
		}
	}))
	defer ts.Close()
	_, err := testClient(ts).Post(context.Background(), []string{"https://x/1.jpg"}, "c", nil)
	if !IsOutcomeUnknown(err) {
		t.Fatalf("want an unknown outcome, got %v", err)
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Errorf("token leaked: %v", err)
	}
}

func TestAnswersAndEarlierFailuresAreNotAnUnknownOutcome(t *testing.T) {
	// Instagram answers the final publish with an error: it did not publish.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/media"):
			fmt.Fprint(w, `{"id":"c1"}`)
		case strings.HasSuffix(r.URL.Path, "/c1"):
			fmt.Fprint(w, `{"status_code":"FINISHED"}`)
		case strings.HasSuffix(r.URL.Path, "/media_publish"):
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":{"message":"Media ID is not available","code":9007}}`)
		}
	}))
	defer ts.Close()
	_, err := testClient(ts).Post(context.Background(), []string{"https://x/1.jpg"}, "c", nil)
	if err == nil || IsOutcomeUnknown(err) {
		t.Fatalf("an API answer is a definite failure, got %v", err)
	}

	// A container that errors never reaches the publish call.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/media"):
			fmt.Fprint(w, `{"id":"c1"}`)
		default:
			fmt.Fprint(w, `{"status_code":"ERROR","status":"Error: media could not be fetched"}`)
		}
	}))
	defer bad.Close()
	_, err = testClient(bad).Post(context.Background(), []string{"https://x/1.jpg"}, "c", nil)
	if err == nil || IsOutcomeUnknown(err) {
		t.Fatalf("a failure before publishing is definite, got %v", err)
	}
}

// Instagram's anti-spam restriction on a publish is an answer, but it has been
// seen to arrive with a post that is live: the caller must check, not retry.
func TestTheSpamRestrictionOnThePublishCallIsAnUnknownOutcome(t *testing.T) {
	serve := func(subcode int, code int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/media"):
				fmt.Fprint(w, `{"id":"c1"}`)
			case strings.HasSuffix(r.URL.Path, "/c1"):
				fmt.Fprint(w, `{"status_code":"FINISHED"}`)
			case strings.HasSuffix(r.URL.Path, "/media_publish"):
				w.WriteHeader(400)
				fmt.Fprintf(w, `{"error":{"message":"We restrict certain activity to protect our community.","code":%d,"error_subcode":%d}}`, code, subcode)
			}
		}))
	}

	spam := serve(2207051, 4)
	defer spam.Close()
	_, err := testClient(spam).Post(context.Background(), []string{"https://x/1.jpg"}, "c", nil)
	if !IsOutcomeUnknown(err) {
		t.Fatalf("the anti-spam restriction should be an unknown outcome, got %v", err)
	}

	// The same call limit without that subcode, or any other answer, is still definite.
	rate := serve(0, 4)
	defer rate.Close()
	_, err = testClient(rate).Post(context.Background(), []string{"https://x/1.jpg"}, "c", nil)
	if err == nil || IsOutcomeUnknown(err) {
		t.Fatalf("a plain rate limit is a definite failure, got %v", err)
	}
}

// A reel and a story are one container each, polled with the longer video
// timeout, then published; options left out are not sent.
func TestPostReelAndStory(t *testing.T) {
	var forms []url.Values
	var published []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/media"):
			forms = append(forms, r.PostForm)
			fmt.Fprintf(w, `{"id":"c%d"}`, len(forms))
		case strings.HasSuffix(r.URL.Path, "/media_publish"):
			published = append(published, r.PostForm.Get("creation_id"))
			fmt.Fprint(w, `{"id":"m1"}`)
		default:
			fmt.Fprint(w, `{"status_code":"FINISHED"}`)
		}
	}))
	defer ts.Close()
	c := testClient(ts)
	ctx := context.Background()

	on, off := true, 1500
	if id, err := c.PostReel(ctx, "https://x/r.mp4", ReelOptions{Caption: "hi", ShareToFeed: &on, ThumbOffsetMS: &off}); err != nil || id != "m1" {
		t.Fatalf("reel: %q, %v", id, err)
	}
	r := forms[0]
	if r.Get("media_type") != "REELS" || r.Get("video_url") != "https://x/r.mp4" || r.Get("caption") != "hi" ||
		r.Get("share_to_feed") != "true" || r.Get("thumb_offset") != "1500" || r.Has("cover_url") {
		t.Errorf("reel form: %v", r)
	}

	if _, err := c.PostStory(ctx, "https://x/s.jpg", false); err != nil {
		t.Fatal(err)
	}
	if s := forms[1]; s.Get("media_type") != "STORIES" || s.Get("image_url") != "https://x/s.jpg" || s.Has("video_url") || s.Has("caption") {
		t.Errorf("image story form: %v", s)
	}
	if _, err := c.PostStory(ctx, "https://x/s.mp4", true); err != nil {
		t.Fatal(err)
	}
	if s := forms[2]; s.Get("video_url") != "https://x/s.mp4" || s.Has("image_url") {
		t.Errorf("video story form: %v", s)
	}
	if strings.Join(published, ",") != "c1,c2,c3" {
		t.Errorf("published %v", published)
	}
}

func TestVideoContainersGetTheLongerTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			fmt.Fprint(w, `{"id":"c1"}`)
			return
		}
		fmt.Fprint(w, `{"status_code":"IN_PROGRESS"}`)
	}))
	defer ts.Close()
	c := testClient(ts)
	c.PollTimeout, c.VideoPollTimeout = time.Millisecond, 100*time.Millisecond

	start := time.Now()
	_, err := c.PostReel(context.Background(), "https://x/r.mp4", ReelOptions{})
	if err == nil || !strings.Contains(err.Error(), "still IN_PROGRESS") {
		t.Fatalf("want a timeout, got %v", err)
	}
	if time.Since(start) < 90*time.Millisecond {
		t.Errorf("gave up after %s: the video timeout was not used", time.Since(start))
	}
}
