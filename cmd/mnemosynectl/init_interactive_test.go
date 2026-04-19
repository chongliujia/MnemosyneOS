package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestNormalizeProviderChoice(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"1", "deepseek", true},
		{"", "", false},
		{"4", "openai-compatible", true},
		{"openai", "openai", true},
		{"SILICONFLOW", "siliconflow", true},
		{"bogus", "", false},
	}
	for _, tc := range cases {
		got, ok := normalizeProviderChoice(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("normalizeProviderChoice(%q) = (%q,%v) want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRunInitInteractiveFillsMissing(t *testing.T) {
	t.Parallel()
	// Simulated user: choose deepseek (1), enter key, blank model
	// provider "1", API key, blank model, skip workspace, N for unrestricted filesystem
	in := strings.NewReader("1\nmy-secret-key\n\n\n\n")
	var out bytes.Buffer
	opts := initOptions{}
	runInitInteractive(&out, in, &opts, false)
	if opts.Provider != "deepseek" {
		t.Fatalf("provider: %q", opts.Provider)
	}
	if opts.APIKey != "my-secret-key" {
		t.Fatalf("api key not set")
	}
}

func TestInitNeedsInteractivePrompt(t *testing.T) {
	t.Parallel()
	if !initNeedsInteractivePrompt(initOptions{}, false) {
		t.Fatal("empty opts should need prompt")
	}
	if initNeedsInteractivePrompt(initOptions{Provider: "none"}, false) {
		t.Fatal("none should not need prompt")
	}
	if !initNeedsInteractivePrompt(initOptions{Provider: "deepseek"}, false) {
		t.Fatal("missing api key should need prompt")
	}
	if initNeedsInteractivePrompt(initOptions{Provider: "deepseek", APIKey: "x"}, false) {
		t.Fatal("complete deepseek should not need prompt")
	}
	if !initNeedsInteractivePrompt(initOptions{Provider: "openai-compatible", APIKey: "x"}, false) {
		t.Fatal("openai-compatible without base should need prompt")
	}
	if !initNeedsInteractivePrompt(initOptions{Provider: "deepseek", APIKey: "x"}, true) {
		t.Fatal("force wizard should always need prompt")
	}
}
