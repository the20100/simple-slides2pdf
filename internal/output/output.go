package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// IsJSON returns true when output should be JSON:
// stdout is not a TTY (piped) OR --json/--pretty flag is set.
func IsJSON(cmd *cobra.Command) bool {
	if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		return true
	}
	j, _ := cmd.Flags().GetBool("json")
	p, _ := cmd.Flags().GetBool("pretty")
	return j || p
}

// IsPretty returns true when JSON should be indented.
func IsPretty(cmd *cobra.Command) bool {
	pretty, _ := cmd.Flags().GetBool("pretty")
	if !pretty {
		isJSON, _ := cmd.Flags().GetBool("json")
		if isJSON && isatty.IsTerminal(os.Stdout.Fd()) {
			return true
		}
	}
	return pretty
}

// PrintJSON encodes v as JSON to stdout.
func PrintJSON(v any, pretty bool) error {
	enc := json.NewEncoder(os.Stdout)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(v)
}

// PrintTable writes a tab-aligned table to stdout.
func PrintTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	defer w.Flush()
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprintln(w)
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, cell)
		}
		fmt.Fprintln(w)
	}
}

// PrintKeyValue prints a two-column key-value table.
func PrintKeyValue(rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	defer w.Flush()
	for _, row := range rows {
		if len(row) == 2 {
			fmt.Fprintf(w, "%s\t%s\n", row[0], row[1])
		}
	}
}

// Truncate shortens a string to maxLen characters, adding "..." if truncated.
func Truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "..."
}

// FormatLabels formats a slice of strings for display.
func FormatLabels(labels []string) string {
	if len(labels) == 0 {
		return "-"
	}
	return strings.Join(labels, ", ")
}

// PrintError prints an error to stderr.
func PrintError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
}
