// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newPerformancecyclesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "performancecycles",
		Short: "Performance review cycles",
	}

	cmd.AddCommand(newPerformancecyclesGetCmd(flags))
	cmd.AddCommand(newPerformancecyclesListCmd(flags))
	return cmd
}
