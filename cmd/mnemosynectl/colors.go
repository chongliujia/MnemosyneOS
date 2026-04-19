package main

// ANSI styling for terminal UX (daemon + chat CLI).
const (
	cBold   = "\x1b[1m"
	cDim    = "\x1b[2m"
	cReset  = "\x1b[0m"
	cRed    = "\x1b[31m"
	cGreen  = "\x1b[32m"
	cYellow = "\x1b[33m"
	cCyan   = "\x1b[36m"
)

// version is embedded in daemon status output; override via -ldflags if needed.
const version = "0.1.0"
