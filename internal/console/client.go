package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mnemosyneos/internal/airuntime"
	"mnemosyneos/internal/approval"
	"mnemosyneos/internal/execution"
	"mnemosyneos/internal/recall"
	"mnemosyneos/internal/skills"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *Client) Health() (map[string]string, error) {
	var resp map[string]string
	err := c.do(http.MethodGet, "/health", nil, &resp)
	return resp, err
}

func (c *Client) RuntimeState() (airuntime.RuntimeState, error) {
	var state airuntime.RuntimeState
	err := c.do(http.MethodGet, "/runtime/state", nil, &state)
	return state, err
}

func (c *Client) ListTasks() ([]airuntime.Task, error) {
	var resp struct {
		Tasks []airuntime.Task `json:"tasks"`
	}
	err := c.do(http.MethodGet, "/tasks", nil, &resp)
	return resp.Tasks, err
}

func (c *Client) GetTask(taskID string) (airuntime.Task, error) {
	var task airuntime.Task
	err := c.do(http.MethodGet, "/tasks/"+url.PathEscape(taskID), nil, &task)
	return task, err
}

func (c *Client) CreateTask(req airuntime.CreateTaskRequest) (airuntime.Task, error) {
	var task airuntime.Task
	err := c.do(http.MethodPost, "/tasks", req, &task)
	return task, err
}

func (c *Client) ApproveTask(taskID, approvedBy string) (airuntime.Task, error) {
	var task airuntime.Task
	err := c.do(http.MethodPost, "/tasks/"+url.PathEscape(taskID)+"/approve", airuntime.ApproveTaskRequest{
		ApprovedBy: approvedBy,
	}, &task)
	return task, err
}

func (c *Client) DenyTask(taskID, deniedBy, reason string) (airuntime.Task, error) {
	var task airuntime.Task
	err := c.do(http.MethodPost, "/tasks/"+url.PathEscape(taskID)+"/deny", airuntime.DenyTaskRequest{
		DeniedBy: deniedBy,
		Reason:   reason,
	}, &task)
	return task, err
}

func (c *Client) RunTask(taskID string) (skills.RunResult, error) {
	var result skills.RunResult
	err := c.do(http.MethodPost, "/tasks/"+url.PathEscape(taskID)+"/run", map[string]string{}, &result)
	return result, err
}

func (c *Client) GetAction(actionID string) (execution.ActionRecord, error) {
	var record execution.ActionRecord
	err := c.do(http.MethodGet, "/actions/"+url.PathEscape(actionID), nil, &record)
	return record, err
}

func (c *Client) ListApprovals(status string) ([]approval.Request, error) {
	path := "/approvals"
	if strings.TrimSpace(status) != "" {
		path += "?status=" + url.QueryEscape(status)
	}
	var resp struct {
		Approvals []approval.Request `json:"approvals"`
	}
	err := c.do(http.MethodGet, path, nil, &resp)
	return resp.Approvals, err
}

func (c *Client) GetApproval(approvalID string) (approval.Request, error) {
	var record approval.Request
	err := c.do(http.MethodGet, "/approvals/"+url.PathEscape(approvalID), nil, &record)
	return record, err
}

func (c *Client) ApproveAction(approvalID, approvedBy string) (approval.Request, error) {
	var record approval.Request
	err := c.do(http.MethodPost, "/approvals/"+url.PathEscape(approvalID)+"/approve", map[string]string{
		"approved_by": approvedBy,
	}, &record)
	return record, err
}

func (c *Client) DenyAction(approvalID, deniedBy, reason string) (approval.Request, error) {
	var record approval.Request
	err := c.do(http.MethodPost, "/approvals/"+url.PathEscape(approvalID)+"/deny", map[string]string{
		"denied_by": deniedBy,
		"reason":    reason,
	}, &record)
	return record, err
}

func (c *Client) Recall(query string, sources []string, limit int) (recall.Response, error) {
	params := url.Values{}
	params.Set("query", query)
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	for _, source := range sources {
		if trimmed := strings.TrimSpace(source); trimmed != "" {
			params.Add("source", trimmed)
		}
	}
	var resp recall.Response
	err := c.do(http.MethodGet, "/recall?"+params.Encode(), nil, &resp)
	return resp, err
}

type apiError struct {
	Error string `json:"error"`
}

func (c *Client) do(method, path string, reqBody any, target any) error {
	var body *bytes.Reader
	if reqBody == nil {
		body = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr apiError
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return fmt.Errorf("request failed with status %d", resp.StatusCode)
		}
		return fmt.Errorf("%s", apiErr.Error)
	}

	if target == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(target)
}
