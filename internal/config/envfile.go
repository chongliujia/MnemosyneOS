package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadDefaultLocalEnv loads KEY=VALUE pairs from the env file used by the CLI
// and server. If MNEMOSYNE_DOTENV_PATH is set, that file is used; otherwise ".env.local".
// Variables already present in the process environment are not overwritten.
func LoadDefaultLocalEnv() error {
	path := strings.TrimSpace(os.Getenv("MNEMOSYNE_DOTENV_PATH"))
	if path == "" {
		path = ".env.local"
	}
	return LoadEnvFile(path)
}

// LoadEnvFile reads simple KEY=VALUE pairs from a local env file if present.
// Existing environment variables are preserved.
func LoadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}
