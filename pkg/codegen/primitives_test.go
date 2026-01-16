package codegen

import (
	"testing"
)

func TestHasPrimitiveImpl(t *testing.T) {
	tests := []struct {
		className string
		selector  string
		expected  bool
	}{
		// File class
		{"File", "at_", true},
		{"File", "read", true},
		{"File", "write_", true},
		{"File", "exists", true},
		{"File", "nonExistent", false},

		// Console class
		{"Console", "print_", true},
		{"Console", "write_", true},
		{"Console", "error_", true},
		{"Console", "newline", true},
		{"Console", "invalid", false},

		// Env class
		{"Env", "get_", true},
		{"Env", "set_to_", true},
		{"Env", "has_", true},
		{"Env", "invalid", false},

		// Unknown class
		{"UnknownClass", "anyMethod", false},

		// Block class
		{"Block", "params_code_captured_", true},
		{"Block", "numArgs", true},
		{"Block", "value", false}, // value requires eval - bash only

		// String class
		{"String", "length_", true},
		{"String", "isEmpty_", true},
		{"String", "uppercase_", true},

		// Shell class
		{"Shell", "exec_", true},
		{"Shell", "run_", true},
	}

	for _, tt := range tests {
		t.Run(tt.className+"_"+tt.selector, func(t *testing.T) {
			result := hasPrimitiveImpl(tt.className, tt.selector)
			if result != tt.expected {
				t.Errorf("hasPrimitiveImpl(%q, %q) = %v, want %v", tt.className, tt.selector, result, tt.expected)
			}
		})
	}
}

func TestPrimitiveRegistry(t *testing.T) {
	// Check that primitiveRegistry is populated
	if len(primitiveRegistry) == 0 {
		t.Fatal("primitiveRegistry is empty")
	}

	// Check that known classes exist
	classes := []string{"File", "Console", "Env", "Block", "String", "Shell"}
	for _, class := range classes {
		if _, ok := primitiveRegistry[class]; !ok {
			t.Errorf("primitiveRegistry missing class %q", class)
		}
	}

	// Check that File has expected methods
	fileMethods := primitiveRegistry["File"]
	expectedMethods := []string{"at_", "read", "write_", "exists", "delete"}
	for _, m := range expectedMethods {
		if !fileMethods[m] {
			t.Errorf("File class missing method %q", m)
		}
	}
}

func TestPrimitiveRegistryCompleteness(t *testing.T) {
	// File class should have many methods
	if len(primitiveRegistry["File"]) < 20 {
		t.Errorf("File class has fewer methods than expected: %d", len(primitiveRegistry["File"]))
	}

	// Console class should have print methods
	if len(primitiveRegistry["Console"]) < 3 {
		t.Errorf("Console class has fewer methods than expected: %d", len(primitiveRegistry["Console"]))
	}

	// String class should have string manipulation methods
	if len(primitiveRegistry["String"]) < 10 {
		t.Errorf("String class has fewer methods than expected: %d", len(primitiveRegistry["String"]))
	}
}
