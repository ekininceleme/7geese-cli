// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newTeamobjectivesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "teamobjectives",
		Short: "Team-level OKRs",
	}

	cmd.AddCommand(newTeamobjectivesGetCmd(flags))
	cmd.AddCommand(newTeamobjectivesListCmd(flags))
	return cmd
}
