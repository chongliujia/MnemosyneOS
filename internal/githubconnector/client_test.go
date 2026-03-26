package githubconnector

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemosyneos/internal/connectors"
)

func TestSearchIssues(t *testing.T) {
	client := &Client{
		baseURL: "https://api.github.test",
		owner:   "mnemosyne",
		repo:    "agentos",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if got := req.URL.Path; got != "/search/issues" {
					t.Fatalf("unexpected path: %s", got)
				}
				if got := req.URL.Query().Get("q"); !strings.Contains(got, "repo:mnemosyne/agentos") {
					t.Fatalf("unexpected query: %s", got)
				}
				return jsonResponse(`{"items":[{"number":7,"title":"Root approval flow","html_url":"https://github.test/7","state":"open","body":"Need formal approval flow"}]}`), nil
			}),
		},
	}

	resp, err := client.SearchIssues(context.Background(), connectors.GitHubIssueRequest{
		Query: "approval",
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected one result, got %d", len(resp.Results))
	}
	if resp.Results[0].Number != 7 {
		t.Fatalf("unexpected issue number: %d", resp.Results[0].Number)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
