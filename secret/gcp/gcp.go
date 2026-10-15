// Package gcp is the Google Secret Manager store for package secret. Register
// it where the service starts:
//
//	r := secret.NewResolver()
//	r.Register("gcp", gcp.New(nil))
//
// and a reference looks like gcp:projects/P/secrets/S (latest version).
//
// It is a separate package so that a build for another cloud does not carry
// it: a binary contains only what its main imports.
package gcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/oauth2/google"

	"github.com/javadebadi/instagram-publisher/secret"
)

// BaseURL is Secret Manager's REST endpoint.
const BaseURL = "https://secretmanager.googleapis.com/v1"

// secretName is a secret's full name, optionally with a version:
// projects/P/secrets/S or projects/P/secrets/S/versions/V.
var secretName = regexp.MustCompile(`^projects/[A-Za-z0-9_-]+/secrets/[A-Za-z0-9_-]+(/versions/[A-Za-z0-9]+)?$`)

type store struct {
	base   string
	client *http.Client // nil: Google's default credentials, found on first use
}

// New returns the Google Secret Manager store. Pass a client to supply
// credentials yourself (tests do); nil uses the environment's default
// credentials: the service account on Cloud Run, or `gcloud auth
// application-default login` on a laptop. Over REST on purpose: no SDK is
// needed for two calls.
func New(client *http.Client) secret.Store { return &store{base: BaseURL, client: client} }

// NewAt is New against another base URL, for tests.
func NewAt(base string, client *http.Client) secret.Store {
	return &store{base: base, client: client}
}

func (g *store) http(ctx context.Context) (*http.Client, error) {
	if g.client != nil {
		return g.client, nil
	}
	c, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("no Google credentials: %w", err)
	}
	return c, nil
}

// Get returns the latest version, or the one the reference names.
func (g *store) Get(ctx context.Context, ref string) (string, error) {
	if !secretName.MatchString(ref) {
		return "", errors.New("want gcp:projects/PROJECT/secrets/NAME")
	}
	if !strings.Contains(ref, "/versions/") {
		ref += "/versions/latest"
	}
	c, err := g.http(ctx)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.base+"/"+ref+":access", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", errors.New("Secret Manager is not reachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", apiError(resp)
	}
	var out struct {
		Payload struct {
			Data string `json:"data"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", errors.New("Secret Manager sent an answer that isn't JSON")
	}
	raw, err := base64.StdEncoding.DecodeString(out.Payload.Data)
	if err != nil {
		return "", errors.New("Secret Manager sent a value that isn't base64")
	}
	return strings.TrimSpace(string(raw)), nil
}

// Set adds a new version; earlier versions stay until they are disabled or
// destroyed.
func (g *store) Set(ctx context.Context, ref, value string) error {
	if !secretName.MatchString(ref) || strings.Contains(ref, "/versions/") {
		return errors.New("want gcp:projects/PROJECT/secrets/NAME (no version)")
	}
	body, _ := json.Marshal(map[string]any{
		"payload": map[string]string{"data": base64.StdEncoding.EncodeToString([]byte(value))},
	})
	c, err := g.http(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.base+"/"+ref+":addVersion", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return errors.New("Secret Manager is not reachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}
	return nil
}

// apiError reports Google's own message, never the request (which holds the
// secret on a Set).
func apiError(resp *http.Response) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = json.Unmarshal(data, &e)
	if e.Error.Message != "" {
		return fmt.Errorf("Secret Manager answered %d: %s", resp.StatusCode, e.Error.Message)
	}
	return fmt.Errorf("Secret Manager answered %d", resp.StatusCode)
}
