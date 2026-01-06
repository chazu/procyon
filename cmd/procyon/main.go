// Procyon - Trashtalk to Go compiler
// Named after the genus for raccoons - because what goes better with trash?
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/chazu/procyon/pkg/ast"
	"github.com/chazu/procyon/pkg/codegen"
)

var (
	strict  = flag.Bool("strict", false, "fail on unsupported constructs instead of warning")
	dryRun  = flag.Bool("dry-run", false, "show what would be generated without outputting")
	version = flag.Bool("version", false, "print version and exit")
	mode    = flag.String("mode", "binary", "output mode: binary (standalone) or plugin (c-shared library)")
)

const versionStr = "0.6.0"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Procyon - Trashtalk to Go compiler\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  procyon [options] < ast.json > output.go\n")
		fmt.Fprintf(os.Stderr, "  trashtalk-parser Class.trash | procyon > class/main.go\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *version {
		fmt.Printf("procyon version %s\n", versionStr)
		os.Exit(0)
	}

	// Read AST from stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	if len(input) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no input provided\n")
		fmt.Fprintf(os.Stderr, "Usage: procyon < ast.json\n")
		os.Exit(1)
	}

	// Parse AST
	class, err := ast.ParseBytes(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing AST: %v\n", err)
		os.Exit(1)
	}

	// Generate code based on mode
	var result *codegen.Result
	switch *mode {
	case "binary":
		result = codegen.Generate(class)
	case "plugin":
		result = codegen.GeneratePlugin(class)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown mode %q (use 'binary' or 'plugin')\n", *mode)
		os.Exit(1)
	}

	// Report skipped methods
	if len(result.SkippedMethods) > 0 {
		fmt.Fprintf(os.Stderr, "procyon: %s.trash\n", class.Name)

		// Count compiled methods
		compiled := len(class.Methods) - len(result.SkippedMethods)

		for _, m := range class.Methods {
			skipped := false
			var reason string
			for _, s := range result.SkippedMethods {
				if s.Selector == m.Selector {
					skipped = true
					reason = s.Reason
					break
				}
			}
			if skipped {
				fmt.Fprintf(os.Stderr, "  ⚠ %s - skipped: %s\n", m.Selector, reason)
			} else {
				fmt.Fprintf(os.Stderr, "  ✓ %s - compiled\n", m.Selector)
			}
		}

		fmt.Fprintf(os.Stderr, "\nGenerated %d/%d methods. %d will fall back to Bash.\n\n",
			compiled, len(class.Methods), len(result.SkippedMethods))

		if *strict {
			fmt.Fprintf(os.Stderr, "Error: --strict mode enabled, refusing to generate with skipped methods\n")
			os.Exit(1)
		}
	}

	// Report warnings
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	// Output
	if *dryRun {
		fmt.Fprintf(os.Stderr, "Dry run - would generate %d bytes of Go code\n", len(result.Code))
		os.Exit(0)
	}

	fmt.Print(result.Code)
}
