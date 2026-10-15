package publisher

import "net/http"

// AccountRequest is the body of POST /account/check.
type AccountRequest struct {
	TokenRef string `json:"token_ref"`
	UserID   string `json:"user_id"` // the account the token must belong to
}

// AccountResponse names the account the token posts as.
type AccountResponse struct {
	Account string `json:"account"` // the Instagram username
}

// handleAccount checks, without posting anything, that the token behind
// token_ref works and belongs to user_id. Callers run it before doing work
// (uploading slides) that only makes sense if posting can succeed.
func (s *service) handleAccount(w http.ResponseWriter, r *http.Request) {
	var req AccountRequest
	if !decode(w, r, 4096, &req) {
		return
	}
	if req.TokenRef == "" {
		fail(w, http.StatusBadRequest, "token_ref is empty")
		return
	}
	if !userIDPattern.MatchString(req.UserID) {
		fail(w, http.StatusBadRequest, "user_id must be the account's numeric Instagram user id")
		return
	}
	_, username, ok := s.account(r.Context(), w, req.TokenRef, req.UserID)
	if !ok {
		return
	}
	writeJSON(w, AccountResponse{Account: username})
}
