// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newUserprofileCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "userprofile",
		Short: "Extended user profile with role and manager info",
	}

	cmd.AddCommand(newUserprofileGetCmd(flags))
	cmd.AddCommand(newUserprofileListCmd(flags))
	return cmd
}
