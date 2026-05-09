// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"github.com/spf13/cobra"
)

func newFeedbackrequestCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feedbackrequest",
		Short: "Feedback requests sent to peers",
	}

	cmd.AddCommand(newFeedbackrequestCreateCmd(flags))
	cmd.AddCommand(newFeedbackrequestListCmd(flags))
	return cmd
}
