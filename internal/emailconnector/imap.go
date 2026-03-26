package emailconnector

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/textproto"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"mnemosyneos/internal/connectors"
)

type IMAPClient struct {
	address  string
	username string
	password string
	mailbox  string
	useTLS   bool
	dial     func(ctx context.Context) (net.Conn, error)
}

func NewIMAPFromEnv() (*IMAPClient, error) {
	host := strings.TrimSpace(os.Getenv("MNEMOSYNE_IMAP_HOST"))
	if host == "" {
		return nil, nil
	}
	port := strings.TrimSpace(os.Getenv("MNEMOSYNE_IMAP_PORT"))
	if port == "" {
		port = "993"
	}
	user := strings.TrimSpace(os.Getenv("MNEMOSYNE_IMAP_USERNAME"))
	pass := os.Getenv("MNEMOSYNE_IMAP_PASSWORD")
	if user == "" || pass == "" {
		return nil, errors.New("imap username and password are required")
	}
	mailbox := strings.TrimSpace(os.Getenv("MNEMOSYNE_IMAP_MAILBOX"))
	if mailbox == "" {
		mailbox = "INBOX"
	}
	useTLS := true
	if raw := strings.TrimSpace(os.Getenv("MNEMOSYNE_IMAP_TLS")); raw != "" {
		useTLS = raw != "0" && !strings.EqualFold(raw, "false")
	}
	address := net.JoinHostPort(host, port)
	client := &IMAPClient{
		address:  address,
		username: user,
		password: pass,
		mailbox:  mailbox,
		useTLS:   useTLS,
	}
	client.dial = client.defaultDial
	return client, nil
}

func (c *IMAPClient) ListMessages(ctx context.Context, req connectors.EmailRequest) (connectors.EmailResponse, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return connectors.EmailResponse{}, err
	}
	defer conn.Close()

	client := newIMAPSession(conn)
	if err := client.readGreeting(); err != nil {
		return connectors.EmailResponse{}, err
	}
	if err := client.login(c.username, c.password); err != nil {
		return connectors.EmailResponse{}, err
	}
	defer client.logout()

	if err := client.selectMailbox(c.mailbox); err != nil {
		return connectors.EmailResponse{}, err
	}
	ids, err := client.searchAll()
	if err != nil {
		return connectors.EmailResponse{}, err
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	resp := connectors.EmailResponse{
		Query:    req.Query,
		Provider: "imap",
		Results:  make([]connectors.EmailMessage, 0, limit),
	}
	for _, id := range ids {
		if len(resp.Results) >= limit {
			break
		}
		msg, err := client.fetchMessage(id, c.mailbox)
		if err != nil {
			continue
		}
		if query != "" && !matchesEmailQuery(msg, query) {
			continue
		}
		resp.Results = append(resp.Results, msg)
	}
	return resp, nil
}

func (c *IMAPClient) defaultDial(ctx context.Context) (net.Conn, error) {
	dialer := &net.Dialer{}
	if c.useTLS {
		return tls.DialWithDialer(dialer, "tcp", c.address, &tls.Config{
			ServerName: serverNameFromAddress(c.address),
			MinVersion: tls.VersionTLS12,
		})
	}
	return dialer.DialContext(ctx, "tcp", c.address)
}

type imapSession struct {
	conn   net.Conn
	reader *textproto.Reader
	writer *textproto.Writer
	tag    int
}

func newIMAPSession(conn net.Conn) *imapSession {
	br := bufio.NewReader(conn)
	bw := bufio.NewWriter(conn)
	return &imapSession{
		conn:   conn,
		reader: textproto.NewReader(br),
		writer: textproto.NewWriter(bw),
		tag:    1,
	}
}

func (s *imapSession) readGreeting() error {
	line, err := s.reader.ReadLine()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, "* OK") {
		return fmt.Errorf("unexpected imap greeting: %s", line)
	}
	return nil
}

func (s *imapSession) login(username, password string) error {
	_, err := s.runSimpleCommand("LOGIN " + quoteIMAP(username) + " " + quoteIMAP(password))
	return err
}

func (s *imapSession) selectMailbox(mailbox string) error {
	_, err := s.runSimpleCommand("SELECT " + quoteIMAP(mailbox))
	return err
}

func (s *imapSession) logout() {
	_, _ = s.runSimpleCommand("LOGOUT")
}

func (s *imapSession) searchAll() ([]int, error) {
	lines, err := s.runSimpleCommand("SEARCH ALL")
	if err != nil {
		return nil, err
	}
	out := make([]int, 0)
	for _, line := range lines {
		if !strings.HasPrefix(line, "* SEARCH") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "* SEARCH"))
		for _, field := range fields {
			id, err := strconv.Atoi(strings.TrimSpace(field))
			if err == nil {
				out = append(out, id)
			}
		}
	}
	return out, nil
}

func (s *imapSession) fetchMessage(id int, mailbox string) (connectors.EmailMessage, error) {
	headerLiteral, flags, err := s.fetchLiteral(fmt.Sprintf("FETCH %d (FLAGS BODY.PEEK[HEADER.FIELDS (MESSAGE-ID SUBJECT FROM DATE)])", id))
	if err != nil {
		return connectors.EmailMessage{}, err
	}
	bodyLiteral, _, err := s.fetchLiteral(fmt.Sprintf("FETCH %d (BODY.PEEK[TEXT]<0.240>)", id))
	if err != nil {
		return connectors.EmailMessage{}, err
	}

	msg, err := mail.ReadMessage(bytes.NewReader(headerLiteral))
	if err != nil {
		return connectors.EmailMessage{}, err
	}
	dateText := strings.TrimSpace(msg.Header.Get("Date"))
	dateValue := dateText
	if parsed, err := mail.ParseDate(dateText); err == nil {
		dateValue = parsed.UTC().Format(time.RFC3339)
	}
	return connectors.EmailMessage{
		MessageID: strings.TrimSpace(msg.Header.Get("Message-Id")),
		Mailbox:   mailbox,
		From:      decodeHeader(msg.Header.Get("From")),
		Subject:   decodeHeader(msg.Header.Get("Subject")),
		Date:      dateValue,
		Snippet:   previewMailBody(string(bodyLiteral), 240),
		Path:      fmt.Sprintf("imap://%s/%s/%d", s.conn.RemoteAddr().String(), mailbox, id),
		Unread:    !strings.Contains(flags, `\Seen`),
	}, nil
}

func (s *imapSession) fetchLiteral(command string) ([]byte, string, error) {
	tag := s.nextTag()
	if err := s.writer.PrintfLine("%s %s", tag, command); err != nil {
		return nil, "", err
	}

	firstLine, err := s.reader.ReadLine()
	if err != nil {
		return nil, "", err
	}
	size, err := extractLiteralSize(firstLine)
	if err != nil {
		return nil, "", err
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(s.reader.R, buf); err != nil {
		return nil, "", err
	}

	var trailer string
	for {
		line, err := s.reader.ReadLine()
		if err != nil {
			return nil, "", err
		}
		if strings.HasPrefix(line, tag+" ") {
			if !strings.Contains(line, "OK") {
				return nil, "", fmt.Errorf("imap command failed: %s", line)
			}
			break
		}
		trailer = trailer + " " + line
	}
	return buf, firstLine + trailer, nil
}

func (s *imapSession) runSimpleCommand(command string) ([]string, error) {
	tag := s.nextTag()
	if err := s.writer.PrintfLine("%s %s", tag, command); err != nil {
		return nil, err
	}
	lines := make([]string, 0)
	for {
		line, err := s.reader.ReadLine()
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(line, tag+" ") {
			if !strings.Contains(line, "OK") {
				return nil, fmt.Errorf("imap command failed: %s", line)
			}
			return lines, nil
		}
		lines = append(lines, line)
	}
}

func (s *imapSession) nextTag() string {
	tag := fmt.Sprintf("A%04d", s.tag)
	s.tag++
	return tag
}

func quoteIMAP(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}

func extractLiteralSize(line string) (int, error) {
	start := strings.LastIndex(line, "{")
	end := strings.LastIndex(line, "}")
	if start == -1 || end == -1 || end <= start+1 {
		return 0, fmt.Errorf("imap literal not found in line: %s", line)
	}
	return strconv.Atoi(line[start+1 : end])
}

func serverNameFromAddress(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	return host
}
