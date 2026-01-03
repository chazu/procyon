package ast

import (
	"encoding/json"
	"fmt"
	"io"
)

// Parse reads AST JSON from a reader and returns a Class.
func Parse(r io.Reader) (*Class, error) {
	var class Class
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&class); err != nil {
		return nil, fmt.Errorf("failed to parse AST: %w", err)
	}
	return &class, nil
}

// ParseBytes parses AST JSON from a byte slice.
func ParseBytes(data []byte) (*Class, error) {
	var class Class
	if err := json.Unmarshal(data, &class); err != nil {
		return nil, fmt.Errorf("failed to parse AST: %w", err)
	}
	return &class, nil
}
