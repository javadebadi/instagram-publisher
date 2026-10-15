package publisher

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/javadebadi/instagram-publisher/internal/imagecheck"
)

const (
	maxCaption = 2200 // Instagram's limit
	maxImages  = 10
)

var userIDPattern = regexp.MustCompile(`^[0-9]{5,25}$`)

// CarouselRequest is the body of POST /publish/carousel: where the token
// lives, the account it must belong to, which slides to post (already public
// URLs, however the caller made them public) and the caption. One image posts
// a single-image post. Guarding against posting twice is the caller's job;
// this only does what it is asked.
type CarouselRequest struct {
	TokenRef string   `json:"token_ref"`
	UserID   string   `json:"user_id"` // the account the token must belong to
	Images   []string `json:"images"`
	Caption  string   `json:"caption"`
	// DryRun checks the token, the account and every image URL, and posts
	// nothing.
	DryRun bool `json:"dry_run"`
}

// CarouselResponse is the answer to POST /publish/carousel.
type CarouselResponse struct {
	DryRun      bool      `json:"dry_run"`
	Account     string    `json:"account"` // the Instagram username
	Message     string    `json:"message,omitempty"`
	MediaID     string    `json:"media_id,omitempty"`
	Permalink   string    `json:"permalink,omitempty"`
	PublishedAt time.Time `json:"published_at,omitzero"`
}

func (r CarouselRequest) validate() error {
	if r.TokenRef == "" {
		return errors.New("token_ref is empty")
	}
	if !userIDPattern.MatchString(r.UserID) {
		return errors.New("user_id must be the account's numeric Instagram user id")
	}
	if n := len(r.Images); n == 0 || n > maxImages {
		return fmt.Errorf("images: want 1 to %d URLs, got %d", maxImages, n)
	}
	for i, raw := range r.Images {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fmt.Errorf("images[%d] is not an http(s) URL", i)
		}
	}
	if len([]rune(r.Caption)) > maxCaption {
		return fmt.Errorf("caption is longer than Instagram's %d characters", maxCaption)
	}
	return nil
}

func (s *service) handleCarousel(w http.ResponseWriter, r *http.Request) {
	var req CarouselRequest
	if !decode(w, r, maxBody, &req) {
		return
	}
	if err := req.validate(); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	// The caller's own timeout (a Cloud Run request) bounds the wait, not a
	// number here: posting a carousel legitimately takes minutes.
	ctx := r.Context()

	ig, username, ok := s.account(ctx, w, req.TokenRef, req.UserID)
	if !ok {
		return
	}
	for _, u := range req.Images {
		if err := imagecheck.Check(ctx, u); err != nil {
			fail(w, http.StatusUnprocessableEntity, redactQuery(err.Error(), u))
			return
		}
	}
	resp := CarouselResponse{DryRun: req.DryRun, Account: username}
	if req.DryRun {
		resp.Message = fmt.Sprintf("dry run: would post %d slide(s) to @%s; nothing was posted", len(req.Images), username)
		writeJSON(w, resp)
		return
	}

	id, err := ig.Post(ctx, req.Images, req.Caption, nil)
	if err != nil {
		failIG(w, fmt.Errorf("posting to Instagram: %w", err))
		return
	}
	resp.MediaID = id
	resp.PublishedAt = time.Now().UTC()
	// The post is live from here on: a failed link lookup must not look like
	// a failed post, or the caller would retry and post twice.
	resp.Permalink, _ = ig.Permalink(ctx, id)
	writeJSON(w, resp)
}

// redactQuery keeps a signed URL's signature out of an error message: the
// message goes back to the caller and into logs.
func redactQuery(msg, rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.RawQuery == "" {
		return msg
	}
	short := *u
	short.RawQuery = ""
	return strings.ReplaceAll(msg, rawURL, short.String()+"?…")
}
