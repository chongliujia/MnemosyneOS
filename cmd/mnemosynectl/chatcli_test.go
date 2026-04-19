package main

import (
	"bytes"
	"strings"
	"testing"

	"mnemosyneos/internal/chat"
)

type fakeChatClient struct {
	sendReqs     []chat.SendRequest
	sendResp     chat.SendResponse
	historyReqs  []string
	historyLimit []int
	historyResp  []chat.Message
}

func (f *fakeChatClient) SendChat(req chat.SendRequest) (chat.SendResponse, error) {
	f.sendReqs = append(f.sendReqs, req)
	return f.sendResp, nil
}

func (f *fakeChatClient) ChatMessages(sessionID string, limit int) ([]chat.Message, error) {
	f.historyReqs = append(f.historyReqs, sessionID)
	f.historyLimit = append(f.historyLimit, limit)
	return append([]chat.Message{}, f.historyResp...), nil
}

func TestRunAskSendsChatAndPrintsAssistantReply(t *testing.T) {
	t.Parallel()

	client := &fakeChatClient{
		sendResp: chat.SendResponse{
			AssistantMessage: chat.Message{
				Content:       "你好。我在这里。",
				TaskID:        "task-1",
				TaskState:     "planned",
				SelectedSkill: "task-plan",
			},
		},
	}
	var out bytes.Buffer
	if err := runAsk(client, askOptions{
		SessionID:        "default",
		ExecutionProfile: "user",
		Message:          "你好",
	}, &out); err != nil {
		t.Fatalf("runAsk returned error: %v", err)
	}
	if len(client.sendReqs) != 1 {
		t.Fatalf("expected one chat request, got %d", len(client.sendReqs))
	}
	if client.sendReqs[0].Message != "你好" {
		t.Fatalf("unexpected message: %+v", client.sendReqs[0])
	}
	body := out.String()
	if !strings.Contains(body, "你好。我在这里。") {
		t.Fatalf("expected assistant reply, got %q", body)
	}
	if !strings.Contains(body, "[task] task-1 state=planned skill=task-plan") {
		t.Fatalf("expected task summary, got %q", body)
	}
}

func TestRunChatShowsHistoryAndQuits(t *testing.T) {
	t.Parallel()

	client := &fakeChatClient{
		historyResp: []chat.Message{
			{Role: "user", Content: "之前的问题"},
			{Role: "assistant", Content: "之前的回答"},
		},
		sendResp: chat.SendResponse{
			AssistantMessage: chat.Message{
				Content: "当前回复",
			},
		},
	}
	var out bytes.Buffer
	in := strings.NewReader("继续\n/history\n/quit\n")
	if err := runChat(client, chatOptions{
		SessionID: "default",
		History:   4,
	}, in, &out); err != nil {
		t.Fatalf("runChat returned error: %v", err)
	}
	if len(client.sendReqs) != 1 || client.sendReqs[0].Message != "继续" {
		t.Fatalf("expected one sent chat turn, got %+v", client.sendReqs)
	}
	body := out.String()
	for _, needle := range []string{
		"MnemosyneOS CLI chat",
		"recent:",
		"user: 之前的问题",
		"assistant: 之前的回答",
		"当前回复",
		"bye",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in output, got %q", needle, body)
		}
	}
}
