package main

import (
	"fmt"
	"os"

	"mnemosyneos/internal/appcli"
)

// mnemosyne-api starts the HTTP server with the web UI disabled (REST API only).
func main() {
	args := append([]string{"--ui", "api"}, os.Args[1:]...)
	if err := appcli.RunServe(args); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
