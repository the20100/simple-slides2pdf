package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InputPath validates an input HTML file path.
func InputPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("input path cannot be empty")
	}

	// Reject control characters
	for i, r := range path {
		if r < 0x20 {
			return "", fmt.Errorf("path contains control character at position %d", i)
		}
	}

	// Resolve to absolute
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid input path: %w", err)
	}

	// Check file exists
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("cannot access %q: %w", path, err)
	}
	if info.IsDir() {
		// Look for index.html inside the directory
		indexPath := filepath.Join(abs, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			return "", fmt.Errorf("%q is a directory and no index.html found inside", path)
		}
		return indexPath, nil
	}

	// Validate extension
	ext := strings.ToLower(filepath.Ext(abs))
	if ext != ".html" && ext != ".htm" {
		return "", fmt.Errorf("expected .html or .htm file, got %q", ext)
	}

	return abs, nil
}

// OutputPath validates an output PDF file path.
func OutputPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("output path cannot be empty")
	}

	// Reject control characters
	for i, r := range path {
		if r < 0x20 {
			return "", fmt.Errorf("path contains control character at position %d", i)
		}
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid output path: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(abs)
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("output directory does not exist: %s", dir)
	}

	// Ensure .pdf extension
	if strings.ToLower(filepath.Ext(abs)) != ".pdf" {
		abs += ".pdf"
	}

	return abs, nil
}

// SafeString validates a string value for common injection patterns.
func SafeString(s string, maxLen int) error {
	if len([]rune(s)) > maxLen {
		return fmt.Errorf("string exceeds maximum length of %d characters", maxLen)
	}
	for i, r := range s {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return fmt.Errorf("string contains control character at position %d", i)
		}
	}
	return nil
}
