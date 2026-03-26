package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

type Response struct {
	Query    string   `json:"query"`
	Provider string   `json:"provider"`
	Results  []Result `json:"results"`
}

type Client interface {
	Search(ctx context.Context, query string) (Response, error)
}

func NewClientFromEnv() (Client, error) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("MNEMOSYNE_WEB_SEARCH_PROVIDER")))
	switch provider {
	case "", "none":
		return nil, nil
	case "serpapi":
		key := strings.TrimSpace(os.Getenv("MNEMOSYNE_WEB_SEARCH_API_KEY"))
		if key == "" {
			return nil, fmt.Errorf("MNEMOSYNE_WEB_SEARCH_API_KEY is required for serpapi")
		}
		endpoint := firstNonEmpty(os.Getenv("MNEMOSYNE_WEB_SEARCH_ENDPOINT"), "https://serpapi.com/search.json")
		return &SerpAPIClient{
			apiKey:   key,
			endpoint: endpoint,
			httpClient: &http.Client{
				Timeout: 20 * time.Second,
			},
		}, nil
	case "tavily":
		key := strings.TrimSpace(os.Getenv("MNEMOSYNE_WEB_SEARCH_API_KEY"))
		if key == "" {
			return nil, fmt.Errorf("MNEMOSYNE_WEB_SEARCH_API_KEY is required for tavily")
		}
		endpoint := firstNonEmpty(os.Getenv("MNEMOSYNE_WEB_SEARCH_ENDPOINT"), "https://api.tavily.com/search")
		return &TavilyClient{
			apiKey:   key,
			endpoint: endpoint,
			httpClient: &http.Client{
				Timeout: 20 * time.Second,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported web search provider %q", provider)
	}
}

type SerpAPIClient struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

func (c *SerpAPIClient) Search(ctx context.Context, query string) (Response, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return Response{}, fmt.Errorf("search query is required")
	}
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return Response{}, err
	}
	params := u.Query()
	params.Set("engine", "google")
	params.Set("q", query)
	params.Set("api_key", c.apiKey)
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Response{}, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Response{}, fmt.Errorf("serpapi search failed with status %d", resp.StatusCode)
	}

	var payload struct {
		OrganicResults []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Response{}, err
	}

	results := make([]Result, 0, len(payload.OrganicResults))
	for _, item := range payload.OrganicResults {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Link) == "" {
			continue
		}
		results = append(results, Result{
			Title:   strings.TrimSpace(item.Title),
			URL:     strings.TrimSpace(item.Link),
			Snippet: strings.TrimSpace(item.Snippet),
		})
	}
	return Response{
		Query:    query,
		Provider: "serpapi",
		Results:  results,
	}, nil
}

type TavilyClient struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

func (c *TavilyClient) Search(ctx context.Context, query string) (Response, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return Response{}, fmt.Errorf("search query is required")
	}
	body, err := json.Marshal(map[string]any{
		"api_key":      c.apiKey,
		"query":        query,
		"search_depth": "basic",
		"max_results":  8,
	})
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Response{}, fmt.Errorf("tavily search failed with status %d", resp.StatusCode)
	}

	var payload struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Response{}, err
	}

	results := make([]Result, 0, len(payload.Results))
	for _, item := range payload.Results {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.URL) == "" {
			continue
		}
		results = append(results, Result{
			Title:   strings.TrimSpace(item.Title),
			URL:     strings.TrimSpace(item.URL),
			Snippet: strings.TrimSpace(item.Content),
		})
	}
	return Response{
		Query:    query,
		Provider: "tavily",
		Results:  results,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
