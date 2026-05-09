// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newObjectivesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "objectives",
		Short: "Personal OKRs and goals",
	}

	cmd.AddCommand(newObjectivesCreateCmd(flags))
	cmd.AddCommand(newObjectivesDeleteCmd(flags))
	cmd.AddCommand(newObjectivesGetCmd(flags))
	cmd.AddCommand(newObjectivesListCmd(flags))
	cmd.AddCommand(newObjectivesUpdateCmd(flags))
	cmd.AddCommand(newObjectivesStaleCmd(flags))
	return cmd
}
