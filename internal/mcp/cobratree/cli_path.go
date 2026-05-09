// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cobratree

import (
	"os"
	"os/exec"
	"path/filepath"
)

// SiblingCLIPath resolves the companion CLI via sibling-of-executable,
// API_7GEESE_CLI_PATH env var, then PATH.
func SiblingCLIPath() (string, error) {
	const cliName = "7geese-cli"
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), cliName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	if v := os.Getenv("API_7GEESE_CLI_PATH"); v != "" {
		return v, nil
	}
	return exec.LookPath(cliName)
}
