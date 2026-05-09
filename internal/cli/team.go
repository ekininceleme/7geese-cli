// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newTeamCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Teams in the organization",
	}

	cmd.AddCommand(newTeamGetCmd(flags))
	cmd.AddCommand(newTeamListCmd(flags))
	return cmd
}
