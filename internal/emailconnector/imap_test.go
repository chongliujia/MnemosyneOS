package emailconnector

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"mnemosyneos/internal/connectors"
)

func TestIMAPListMessages(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	go func() {
		defer serverConn.Close()
		writeIMAP(serverConn, "* OK IMAP4 ready\r\n")
		expectIMAPCommand(t, serverConn, "LOGIN")
		writeIMAP(serverConn, "A0001 OK LOGIN completed\r\n")
		expectIMAPCommand(t, serverConn, "SELECT")
		writeIMAP(serverConn, "* 2 EXISTS\r\nA0002 OK [READ-ONLY] SELECT completed\r\n")
		expectIMAPCommand(t, serverConn, "SEARCH ALL")
		writeIMAP(serverConn, "* SEARCH 1 2\r\nA0003 OK SEARCH completed\r\n")
		expectIMAPCommand(t, serverConn, "FETCH 2 (FLAGS BODY.PEEK[HEADER.FIELDS (MESSAGE-ID SUBJECT FROM DATE)])")
		header := "Message-Id: <msg2@example.com>\r\nSubject: Root approval required\r\nFrom: Agent Test <agent@example.com>\r\nDate: Mon, 23 Mar 2026 10:00:00 +0000\r\n\r\n"
		writeIMAP(serverConn, fmt.Sprintf("* 2 FETCH (FLAGS () BODY[HEADER.FIELDS (MESSAGE-ID SUBJECT FROM DATE)] {%d}\r\n%s)\r\nA0004 OK FETCH completed\r\n", len(header), header))
		expectIMAPCommand(t, serverConn, "FETCH 2 (BODY.PEEK[TEXT]<0.240>)")
		body := "Please approve the root action in AgentOS."
		writeIMAP(serverConn, fmt.Sprintf("* 2 FETCH (BODY[TEXT]<0> {%d}\r\n%s)\r\nA0005 OK FETCH completed\r\n", len(body), body))
		expectIMAPCommand(t, serverConn, "FETCH 1 (FLAGS BODY.PEEK[HEADER.FIELDS (MESSAGE-ID SUBJECT FROM DATE)])")
		header2 := "Message-Id: <msg1@example.com>\r\nSubject: Status update\r\nFrom: GitHub <noreply@github.com>\r\n\r\n"
		writeIMAP(serverConn, fmt.Sprintf("* 1 FETCH (FLAGS (\\Seen) BODY[HEADER.FIELDS (MESSAGE-ID SUBJECT FROM DATE)] {%d}\r\n%s)\r\nA0006 OK FETCH completed\r\n", len(header2), header2))
		expectIMAPCommand(t, serverConn, "FETCH 1 (BODY.PEEK[TEXT]<0.240>)")
		body2 := "Issue updated."
		writeIMAP(serverConn, fmt.Sprintf("* 1 FETCH (BODY[TEXT]<0> {%d}\r\n%s)\r\nA0007 OK FETCH completed\r\n", len(body2), body2))
		expectIMAPCommand(t, serverConn, "LOGOUT")
		writeIMAP(serverConn, "* BYE logging out\r\nA0008 OK LOGOUT completed\r\n")
	}()

	client := &IMAPClient{
		address:  "imap.example.test:993",
		username: "agent",
		password: "secret",
		mailbox:  "INBOX",
		useTLS:   false,
		dial: func(context.Context) (net.Conn, error) {
			return clientConn, nil
		},
	}

	resp, err := client.ListMessages(context.Background(), connectors.EmailRequest{
		Query: "approval",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected one filtered result, got %d", len(resp.Results))
	}
	if !resp.Results[0].Unread {
		t.Fatalf("expected unread message")
	}
	if got := resp.Results[0].Subject; got != "Root approval required" {
		t.Fatalf("unexpected subject: %s", got)
	}
}

func expectIMAPCommand(t *testing.T, conn net.Conn, suffix string) {
	t.Helper()
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("server read command: %v", err)
	}
	got := string(buf[:n])
	if !strings.Contains(got, suffix) {
		t.Fatalf("expected command containing %q, got %q", suffix, got)
	}
}

func writeIMAP(conn net.Conn, payload string) {
	_, _ = conn.Write([]byte(payload))
}
