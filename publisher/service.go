// Package publisher is a small HTTP service that publishes to Instagram. A
// caller says what to post (images that are already at public URLs, a
// caption), which account, and where that account's access token lives; the
// service posts it and answers with the link.
//
// It holds no token and no state. Every call names its token by reference
// (see package secret), and the value is read when needed, so the same
// service can post for several accounts and move between clouds by changing
// which secret stores the program registers.
//
// Each kind of post is its own route with its own request and response:
// POST /publish/carousel, POST /publish/reel and POST /publish/story. A new
// kind is a new route added without touching the others. What the kinds share (reading the token, checking it belongs to the
// account, mapping Instagram's failures to answers) lives in this file.
//
// There is no authentication here: run the service where something else
// decides who may call it (Cloud Run's IAM, a private network).
package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/javadebadi/instagram-publisher/internal/instagram"
	"github.com/javadebadi/instagram-publisher/secret"
)

// maxBody bounds a request body; a caption and ten URLs are far smaller.
const maxBody = 256 << 10

// Config is what a publisher needs. The zero value works, reading tokens
// from the environment and env files only.
type Config struct {
	// Secrets resolves token references. Default: env: and envfile: only;
	// register a cloud store (secret/gcp) to read from one.
	Secrets *secret.Resolver
	// InstagramBaseURL overrides the API root; tests point it at a fake.
	InstagramBaseURL string
	// PollInterval overrides how often Instagram containers are checked.
	PollInterval time.Duration
}

type service struct{ cfg Config }

// New returns the service's handler:
//
//	GET  /health
//	POST /publish/carousel   post a carousel (or a single image)
//	POST /publish/reel       post a reel from a video
//	POST /publish/story      post a story from an image or a video
//	POST /publish            the same, kept for callers from before kinds had routes
//	POST /account/check      check a token works and belongs to the account
//	POST /token/refresh      refresh the account's long-lived token
func New(cfg Config) http.Handler {
	if cfg.Secrets == nil {
		cfg.Secrets = secret.NewResolver()
	}
	s := &service{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("POST /publish/carousel", s.handleCarousel)
	mux.HandleFunc("POST /publish/reel", s.handleReel)
	mux.HandleFunc("POST /publish/story", s.handleStory)
	mux.HandleFunc("POST /publish", s.handleCarousel)
	mux.HandleFunc("POST /account/check", s.handleAccount)
	mux.HandleFunc("POST /token/refresh", s.handleRefresh)
	return mux
}

func (s *service) igClient(userID, token string) *instagram.Client {
	c := instagram.New(userID, token)
	if s.cfg.InstagramBaseURL != "" {
		c.BaseURL = s.cfg.InstagramBaseURL
	}
	if s.cfg.PollInterval > 0 {
		c.PollInterval = s.cfg.PollInterval
	}
	return c
}

// account reads the token behind tokenRef and checks it belongs to userID. It
// answers the request itself and returns ok=false when it cannot go on.
func (s *service) account(ctx context.Context, w http.ResponseWriter, tokenRef, userID string) (ig *instagram.Client, username string, ok bool) {
	token, err := s.cfg.Secrets.Get(ctx, tokenRef)
	if err != nil {
		failIG(w, err)
		return nil, "", false
	}
	ig = s.igClient(userID, token)
	me, err := ig.Me(ctx)
	if err != nil {
		failIG(w, fmt.Errorf("checking the Instagram account: %w", err))
		return nil, "", false
	}
	if me.ID != "" && me.ID != userID {
		fail(w, http.StatusConflict, fmt.Sprintf("the token belongs to @%s (%s), not user id %s", me.Username, me.ID, userID))
		return nil, "", false
	}
	return ig, me.Username, true
}

// decode reads a JSON request body of at most limit bytes into v.
func decode(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return false
	}
	return true
}

func fail(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// failIG answers 502 for a failure of something we depend on (Instagram, the
// secret store). A token Instagram no longer accepts is marked, because it
// needs a person, not a retry; an outcome we cannot know is marked too,
// because a retry could post twice.
func failIG(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(w).Encode(map[string]any{
		"error":       err.Error(),
		"token_error": instagram.IsTokenError(err),
		// True when the post may have gone out although we did not hear back:
		// the caller must not retry before checking Instagram.
		"outcome_unknown": instagram.IsOutcomeUnknown(err),
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
