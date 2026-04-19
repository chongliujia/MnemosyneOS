package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	"mnemosyneos/internal/chat"
	"mnemosyneos/internal/console"
)

type chatAPI interface {
	SendChat(req chat.SendRequest) (chat.SendResponse, error)
	ChatMessages(sessionID string, limit int) ([]chat.Message, error)
}

func friendlyConsoleAPIError(endpoint string, err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Sprintf("Cannot reach the MnemosyneOS API at %s (connection refused).\n\n"+
			"The interactive CLI calls the local HTTP server. Start it in another terminal:\n"+
			"  mnemosynectl serve\n"+
			"or in the background:\n"+
			"  mnemosynectl start\n\n"+
			"Then run chat again from this directory (so .env.local or MNEMOSYNE_DOTENV_PATH applies).",
			endpoint)
	}
	return err.Error()
}

type askOptions struct {
	SessionID        string
	ExecutionProfile string
	JSON             bool
	Message          string
}

type chatOptions struct {
	SessionID        string
	ExecutionProfile string
	History          int
}

func handleAsk(client *console.Client, args []string) {
	fs := flag.NewFlagSet("ask", flag.ExitOnError)
	sessionID := fs.String("session", "default", "chat session id")
	executionProfile := fs.String("execution-profile", "", "execution profile for task-capable turns")
	jsonOutput := fs.Bool("json", false, "print raw JSON response")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	message := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if message == "" {
		log.Fatal("ask requires a message")
	}
	if err := runAsk(client, askOptions{
		SessionID:        *sessionID,
		ExecutionProfile: *executionProfile,
		JSON:             *jsonOutput,
		Message:          message,
	}, osStdout); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", friendlyConsoleAPIError(client.Endpoint(), err))
		os.Exit(1)
	}
}

func handleChat(client *console.Client, args []string) {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	sessionID := fs.String("session", "default", "chat session id")
	executionProfile := fs.String("execution-profile", "", "execution profile for task-capable turns")
	history := fs.Int("history", 8, "recent messages to show on startup")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	if err := runChat(client, chatOptions{
		SessionID:        *sessionID,
		ExecutionProfile: *executionProfile,
		History:          *history,
	}, osStdin, osStdout); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", friendlyConsoleAPIError(client.Endpoint(), err))
		os.Exit(1)
	}
}

var (
	osStdin  io.Reader = os.Stdin
	osStdout io.Writer = os.Stdout
)

// ---------------------------------------------------------------------------
// TTY detection — enables colors, spinner, and streaming only in real terminals
// ---------------------------------------------------------------------------

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// ---------------------------------------------------------------------------
// ask (single turn)
// ---------------------------------------------------------------------------

func runAsk(client chatAPI, opts askOptions, out io.Writer) error {
	tty := isTTY(out)

	if tty {
		fmt.Fprintf(out, "\n  %s%syou%s  %s\n\n", cBold, cGreen, cReset, strings.TrimSpace(opts.Message))
	}

	spin := newSpinner(out, tty)
	spin.Start("Thinking")

	resp, err := client.SendChat(chat.SendRequest{
		SessionID:        strings.TrimSpace(opts.SessionID),
		Message:          strings.TrimSpace(opts.Message),
		RequestedBy:      "mnemosynectl",
		Source:           "console",
		ExecutionProfile: strings.TrimSpace(opts.ExecutionProfile),
	})
	spin.Stop()

	if err != nil {
		return err
	}
	if opts.JSON {
		writeJSONTo(out, resp)
		return nil
	}
	renderAssistantReply(out, resp.AssistantMessage, tty)
	return nil
}

// ---------------------------------------------------------------------------
// chat (interactive REPL)
// ---------------------------------------------------------------------------

func runChat(client chatAPI, opts chatOptions, in io.Reader, out io.Writer) error {
	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID == "" {
		sessionID = "default"
	}
	tty := isTTY(out)

	if tty {
		printChatBanner(out, sessionID)
	} else {
		fmt.Fprintf(out, "MnemosyneOS CLI chat\nsession: %s\ncommands: /help, /history, /quit\n\n", sessionID)
	}

	if opts.History > 0 {
		if messages, err := client.ChatMessages(sessionID, opts.History); err == nil && len(messages) > 0 {
			if tty {
				fmt.Fprintf(out, "  %s── recent ──%s\n\n", cDim, cReset)
			} else {
				fmt.Fprintln(out, "recent:")
			}
			renderMessages(out, messages, tty)
			fmt.Fprintln(out)
		}
	}

	scanner := bufio.NewScanner(in)
	for {
		if tty {
			fmt.Fprintf(out, "  %s%s❯%s ", cBold, cGreen, cReset)
		} else {
			fmt.Fprintf(out, "%s> ", sessionID)
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			fmt.Fprintln(out)
			return nil
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		switch line {
		case "/quit", "/exit":
			if tty {
				fmt.Fprintf(out, "\n  %sbye%s\n\n", cDim, cReset)
			} else {
				fmt.Fprintln(out, "bye")
			}
			return nil
		case "/help":
			printChatHelp(out, tty)
			continue
		case "/history":
			messages, err := client.ChatMessages(sessionID, maxInt(opts.History, 8))
			if err != nil {
				return err
			}
			if len(messages) == 0 {
				fmt.Fprintln(out, "(no messages)")
				continue
			}
			renderMessages(out, messages, tty)
			continue
		case "/clear":
			if tty {
				fmt.Fprint(out, "\033[2J\033[H")
				printChatBanner(out, sessionID)
			}
			continue
		}

		if tty {
			fmt.Fprintf(out, "\n  %s%syou%s  %s\n\n", cBold, cGreen, cReset, line)
		}

		spin := newSpinner(out, tty)
		spin.Start("Thinking")

		resp, err := client.SendChat(chat.SendRequest{
			SessionID:        sessionID,
			Message:          line,
			RequestedBy:      "mnemosynectl",
			Source:           "console",
			ExecutionProfile: strings.TrimSpace(opts.ExecutionProfile),
		})
		spin.Stop()

		if err != nil {
			return err
		}
		renderAssistantReply(out, resp.AssistantMessage, tty)
	}
}

// ---------------------------------------------------------------------------
// Banner & help
// ---------------------------------------------------------------------------

func printChatBanner(out io.Writer, sessionID string) {
	fmt.Fprintf(out, "\n")
	fmt.Fprintf(out, "  %s%sMnemosyneOS%s  %sv%s%s\n", cBold, cCyan, cReset, cDim, version, cReset)
	fmt.Fprintf(out, "  %ssession: %s%s\n", cDim, sessionID, cReset)
	fmt.Fprintf(out, "  %s/help  /history  /clear  /quit%s\n\n", cDim, cReset)
}

func printChatHelp(out io.Writer, tty bool) {
	if tty {
		fmt.Fprintf(out, "\n  %sCommands%s\n", cBold, cReset)
		fmt.Fprintf(out, "  %s/help%s     show this help\n", cCyan, cReset)
		fmt.Fprintf(out, "  %s/history%s  show recent messages\n", cCyan, cReset)
		fmt.Fprintf(out, "  %s/clear%s    clear screen\n", cCyan, cReset)
		fmt.Fprintf(out, "  %s/quit%s     exit chat\n\n", cCyan, cReset)
	} else {
		fmt.Fprintln(out, "/help    show commands")
		fmt.Fprintln(out, "/history show recent messages")
		fmt.Fprintln(out, "/quit    exit chat")
	}
}

// ---------------------------------------------------------------------------
// Message rendering
// ---------------------------------------------------------------------------

func renderMessages(out io.Writer, messages []chat.Message, tty bool) {
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "assistant"
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			content = "(empty)"
		}
		if tty {
			if role == "user" {
				fmt.Fprintf(out, "  %s%syou%s  %s\n", cBold, cGreen, cReset, truncateTerminal(content, 100))
			} else {
				fmt.Fprintf(out, "  %s%sai%s   %s\n", cBold, cYellow, cReset, truncateTerminal(content, 100))
			}
		} else {
			fmt.Fprintf(out, "%s: %s\n", role, content)
		}
	}
}

func renderAssistantReply(out io.Writer, message chat.Message, tty bool) {
	content := strings.TrimSpace(message.Content)
	if content == "" {
		content = "(empty assistant reply)"
	}

	// Show the tool trace BEFORE the final reply. This gives the user a
	// receipt of what the agent actually did ("used search_files → 3 matches")
	// so they can trust or push back on the natural-language answer.
	renderToolTraceCLI(out, message.ToolTrace, tty)

	if tty {
		rendered := renderTerminalMarkdown(content)
		streamToTerminal(out, rendered)
		fmt.Fprintln(out)
	} else {
		fmt.Fprintln(out, content)
	}

	if strings.TrimSpace(message.TaskID) != "" {
		if tty {
			fmt.Fprintf(out, "  %s[task]%s %s", cDim, cReset, message.TaskID)
		} else {
			fmt.Fprintf(out, "\n[task] %s", message.TaskID)
		}
		if state := strings.TrimSpace(message.TaskState); state != "" {
			fmt.Fprintf(out, " state=%s", state)
		}
		if skill := strings.TrimSpace(message.SelectedSkill); skill != "" {
			fmt.Fprintf(out, " skill=%s", skill)
		}
		fmt.Fprintln(out)
	}

	if len(message.Actions) > 0 {
		if tty {
			fmt.Fprintf(out, "\n  %sActions%s\n", cBold, cReset)
			for _, action := range message.Actions {
				fmt.Fprintf(out, "  %s•%s %s %s(%s %s)%s\n", cCyan, cReset, action.Label, cDim, firstNonEmpty(action.Method, "get"), action.Href, cReset)
			}
		} else {
			fmt.Fprintln(out, "actions:")
			for _, action := range message.Actions {
				fmt.Fprintf(out, "- %s (%s %s)\n", action.Label, firstNonEmpty(action.Method, "get"), action.Href)
			}
		}
	}
	fmt.Fprintln(out)
}

// renderToolTraceCLI prints a compact receipt of every tool the agent loop
// invoked on behalf of this turn. It's intentionally visually distinct from
// the final reply (dim prefix, indented) so the reader knows it's metadata.
// Args are trimmed to a preview — the full JSON is kept server-side.
func renderToolTraceCLI(out io.Writer, trace []chat.ToolTraceEntry, tty bool) {
	if len(trace) == 0 {
		return
	}
	if tty {
		fmt.Fprintf(out, "\n  %s⎯ used tools%s\n", cDim, cReset)
		for _, entry := range trace {
			name := entry.ToolName
			if name == "" {
				name = "(tool)"
			}
			args := compactToolArgs(entry.Arguments)
			head := fmt.Sprintf("  %s·%s %s%s%s", cDim, cReset, cCyan, name, cReset)
			if args != "" {
				head = fmt.Sprintf("%s %s(%s)%s", head, cDim, args, cReset)
			}
			fmt.Fprintln(out, head)
			if strings.TrimSpace(entry.Error) != "" {
				fmt.Fprintf(out, "    %s✗ %s%s\n", cRed, entry.Error, cReset)
			} else if preview := strings.TrimSpace(entry.ResultPreview); preview != "" {
				fmt.Fprintf(out, "    %s→%s %s\n", cDim, cReset, truncateTerminal(preview, 120))
			}
		}
		fmt.Fprintln(out)
		return
	}
	fmt.Fprintln(out, "tools:")
	for _, entry := range trace {
		name := entry.ToolName
		if name == "" {
			name = "(tool)"
		}
		args := compactToolArgs(entry.Arguments)
		if args != "" {
			fmt.Fprintf(out, "- %s(%s)\n", name, args)
		} else {
			fmt.Fprintf(out, "- %s\n", name)
		}
		if strings.TrimSpace(entry.Error) != "" {
			fmt.Fprintf(out, "  error: %s\n", entry.Error)
		} else if preview := strings.TrimSpace(entry.ResultPreview); preview != "" {
			fmt.Fprintf(out, "  → %s\n", truncateTerminal(preview, 120))
		}
	}
}

// compactToolArgs turns `{"pattern":"lab","directory":"~"}` into
// `pattern="lab", directory="~"` for a readable one-liner in the receipt.
// Falls back to the raw string if decoding fails.
func compactToolArgs(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		if len(raw) > 80 {
			return raw[:79] + "…"
		}
		return raw
	}
	parts := make([]string, 0, len(parsed))
	for key, value := range parsed {
		if key == "_locale" {
			continue
		}
		switch v := value.(type) {
		case string:
			parts = append(parts, fmt.Sprintf("%s=%q", key, v))
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", key, v))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	joined := strings.Join(parts, ", ")
	if len(joined) > 120 {
		joined = joined[:119] + "…"
	}
	return joined
}

// ---------------------------------------------------------------------------
// Terminal markdown rendering
// ---------------------------------------------------------------------------

var (
	termBoldRe       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	termInlineCodeRe = regexp.MustCompile("`([^`]+)`")
)

func renderTerminalMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	var out strings.Builder
	inCode := false
	indent := "  "

	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)

		if inCode {
			if trimmed == "```" {
				out.WriteString(indent + cDim + "  └" + strings.Repeat("─", 50) + cReset + "\n")
				inCode = false
				continue
			}
			out.WriteString(indent + cDim + "  │ " + cReset + "\033[38;5;252m" + raw + cReset + "\n")
			continue
		}

		if strings.HasPrefix(trimmed, "```") {
			lang := strings.TrimPrefix(trimmed, "```")
			label := ""
			if lang != "" {
				label = " " + lang + " "
			}
			out.WriteString(indent + cDim + "  ┌" + strings.Repeat("─", 4) + label + strings.Repeat("─", maxInt(46-len(label), 4)) + cReset + "\n")
			inCode = true
			continue
		}

		if trimmed == "" {
			if i > 0 {
				out.WriteString("\n")
			}
			continue
		}

		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			out.WriteString(indent + cDim + "  " + strings.Repeat("─", 50) + cReset + "\n")
			continue
		}

		if strings.HasPrefix(trimmed, "### ") {
			heading := strings.TrimPrefix(trimmed, "### ")
			out.WriteString(indent + cBold + heading + cReset + "\n")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimPrefix(trimmed, "## ")
			out.WriteString(indent + cBold + cYellow + heading + cReset + "\n")
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			heading := strings.TrimPrefix(trimmed, "# ")
			out.WriteString(indent + cBold + cCyan + heading + cReset + "\n")
			continue
		}

		if strings.HasPrefix(trimmed, "> ") {
			quote := strings.TrimPrefix(trimmed, "> ")
			out.WriteString(indent + cDim + "  │ " + cReset + renderTerminalInline(quote) + "\n")
			continue
		}

		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "• ") {
			item := strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "• ")
			out.WriteString(indent + cDim + "  • " + cReset + renderTerminalInline(item) + "\n")
			continue
		}

		if isNumberedItem(trimmed) {
			out.WriteString(indent + "  " + renderTerminalInline(trimmed) + "\n")
			continue
		}

		out.WriteString(indent + renderTerminalInline(trimmed) + "\n")
	}

	if inCode {
		out.WriteString(indent + cDim + "  └" + strings.Repeat("─", 50) + cReset + "\n")
	}

	return out.String()
}

func renderTerminalInline(text string) string {
	text = termBoldRe.ReplaceAllString(text, cBold+"$1"+cReset)
	text = termInlineCodeRe.ReplaceAllString(text, cCyan+"`"+"$1"+"`"+cReset)
	return text
}

func isNumberedItem(line string) bool {
	for i := 0; i < len(line) && i < 4; i++ {
		if line[i] == '.' && i > 0 {
			return i+1 < len(line) && line[i+1] == ' '
		}
		if line[i] < '0' || line[i] > '9' {
			return false
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Streaming output — prints characters gradually for TTY feel
// ---------------------------------------------------------------------------

func streamToTerminal(out io.Writer, rendered string) {
	charDelay := 6 * time.Millisecond
	lineCount := 0
	for _, r := range rendered {
		fmt.Fprintf(out, "%c", r)
		if r == '\n' {
			lineCount++
		}
		if r == '\033' || lineCount > 80 {
			continue
		}
		time.Sleep(charDelay)
	}
}

// ---------------------------------------------------------------------------
// Spinner — braille animation while waiting for API response
// ---------------------------------------------------------------------------

type spinner struct {
	writer  io.Writer
	tty     bool
	stopCh  chan struct{}
	doneCh  chan struct{}
	running bool
}

func newSpinner(w io.Writer, tty bool) *spinner {
	return &spinner{writer: w, tty: tty, stopCh: make(chan struct{}), doneCh: make(chan struct{})}
}

func (s *spinner) Start(label string) {
	if !s.tty {
		close(s.doneCh)
		return
	}
	s.running = true
	go func() {
		defer close(s.doneCh)
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		i := 0
		for {
			select {
			case <-s.stopCh:
				fmt.Fprintf(s.writer, "\r\033[K")
				return
			default:
				fmt.Fprintf(s.writer, "\r  %s%s %s%s%s", cCyan, frames[i%len(frames)], cDim, label, cReset)
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
}

func (s *spinner) Stop() {
	if s.running {
		close(s.stopCh)
	}
	<-s.doneCh
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func truncateTerminal(text string, max int) string {
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func writeJSONTo(out io.Writer, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Fprintln(out, string(data))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
