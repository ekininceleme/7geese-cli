// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newUserCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Users in the organization",
	}

	cmd.AddCommand(newUserGetCmd(flags))
	cmd.AddCommand(newUserListCmd(flags))
	return cmd
}
