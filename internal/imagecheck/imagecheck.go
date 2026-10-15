// Package imagecheck asks whether a URL serves something Instagram can use,
// before anything is posted. It stands alone so the publisher does not need
// the rest of the carousel code.
package imagecheck

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HTTPClient fetches the URLs; tests swap it.
var HTTPClient = http.DefaultClient

// Check fetches url the way Instagram will and reports anything that
// would make Instagram's fetch fail. It asks with HEAD, and with a one-byte
// GET when the host refuses HEAD: a signed URL is valid for the one method it
// was signed for, which is GET.
func Check(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := probe(ctx, http.MethodHead, url)
	if err == nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented) {
		resp, err = probe(ctx, http.MethodGet, url)
	}
	if err != nil {
		return fmt.Errorf("%s is not reachable: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("%s returned %s; is the file public (and deployed, for a folder that is synced elsewhere)?", url, resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/jpeg") {
		return fmt.Errorf("%s is served as %q; Instagram needs image/jpeg", url, ct)
	}
	return nil
}

func probe(ctx context.Context, method, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	if method == http.MethodGet {
		req.Header.Set("Range", "bytes=0-0")
	}
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	return resp, nil
}
