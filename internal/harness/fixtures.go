package harness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"mnemosyneos/internal/connectors"
)

type fixtureSearchConnector struct {
	resp connectors.SearchResponse
}

func (c fixtureSearchConnector) Search(_ context.Context, req connectors.SearchRequest) (connectors.SearchResponse, error) {
	resp := c.resp
	if strings.TrimSpace(resp.Query) == "" {
		resp.Query = req.Query
	}
	if req.Limit > 0 && len(resp.Results) > req.Limit {
		resp.Results = resp.Results[:req.Limit]
	}
	return resp, nil
}

type fixtureGitHubConnector struct {
	resp connectors.GitHubIssueResponse
}

func (c fixtureGitHubConnector) SearchIssues(_ context.Context, req connectors.GitHubIssueRequest) (connectors.GitHubIssueResponse, error) {
	resp := c.resp
	if strings.TrimSpace(resp.Query) == "" {
		resp.Query = req.Query
	}
	if req.Limit > 0 && len(resp.Results) > req.Limit {
		resp.Results = resp.Results[:req.Limit]
	}
	return resp, nil
}

type fixtureEmailConnector struct {
	resp connectors.EmailResponse
}

func (c fixtureEmailConnector) ListMessages(_ context.Context, req connectors.EmailRequest) (connectors.EmailResponse, error) {
	resp := c.resp
	if strings.TrimSpace(resp.Query) == "" {
		resp.Query = req.Query
	}
	if req.Limit > 0 && len(resp.Results) > req.Limit {
		resp.Results = resp.Results[:req.Limit]
	}
	return resp, nil
}

func loadFixtureConnectors(scenario Scenario) (*connectors.Runtime, error) {
	search, err := loadSearchConnector(scenario)
	if err != nil {
		return nil, err
	}
	github, err := loadGitHubConnector(scenario)
	if err != nil {
		return nil, err
	}
	email, err := loadEmailConnector(scenario)
	if err != nil {
		return nil, err
	}
	return connectors.NewRuntime(search, github, email), nil
}

func loadSearchConnector(scenario Scenario) (connectors.SearchConnector, error) {
	if strings.TrimSpace(scenario.Fixtures.SearchResponseFile) == "" {
		return nil, nil
	}
	var resp connectors.SearchResponse
	if err := readFixtureJSON(filepath.Join(scenario.Dir, scenario.Fixtures.SearchResponseFile), &resp); err != nil {
		return nil, err
	}
	return fixtureSearchConnector{resp: resp}, nil
}

func loadGitHubConnector(scenario Scenario) (connectors.GitHubConnector, error) {
	if strings.TrimSpace(scenario.Fixtures.GitHubResponseFile) == "" {
		return nil, nil
	}
	var resp connectors.GitHubIssueResponse
	if err := readFixtureJSON(filepath.Join(scenario.Dir, scenario.Fixtures.GitHubResponseFile), &resp); err != nil {
		return nil, err
	}
	return fixtureGitHubConnector{resp: resp}, nil
}

func loadEmailConnector(scenario Scenario) (connectors.EmailConnector, error) {
	if strings.TrimSpace(scenario.Fixtures.EmailResponseFile) == "" {
		return nil, nil
	}
	var resp connectors.EmailResponse
	if err := readFixtureJSON(filepath.Join(scenario.Dir, scenario.Fixtures.EmailResponseFile), &resp); err != nil {
		return nil, err
	}
	return fixtureEmailConnector{resp: resp}, nil
}

func readFixtureJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}
