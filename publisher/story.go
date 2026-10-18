package publisher

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/javadebadi/instagram-publisher/internal/imagecheck"
)

// StoryRequest is the body of POST /publish/story: the account and one
// picture (a JPEG) or one video (MP4 or MOV) already at a public URL. Stories
// take no caption. Exactly one of Image and Video is set.
type StoryRequest struct {
	TokenRef string `json:"token_ref"`
	UserID   string `json:"user_id"` // the account the token must belong to
	Image    string `json:"image,omitempty"`
	Video    string `json:"video,omitempty"`
	// DryRun checks the token, the account and the URL, and posts nothing.
	DryRun bool `json:"dry_run"`
}

// StoryResponse is the answer to POST /publish/story.
type StoryResponse CarouselResponse

func (r StoryRequest) validate() error {
	if err := validateAccount(r.TokenRef, r.UserID); err != nil {
		return err
	}
	if (r.Image == "") == (r.Video == "") {
		return errors.New("set exactly one of image and video")
	}
	if r.Image != "" && !isHTTPURL(r.Image) {
		return errors.New("image is not an http(s) URL")
	}
	if r.Video != "" && !isHTTPURL(r.Video) {
		return errors.New("video is not an http(s) URL")
	}
	return nil
}

func (s *service) handleStory(w http.ResponseWriter, r *http.Request) {
	var req StoryRequest
	if !decode(w, r, maxBody, &req) {
		return
	}
	if err := req.validate(); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()

	ig, username, ok := s.account(ctx, w, req.TokenRef, req.UserID)
	if !ok {
		return
	}
	isVideo := req.Video != ""
	mediaURL, check := req.Image, imagecheck.Check
	if isVideo {
		mediaURL, check = req.Video, imagecheck.CheckVideo
	}
	if err := check(ctx, mediaURL); err != nil {
		fail(w, http.StatusUnprocessableEntity, redactQuery(err.Error(), mediaURL))
		return
	}
	resp := CarouselResponse{DryRun: req.DryRun, Account: username}
	if req.DryRun {
		resp.Message = fmt.Sprintf("dry run: would post a story to @%s; nothing was posted", username)
		writeJSON(w, StoryResponse(resp))
		return
	}

	id, err := ig.PostStory(ctx, mediaURL, isVideo)
	if err != nil {
		failIG(w, fmt.Errorf("posting to Instagram: %w", err))
		return
	}
	markPublished(ctx, ig, id, &resp)
	writeJSON(w, StoryResponse(resp))
}
