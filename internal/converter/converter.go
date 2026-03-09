package converter

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const (
	chromeVersionsURL = "https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json"
	cacheDir          = "slides2pdf"
)

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

// --- Chrome resolution: system → cache → auto-download ---

// findChrome returns the path to a working Chrome/Chromium binary.
// Resolution order: system install → cached headless shell → auto-download.
func findChrome() (string, error) {
	// 1. System Chrome
	if p := findSystemChrome(); p != "" {
		return p, nil
	}

	// 2. Cached headless shell
	if p := cachedChromePath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// 3. Auto-download
	fmt.Println("No Chrome found — downloading chrome-headless-shell...")
	return downloadChromeHeadlessShell()
}

// findSystemChrome looks for Chrome/Chromium already installed on the system.
func findSystemChrome() string {
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
	if runtime.GOOS == "linux" {
		paths := []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// cacheBasePath returns ~/.cache/slides2pdf (Linux) or ~/Library/Caches/slides2pdf (macOS).
func cacheBasePath() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, cacheDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Caches", cacheDir)
	}
	return filepath.Join(home, ".cache", cacheDir)
}

// platform returns the Chrome for Testing platform string.
func platform() string {
	switch runtime.GOOS {
	case "linux":
		return "linux64"
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "mac-arm64"
		}
		return "mac-x64"
	case "windows":
		return "win64"
	}
	return ""
}

// cachedChromePath returns the expected path to the cached headless shell binary.
func cachedChromePath() string {
	base := cacheBasePath()
	if base == "" {
		return ""
	}
	plat := platform()
	bin := "chrome-headless-shell"
	if runtime.GOOS == "windows" {
		bin = "chrome-headless-shell.exe"
	}
	return filepath.Join(base, "chrome-headless-shell-"+plat, bin)
}

// --- Chrome for Testing download ---

type cftResponse struct {
	Channels map[string]cftChannel `json:"channels"`
}

type cftChannel struct {
	Version   string            `json:"version"`
	Downloads map[string][]cftDL `json:"downloads"`
}

type cftDL struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

// downloadChromeHeadlessShell fetches the latest stable chrome-headless-shell
// from Google's Chrome for Testing infrastructure, extracts it, and returns the binary path.
func downloadChromeHeadlessShell() (string, error) {
	plat := platform()
	if plat == "" {
		return "", fmt.Errorf("unsupported OS/arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	base := cacheBasePath()
	if base == "" {
		return "", fmt.Errorf("cannot determine cache directory")
	}

	// Fetch version metadata
	fmt.Printf("  Fetching latest Chrome version info...\n")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(chromeVersionsURL)
	if err != nil {
		return "", fmt.Errorf("fetching Chrome versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Chrome versions API returned HTTP %d", resp.StatusCode)
	}

	var data cftResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("parsing Chrome versions: %w", err)
	}

	stable, ok := data.Channels["Stable"]
	if !ok {
		return "", fmt.Errorf("no Stable channel in Chrome for Testing response")
	}

	downloads, ok := stable.Downloads["chrome-headless-shell"]
	if !ok {
		return "", fmt.Errorf("no chrome-headless-shell downloads for Stable channel")
	}

	var dlURL string
	for _, dl := range downloads {
		if dl.Platform == plat {
			dlURL = dl.URL
			break
		}
	}
	if dlURL == "" {
		return "", fmt.Errorf("no chrome-headless-shell download for platform %s", plat)
	}

	// Download zip
	fmt.Printf("  Downloading Chrome %s for %s...\n", stable.Version, plat)
	dlResp, err := client.Get(dlURL)
	if err != nil {
		return "", fmt.Errorf("downloading Chrome: %w", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != 200 {
		return "", fmt.Errorf("Chrome download returned HTTP %d", dlResp.StatusCode)
	}

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "chrome-headless-shell-*.zip")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	size, err := io.Copy(tmpFile, dlResp.Body)
	tmpFile.Close()
	if err != nil {
		return "", fmt.Errorf("downloading Chrome: %w", err)
	}
	fmt.Printf("  Downloaded %s\n", formatBytes(int(size)))

	// Extract
	fmt.Printf("  Extracting to %s...\n", base)
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", fmt.Errorf("creating cache dir: %w", err)
	}

	if err := extractZip(tmpPath, base); err != nil {
		return "", fmt.Errorf("extracting Chrome: %w", err)
	}

	binPath := cachedChromePath()
	if _, err := os.Stat(binPath); err != nil {
		return "", fmt.Errorf("Chrome binary not found after extraction at %s", binPath)
	}

	// Ensure executable
	if err := os.Chmod(binPath, 0755); err != nil {
		return "", fmt.Errorf("making Chrome executable: %w", err)
	}

	fmt.Printf("  Chrome headless shell ready: %s\n\n", binPath)
	return binPath, nil
}

// extractZip extracts a zip archive into destDir.
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		path := filepath.Join(destDir, f.Name)

		// Security: prevent zip slip
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// --- Conversion ---

// Convert renders the HTML presentation and outputs a PDF with one page per slide.
func Convert(opts Options) error {
	if opts.SlideSelector == "" {
		opts.SlideSelector = ".slide"
	}
	if opts.DeckSelector == "" {
		opts.DeckSelector = ".deck"
	}

	// Find or download Chrome binary
	chromePath, err := findChrome()
	if err != nil {
		return err
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
	err = chromedp.Run(ctx,
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
