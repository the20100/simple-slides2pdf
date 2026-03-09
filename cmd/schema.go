package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// SchemaEntry describes a single CLI command for agent introspection.
type SchemaEntry struct {
	Command     string       `json:"command"`
	Description string       `json:"description"`
	Args        []SchemaArg  `json:"args,omitempty"`
	Flags       []SchemaFlag `json:"flags,omitempty"`
	Examples    []string     `json:"examples,omitempty"`
	Mutating    bool         `json:"mutating"`
}

type SchemaArg struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Desc     string `json:"description"`
}

type SchemaFlag struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
	Desc     string `json:"description"`
}

// schemaRegistry is populated by each command file's init().
var schemaRegistry = map[string]SchemaEntry{}

// RegisterSchema adds a command schema entry. Call from init() in each command file.
func RegisterSchema(key string, entry SchemaEntry) {
	schemaRegistry[key] = entry
}

var schemaCmd = &cobra.Command{
	Use:   "schema [command]",
	Short: "Dump command schemas as JSON for agent introspection",
	Long: `Dump machine-readable command descriptions, parameters, and types.

With no argument, dumps all commands. With an argument (e.g. "convert"),
dumps only that command's schema.

This command exists so AI agents can self-serve documentation at runtime
without needing pre-loaded docs or consuming context window on --help text.

Examples:
  slides2pdf schema
  slides2pdf schema convert`,
	RunE: func(cmd *cobra.Command, args []string) error {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")

		if len(args) == 0 {
			return enc.Encode(schemaRegistry)
		}

		entry, ok := schemaRegistry[args[0]]
		if !ok {
			return fmt.Errorf("unknown command %q — run 'slides2pdf schema' to see all", args[0])
		}
		return enc.Encode(entry)
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}
