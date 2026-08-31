package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	markdown "github.com/ideras/md-to-pdf"
)

func main() {
	fontConfig := flag.String("font-config", "", "path to a TOML custom-font configuration")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: md2pdf [--font-config fonts.toml] <markdown-file> [output-pdf]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 || flag.NArg() > 2 {
		flag.Usage()
		os.Exit(1)
	}

	input := flag.Arg(0)
	output := strings.TrimSuffix(input, filepath.Ext(input)) + ".pdf"
	if flag.NArg() == 2 {
		output = flag.Arg(1)
	}

	var options []markdown.Option
	if *fontConfig != "" {
		registry, err := markdown.LoadFontRegistry(*fontConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading font config: %v\n", err)
			os.Exit(1)
		}
		options = append(options, markdown.WithFontRegistry(registry))
	}
	if err := markdown.ConvertFile(input, output, options...); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("PDF created: %s\n", output)
}
