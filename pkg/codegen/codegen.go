// Package codegen generates Go code from Trashtalk AST.
package codegen

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/chazu/procyon/pkg/ast"
	"github.com/chazu/procyon/pkg/parser"
	"github.com/dave/jennifer/jen"
)

// Result contains the generated code and any warnings.
type Result struct {
	Code           string
	Warnings       []string
	SkippedMethods []SkippedMethod
}

// SkippedMethod records a method that couldn't be compiled.
type SkippedMethod struct {
	Selector string
	Reason   string
}

// Generate produces Go source code from a Trashtalk class AST.
func Generate(class *ast.Class) *Result {
	g := &generator{
		class:        class,
		warnings:     []string{},
		skipped:      []SkippedMethod{},
		instanceVars: map[string]bool{},
	}

	// Build instance var lookup
	for _, iv := range class.InstanceVars {
		g.instanceVars[iv.Name] = true
	}

	return g.generate()
}

type generator struct {
	class        *ast.Class
	warnings     []string
	skipped      []SkippedMethod
	instanceVars map[string]bool
}

type compiledMethod struct {
	selector   string
	goName     string
	args       []string
	body       *parser.MethodBody
	hasReturn  bool
	isClass    bool
	returnsErr bool
}

func (g *generator) generate() *Result {
	f := jen.NewFile("main")

	// Add blank imports for embed and sqlite3
	f.Anon("embed")
	f.Anon("github.com/mattn/go-sqlite3")

	// Embed directive and source hash
	f.Comment("//go:embed " + g.class.Name + ".trash")
	f.Var().Id("_sourceCode").String()
	f.Line()
	f.Var().Id("_contentHash").String()
	f.Line()

	// init() to compute hash
	f.Func().Id("init").Params().Block(
		jen.Id("hash").Op(":=").Qual("crypto/sha256", "Sum256").Call(jen.Index().Byte().Parens(jen.Id("_sourceCode"))),
		jen.Id("_contentHash").Op("=").Qual("encoding/hex", "EncodeToString").Call(jen.Id("hash").Index(jen.Op(":"))),
	)
	f.Line()

	// ErrUnknownSelector
	f.Var().Id("ErrUnknownSelector").Op("=").Qual("errors", "New").Call(jen.Lit("unknown selector"))
	f.Line()

	// Struct definition
	g.generateStruct(f)
	f.Line()

	// main()
	g.generateMain(f)
	f.Line()

	// Helper functions
	g.generateHelpers(f)
	f.Line()

	// Compile methods and generate dispatch
	compiled := g.compileMethods()
	g.generateDispatch(f, compiled)
	f.Line()

	// Generate method implementations
	for _, m := range compiled {
		g.generateMethod(f, m)
	}

	// Render to string
	buf := &bytes.Buffer{}
	if err := f.Render(buf); err != nil {
		return &Result{
			Code:           fmt.Sprintf("// Error rendering: %v", err),
			Warnings:       g.warnings,
			SkippedMethods: g.skipped,
		}
	}

	return &Result{
		Code:           buf.String(),
		Warnings:       g.warnings,
		SkippedMethods: g.skipped,
	}
}

func (g *generator) generateStruct(f *jen.File) {
	fields := []jen.Code{
		jen.Id("Class").String().Tag(map[string]string{"json": "class"}),
		jen.Id("CreatedAt").String().Tag(map[string]string{"json": "created_at"}),
		jen.Id("Vars").Index().String().Tag(map[string]string{"json": "_vars"}),
	}

	for _, iv := range g.class.InstanceVars {
		goName := capitalize(iv.Name)
		goType := g.inferType(iv)
		fields = append(fields, jen.Id(goName).Add(goType).Tag(map[string]string{"json": iv.Name}))
	}

	f.Type().Id(g.class.Name).Struct(fields...)
}

func (g *generator) inferType(iv ast.InstanceVar) *jen.Statement {
	if iv.Default.Type == "number" {
		return jen.Int()
	}
	return jen.String()
}

func (g *generator) generateMain(f *jen.File) {
	className := g.class.Name

	f.Func().Id("main").Params().Block(
		// Check for minimum args
		jen.If(jen.Len(jen.Qual("os", "Args")).Op("<").Lit(2)).Block(
			jen.Qual("fmt", "Fprintln").Call(jen.Qual("os", "Stderr"), jen.Lit("Usage: "+className+".native <instance_id> <selector> [args...]")),
			jen.Qual("fmt", "Fprintln").Call(jen.Qual("os", "Stderr"), jen.Lit("       "+className+".native --source")),
			jen.Qual("fmt", "Fprintln").Call(jen.Qual("os", "Stderr"), jen.Lit("       "+className+".native --hash")),
			jen.Qual("os", "Exit").Call(jen.Lit(1)),
		),
		jen.Line(),

		// Handle metadata commands
		jen.Switch(jen.Qual("os", "Args").Index(jen.Lit(1))).Block(
			jen.Case(jen.Lit("--source")).Block(
				jen.Qual("fmt", "Print").Call(jen.Id("_sourceCode")),
				jen.Return(),
			),
			jen.Case(jen.Lit("--hash")).Block(
				jen.Qual("fmt", "Println").Call(jen.Id("_contentHash")),
				jen.Return(),
			),
			jen.Case(jen.Lit("--info")).Block(
				jen.Qual("fmt", "Printf").Call(jen.Lit("Class: "+className+"\nHash: %s\nSource length: %d bytes\n"), jen.Id("_contentHash"), jen.Len(jen.Id("_sourceCode"))),
				jen.Return(),
			),
		),
		jen.Line(),

		// Check for selector arg
		jen.If(jen.Len(jen.Qual("os", "Args")).Op("<").Lit(3)).Block(
			jen.Qual("fmt", "Fprintln").Call(jen.Qual("os", "Stderr"), jen.Lit("Usage: "+className+".native <instance_id> <selector> [args...]")),
			jen.Qual("os", "Exit").Call(jen.Lit(1)),
		),
		jen.Line(),

		// Parse args
		jen.Id("receiver").Op(":=").Qual("os", "Args").Index(jen.Lit(1)),
		jen.Id("selector").Op(":=").Qual("os", "Args").Index(jen.Lit(2)),
		jen.Id("args").Op(":=").Qual("os", "Args").Index(jen.Lit(3).Op(":")),
		jen.Line(),

		// Open database
		jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Qual("fmt", "Fprintf").Call(jen.Qual("os", "Stderr"), jen.Lit("Error opening database: %v\n"), jen.Err()),
			jen.Qual("os", "Exit").Call(jen.Lit(1)),
		),
		jen.Defer().Id("db").Dot("Close").Call(),
		jen.Line(),

		// Load instance
		jen.List(jen.Id("instance"), jen.Err()).Op(":=").Id("loadInstance").Call(jen.Id("db"), jen.Id("receiver")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Qual("os", "Exit").Call(jen.Lit(200)),
		),
		jen.Line(),

		// Dispatch
		jen.List(jen.Id("result"), jen.Err()).Op(":=").Id("dispatch").Call(jen.Id("instance"), jen.Id("selector"), jen.Id("args")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.If(jen.Qual("errors", "Is").Call(jen.Err(), jen.Id("ErrUnknownSelector"))).Block(
				jen.Qual("os", "Exit").Call(jen.Lit(200)),
			),
			jen.Qual("fmt", "Fprintf").Call(jen.Qual("os", "Stderr"), jen.Lit("Error: %v\n"), jen.Err()),
			jen.Qual("os", "Exit").Call(jen.Lit(1)),
		),
		jen.Line(),

		// Save instance
		jen.If(jen.Err().Op(":=").Id("saveInstance").Call(jen.Id("db"), jen.Id("receiver"), jen.Id("instance")), jen.Err().Op("!=").Nil()).Block(
			jen.Qual("fmt", "Fprintf").Call(jen.Qual("os", "Stderr"), jen.Lit("Error saving instance: %v\n"), jen.Err()),
			jen.Qual("os", "Exit").Call(jen.Lit(1)),
		),
		jen.Line(),

		// Print result
		jen.If(jen.Id("result").Op("!=").Lit("")).Block(
			jen.Qual("fmt", "Println").Call(jen.Id("result")),
		),
	)
}

func (g *generator) generateHelpers(f *jen.File) {
	className := g.class.Name

	// openDB
	f.Func().Id("openDB").Params().Parens(jen.List(jen.Op("*").Qual("database/sql", "DB"), jen.Error())).Block(
		jen.Id("dbPath").Op(":=").Qual("os", "Getenv").Call(jen.Lit("SQLITE_JSON_DB")),
		jen.If(jen.Id("dbPath").Op("==").Lit("")).Block(
			jen.List(jen.Id("home"), jen.Id("_")).Op(":=").Qual("os", "UserHomeDir").Call(),
			jen.Id("dbPath").Op("=").Qual("path/filepath", "Join").Call(jen.Id("home"), jen.Lit(".trashtalk"), jen.Lit("instances.db")),
		),
		jen.Return(jen.Qual("database/sql", "Open").Call(jen.Lit("sqlite3"), jen.Id("dbPath"))),
	)
	f.Line()

	// loadInstance
	f.Func().Id("loadInstance").Params(
		jen.Id("db").Op("*").Qual("database/sql", "DB"),
		jen.Id("id").String(),
	).Parens(jen.List(jen.Op("*").Id(className), jen.Error())).Block(
		jen.Var().Id("data").String(),
		jen.Err().Op(":=").Id("db").Dot("QueryRow").Call(jen.Lit("SELECT data FROM instances WHERE id = ?"), jen.Id("id")).Dot("Scan").Call(jen.Op("&").Id("data")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Nil(), jen.Err()),
		),
		jen.Var().Id("instance").Id(className),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(jen.Index().Byte().Parens(jen.Id("data")), jen.Op("&").Id("instance")), jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Nil(), jen.Err()),
		),
		jen.Return(jen.Op("&").Id("instance"), jen.Nil()),
	)
	f.Line()

	// saveInstance
	f.Func().Id("saveInstance").Params(
		jen.Id("db").Op("*").Qual("database/sql", "DB"),
		jen.Id("id").String(),
		jen.Id("instance").Op("*").Id(className),
	).Error().Block(
		jen.List(jen.Id("data"), jen.Err()).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("instance")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Err()),
		),
		jen.List(jen.Id("_"), jen.Err()).Op("=").Id("db").Dot("Exec").Call(jen.Lit("INSERT OR REPLACE INTO instances (id, data) VALUES (?, json(?))"), jen.Id("id"), jen.String().Parens(jen.Id("data"))),
		jen.Return(jen.Err()),
	)
}

func (g *generator) compileMethods() []*compiledMethod {
	var compiled []*compiledMethod

	for _, m := range g.class.Methods {
		// Skip class methods for now
		if m.Kind == "class" {
			g.skipped = append(g.skipped, SkippedMethod{
				Selector: m.Selector,
				Reason:   "class methods not yet supported",
			})
			continue
		}

		// Skip raw methods
		if m.Raw {
			g.skipped = append(g.skipped, SkippedMethod{
				Selector: m.Selector,
				Reason:   "raw method",
			})
			continue
		}

		// Parse the method body
		result := parser.ParseMethod(m.Body.Tokens)
		if result.Unsupported {
			g.skipped = append(g.skipped, SkippedMethod{
				Selector: m.Selector,
				Reason:   result.Reason,
			})
			continue
		}

		// Check if method has return
		hasReturn := false
		for _, stmt := range result.Body.Statements {
			if _, ok := stmt.(*parser.Return); ok {
				hasReturn = true
				break
			}
		}

		// Check if any args require error handling (string to int conversion)
		returnsErr := len(m.Args) > 0

		compiled = append(compiled, &compiledMethod{
			selector:   m.Selector,
			goName:     selectorToGoName(m.Selector),
			args:       m.Args,
			body:       result.Body,
			hasReturn:  hasReturn,
			isClass:    m.Kind == "class",
			returnsErr: returnsErr,
		})
	}

	return compiled
}

func (g *generator) generateDispatch(f *jen.File, methods []*compiledMethod) {
	className := g.class.Name

	cases := []jen.Code{}
	for _, m := range methods {
		var callExpr *jen.Statement
		if len(m.args) > 0 {
			// Check args length
			argCheck := jen.If(jen.Len(jen.Id("args")).Op("<").Lit(len(m.args))).Block(
				jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit(m.selector+" requires "+fmt.Sprintf("%d", len(m.args))+" argument"))),
			)

			// Build call with args
			callArgs := []jen.Code{}
			for i := range m.args {
				callArgs = append(callArgs, jen.Id("args").Index(jen.Lit(i)))
			}
			callExpr = jen.Id("c").Dot(m.goName).Call(callArgs...)

			if m.returnsErr {
				cases = append(cases, jen.Case(jen.Lit(m.selector)).Block(
					argCheck,
					jen.Return(callExpr),
				))
			} else {
				cases = append(cases, jen.Case(jen.Lit(m.selector)).Block(
					argCheck,
					jen.Return(callExpr, jen.Nil()),
				))
			}
		} else {
			callExpr = jen.Id("c").Dot(m.goName).Call()
			if m.hasReturn {
				cases = append(cases, jen.Case(jen.Lit(m.selector)).Block(
					jen.Return(callExpr, jen.Nil()),
				))
			} else {
				cases = append(cases, jen.Case(jen.Lit(m.selector)).Block(
					callExpr,
					jen.Return(jen.Lit(""), jen.Nil()),
				))
			}
		}
	}

	// Default case
	cases = append(cases, jen.Default().Block(
		jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("%w: %s"), jen.Id("ErrUnknownSelector"), jen.Id("selector"))),
	))

	f.Func().Id("dispatch").Params(
		jen.Id("c").Op("*").Id(className),
		jen.Id("selector").String(),
		jen.Id("args").Index().String(),
	).Parens(jen.List(jen.String(), jen.Error())).Block(
		jen.Switch(jen.Id("selector")).Block(cases...),
	)
}

func (g *generator) generateMethod(f *jen.File, m *compiledMethod) {
	className := g.class.Name

	// Build parameter list
	params := []jen.Code{}
	for _, arg := range m.args {
		params = append(params, jen.Id(arg).String())
	}

	// Determine return type
	var returnType *jen.Statement
	if m.returnsErr {
		returnType = jen.Parens(jen.List(jen.String(), jen.Error()))
	} else if m.hasReturn {
		returnType = jen.String()
	}

	// Generate body
	body := g.generateMethodBody(m)

	if returnType != nil {
		f.Func().Parens(jen.Id("c").Op("*").Id(className)).Id(m.goName).Params(params...).Add(returnType).Block(body...)
	} else {
		f.Func().Parens(jen.Id("c").Op("*").Id(className)).Id(m.goName).Params(params...).Block(body...)
	}
	f.Line()
}

func (g *generator) generateMethodBody(m *compiledMethod) []jen.Code {
	var stmts []jen.Code

	// Convert string args to int if needed
	for _, arg := range m.args {
		localVar := arg[0:1] // First letter as variable name (simple approach)
		stmts = append(stmts,
			jen.List(jen.Id(localVar), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id(arg)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
		)
	}

	// Local variables
	for _, v := range m.body.LocalVars {
		stmts = append(stmts, jen.Var().Id(v).Int())
	}

	// Statements
	for _, stmt := range m.body.Statements {
		stmts = append(stmts, g.generateStatement(stmt, m)...)
	}

	// Add implicit return for methods that don't have explicit return
	if m.returnsErr && !m.hasReturn {
		stmts = append(stmts, jen.Return(jen.Lit(""), jen.Nil()))
	}

	return stmts
}

func (g *generator) generateStatement(stmt parser.Statement, m *compiledMethod) []jen.Code {
	switch s := stmt.(type) {
	case *parser.Assignment:
		expr := g.generateExpr(s.Value, m)
		target := s.Target
		// Check if it's an instance variable
		if g.instanceVars[target] {
			return []jen.Code{jen.Id("c").Dot(capitalize(target)).Op("=").Add(expr)}
		}
		return []jen.Code{jen.Id(target).Op("=").Add(expr)}

	case *parser.Return:
		expr := g.generateExpr(s.Value, m)
		if m.returnsErr {
			return []jen.Code{jen.Return(jen.Qual("strconv", "Itoa").Call(expr), jen.Nil())}
		}
		return []jen.Code{jen.Return(jen.Qual("strconv", "Itoa").Call(expr))}

	case *parser.ExprStmt:
		return []jen.Code{g.generateExpr(s.Expr, m)}

	default:
		return []jen.Code{jen.Comment("unknown statement")}
	}
}

func (g *generator) generateExpr(expr parser.Expr, m *compiledMethod) *jen.Statement {
	switch e := expr.(type) {
	case *parser.BinaryExpr:
		left := g.generateExpr(e.Left, m)
		right := g.generateExpr(e.Right, m)
		switch e.Op {
		case "+":
			return left.Op("+").Add(right)
		case "-":
			return left.Op("-").Add(right)
		case "*":
			return left.Op("*").Add(right)
		case "/":
			return left.Op("/").Add(right)
		}
		return jen.Comment("unknown op: " + e.Op)

	case *parser.Identifier:
		name := e.Name
		// Check if it's an instance variable
		if g.instanceVars[name] {
			return jen.Id("c").Dot(capitalize(name))
		}
		// Check if it's a method arg (use converted local var)
		for _, arg := range m.args {
			if arg == name {
				return jen.Id(name[0:1]) // Use first letter as converted var
			}
		}
		return jen.Id(name)

	case *parser.NumberLit:
		return jen.Lit(mustAtoi(e.Value))

	case *parser.StringLit:
		return jen.Lit(e.Value)

	default:
		return jen.Comment("unknown expr")
	}
}

// Helper functions

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[0:1]) + s[1:]
}

func selectorToGoName(selector string) string {
	// Remove trailing underscore and capitalize
	name := strings.TrimSuffix(selector, "_")
	return capitalize(name)
}

func mustAtoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
