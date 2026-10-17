// Package client calls an instagram-publisher service over HTTP. It speaks
// the request and response types of package publisher, so a caller and the
// service cannot drift apart.
//
// A Client is one account on one service: every call names the account's
// token by reference (never the token) and the user id it must belong to.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/javadebadi/instagram-publisher/publisher"
)

// Client is one account on one publisher service.
type Client struct {
	BaseURL  string // e.g. http://localhost:8080
	TokenRef string // where the service finds the account's token
	UserID   string // the account the token must belong to
	// HTTP is the client requests go through. Default: one without an
	// overall timeout, because posting a carousel legitimately takes minutes;
	// the context is what ends the wait.
	HTTP *http.Client
}

// Error is an answer from the service that is not a success.
type Error struct {
	Status         int
	Message        string
	TokenError     bool // Instagram refused the token: a person must renew it
	OutcomeUnknown bool // the post may be live: look at the account before retrying
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("publisher answered %d: %s", e.Status, e.Message)
	if e.TokenError {
		msg += " (the token was refused: it needs renewing, not retrying)"
	}
	if e.OutcomeUnknown {
		msg += " (the post may have gone out: check the Instagram account before trying again)"
	}
	return msg
}

// ImageNotFetchable reports whether the service could not fetch an image
// URL, which usually means the images are uploaded but not live yet.
func ImageNotFetchable(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusUnprocessableEntity
}

// Check asks the service whether the token works and belongs to the account,
// and returns the account's username. Nothing is posted.
func (c *Client) Check(ctx context.Context) (string, error) {
	var out publisher.AccountResponse
	err := c.post(ctx, "/account/check", publisher.AccountRequest{TokenRef: c.TokenRef, UserID: c.UserID}, &out)
	return out.Account, err
}

// Carousel posts the images (already at public URLs) as a carousel; one
// image posts a single image.
func (c *Client) Carousel(ctx context.Context, images []string, caption string) (*publisher.CarouselResponse, error) {
	var out publisher.CarouselResponse
	err := c.post(ctx, "/publish/carousel", publisher.CarouselRequest{
		TokenRef: c.TokenRef, UserID: c.UserID, Images: images, Caption: caption,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Refresh has the service exchange the account's token for a fresh one,
// stored at the same reference, and says when it runs out.
func (c *Client) Refresh(ctx context.Context) (*publisher.RefreshResponse, error) {
	var out publisher.RefreshResponse
	if err := c.post(ctx, "/token/refresh", publisher.RefreshRequest{TokenRef: c.TokenRef}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("the publisher at %s is not reachable: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		e := &Error{Status: resp.StatusCode}
		var f struct {
			Error          string `json:"error"`
			TokenError     bool   `json:"token_error"`
			OutcomeUnknown bool   `json:"outcome_unknown"`
		}
		if json.Unmarshal(data, &f) == nil && f.Error != "" {
			e.Message, e.TokenError, e.OutcomeUnknown = f.Error, f.TokenError, f.OutcomeUnknown
		} else {
			e.Message = strings.TrimSpace(string(data))
		}
		return e
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("the publisher sent an answer that isn't JSON: %w", err)
	}
	return nil
}
