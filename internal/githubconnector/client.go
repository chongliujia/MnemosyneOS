package githubconnector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"mnemosyneos/internal/connectors"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
	owner      string
	repo       string
}

func NewClientFromEnv() (*Client, error) {
	owner := strings.TrimSpace(os.Getenv("MNEMOSYNE_GITHUB_OWNER"))
	repo := strings.TrimSpace(os.Getenv("MNEMOSYNE_GITHUB_REPO"))
	if owner == "" || repo == "" {
		return nil, nil
	}
	baseURL := strings.TrimSpace(os.Getenv("MNEMOSYNE_GITHUB_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return &Client{
		httpClient: http.DefaultClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      strings.TrimSpace(os.Getenv("MNEMOSYNE_GITHUB_TOKEN")),
		owner:      owner,
		repo:       repo,
	}, nil
}

func (c *Client) SearchIssues(ctx context.Context, req connectors.GitHubIssueRequest) (connectors.GitHubIssueResponse, error) {
	if c == nil {
		return connectors.GitHubIssueResponse{}, fmt.Errorf("%w: github connector is not configured", connectors.ErrConnectorUnavailable)
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}

	query := fmt.Sprintf("repo:%s/%s is:issue %s", c.owner, c.repo, strings.TrimSpace(req.Query))
	endpoint := fmt.Sprintf("%s/search/issues?q=%s&per_page=%d", c.baseURL, url.QueryEscape(query), limit)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return connectors.GitHubIssueResponse{}, err
	}
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return connectors.GitHubIssueResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return connectors.GitHubIssueResponse{}, fmt.Errorf("github issue search failed with status %d", resp.StatusCode)
	}

	var payload struct {
		Items []struct {
			Number  int    `json:"number"`
			Title   string `json:"title"`
			HTMLURL string `json:"html_url"`
			State   string `json:"state"`
			Body    string `json:"body"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return connectors.GitHubIssueResponse{}, err
	}

	out := connectors.GitHubIssueResponse{
		Query:    req.Query,
		Provider: "github",
		Results:  make([]connectors.GitHubIssue, 0, len(payload.Items)),
	}
	repoName := c.owner + "/" + c.repo
	for _, item := range payload.Items {
		out.Results = append(out.Results, connectors.GitHubIssue{
			Number: item.Number,
			Title:  item.Title,
			URL:    item.HTMLURL,
			State:  item.State,
			Body:   item.Body,
			Repo:   repoName,
		})
	}
	return out, nil
}
