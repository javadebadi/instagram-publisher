package publisher

import (
	"fmt"
	"net/http"
	"time"
)

// RefreshRequest is the body of POST /token/refresh.
type RefreshRequest struct {
	TokenRef string `json:"token_ref"`
}

// RefreshResponse says when the new token runs out. The token itself is
// never returned.
type RefreshResponse struct {
	RefreshedAt time.Time `json:"refreshed_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// handleRefresh exchanges the 60-day token for a fresh one and stores it at
// the same reference. Run it on a schedule: a token that runs out needs a
// person to generate a new one.
func (s *service) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if !decode(w, r, 4096, &req) {
		return
	}
	if req.TokenRef == "" {
		fail(w, http.StatusBadRequest, `want {"token_ref": "..."}`)
		return
	}
	ctx := r.Context()
	token, err := s.cfg.Secrets.Get(ctx, req.TokenRef)
	if err != nil {
		failIG(w, err)
		return
	}
	refreshed, err := s.igClient("", token).RefreshToken(ctx)
	if err != nil {
		failIG(w, fmt.Errorf("refreshing the token: %w", err))
		return
	}
	if err := s.cfg.Secrets.Set(ctx, req.TokenRef, refreshed.Token); err != nil {
		failIG(w, err)
		return
	}
	writeJSON(w, RefreshResponse{RefreshedAt: time.Now().UTC(), ExpiresAt: refreshed.ExpiresAt.UTC()})
}
