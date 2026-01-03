package codegen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chazu/procyon/pkg/ast"
	"github.com/chazu/procyon/pkg/codegen"
)

func TestCodegenAcceptance(t *testing.T) {
	// Find all test cases in testdata
	testdataDir := "../../testdata"
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Fatalf("Failed to read testdata directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		testName := entry.Name()
		t.Run(testName, func(t *testing.T) {
			testDir := filepath.Join(testdataDir, testName)

			// Read input AST
			inputPath := filepath.Join(testDir, "input.json")
			inputData, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("Failed to read input.json: %v", err)
			}

			// Parse AST
			class, err := ast.ParseBytes(inputData)
			if err != nil {
				t.Fatalf("Failed to parse AST: %v", err)
			}

			// Generate code
			result := codegen.Generate(class)

			// Read expected output
			expectedPath := filepath.Join(testDir, "expected.go")
			expectedData, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("Failed to read expected.go: %v", err)
			}

			expected := string(expectedData)
			actual := result.Code

			// Compare (normalize whitespace for comparison)
			if normalizeWhitespace(actual) != normalizeWhitespace(expected) {
				t.Errorf("Generated code does not match expected.\n\n=== EXPECTED ===\n%s\n\n=== ACTUAL ===\n%s", expected, actual)
			}

			// Check for warnings
			if len(result.Warnings) > 0 {
				t.Logf("Warnings: %v", result.Warnings)
			}

			// Log skipped methods
			if len(result.SkippedMethods) > 0 {
				t.Logf("Skipped methods: %v", result.SkippedMethods)
			}
		})
	}
}

func normalizeWhitespace(s string) string {
	// Trim trailing whitespace from each line and normalize line endings
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
