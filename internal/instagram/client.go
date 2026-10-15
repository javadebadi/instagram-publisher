// Package instagram is a small client for publishing through the Instagram
// API with Instagram Login (graph.instagram.com): image and carousel posts,
// container status polling, and long-lived token refresh.
//
// The access token travels in the request (query or form) as Instagram's
// docs show. It is never put into an error message: transport errors are
// rebuilt without the URL, and API errors carry only Meta's own message.
package instagram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the API root this client was written against.
const DefaultBaseURL = "https://graph.instagram.com/v23.0"

// Client calls the API as one Instagram professional account.
type Client struct {
	BaseURL string
	UserID  string // the account's Instagram user id (not the app-scoped id)
	Token   string
	HTTP    *http.Client

	// PollInterval and PollTimeout govern waiting for media containers.
	PollInterval time.Duration
	PollTimeout  time.Duration
}

// New returns a client with sensible defaults.
func New(userID, token string) *Client {
	return &Client{
		BaseURL:      DefaultBaseURL,
		UserID:       userID,
		Token:        token,
		HTTP:         &http.Client{Timeout: 60 * time.Second},
		PollInterval: 3 * time.Second,
		PollTimeout:  3 * time.Minute,
	}
}

// APIError is an error response from Meta.
type APIError struct {
	Status    int
	Message   string `json:"message"`
	Type      string `json:"type"`
	Code      int    `json:"code"`
	Subcode   int    `json:"error_subcode"`
	UserTitle string `json:"error_user_title"`
	UserMsg   string `json:"error_user_msg"`
	TraceID   string `json:"fbtrace_id"`
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("instagram: %s (code %d", e.Message, e.Code)
	if e.Subcode != 0 {
		msg += fmt.Sprintf(", subcode %d", e.Subcode)
	}
	msg += ")"
	if e.UserMsg != "" {
		msg += ": " + e.UserMsg
	}
	if h := e.Hint(); h != "" {
		msg += ". " + h
	}
	return msg
}

// Hint says what to do about the common errors.
func (e *APIError) Hint() string {
	switch {
	case e.Code == 190:
		return "The access token is invalid or expired: refresh it (POST /token/refresh) or generate a new one in the Meta app dashboard"
	case e.Code == 4 || e.Code == 17 || e.Code == 32 || e.Code == 613:
		return "Rate limited: wait and try again later"
	case e.Code == 9 || e.Subcode == 2207042:
		return "The account has reached its publishing limit (about 100 API posts per 24 hours)"
	case e.Code == 9004 || e.Subcode == 2207052:
		return "Instagram could not fetch an image: check that every slide URL is public and serves a JPEG"
	case e.Subcode == 2207026 || e.Subcode == 2207009:
		return "The image format or aspect ratio is not accepted (JPEG, 4:5 to 1.91:1)"
	case e.Code == 10 || e.Code == 200:
		return "Permission missing: the token needs instagram_business_basic and instagram_business_content_publish"
	}
	return ""
}

// OutcomeUnknownError means the final publish call failed in a way that does
// not say whether Instagram published the post: the connection dropped or
// timed out after the request may have gone through. Anything the caller does
// next (a retry above all) can post twice, so it must check Instagram first.
// An answer from Instagram, an error or not, is never this: it says what
// happened - except the anti-spam restriction (spamRestriction), which has been
// seen to come with a post that went out.
type OutcomeUnknownError struct{ Err error }

func (e *OutcomeUnknownError) Error() string {
	return "the post may or may not have been published: " + e.Err.Error()
}

func (e *OutcomeUnknownError) Unwrap() error { return e.Err }

// IsOutcomeUnknown reports whether err is an OutcomeUnknownError.
func IsOutcomeUnknown(err error) bool {
	var oe *OutcomeUnknownError
	return errors.As(err, &oe)
}

// spamRestriction is the error subcode Instagram's anti-spam system answers a
// publish with ("We restrict certain activity to protect our community").
// Developers report the post going out anyway, so the answer does not say that
// nothing was posted.
const spamRestriction = 2207051

// publishFinal publishes the ready container. A failure that is not an answer
// from Instagram is an unknown outcome, and so is the anti-spam restriction:
// both can leave a post that is live although the call reports failure.
func (c *Client) publishFinal(ctx context.Context, containerID string) (string, error) {
	id, err := c.Publish(ctx, containerID)
	if err != nil {
		var ae *APIError
		if !errors.As(err, &ae) || ae.Subcode == spamRestriction {
			return "", &OutcomeUnknownError{Err: err}
		}
		return "", err
	}
	return id, nil
}

// IsTokenError reports whether err means the token is bad or expired.
func IsTokenError(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Code == 190
}

func (c *Client) do(ctx context.Context, method, path string, params url.Values, out any) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("access_token", c.Token)
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + "/" + strings.TrimPrefix(path, "/")

	var req *http.Request
	var err error
	if method == http.MethodGet {
		req, err = http.NewRequestWithContext(ctx, method, endpoint+"?"+params.Encode(), nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(params.Encode()))
		if req != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// *url.Error includes the full URL, token and all; keep only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%s %s: reading response: %w", method, path, err)
	}
	if resp.StatusCode >= 400 {
		var wrapper struct {
			Error *APIError `json:"error"`
		}
		if json.Unmarshal(body, &wrapper) == nil && wrapper.Error != nil {
			wrapper.Error.Status = resp.StatusCode
			return wrapper.Error
		}
		return fmt.Errorf("%s %s: HTTP %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s %s: decoding response: %w", method, path, err)
	}
	return nil
}

// Account is who the token posts as.
type Account struct {
	ID       string `json:"user_id"`
	Username string `json:"username"`
}

// Me returns the account the token belongs to.
func (c *Client) Me(ctx context.Context) (*Account, error) {
	var a Account
	err := c.do(ctx, http.MethodGet, "me", url.Values{"fields": {"user_id,username"}}, &a)
	return &a, err
}

type idResponse struct {
	ID string `json:"id"`
}

// CreateImage makes an image container. A carousel item gets no caption.
func (c *Client) CreateImage(ctx context.Context, imageURL, caption string, carouselItem bool) (string, error) {
	p := url.Values{"image_url": {imageURL}}
	if carouselItem {
		p.Set("is_carousel_item", "true")
	} else if caption != "" {
		p.Set("caption", caption)
	}
	var r idResponse
	if err := c.do(ctx, http.MethodPost, c.UserID+"/media", p, &r); err != nil {
		return "", err
	}
	return r.ID, nil
}

// CreateCarousel makes the parent container for 2-10 child containers.
func (c *Client) CreateCarousel(ctx context.Context, children []string, caption string) (string, error) {
	p := url.Values{
		"media_type": {"CAROUSEL"},
		"children":   {strings.Join(children, ",")},
	}
	if caption != "" {
		p.Set("caption", caption)
	}
	var r idResponse
	if err := c.do(ctx, http.MethodPost, c.UserID+"/media", p, &r); err != nil {
		return "", err
	}
	return r.ID, nil
}

// WaitReady polls a container until Instagram has processed it.
func (c *Client) WaitReady(ctx context.Context, containerID string) error {
	deadline := time.Now().Add(c.PollTimeout)
	for {
		var r struct {
			StatusCode string `json:"status_code"`
			Status     string `json:"status"`
		}
		if err := c.do(ctx, http.MethodGet, containerID, url.Values{"fields": {"status_code,status"}}, &r); err != nil {
			return err
		}
		switch r.StatusCode {
		case "FINISHED", "PUBLISHED":
			return nil
		case "ERROR", "EXPIRED":
			return fmt.Errorf("instagram: container %s %s: %s", containerID, strings.ToLower(r.StatusCode), r.Status)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("instagram: container %s still %s after %s", containerID, r.StatusCode, c.PollTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.PollInterval):
		}
	}
}

// Publish publishes a ready container and returns the new media id.
func (c *Client) Publish(ctx context.Context, containerID string) (string, error) {
	var r idResponse
	if err := c.do(ctx, http.MethodPost, c.UserID+"/media_publish", url.Values{"creation_id": {containerID}}, &r); err != nil {
		return "", err
	}
	return r.ID, nil
}

// Permalink returns a published media's public URL.
func (c *Client) Permalink(ctx context.Context, mediaID string) (string, error) {
	var r struct {
		Permalink string `json:"permalink"`
	}
	err := c.do(ctx, http.MethodGet, mediaID, url.Values{"fields": {"permalink"}}, &r)
	return r.Permalink, err
}

// Refreshed is a refreshed long-lived token.
type Refreshed struct {
	Token     string    `json:"access_token"`
	ExpiresIn int64     `json:"expires_in"` // seconds
	ExpiresAt time.Time `json:"-"`
}

// RefreshToken exchanges the current long-lived token (at least 24 hours
// old) for a new one valid 60 days. The client keeps using the old token;
// the caller decides where to store the new one.
func (c *Client) RefreshToken(ctx context.Context) (*Refreshed, error) {
	var r Refreshed
	// The refresh endpoint is not versioned.
	base := c.BaseURL
	if i := strings.LastIndex(base, "/v"); i > len("https://") {
		base = base[:i]
	}
	cc := *c
	cc.BaseURL = base
	if err := cc.do(ctx, http.MethodGet, "refresh_access_token", url.Values{"grant_type": {"ig_refresh_token"}}, &r); err != nil {
		return nil, err
	}
	r.ExpiresAt = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	return &r, nil
}

// Post publishes one image, or a carousel of 2-10, and returns the media id.
// progress, if set, is told about each step.
func (c *Client) Post(ctx context.Context, imageURLs []string, caption string, progress func(string)) (string, error) {
	say := func(format string, args ...any) {
		if progress != nil {
			progress(fmt.Sprintf(format, args...))
		}
	}
	switch n := len(imageURLs); {
	case n == 0:
		return "", errors.New("instagram: nothing to post")
	case n > 10:
		return "", fmt.Errorf("instagram: %d images; a carousel holds at most 10", n)
	case n == 1:
		id, err := c.CreateImage(ctx, imageURLs[0], caption, false)
		if err != nil {
			return "", err
		}
		say("container created")
		if err := c.WaitReady(ctx, id); err != nil {
			return "", err
		}
		return c.publishFinal(ctx, id)
	}

	children := make([]string, len(imageURLs))
	for i, u := range imageURLs {
		id, err := c.CreateImage(ctx, u, "", true)
		if err != nil {
			return "", fmt.Errorf("slide %d: %w", i+1, err)
		}
		children[i] = id
		say("slide %d container created", i+1)
	}
	for i, id := range children {
		if err := c.WaitReady(ctx, id); err != nil {
			return "", fmt.Errorf("slide %d: %w", i+1, err)
		}
	}
	say("all slides processed")
	parent, err := c.CreateCarousel(ctx, children, caption)
	if err != nil {
		return "", err
	}
	if err := c.WaitReady(ctx, parent); err != nil {
		return "", err
	}
	say("carousel processed; publishing")
	return c.publishFinal(ctx, parent)
}
