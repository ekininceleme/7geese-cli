// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newOneononesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "oneonones",
		Short: "One-on-one meetings between manager and report",
	}

	cmd.AddCommand(newOneononesGetCmd(flags))
	cmd.AddCommand(newOneononesListCmd(flags))
	cmd.AddCommand(newOneononesHistoryCmd(flags))
	cmd.AddCommand(newOneononesNotesCmd(flags))
	cmd.AddCommand(newOneononesSearchCmd(flags))
	return cmd
}
