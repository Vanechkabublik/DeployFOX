package github

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"deployfox/internal/auth"
)

func (h Handler) Branches(w http.ResponseWriter, r *http.Request) {
	repository := r.URL.Query().Get("repository")
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		auth.Error(w, 400, "Repository owner/name required")
		return
	}
	for _, part := range parts {
		if !regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`).MatchString(part) || part == "." || part == ".." {
			auth.Error(w, 400, "Invalid repository")
			return
		}
	}
	token, err := TokenFromStore(r.Context(), h.Store)
	if err != nil {
		auth.Error(w, 503, "Connect GitHub App first")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), "GET", "https://api.github.com/repos/"+url.PathEscape(parts[0])+"/"+url.PathEscape(parts[1])+"/branches?per_page=100", nil)
	if err != nil {
		auth.Error(w, 500, "Cannot prepare GitHub request")
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		auth.Error(w, 502, "GitHub unavailable")
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		auth.Error(w, 502, "Cannot list branches for this repository")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, io.LimitReader(response.Body, 1<<20))
}
