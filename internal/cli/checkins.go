// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newCheckinsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkins",
		Short: "Weekly check-ins on goals and progress",
	}

	cmd.AddCommand(newCheckinsCreateCmd(flags))
	cmd.AddCommand(newCheckinsGetCmd(flags))
	cmd.AddCommand(newCheckinsListCmd(flags))
	cmd.AddCommand(newCheckinsStreakCmd(flags))
	return cmd
}
