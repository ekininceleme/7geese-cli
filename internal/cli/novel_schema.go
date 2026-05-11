// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
	"github.com/spf13/cobra"
)

func newMeSchemaCmd(_ *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON Schema for the 'me export' output",
		Long:  `Prints a JSON Schema describing the structure of the JSON produced by 'me export'.`,
		Example: `  7geese-cli me schema
  7geese-cli me schema | jq '.properties.objectives'`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			schema := jsonschema.Reflect(&exportOutput{})
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(schema)
		},
	}
}
