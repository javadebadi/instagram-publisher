package publisher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/javadebadi/instagram-publisher/internal/imagecheck"
	"github.com/javadebadi/instagram-publisher/internal/instagram"
)

// ReelRequest is the body of POST /publish/reel: the account, the video (an
// MP4 or MOV already at a public URL) and how the reel should look. Guarding
// against posting twice is the caller's job, as for every kind of post.
type ReelRequest struct {
	TokenRef string `json:"token_ref"`
	UserID   string `json:"user_id"` // the account the token must belong to
	Video    string `json:"video"`
	Caption  string `json:"caption"`
	// ShareToFeed also shows the reel on the profile grid, not only in the
	// Reels tab. Left out, Instagram's own default applies.
	ShareToFeed *bool `json:"share_to_feed,omitempty"`
	// Cover is a public JPEG used as the reel's cover. Left out, Instagram
	// takes a frame of the video.
	Cover string `json:"cover,omitempty"`
	// ThumbOffsetMS picks the frame, in milliseconds from the start of the
	// video, used as the cover. It cannot be combined with Cover.
	ThumbOffsetMS *int `json:"thumb_offset_ms,omitempty"`
	// DryRun checks the token, the account and the URLs, and posts nothing.
	DryRun bool `json:"dry_run"`
}

// ReelResponse is the answer to POST /publish/reel.
type ReelResponse CarouselResponse

func (r ReelRequest) validate() error {
	if err := validateAccount(r.TokenRef, r.UserID); err != nil {
		return err
	}
	if !isHTTPURL(r.Video) {
		return errors.New("video is not an http(s) URL")
	}
	if r.Cover != "" && !isHTTPURL(r.Cover) {
		return errors.New("cover is not an http(s) URL")
	}
	if r.ThumbOffsetMS != nil {
		if *r.ThumbOffsetMS < 0 {
			return errors.New("thumb_offset_ms cannot be negative")
		}
		if r.Cover != "" {
			return errors.New("set either cover or thumb_offset_ms, not both")
		}
	}
	if len([]rune(r.Caption)) > maxCaption {
		return fmt.Errorf("caption is longer than Instagram's %d characters", maxCaption)
	}
	return nil
}

func (s *service) handleReel(w http.ResponseWriter, r *http.Request) {
	var req ReelRequest
	if !decode(w, r, maxBody, &req) {
		return
	}
	if err := req.validate(); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	// As for a carousel, the caller's timeout bounds the wait: Instagram can
	// take several minutes to process a video.
	ctx := r.Context()

	ig, username, ok := s.account(ctx, w, req.TokenRef, req.UserID)
	if !ok {
		return
	}
	if err := imagecheck.CheckVideo(ctx, req.Video); err != nil {
		fail(w, http.StatusUnprocessableEntity, redactQuery(err.Error(), req.Video))
		return
	}
	if req.Cover != "" {
		if err := imagecheck.Check(ctx, req.Cover); err != nil {
			fail(w, http.StatusUnprocessableEntity, redactQuery(err.Error(), req.Cover))
			return
		}
	}
	resp := CarouselResponse{DryRun: req.DryRun, Account: username}
	if req.DryRun {
		resp.Message = fmt.Sprintf("dry run: would post a reel to @%s; nothing was posted", username)
		writeJSON(w, ReelResponse(resp))
		return
	}

	id, err := ig.PostReel(ctx, req.Video, instagram.ReelOptions{
		Caption:       req.Caption,
		CoverURL:      req.Cover,
		ShareToFeed:   req.ShareToFeed,
		ThumbOffsetMS: req.ThumbOffsetMS,
	})
	if err != nil {
		failIG(w, fmt.Errorf("posting to Instagram: %w", err))
		return
	}
	markPublished(ctx, ig, id, &resp)
	writeJSON(w, ReelResponse(resp))
}

// markPublished fills in the answer for a post that is live. A failed link
// lookup must not look like a failed post, or the caller would retry and post
// twice.
func markPublished(ctx context.Context, ig *instagram.Client, id string, resp *CarouselResponse) {
	resp.MediaID = id
	resp.PublishedAt = time.Now().UTC()
	resp.Permalink, _ = ig.Permalink(ctx, id)
}
