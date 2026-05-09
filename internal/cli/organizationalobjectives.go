// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newOrganizationalobjectivesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "organizationalobjectives",
		Short: "Company-wide OKRs",
	}

	cmd.AddCommand(newOrganizationalobjectivesGetCmd(flags))
	cmd.AddCommand(newOrganizationalobjectivesListCmd(flags))
	return cmd
}
