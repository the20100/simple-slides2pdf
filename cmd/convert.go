package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/the20100/slides2pdf-cli/internal/converter"
	"github.com/the20100/slides2pdf-cli/internal/output"
	"github.com/the20100/slides2pdf-cli/internal/validate"
)

var (
	convertOutput        string
	convertWidth         int
	convertHeight        int
	convertSlideSelector string
	convertDeckSelector  string
)

var convertCmd = &cobra.Command{
	Use:   "convert <input>",
	Short: "Convert an HTML slide presentation to PDF",
	Long: `Convert an HTML slide presentation to PDF with one page per slide.

Input can be an HTML file or a directory containing index.html.

The tool uses headless Chrome to render the presentation, injects CSS
to reflow the slides vertically with page breaks, and prints to PDF.

Requires Google Chrome or Chromium to be installed.

Examples:
  slides2pdf convert presentation/index.html
  slides2pdf convert presentation/
  slides2pdf convert deck.html -o slides.pdf
  slides2pdf convert deck.html --width 1920 --height 1080
  slides2pdf convert deck.html --slide-selector "section.slide"
  slides2pdf convert deck.html --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Validate input path
		inputPath, err := validate.InputPath(args[0])
		if err != nil {
			return fmt.Errorf("invalid input: %w", err)
		}

		// Determine output path
		outputPath := convertOutput
		if outputPath == "" {
			// Default: same directory as input, same base name with .pdf extension
			base := filepath.Base(inputPath)
			ext := filepath.Ext(base)
			name := strings.TrimSuffix(base, ext)
			outputPath = filepath.Join(filepath.Dir(inputPath), name+".pdf")
		}

		outputPath, err = validate.OutputPath(outputPath)
		if err != nil {
			return fmt.Errorf("invalid output: %w", err)
		}

		opts := converter.Options{
			InputPath:     inputPath,
			OutputPath:    outputPath,
			Width:         convertWidth,
			Height:        convertHeight,
			SlideSelector: convertSlideSelector,
			DeckSelector:  convertDeckSelector,
		}

		// Dry-run: validate inputs and show what would happen
		if dryRunFlag {
			result := map[string]any{
				"action":         "convert",
				"input":          opts.InputPath,
				"output":         opts.OutputPath,
				"width":          opts.Width,
				"height":         opts.Height,
				"slide_selector": opts.SlideSelector,
				"deck_selector":  opts.DeckSelector,
				"dry_run":        true,
			}
			if output.IsJSON(cmd) {
				return output.PrintJSON(result, output.IsPretty(cmd))
			}
			fmt.Println("DRY RUN — would convert:")
			fmt.Printf("  input:          %s\n", opts.InputPath)
			fmt.Printf("  output:         %s\n", opts.OutputPath)
			fmt.Printf("  viewport:       %dx%d\n", opts.Width, opts.Height)
			fmt.Printf("  slide selector: %s\n", opts.SlideSelector)
			fmt.Printf("  deck selector:  %s\n", opts.DeckSelector)
			fmt.Println("No conversion performed.")
			return nil
		}

		// Run conversion
		if err := converter.Convert(opts); err != nil {
			return err
		}

		// JSON output for agent consumption
		if output.IsJSON(cmd) {
			result := map[string]any{
				"status": "success",
				"input":  opts.InputPath,
				"output": opts.OutputPath,
			}
			return output.PrintJSON(result, output.IsPretty(cmd))
		}

		return nil
	},
}

func init() {
	convertCmd.Flags().StringVarP(&convertOutput, "output", "o", "", "Output PDF file path (default: same name as input with .pdf extension)")
	convertCmd.Flags().IntVar(&convertWidth, "width", 1920, "Viewport width in pixels")
	convertCmd.Flags().IntVar(&convertHeight, "height", 1080, "Viewport height in pixels")
	convertCmd.Flags().StringVar(&convertSlideSelector, "slide-selector", ".slide", "CSS selector for individual slides")
	convertCmd.Flags().StringVar(&convertDeckSelector, "deck-selector", ".deck", "CSS selector for the slide deck container")

	rootCmd.AddCommand(convertCmd)

	RegisterSchema("convert", SchemaEntry{
		Command:     "slides2pdf convert <input>",
		Description: "Convert an HTML slide presentation to PDF with one page per slide",
		Args: []SchemaArg{
			{Name: "input", Required: true, Desc: "Path to HTML file or directory containing index.html"},
		},
		Flags: []SchemaFlag{
			{Name: "--output", Type: "string", Desc: "Output PDF file path (default: input name with .pdf extension)"},
			{Name: "--width", Type: "int", Default: "1920", Desc: "Viewport width in pixels"},
			{Name: "--height", Type: "int", Default: "1080", Desc: "Viewport height in pixels"},
			{Name: "--slide-selector", Type: "string", Default: ".slide", Desc: "CSS selector for individual slides"},
			{Name: "--deck-selector", Type: "string", Default: ".deck", Desc: "CSS selector for the slide deck container"},
			{Name: "--dry-run", Type: "bool", Default: "false", Desc: "Validate inputs without converting"},
		},
		Examples: []string{
			"slides2pdf convert presentation/index.html",
			"slides2pdf convert presentation/ -o output.pdf",
			"slides2pdf convert deck.html --width 1920 --height 1080",
		},
		Mutating: false,
	})
}
