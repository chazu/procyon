// Package ast defines types for the Trashtalk AST produced by the jq parser.
package ast

// Class represents a Trashtalk class definition.
type Class struct {
	Type               string        `json:"type"`
	Name               string        `json:"name"`
	Parent             string        `json:"parent"`
	Package            string        `json:"package"`       // Namespace: "MyApp" or ""
	Imports            []string      `json:"imports"`       // Imported packages
	IsTrait            bool          `json:"isTrait"`
	Location           Location      `json:"location"`
	InstanceVars       []InstanceVar `json:"instanceVars"`
	ClassInstanceVars  []InstanceVar `json:"classInstanceVars"`
	Traits             []string      `json:"traits"`
	Requires           []string      `json:"requires"`
	MethodRequirements []string      `json:"methodRequirements"`
	Methods            []Method      `json:"methods"`
	Aliases            []Alias       `json:"aliases"`
	Advice             []Advice      `json:"advice"`
}

// QualifiedName returns the fully qualified name of the class.
// Returns "MyApp::Counter" for namespaced, "Counter" for non-namespaced.
func (c *Class) QualifiedName() string {
	if c.Package != "" {
		return c.Package + "::" + c.Name
	}
	return c.Name
}

// CompiledName returns the name for the compiled binary.
// Returns "MyApp__Counter" for namespaced, "Counter" for non-namespaced.
func (c *Class) CompiledName() string {
	if c.Package != "" {
		return c.Package + "__" + c.Name
	}
	return c.Name
}

// IsNamespaced returns true if the class belongs to a package.
func (c *Class) IsNamespaced() bool {
	return c.Package != ""
}

// Location represents a position in the source file.
type Location struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// InstanceVar represents an instance variable declaration.
type InstanceVar struct {
	Name     string       `json:"name"`
	Default  DefaultValue `json:"default"`
	Location Location     `json:"location"`
}

// DefaultValue represents a default value for an instance variable.
type DefaultValue struct {
	Type  string `json:"type"`  // "number", "string", etc.
	Value string `json:"value"` // The literal value as a string
}

// Method represents a method definition.
type Method struct {
	Type      string   `json:"type"`      // Always "method"
	Kind      string   `json:"kind"`      // "instance" or "class"
	Raw       bool     `json:"raw"`       // True if this is a raw method (can't compile)
	Primitive bool     `json:"primitive"` // True if this is a primitive method (has native Procyon impl)
	Selector  string   `json:"selector"`  // Method name (e.g., "increment", "setValue_")
	Keywords  []string `json:"keywords"`  // For keyword methods (e.g., ["setValue"])
	Args      []string `json:"args"`      // Argument names
	Pragmas   []string `json:"pragmas"`   // Method pragmas (e.g., ["procyonOnly", "direct"])
	Body      Block    `json:"body"`
	Location  Location `json:"location"`
}

// HasPragma checks if the method has a specific pragma.
func (m *Method) HasPragma(pragma string) bool {
	for _, p := range m.Pragmas {
		if p == pragma {
			return true
		}
	}
	return false
}

// Block represents a method body as a token stream.
type Block struct {
	Type   string  `json:"type"` // Always "block"
	Tokens []Token `json:"tokens"`
}

// Token represents a lexical token in a method body.
type Token struct {
	Type  string `json:"type"`  // IDENTIFIER, NUMBER, PLUS, ASSIGN, etc.
	Value string `json:"value"` // The token's text
	Line  int    `json:"line"`
	Col   int    `json:"col"`
}

// Alias represents a method alias.
type Alias struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Advice represents before/after advice on a method.
type Advice struct {
	Type     string `json:"type"` // "before" or "after"
	Selector string `json:"selector"`
	Body     Block  `json:"body"`
}

// Token type constants
const (
	TokenNewline    = "NEWLINE"
	TokenPipe       = "PIPE"
	TokenIdentifier = "IDENTIFIER"
	TokenAssign     = "ASSIGN"
	TokenSubshell   = "SUBSHELL"
	TokenVariable   = "VARIABLE"
	TokenCaret      = "CARET"
	TokenNumber     = "NUMBER"
	TokenPlus       = "PLUS"
	TokenMinus      = "MINUS"
	TokenStar       = "STAR"
	TokenSlash      = "SLASH"
	TokenDString    = "DSTRING"
	TokenSString    = "SSTRING"
	TokenAt         = "AT"
	TokenColon      = "COLON"
	TokenLBracket   = "LBRACKET"
	TokenRBracket   = "RBRACKET"

	// Comparison operators
	TokenGT      = "GT"      // >
	TokenLT      = "LT"      // <
	TokenGE      = "GE"      // >=
	TokenLE      = "LE"      // <=
	TokenEQ      = "EQ"      // ==
	TokenNE      = "NE"      // !=
	TokenEquals  = "EQUALS"  // = (single equals, for comparisons)
	TokenPercent = "PERCENT" // % (modulo)
	TokenDot     = "DOT"     // . (statement separator)

	// Control flow
	TokenKeyword = "KEYWORD" // identifier followed by colon (e.g., "ifTrue:")
	TokenLParen  = "LPAREN"  // (
	TokenRParen  = "RPAREN"  // )

	// Blocks
	TokenBlockParam = "BLOCK_PARAM" // :param in [:param | body]

	// Namespaces
	TokenNamespaceSep = "NAMESPACE_SEP" // :: for qualified names like Pkg::Class
)
