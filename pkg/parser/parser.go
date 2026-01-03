// Package parser converts token streams into expression trees.
package parser

import (
	"fmt"

	"github.com/chazu/procyon/pkg/ast"
)

// Expr represents an expression in the parsed method body.
type Expr interface {
	exprNode()
}

// Statement represents a statement in the parsed method body.
type Statement interface {
	stmtNode()
}

// LocalVarDecl declares local variables: | x y z |
type LocalVarDecl struct {
	Names []string
}

func (LocalVarDecl) stmtNode() {}

// Assignment represents: x := expr
type Assignment struct {
	Target string
	Value  Expr
}

func (Assignment) stmtNode() {}

// Return represents: ^ expr
type Return struct {
	Value Expr
}

func (Return) stmtNode() {}

// ExprStmt wraps an expression as a statement
type ExprStmt struct {
	Expr Expr
}

func (ExprStmt) stmtNode() {}

// BinaryExpr represents: left op right
type BinaryExpr struct {
	Left  Expr
	Op    string
	Right Expr
}

func (BinaryExpr) exprNode() {}

// Identifier represents a variable reference
type Identifier struct {
	Name string
}

func (Identifier) exprNode() {}

// NumberLit represents a numeric literal
type NumberLit struct {
	Value string
}

func (NumberLit) exprNode() {}

// StringLit represents a string literal
type StringLit struct {
	Value string
}

func (StringLit) exprNode() {}

// UnsupportedExpr represents an expression we can't compile
type UnsupportedExpr struct {
	Reason string
	Token  ast.Token
}

func (UnsupportedExpr) exprNode() {}

// MethodBody represents a parsed method body
type MethodBody struct {
	LocalVars  []string
	Statements []Statement
}

// ParseResult contains the parsed method body and any errors
type ParseResult struct {
	Body        *MethodBody
	Unsupported bool
	Reason      string
}

// Parser converts token streams to expression trees
type Parser struct {
	tokens []ast.Token
	pos    int
}

// ParseMethod parses a method body from tokens
func ParseMethod(tokens []ast.Token) *ParseResult {
	p := &Parser{tokens: tokens, pos: 0}
	return p.parseBody()
}

func (p *Parser) parseBody() *ParseResult {
	body := &MethodBody{
		LocalVars:  []string{},
		Statements: []Statement{},
	}

	// Skip leading newlines
	p.skipNewlines()

	// Check for local variable declarations: | x y z |
	if p.peek().Type == ast.TokenPipe {
		vars, err := p.parseLocalVars()
		if err != nil {
			return &ParseResult{Unsupported: true, Reason: err.Error()}
		}
		body.LocalVars = vars
	}

	// Parse statements
	for !p.atEnd() {
		p.skipNewlines()
		if p.atEnd() {
			break
		}

		stmt, err := p.parseStatement()
		if err != nil {
			return &ParseResult{Unsupported: true, Reason: err.Error()}
		}
		if stmt != nil {
			body.Statements = append(body.Statements, stmt)
		}
	}

	return &ParseResult{Body: body}
}

func (p *Parser) parseLocalVars() ([]string, error) {
	// Consume opening |
	p.advance() // skip |

	vars := []string{}
	for !p.atEnd() && p.peek().Type != ast.TokenPipe {
		if p.peek().Type == ast.TokenIdentifier {
			vars = append(vars, p.peek().Value)
			p.advance()
		} else if p.peek().Type == ast.TokenNewline {
			p.advance()
		} else {
			return nil, fmt.Errorf("expected identifier in local var declaration, got %s", p.peek().Type)
		}
	}

	if p.atEnd() {
		return nil, fmt.Errorf("unclosed local variable declaration")
	}
	p.advance() // skip closing |

	return vars, nil
}

func (p *Parser) parseStatement() (Statement, error) {
	p.skipNewlines()
	if p.atEnd() {
		return nil, nil
	}

	tok := p.peek()

	// Return statement: ^ expr
	if tok.Type == ast.TokenCaret {
		p.advance()
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return &Return{Value: expr}, nil
	}

	// Check for assignment: identifier := expr
	if tok.Type == ast.TokenIdentifier {
		// Look ahead for :=
		if p.peekAhead(1).Type == ast.TokenAssign {
			name := tok.Value
			p.advance() // skip identifier
			p.advance() // skip :=
			expr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			return &Assignment{Target: name, Value: expr}, nil
		}
	}

	// Otherwise try to parse as expression statement
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &ExprStmt{Expr: expr}, nil
}

func (p *Parser) parseExpr() (Expr, error) {
	return p.parseAddSub()
}

func (p *Parser) parseAddSub() (Expr, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return nil, err
	}

	for !p.atEnd() {
		tok := p.peek()
		if tok.Type == ast.TokenPlus || tok.Type == ast.TokenMinus {
			op := tok.Value
			p.advance()
			right, err := p.parseMulDiv()
			if err != nil {
				return nil, err
			}
			left = &BinaryExpr{Left: left, Op: op, Right: right}
		} else {
			break
		}
	}

	return left, nil
}

func (p *Parser) parseMulDiv() (Expr, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	for !p.atEnd() {
		tok := p.peek()
		if tok.Type == ast.TokenStar || tok.Type == ast.TokenSlash {
			op := tok.Value
			p.advance()
			right, err := p.parsePrimary()
			if err != nil {
				return nil, err
			}
			left = &BinaryExpr{Left: left, Op: op, Right: right}
		} else {
			break
		}
	}

	return left, nil
}

func (p *Parser) parsePrimary() (Expr, error) {
	tok := p.peek()

	switch tok.Type {
	case ast.TokenNumber:
		p.advance()
		return &NumberLit{Value: tok.Value}, nil

	case ast.TokenIdentifier:
		p.advance()
		return &Identifier{Name: tok.Value}, nil

	case ast.TokenDString, ast.TokenSString:
		p.advance()
		// Remove quotes from the value
		val := tok.Value
		if len(val) >= 2 {
			val = val[1 : len(val)-1]
		}
		return &StringLit{Value: val}, nil

	case ast.TokenVariable:
		// $variable - can't compile
		return nil, fmt.Errorf("bash variable references ($var) not supported")

	case ast.TokenSubshell:
		return nil, fmt.Errorf("subshell expressions not supported")

	case ast.TokenAt:
		return nil, fmt.Errorf("message sends not yet supported")

	case ast.TokenNewline:
		// End of expression
		return nil, fmt.Errorf("unexpected end of expression")

	default:
		return nil, fmt.Errorf("unexpected token: %s (%s)", tok.Type, tok.Value)
	}
}

func (p *Parser) peek() ast.Token {
	if p.pos >= len(p.tokens) {
		return ast.Token{Type: "EOF"}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peekAhead(n int) ast.Token {
	pos := p.pos + n
	if pos >= len(p.tokens) {
		return ast.Token{Type: "EOF"}
	}
	return p.tokens[pos]
}

func (p *Parser) advance() ast.Token {
	if p.pos >= len(p.tokens) {
		return ast.Token{Type: "EOF"}
	}
	tok := p.tokens[p.pos]
	p.pos++
	return tok
}

func (p *Parser) atEnd() bool {
	return p.pos >= len(p.tokens)
}

func (p *Parser) skipNewlines() {
	for !p.atEnd() && p.peek().Type == ast.TokenNewline {
		p.advance()
	}
}
