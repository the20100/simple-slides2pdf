package converter

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// findChrome returns the path to Chrome/Chromium binary.
func findChrome() string {
	// Check common paths based on OS
	if runtime.GOOS == "darwin" {
		paths := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	// Fallback: check PATH
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// Options configures the HTML-to-PDF conversion.
type Options struct {
	InputPath  string
	OutputPath string
	Width      int
	Height     int
	// SlideSelector is the CSS selector for individual slides (default: ".slide")
	SlideSelector string
	// DeckSelector is the CSS selector for the slide deck container (default: ".deck")
	DeckSelector string
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{
		Width:         1920,
		Height:        1080,
		SlideSelector: ".slide",
		DeckSelector:  ".deck",
	}
}

// Convert renders the HTML presentation and outputs a PDF with one page per slide.
func Convert(opts Options) error {
	if opts.SlideSelector == "" {
		opts.SlideSelector = ".slide"
	}
	if opts.DeckSelector == "" {
		opts.DeckSelector = ".deck"
	}

	// Find Chrome binary
	chromePath := findChrome()
	if chromePath == "" {
		return fmt.Errorf("Chrome or Chromium not found — install Google Chrome")
	}

	// Create Chrome context
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.WindowSize(opts.Width, opts.Height),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocOpts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(string, ...interface{}) {}))
	defer cancel()

	// Set a timeout
	ctx, cancel = context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	fileURL := "file://" + opts.InputPath

	// CSS to override horizontal scroll layout to vertical paged layout
	printCSS := fmt.Sprintf(`
		/* Reset the horizontal scroll deck to vertical flow */
		%s {
			display: block !important;
			flex-direction: column !important;
			overflow: visible !important;
			height: auto !important;
			width: auto !important;
			scroll-snap-type: none !important;
		}

		/* Each slide becomes a full page */
		%s {
			flex: none !important;
			width: 100vw !important;
			height: 100vh !important;
			page-break-after: always !important;
			break-after: page !important;
			scroll-snap-align: none !important;
			overflow: hidden !important;
			position: relative !important;
		}

		/* Remove the last page break to avoid trailing blank page */
		%s:last-child {
			page-break-after: auto !important;
			break-after: auto !important;
		}

		/* Hide navigation and UI elements */
		.nav-zone, .nav-btn, .progress-bar-container,
		#progress-bar, #slide-counter,
		[class*="nav-"], [class*="progress"] {
			display: none !important;
		}

		/* Disable grain/noise overlay that may cause rendering issues */
		.grain {
			display: none !important;
		}

		/* Ensure blobs and decorations stay within slide bounds */
		.blobs {
			overflow: hidden !important;
		}

		/* Reset any scroll-related transforms */
		* {
			scroll-behavior: auto !important;
		}
	`, opts.DeckSelector, opts.SlideSelector, opts.SlideSelector)

	var slideCount int
	var pdfBuf []byte

	// Navigate, inject CSS, count slides, then print to PDF
	err := chromedp.Run(ctx,
		// Set viewport
		emulation.SetDeviceMetricsOverride(int64(opts.Width), int64(opts.Height), 1.0, false),

		// Navigate to the HTML file
		chromedp.Navigate(fileURL),

		// Wait for the deck to be ready
		chromedp.WaitReady(opts.DeckSelector),

		// Small delay to let fonts/images load
		chromedp.Sleep(2*time.Second),

		// Count slides
		chromedp.Evaluate(fmt.Sprintf(
			`document.querySelectorAll('%s').length`, opts.SlideSelector),
			&slideCount,
		),

		// Inject the print CSS
		chromedp.ActionFunc(func(ctx context.Context) error {
			js := fmt.Sprintf(`
				(function() {
					var style = document.createElement('style');
					style.id = 'slides2pdf-print-override';
					style.textContent = %q;
					document.head.appendChild(style);
				})();
			`, printCSS)
			var res interface{}
			return chromedp.Evaluate(js, &res).Do(ctx)
		}),

		// Small delay after CSS injection for re-layout
		chromedp.Sleep(500*time.Millisecond),

		// Print to PDF
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Calculate paper dimensions in inches (Chrome expects inches)
			// Using 96 DPI as the standard CSS reference
			paperWidth := float64(opts.Width) / 96.0
			paperHeight := float64(opts.Height) / 96.0

			printParams := page.PrintToPDF().
				WithPaperWidth(paperWidth).
				WithPaperHeight(paperHeight).
				WithMarginTop(0).
				WithMarginBottom(0).
				WithMarginLeft(0).
				WithMarginRight(0).
				WithPrintBackground(true).
				WithPreferCSSPageSize(false).
				WithScale(1.0)

			buf, _, err := printParams.Do(ctx)
			if err != nil {
				return fmt.Errorf("print to PDF: %w", err)
			}
			pdfBuf = buf
			return nil
		}),
	)

	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	// Write PDF
	if err := os.WriteFile(opts.OutputPath, pdfBuf, 0644); err != nil {
		return fmt.Errorf("writing PDF: %w", err)
	}

	// Calculate expected pages (rough estimate based on PDF size)
	pdfPages := slideCount
	if pdfPages == 0 {
		pdfPages = int(math.Max(1, float64(len(pdfBuf))/50000))
	}

	fmt.Printf("Converted %d slides to %s (%s)\n",
		slideCount, opts.OutputPath, formatBytes(len(pdfBuf)))

	return nil
}

func formatBytes(b int) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
