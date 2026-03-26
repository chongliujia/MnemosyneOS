package emailconnector

import (
	"context"
	"io/fs"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mnemosyneos/internal/connectors"
)

type MaildirClient struct {
	rootDir string
	mailbox string
}

func NewMaildirFromEnv() (*MaildirClient, error) {
	root := strings.TrimSpace(os.Getenv("MNEMOSYNE_MAILDIR_ROOT"))
	if root == "" {
		return nil, nil
	}
	mailbox := strings.TrimSpace(os.Getenv("MNEMOSYNE_MAILDIR_MAILBOX"))
	if mailbox == "" {
		mailbox = "INBOX"
	}
	return &MaildirClient{
		rootDir: root,
		mailbox: mailbox,
	}, nil
}

func (c *MaildirClient) ListMessages(ctx context.Context, req connectors.EmailRequest) (connectors.EmailResponse, error) {
	mailboxDir := filepath.Join(c.rootDir, c.mailbox)
	entries := make([]maildirEntry, 0)
	for _, bucket := range []string{"new", "cur"} {
		dir := filepath.Join(mailboxDir, bucket)
		items, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return connectors.EmailResponse{}, err
		}
		for _, item := range items {
			select {
			case <-ctx.Done():
				return connectors.EmailResponse{}, ctx.Err()
			default:
			}
			if item.IsDir() {
				continue
			}
			info, err := item.Info()
			if err != nil {
				return connectors.EmailResponse{}, err
			}
			entries = append(entries, maildirEntry{
				path:    filepath.Join(dir, item.Name()),
				unread:  bucket == "new" || !strings.Contains(item.Name(), "S"),
				modTime: info.ModTime(),
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime.After(entries[j].modTime)
	})

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	resp := connectors.EmailResponse{
		Query:    req.Query,
		Provider: "maildir",
		Results:  make([]connectors.EmailMessage, 0, limit),
	}
	for _, entry := range entries {
		if len(resp.Results) >= limit {
			break
		}
		message, err := parseMaildirMessage(entry.path, c.mailbox, entry.unread)
		if err != nil {
			continue
		}
		if query != "" && !matchesEmailQuery(message, query) {
			continue
		}
		resp.Results = append(resp.Results, message)
	}
	return resp, nil
}

type maildirEntry struct {
	path    string
	unread  bool
	modTime time.Time
}

func parseMaildirMessage(path, mailbox string, unread bool) (connectors.EmailMessage, error) {
	file, err := os.Open(path)
	if err != nil {
		return connectors.EmailMessage{}, err
	}
	defer file.Close()

	msg, err := mail.ReadMessage(file)
	if err != nil {
		return connectors.EmailMessage{}, err
	}
	body, err := fs.ReadFile(os.DirFS(filepath.Dir(path)), filepath.Base(path))
	if err != nil {
		return connectors.EmailMessage{}, err
	}
	subject := decodeHeader(msg.Header.Get("Subject"))
	from := decodeHeader(msg.Header.Get("From"))
	dateText := strings.TrimSpace(msg.Header.Get("Date"))
	dateValue := dateText
	if parsed, err := mail.ParseDate(dateText); err == nil {
		dateValue = parsed.UTC().Format(time.RFC3339)
	}

	return connectors.EmailMessage{
		MessageID: strings.TrimSpace(msg.Header.Get("Message-Id")),
		Mailbox:   mailbox,
		From:      from,
		Subject:   subject,
		Date:      dateValue,
		Snippet:   previewMailBody(string(body), 240),
		Path:      path,
		Unread:    unread,
	}, nil
}

func decodeHeader(value string) string {
	decoder := new(mime.WordDecoder)
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(decoded)
}

func previewMailBody(body string, max int) string {
	sep := "\n\n"
	if idx := strings.Index(body, sep); idx >= 0 {
		body = body[idx+len(sep):]
	}
	body = strings.TrimSpace(body)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", " ")
	if len(body) <= max {
		return body
	}
	if max <= 3 {
		return body[:max]
	}
	return body[:max-3] + "..."
}

func matchesEmailQuery(message connectors.EmailMessage, query string) bool {
	text := strings.ToLower(strings.Join([]string{
		message.Subject,
		message.From,
		message.Snippet,
	}, "\n"))
	return strings.Contains(text, query)
}
