package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	jsonFlag   bool
	prettyFlag bool
	dryRunFlag bool
)

var rootCmd = &cobra.Command{
	Use:   "slides2pdf",
	Short: "slides2pdf — convert HTML slide presentations to PDF",
	Long: `slides2pdf converts HTML slide presentations to PDF documents,
with one page per slide.

It uses headless Chrome to render the HTML exactly as it appears in a browser,
then outputs a paginated PDF.

It outputs JSON when piped (for agent use) and human-readable text in a terminal.

Examples:
  slides2pdf convert presentation/index.html
  slides2pdf convert presentation/ -o output.pdf
  slides2pdf convert deck.html --width 1920 --height 1080
  slides2pdf schema convert`,
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Force JSON output")
	rootCmd.PersistentFlags().BoolVar(&prettyFlag, "pretty", false, "Force pretty-printed JSON output (implies --json)")
	rootCmd.PersistentFlags().BoolVar(&dryRunFlag, "dry-run", false, "Validate inputs without converting")

	rootCmd.AddCommand(infoCmd)
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show tool info: binary path, OS, and environment",
	Run: func(cmd *cobra.Command, args []string) {
		printInfo()
	},
}

func printInfo() {
	fmt.Printf("slides2pdf — HTML slide presentation to PDF converter\n\n")
	exe, _ := os.Executable()
	fmt.Printf("  binary:  %s\n", exe)
	fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Println()
	fmt.Println("  requires: Google Chrome or Chromium installed")
}
