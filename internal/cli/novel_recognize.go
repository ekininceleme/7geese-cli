// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"7geese-cli/internal/store"
	"github.com/spf13/cobra"
)

func newRecognizeCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recognize",
		Short: "Recognition commands",
	}
	cmd.AddCommand(newRecognizeLeaderboardCmd(flags))
	return cmd
}

func newRecognizeLeaderboardCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var period string
	var topN int

	cmd := &cobra.Command{
		Use:   "leaderboard",
		Short: "See who gives and receives the most recognition this month",
		Long: `Aggregates recognitionbadges from the local SQLite store by sender and
recipient within the specified time period.

Period values: month, quarter, year, all`,
		Example: `  7geese-cli recognize leaderboard
  7geese-cli recognize leaderboard --period quarter --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbPath == "" {
				dbPath = defaultDBPath("7geese-cli")
			}
			db, err := store.OpenReadOnly(dbPath)
			if err != nil {
				return fmt.Errorf("opening local database (run 'sync' first): %w", err)
			}
			defer db.Close()

			cutoff := periodCutoff(period)

			query := `
				SELECT
					json_extract(data,'$.sender') as sender,
					json_extract(data,'$.recipient') as recipient,
					json_extract(data,'$.created') as created_at
				FROM resources
				WHERE resource_type = 'recognitionbadges'
			`
			var queryArgs []any
			if cutoff != "" {
				query += " AND json_extract(data,'$.created') >= ?"
				queryArgs = append(queryArgs, cutoff)
			}

			rows, err := db.Query(query, queryArgs...)
			if err != nil {
				return fmt.Errorf("querying recognition badges: %w", err)
			}
			defer rows.Close()

			senderCount := map[string]int{}
			recipientCount := map[string]int{}
			total := 0

			for rows.Next() {
				var sender, recipient, createdAt string
				if err := rows.Scan(&sender, &recipient, &createdAt); err != nil {
					continue
				}
				senderCount[sender]++
				recipientCount[recipient]++
				total++
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating results: %w", err)
			}

			topSenders := topNEntries(senderCount, topN)
			topRecipients := topNEntries(recipientCount, topN)

			type leaderboard struct {
				Period        string              `json:"period"`
				Total         int                 `json:"total_recognitions"`
				TopSenders    []recognitionEntry  `json:"top_senders"`
				TopRecipients []recognitionEntry  `json:"top_recipients"`
			}
			result := leaderboard{
				Period:        period,
				Total:         total,
				TopSenders:    topSenders,
				TopRecipients: topRecipients,
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Recognition Leaderboard (%s) — %d total\n\n", period, total)
			fmt.Fprintln(w, "Top Senders:")
			for i, e := range topSenders {
				fmt.Fprintf(w, "  %d. %s (%d given)\n", i+1, e.ID, e.Count)
			}
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Top Recipients:")
			for i, e := range topRecipients {
				fmt.Fprintf(w, "  %d. %s (%d received)\n", i+1, e.ID, e.Count)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&period, "period", "month", "Time period: month, quarter, year, all")
	cmd.Flags().IntVar(&topN, "top", 10, "Number of entries to show")
	return cmd
}

func periodCutoff(period string) string {
	now := time.Now()
	switch period {
	case "month":
		return now.AddDate(0, -1, 0).Format(time.RFC3339)
	case "quarter":
		return now.AddDate(0, -3, 0).Format(time.RFC3339)
	case "year":
		return now.AddDate(-1, 0, 0).Format(time.RFC3339)
	default:
		return ""
	}
}

type recognitionEntry struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

func topNEntries(counts map[string]int, n int) []recognitionEntry {
	var sorted []recognitionEntry
	for id, count := range counts {
		sorted = append(sorted, recognitionEntry{ID: id, Count: count})
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Count > sorted[i].Count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	if n > 0 && len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}
