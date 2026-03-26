package emailconnector

import (
	"os"
	"strings"

	"mnemosyneos/internal/connectors"
)

func NewClientFromEnv() (connectors.EmailConnector, error) {
	provider := strings.TrimSpace(os.Getenv("MNEMOSYNE_EMAIL_PROVIDER"))
	switch provider {
	case "", "auto":
		if strings.TrimSpace(os.Getenv("MNEMOSYNE_IMAP_HOST")) != "" {
			return NewIMAPFromEnv()
		}
		if strings.TrimSpace(os.Getenv("MNEMOSYNE_MAILDIR_ROOT")) != "" {
			return NewMaildirFromEnv()
		}
		return nil, nil
	case "maildir":
		return NewMaildirFromEnv()
	case "imap":
		return NewIMAPFromEnv()
	default:
		return nil, nil
	}
}
