// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newRecognitionbadgesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recognitionbadges",
		Short: "Recognition and kudos sent between users",
	}

	cmd.AddCommand(newRecognitionbadgesCreateCmd(flags))
	cmd.AddCommand(newRecognitionbadgesGetCmd(flags))
	cmd.AddCommand(newRecognitionbadgesListCmd(flags))
	return cmd
}
