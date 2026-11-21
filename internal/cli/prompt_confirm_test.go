// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPromptConfirm(t *testing.T) {
	isolateSyncEnvironment(t)
	cases := []struct {
		name     string
		yes      bool
		noInput  bool
		terminal bool
		input    string
		want     bool
	}{
		{name: "yes without a terminal", yes: true, want: true},
		{name: "yes with no-input", yes: true, noInput: true, want: true},
		{name: "no terminal", input: "y\n", want: false},
		{name: "no-input on a terminal", noInput: true, terminal: true, input: "y\n", want: false},
		{name: "enter", terminal: true, input: "\n", want: true},
		{name: "y", terminal: true, input: "y\n", want: true},
		{name: "YES", terminal: true, input: "YES\n", want: true},
		{name: "n", terminal: true, input: "n\n", want: false},
		{name: "other text", terminal: true, input: "maybe\n", want: false},
		{name: "end of input", terminal: true, input: "", want: false},
		{name: "y without a line end", terminal: true, input: "y", want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdinIsTerminal = func() bool { return c.terminal }
			sharedStdin = bufio.NewReader(strings.NewReader(c.input))
			cmd := &cobra.Command{}
			var errOut bytes.Buffer
			cmd.SetErr(&errOut)
			got := promptConfirm(cmd, &rootFlags{yes: c.yes, noInput: c.noInput}, "Continue? ")
			if got != c.want {
				t.Errorf("promptConfirm = %v, want %v", got, c.want)
			}
		})
	}
}

func TestTruncate_KeepsCharactersWhole(t *testing.T) {
	if got := truncate("ééééééé", 5); got != "éé..." {
		t.Errorf("truncate = %q, want %q", got, "éé...")
	}
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate = %q, want short", got)
	}
}
