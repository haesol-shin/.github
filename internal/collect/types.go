package collect

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultAPIURL  = "https://api.github.com"
	UserAgent      = "repo-ops-validator/0.1"
	APIVersion     = "2022-11-28"
	RequestTimeout = 30 * time.Second
)

// HTTPDoer is the injectable GitHub HTTP seam.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// GitMetadataRunner is the injectable external-git seam.
type GitMetadataRunner interface {
	Metadata(ctx context.Context, pullRequest map[string]any, token, mergeBase string) (mergeBaseOut, diffDigest string, err error)
}

// GitHubClient is a GitHub REST client matching validate.py GitHubClient.
type GitHubClient struct {
	BaseURL string
	Token   string
	HTTP    HTTPDoer
}

// NewGitHubClient builds a production client. A nil doer uses a 30s http.Client.
// apiURL defaults to DefaultAPIURL and has trailing slashes stripped.
func NewGitHubClient(token, apiURL string, doer HTTPDoer) *GitHubClient {
	if strings.TrimSpace(apiURL) == "" {
		apiURL = DefaultAPIURL
	}
	if doer == nil {
		doer = &http.Client{Timeout: RequestTimeout}
	}
	return &GitHubClient{
		BaseURL: strings.TrimRight(apiURL, "/"),
		Token:   token,
		HTTP:    doer,
	}
}

// GitRunner runs the frozen external git metadata procedure.
type GitRunner struct{}

// NewGitRunner returns the production git seam.
func NewGitRunner() *GitRunner {
	return &GitRunner{}
}
