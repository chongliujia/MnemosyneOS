package connectors

import (
	"context"

	"mnemosyneos/internal/websearch"
)

type WebSearchAdapter struct {
	client websearch.Client
}

func NewWebSearchAdapter(client websearch.Client) *WebSearchAdapter {
	if client == nil {
		return nil
	}
	return &WebSearchAdapter{client: client}
}

func (a *WebSearchAdapter) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	resp, err := a.client.Search(ctx, req.Query)
	if err != nil {
		return SearchResponse{}, err
	}
	out := SearchResponse{
		Query:    resp.Query,
		Provider: resp.Provider,
		Results:  make([]SearchResult, 0, len(resp.Results)),
	}
	for _, result := range resp.Results {
		out.Results = append(out.Results, SearchResult{
			Title:   result.Title,
			URL:     result.URL,
			Snippet: result.Snippet,
		})
	}
	if req.Limit > 0 && len(out.Results) > req.Limit {
		out.Results = out.Results[:req.Limit]
	}
	return out, nil
}
