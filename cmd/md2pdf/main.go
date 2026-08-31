package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	markdown "github.com/ideras/md-to-pdf"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: md2pdf <markdown-file> [output-pdf]\n")
		os.Exit(1)
	}

	input := os.Args[1]
	output := strings.TrimSuffix(input, filepath.Ext(input)) + ".pdf"
	if len(os.Args) >= 3 {
		output = os.Args[2]
	}

	if err := markdown.ConvertFile(input, output); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("PDF created: %s\n", output)
}
