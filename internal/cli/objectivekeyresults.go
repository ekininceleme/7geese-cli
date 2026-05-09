// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newObjectivekeyresultsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "objectivekeyresults",
		Short: "Key results belonging to objectives",
	}

	cmd.AddCommand(newObjectivekeyresultsCreateCmd(flags))
	cmd.AddCommand(newObjectivekeyresultsGetCmd(flags))
	cmd.AddCommand(newObjectivekeyresultsListCmd(flags))
	cmd.AddCommand(newObjectivekeyresultsUpdateCmd(flags))
	return cmd
}
