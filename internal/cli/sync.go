// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"7geese-cli/internal/store"
	"github.com/spf13/cobra"
)

func newSyncCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var full bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Download your 7Geese data to the local database",
		Long: `Downloads your 7Geese data and replaces the local copy.

Each run is a full refresh. Records that were changed in 7Geese are updated.
Records that were deleted in 7Geese are deleted from the local database.

Datasets: user profiles, 1:1 meetings and their notes, objectives with the
full check-in history, reviews, recognition (given and received), check-ins,
notifications, feedback requests and peer feedback. The objectives and reviews
of your direct reports are included.

If a dataset fails, its earlier local data stays and the exit code is not 0.
Run the command again to complete the sync.

Output: one JSON event for each dataset on stdout (sync_start, sync_complete,
sync_error, sync_purge) and a final sync_summary. Use --human-friendly for
text output on stderr.`,
		Example: `  7geese-cli sync
  7geese-cli sync --human-friendly
  7geese-cli sync --db /path/to/data.db`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbPath == "" {
				dbPath = defaultDBPath("7geese-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			var reporter syncReporter = &jsonSyncReporter{out: cmd.OutOrStdout()}
			if humanFriendly {
				reporter = &humanSyncReporter{out: cmd.ErrOrStderr()}
			}
			return runSync(cmd.Context(), db, syncOptionsFromFlags(flags, reporter))
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/7geese-cli/data.db)")
	cmd.Flags().BoolVar(&full, "full", false, "No effect: each sync is a full refresh")
	if err := cmd.Flags().MarkDeprecated("full", "each sync is a full refresh"); err != nil {
		panic(err)
	}

	return cmd
}
