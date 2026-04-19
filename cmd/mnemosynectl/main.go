package main

import (
	"fmt"
	"os"

	"mnemosyneos/internal/console"
)

func main() {
	_ = loadEnv()

	if len(os.Args) < 2 {
		handleChat(console.NewClient(resolveAPIBase("")), []string{})
		return
	}

	switch os.Args[1] {
	case "help", "-h", "--help":
		printRootHelp()
	case "init":
		cmdInitCLI(os.Args[2:])
	case "doctor":
		cmdDoctorCLI(os.Args[2:])
	case "serve":
		cmdServeCLI(os.Args[2:])
	case "start":
		cmdStart(os.Args[2:])
	case "stop":
		cmdStop(os.Args[2:])
	case "restart":
		cmdRestart(os.Args[2:])
	case "logs":
		cmdLogs(os.Args[2:])
	case "chat":
		handleChat(console.NewClient(resolveAPIBase("")), os.Args[2:])
	case "ask":
		handleAsk(console.NewClient(resolveAPIBase("")), os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		printRootHelp()
		os.Exit(2)
	}
}
