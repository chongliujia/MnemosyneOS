package connectors

import (
	"context"
	"errors"
	"strings"
)

var ErrConnectorUnavailable = errors.New("connector unavailable")

type SearchRequest struct {
	Query string
	Limit int
}

type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

type SearchResponse struct {
	Query    string
	Provider string
	Results  []SearchResult
}

type GitHubIssueRequest struct {
	Query string
	Limit int
}

type GitHubIssue struct {
	Number int
	Title  string
	URL    string
	State  string
	Body   string
	Repo   string
}

type GitHubIssueResponse struct {
	Query    string
	Provider string
	Results  []GitHubIssue
}

type EmailRequest struct {
	Query string
	Limit int
}

type EmailMessage struct {
	MessageID string
	Mailbox   string
	From      string
	Subject   string
	Date      string
	Snippet   string
	Path      string
	Unread    bool
}

type EmailResponse struct {
	Query    string
	Provider string
	Results  []EmailMessage
}

type SearchConnector interface {
	Search(ctx context.Context, req SearchRequest) (SearchResponse, error)
}

type GitHubConnector interface {
	SearchIssues(ctx context.Context, req GitHubIssueRequest) (GitHubIssueResponse, error)
}

type EmailConnector interface {
	ListMessages(ctx context.Context, req EmailRequest) (EmailResponse, error)
}

type Runtime struct {
	search SearchConnector
	github GitHubConnector
	email  EmailConnector
}

func NewRuntime(search SearchConnector, github GitHubConnector, email EmailConnector) *Runtime {
	return &Runtime{
		search: search,
		github: github,
		email:  email,
	}
}

func (r *Runtime) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	if r == nil || r.search == nil {
		return SearchResponse{}, ErrConnectorUnavailable
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		return SearchResponse{}, errors.New("search query is required")
	}
	return r.search.Search(ctx, req)
}

func (r *Runtime) SearchGitHubIssues(ctx context.Context, req GitHubIssueRequest) (GitHubIssueResponse, error) {
	if r == nil || r.github == nil {
		return GitHubIssueResponse{}, ErrConnectorUnavailable
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		return GitHubIssueResponse{}, errors.New("github issue query is required")
	}
	return r.github.SearchIssues(ctx, req)
}

func (r *Runtime) ListEmails(ctx context.Context, req EmailRequest) (EmailResponse, error) {
	if r == nil || r.email == nil {
		return EmailResponse{}, ErrConnectorUnavailable
	}
	req.Query = strings.TrimSpace(req.Query)
	return r.email.ListMessages(ctx, req)
}
