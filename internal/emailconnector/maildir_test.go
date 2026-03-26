package emailconnector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mnemosyneos/internal/connectors"
)

func TestMaildirListMessages(t *testing.T) {
	root := t.TempDir()
	inboxNew := filepath.Join(root, "INBOX", "new")
	inboxCur := filepath.Join(root, "INBOX", "cur")
	if err := os.MkdirAll(inboxNew, 0o755); err != nil {
		t.Fatalf("MkdirAll new: %v", err)
	}
	if err := os.MkdirAll(inboxCur, 0o755); err != nil {
		t.Fatalf("MkdirAll cur: %v", err)
	}

	if err := os.WriteFile(filepath.Join(inboxNew, "msg1"), []byte(strings.Join([]string{
		"From: Agent Test <agent@example.com>",
		"Subject: Root approval required",
		"Date: Mon, 23 Mar 2026 10:00:00 +0000",
		"Message-Id: <msg1@example.com>",
		"",
		"Please approve the root action.",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile new: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inboxCur, "msg2:2,S"), []byte(strings.Join([]string{
		"From: GitHub <noreply@github.com>",
		"Subject: Issue updated",
		"",
		"Approval issue was updated.",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile cur: %v", err)
	}

	client := &MaildirClient{rootDir: root, mailbox: "INBOX"}
	resp, err := client.ListMessages(context.Background(), connectors.EmailRequest{
		Query: "approval",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected two results, got %d", len(resp.Results))
	}
	foundUnread := false
	for _, msg := range resp.Results {
		if msg.Unread {
			foundUnread = true
			break
		}
	}
	if !foundUnread {
		t.Fatalf("expected at least one unread message")
	}
}
