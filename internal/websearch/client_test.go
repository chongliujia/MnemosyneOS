package websearch

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSerpAPIClientSearch(t *testing.T) {
	client := &SerpAPIClient{
		apiKey:   "test-key",
		endpoint: "https://search.test/search.json",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Query().Get("q") != "agent os" {
				t.Fatalf("unexpected query: %s", req.URL.Query().Get("q"))
			}
			return jsonResponse(`{"organic_results":[{"title":"Result A","link":"https://example.com/a","snippet":"alpha"}]}`), nil
		})},
	}
	resp, err := client.Search(context.Background(), "agent os")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if resp.Provider != "serpapi" {
		t.Fatalf("unexpected provider: %s", resp.Provider)
	}
	if len(resp.Results) != 1 || resp.Results[0].URL != "https://example.com/a" {
		t.Fatalf("unexpected results: %+v", resp.Results)
	}
}

func TestTavilyClientSearch(t *testing.T) {
	client := &TavilyClient{
		apiKey:   "test-key",
		endpoint: "https://search.test/search",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", req.Method)
			}
			return jsonResponse(`{"results":[{"title":"Result B","url":"https://example.com/b","content":"beta"}]}`), nil
		})},
	}
	resp, err := client.Search(context.Background(), "agent os")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if resp.Provider != "tavily" {
		t.Fatalf("unexpected provider: %s", resp.Provider)
	}
	if len(resp.Results) != 1 || !strings.Contains(resp.Results[0].Snippet, "beta") {
		t.Fatalf("unexpected results: %+v", resp.Results)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
