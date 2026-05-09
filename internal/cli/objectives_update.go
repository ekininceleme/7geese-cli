// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func newObjectivesUpdateCmd(flags *rootFlags) *cobra.Command {
	var bodyName string
	var bodyDescription string
	var bodyProgress float64
	var bodyClosed bool
	var stdinBody bool

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "",
		Example: "  7geese-cli objectives update 550e8400-e29b-41d4-a716-446655440000",
		Annotations: map[string]string{"pp:endpoint": "objectives.update", "pp:method": "PATCH", "pp:path": "/api/v1/objectives/{id}/"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if !stdinBody {
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}

			path := "/api/v1/objectives/{id}/"
			path = replacePathParam(path, "id", args[0])
			var body map[string]any
			if stdinBody {
				stdinData, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("reading stdin: %w", err)
				}
				var jsonBody map[string]any
				if err := json.Unmarshal(stdinData, &jsonBody); err != nil {
					return fmt.Errorf("parsing stdin JSON: %w", err)
				}
				body = jsonBody
			} else {
				body = map[string]any{}
				if bodyName != "" {
					body["name"] = bodyName
				}
				if bodyDescription != "" {
					body["description"] = bodyDescription
				}
				if bodyProgress != 0.0 {
					body["progress"] = bodyProgress
				}
				if bodyClosed != false {
					body["closed"] = bodyClosed
				}
			}
			data, statusCode, err := c.Patch(path, body)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				// Check if response contains an array (directly or wrapped in "data")
				var items []map[string]any
				if json.Unmarshal(data, &items) == nil && len(items) > 0 {
					if err := printAutoTable(cmd.OutOrStdout(), items); err != nil {
						fmt.Fprintf(os.Stderr, "warning: table rendering failed, falling back to JSON: %v\n", err)
					} else {
						return nil
					}
				} else {
					var wrapped struct{ Data []map[string]any `json:"data"` }
					if json.Unmarshal(data, &wrapped) == nil && len(wrapped.Data) > 0 {
						if err := printAutoTable(cmd.OutOrStdout(), wrapped.Data); err != nil {
							fmt.Fprintf(os.Stderr, "warning: table rendering failed, falling back to JSON: %v\n", err)
						} else {
							return nil
						}
					}
				}
			}
			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				if flags.quiet {
					return nil
				}
				// Apply --compact and --select to the API response before wrapping.
				// --select wins when both are set: explicit field choice trumps the
				// generic high-gravity allow-list. Otherwise --compact still applies
				// when --agent is on but the user did not name fields.
				filtered := data
				if flags.selectFields != "" {
					filtered = filterFields(filtered, flags.selectFields)
				} else if flags.compact {
					filtered = compactFields(filtered)
				}
				envelope := map[string]any{
					"action":   "patch",
					"resource": "objectives",
					"path":     path,
					"status":   statusCode,
					"success":  statusCode >= 200 && statusCode < 300,
				}
				if flags.dryRun {
					envelope["dry_run"] = true
					envelope["status"] = 0
					envelope["success"] = false
				}
				if len(filtered) > 0 {
					var parsed any
					if err := json.Unmarshal(filtered, &parsed); err == nil {
						envelope["data"] = parsed
					}
				}
				envelopeJSON, err := json.Marshal(envelope)
				if err != nil {
					return err
				}
				return printOutput(cmd.OutOrStdout(), json.RawMessage(envelopeJSON), true)
			}
			return printOutputWithFlags(cmd.OutOrStdout(), data, flags)
		},
	}
	cmd.Flags().StringVar(&bodyName, "name", "", "")
	cmd.Flags().StringVar(&bodyDescription, "description", "", "")
	cmd.Flags().Float64Var(&bodyProgress, "progress", 0.0, "Progress value (0-100 for percent type)")
	cmd.Flags().BoolVar(&bodyClosed, "closed", false, "")
	cmd.Flags().BoolVar(&stdinBody, "stdin", false, "Read request body as JSON from stdin")

	return cmd
}
