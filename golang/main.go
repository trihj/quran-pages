package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"quran-images/config"
	"quran-images/db"
	"quran-images/image"
)

func main() {
	var pagesStr string
	var width int
	var output string
	var help bool

	flag.StringVar(&pagesStr, "pages", "1..604", "page number or page range to process (e.g., 1..604 or 50)")
	flag.IntVar(&width, "width", 1024, "width of image to generate in pixels")
	flag.StringVar(&output, "output", "images", "target directory of generated images")
	flag.BoolVar(&help, "help", false, "view these helpful instructions")
	flag.BoolVar(&help, "h", false, "view these helpful instructions")

	flag.Parse()

	if help {
		printUsage()
		os.Exit(0)
	}

	// Parse pages
	pages, err := parsePages(pagesStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing pages: %v\n", err)
		os.Exit(1)
	}

	// Get root directory
	rootDir, err := getRootDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting root directory: %v\n", err)
		os.Exit(1)
	}

	// Load config
	configPath := filepath.Join(rootDir, "golang", "config", "database.yaml")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Connect to database
	database, err := db.NewDB(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Get font directory
	fontDir, err := config.GetFontDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting font directory: %v\n", err)
		os.Exit(1)
	}

	// Verify font directory exists
	if _, err := os.Stat(fontDir); err != nil {
		fmt.Fprintf(os.Stderr, "Font directory does not exist: %s\n", fontDir)
		os.Exit(1)
	}

	// Verify default font exists
	defaultFont := filepath.Join(fontDir, "QCF_BSML.TTF")
	if _, err := os.Stat(defaultFont); err != nil {
		fmt.Fprintf(os.Stderr, "Default font not found: %s\n", defaultFont)
		fmt.Fprintf(os.Stderr, "Font directory: %s\n", fontDir)
		os.Exit(1)
	}

	fmt.Printf("Using font directory: %s\n", fontDir)

	// Prepare output path
	var outputPath string
	if filepath.IsAbs(output) {
		outputPath = output
	} else {
		outputPath = filepath.Join(rootDir, output)
	}

	// Create page generator
	generator := image.NewPageGenerator(database, fontDir, outputPath, width)

	// Generate images
	if err := generator.Generate(pages); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating images: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Image generation completed successfully!")
}

func parsePages(pagesStr string) ([]int, error) {
	// Handle range like "1..604"
	if strings.Contains(pagesStr, "..") {
		parts := strings.Split(pagesStr, "..")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range format: %s", pagesStr)
		}

		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid start page: %v", err)
		}

		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid end page: %v", err)
		}

		if start > end {
			return nil, fmt.Errorf("start page must be <= end page")
		}

		pages := make([]int, 0, end-start+1)
		for i := start; i <= end; i++ {
			pages = append(pages, i)
		}
		return pages, nil
	}

	// Handle single page
	page, err := strconv.Atoi(strings.TrimSpace(pagesStr))
	if err != nil {
		return nil, fmt.Errorf("invalid page number: %v", err)
	}

	return []int{page}, nil
}

func getRootDir() (string, error) {
	// Get current working directory (like Perl's FindBin)
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Check if we're in golang directory, go up one level to project root
	if filepath.Base(wd) == "golang" {
		rootDir := filepath.Dir(wd)
		absRoot, err := filepath.Abs(rootDir)
		if err != nil {
			return "", err
		}
		return absRoot, nil
	}

	// If we're already in project root, check if golang subdirectory exists
	golangDir := filepath.Join(wd, "golang")
	if _, err := os.Stat(golangDir); err == nil {
		// We're in project root
		absRoot, err := filepath.Abs(wd)
		if err != nil {
			return "", err
		}
		return absRoot, nil
	}

	// Try to find project root by looking for golang directory in parent
	current := wd
	for i := 0; i < 5; i++ { // Max 5 levels up
		golangDir := filepath.Join(current, "golang")
		if _, err := os.Stat(golangDir); err == nil {
			absRoot, err := filepath.Abs(current)
			if err != nil {
				return "", err
			}
			return absRoot, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break // Reached root
		}
		current = parent
	}

	// Fallback: assume current directory is root
	return wd, nil
}

func printUsage() {
	fmt.Println("Usage: generate [OPTIONS]")
	fmt.Println()
	fmt.Println("Generate Qur'an Images")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --pages, -p    page number or page range to process (e.g., 1..604 or 50)")
	fmt.Println("  --width, -w    width of image to generate in pixels (default: 1024)")
	fmt.Println("  --output, -o   target directory of generated images (default: images)")
	fmt.Println("  --help, -h     view these helpful instructions")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  ./generate --pages 1..604 --width 1280 --output ./images")
	fmt.Println("  ./generate --pages 293 --width 640 --output ./images/pages/")
}
