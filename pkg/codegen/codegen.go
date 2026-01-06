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

	// Note if class has traits (they fall back to Bash for now)
	if len(g.class.Traits) > 0 {
		g.warnings = append(g.warnings,
			fmt.Sprintf("class includes %d trait(s): %v - trait methods fall back to Bash",
				len(g.class.Traits), g.class.Traits))
	}

	// Add blank imports for embed and sqlite3
	f.Anon("embed")
	f.Anon("github.com/mattn/go-sqlite3")

	// Embed directive and source hash
	// Use CompiledName for namespaced classes (MyApp__Counter.trash)
	f.Comment("//go:embed " + g.class.CompiledName() + ".trash")
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

	// Compile methods and separate into class/instance
	compiled := g.compileMethods()

	// Split into class and instance methods
	var instanceMethods, classMethods []*compiledMethod
	for _, m := range compiled {
		if m.isClass {
			classMethods = append(classMethods, m)
		} else {
			instanceMethods = append(instanceMethods, m)
		}
	}

	// Generate dispatch functions
	g.generateDispatch(f, instanceMethods)
	f.Line()
	g.generateClassDispatch(f, classMethods)
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
	compiledName := g.class.CompiledName()
	qualifiedName := g.class.QualifiedName()

	// Build --info format string based on whether class is namespaced
	var infoFormat string
	if g.class.IsNamespaced() {
		infoFormat = "Class: " + qualifiedName + "\nPackage: " + g.class.Package + "\nHash: %s\nSource length: %d bytes\n"
	} else {
		infoFormat = "Class: " + className + "\nHash: %s\nSource length: %d bytes\n"
	}

	f.Func().Id("main").Params().Block(
		// Check for minimum args
		jen.If(jen.Len(jen.Qual("os", "Args")).Op("<").Lit(2)).Block(
			jen.Qual("fmt", "Fprintln").Call(jen.Qual("os", "Stderr"), jen.Lit("Usage: "+compiledName+".native <instance_id> <selector> [args...]")),
			jen.Qual("fmt", "Fprintln").Call(jen.Qual("os", "Stderr"), jen.Lit("       "+compiledName+".native --source")),
			jen.Qual("fmt", "Fprintln").Call(jen.Qual("os", "Stderr"), jen.Lit("       "+compiledName+".native --hash")),
			jen.Qual("os", "Exit").Call(jen.Lit(1)),
		),
		jen.Line(),

		// Handle metadata commands and serve mode
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
				jen.Qual("fmt", "Printf").Call(jen.Lit(infoFormat), jen.Id("_contentHash"), jen.Len(jen.Id("_sourceCode"))),
				jen.Return(),
			),
			jen.Case(jen.Lit("--serve")).Block(
				jen.Id("runServeMode").Call(),
				jen.Return(),
			),
		),
		jen.Line(),

		// Check for selector arg
		jen.If(jen.Len(jen.Qual("os", "Args")).Op("<").Lit(3)).Block(
			jen.Qual("fmt", "Fprintln").Call(jen.Qual("os", "Stderr"), jen.Lit("Usage: "+compiledName+".native <instance_id> <selector> [args...]")),
			jen.Qual("os", "Exit").Call(jen.Lit(1)),
		),
		jen.Line(),

		// Parse args
		jen.Id("receiver").Op(":=").Qual("os", "Args").Index(jen.Lit(1)),
		jen.Id("selector").Op(":=").Qual("os", "Args").Index(jen.Lit(2)),
		jen.Id("args").Op(":=").Qual("os", "Args").Index(jen.Lit(3).Op(":")),
		jen.Line(),

		// Check for class method call (receiver is the class name)
		jen.If(jen.Id("receiver").Op("==").Lit(className).Op("||").Id("receiver").Op("==").Lit(qualifiedName)).Block(
			jen.List(jen.Id("result"), jen.Err()).Op(":=").Id("dispatchClass").Call(jen.Id("selector"), jen.Id("args")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.If(jen.Qual("errors", "Is").Call(jen.Err(), jen.Id("ErrUnknownSelector"))).Block(
					jen.Qual("os", "Exit").Call(jen.Lit(200)),
				),
				jen.Qual("fmt", "Fprintf").Call(jen.Qual("os", "Stderr"), jen.Lit("Error: %v\n"), jen.Err()),
				jen.Qual("os", "Exit").Call(jen.Lit(1)),
			),
			jen.If(jen.Id("result").Op("!=").Lit("")).Block(
				jen.Qual("fmt", "Println").Call(jen.Id("result")),
			),
			jen.Return(),
		),
		jen.Line(),

		// Instance method call - open database
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

		// Dispatch to instance method
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
	f.Line()

	// sendMessage - shell out to bash runtime for non-self message sends
	f.Func().Id("sendMessage").Params(
		jen.Id("receiver").Interface(),
		jen.Id("selector").String(),
		jen.Id("args").Op("...").Interface(),
	).Parens(jen.List(jen.String(), jen.Error())).Block(
		// Convert receiver to string
		jen.Id("receiverStr").Op(":=").Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("receiver")),
		// Build command args: @ receiver selector args...
		jen.Id("cmdArgs").Op(":=").Index().String().Values(jen.Id("receiverStr"), jen.Id("selector")),
		jen.For(jen.List(jen.Id("_"), jen.Id("arg")).Op(":=").Range().Id("args")).Block(
			jen.Id("cmdArgs").Op("=").Append(jen.Id("cmdArgs"), jen.Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("arg"))),
		),
		// Find the trashtalk dispatch script
		jen.List(jen.Id("home"), jen.Id("_")).Op(":=").Qual("os", "UserHomeDir").Call(),
		jen.Id("dispatchScript").Op(":=").Qual("path/filepath", "Join").Call(jen.Id("home"), jen.Lit(".trashtalk"), jen.Lit("bin"), jen.Lit("trash-send")),
		// Execute: trash-send receiver selector args...
		jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Id("dispatchScript"), jen.Id("cmdArgs").Op("...")),
		jen.List(jen.Id("output"), jen.Err()).Op(":=").Id("cmd").Dot("Output").Call(),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Err()),
		),
		jen.Return(jen.Qual("strings", "TrimSpace").Call(jen.String().Parens(jen.Id("output"))), jen.Nil()),
	)
	f.Line()

	// runServeMode - daemon mode that reads JSON requests from stdin
	g.generateServeMode(f)
}

// generateServeMode generates the daemon loop that reads JSON from stdin
func (g *generator) generateServeMode(f *jen.File) {
	className := g.class.Name
	qualifiedName := g.class.QualifiedName()

	// Request/Response structs
	f.Comment("// ServeRequest is the JSON request format for --serve mode")
	f.Type().Id("ServeRequest").Struct(
		jen.Id("Instance").String().Tag(map[string]string{"json": "instance"}),
		jen.Id("Selector").String().Tag(map[string]string{"json": "selector"}),
		jen.Id("Args").Index().String().Tag(map[string]string{"json": "args"}),
	)
	f.Line()

	f.Comment("// ServeResponse is the JSON response format for --serve mode")
	f.Type().Id("ServeResponse").Struct(
		jen.Id("Instance").String().Tag(map[string]string{"json": "instance,omitempty"}),
		jen.Id("Result").String().Tag(map[string]string{"json": "result,omitempty"}),
		jen.Id("ExitCode").Int().Tag(map[string]string{"json": "exit_code"}),
		jen.Id("Error").String().Tag(map[string]string{"json": "error,omitempty"}),
	)
	f.Line()

	f.Func().Id("runServeMode").Params().Block(
		// Open database once for all requests
		jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Qual("fmt", "Fprintf").Call(jen.Qual("os", "Stderr"), jen.Lit("Error opening database: %v\n"), jen.Err()),
			jen.Qual("os", "Exit").Call(jen.Lit(1)),
		),
		jen.Defer().Id("db").Dot("Close").Call(),
		jen.Line(),

		// Scanner for reading lines from stdin
		jen.Id("scanner").Op(":=").Qual("bufio", "NewScanner").Call(jen.Qual("os", "Stdin")),
		jen.Comment("// Increase buffer for large instance JSON"),
		jen.Id("buf").Op(":=").Make(jen.Index().Byte(), jen.Lit(1024*1024)),
		jen.Id("scanner").Dot("Buffer").Call(jen.Id("buf"), jen.Len(jen.Id("buf"))),
		jen.Line(),

		// Main loop
		jen.For(jen.Id("scanner").Dot("Scan").Call()).Block(
			jen.Id("line").Op(":=").Id("scanner").Dot("Text").Call(),
			jen.If(jen.Id("line").Op("==").Lit("")).Block(jen.Continue()),
			jen.Line(),

			jen.Var().Id("req").Id("ServeRequest"),
			jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(jen.Id("line")),
				jen.Op("&").Id("req"),
			).Op(";").Err().Op("!=").Nil()).Block(
				jen.Id("respond").Call(jen.Id("ServeResponse").Values(jen.Dict{
					jen.Id("ExitCode"): jen.Lit(1),
					jen.Id("Error"):    jen.Lit("invalid JSON: ").Op("+").Err().Dot("Error").Call(),
				})),
				jen.Continue(),
			),
			jen.Line(),

			jen.Id("resp").Op(":=").Id("handleServeRequest").Call(jen.Id("db"), jen.Op("&").Id("req")),
			jen.Id("respond").Call(jen.Id("resp")),
		),
	)
	f.Line()

	// respond helper
	f.Func().Id("respond").Params(jen.Id("resp").Id("ServeResponse")).Block(
		jen.List(jen.Id("out"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("resp")),
		jen.Qual("fmt", "Println").Call(jen.String().Parens(jen.Id("out"))),
	)
	f.Line()

	// handleServeRequest - dispatch a single request
	f.Func().Id("handleServeRequest").Params(
		jen.Id("db").Op("*").Qual("database/sql", "DB"),
		jen.Id("req").Op("*").Id("ServeRequest"),
	).Id("ServeResponse").Block(
		// Check for class method call (empty instance or class name)
		jen.If(jen.Id("req").Dot("Instance").Op("==").Lit("").Op("||").
			Id("req").Dot("Instance").Op("==").Lit(className).Op("||").
			Id("req").Dot("Instance").Op("==").Lit(qualifiedName)).Block(
			jen.List(jen.Id("result"), jen.Err()).Op(":=").Id("dispatchClass").Call(
				jen.Id("req").Dot("Selector"),
				jen.Id("req").Dot("Args"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.If(jen.Qual("errors", "Is").Call(jen.Err(), jen.Id("ErrUnknownSelector"))).Block(
					jen.Return(jen.Id("ServeResponse").Values(jen.Dict{jen.Id("ExitCode"): jen.Lit(200)})),
				),
				jen.Return(jen.Id("ServeResponse").Values(jen.Dict{
					jen.Id("ExitCode"): jen.Lit(1),
					jen.Id("Error"):    jen.Err().Dot("Error").Call(),
				})),
			),
			jen.Return(jen.Id("ServeResponse").Values(jen.Dict{
				jen.Id("Result"):   jen.Id("result"),
				jen.Id("ExitCode"): jen.Lit(0),
			})),
		),
		jen.Line(),

		// Instance method - parse instance from JSON
		jen.Var().Id("instance").Id(className),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("req").Dot("Instance")),
			jen.Op("&").Id("instance"),
		).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.Id("ServeResponse").Values(jen.Dict{
				jen.Id("ExitCode"): jen.Lit(1),
				jen.Id("Error"):    jen.Lit("invalid instance JSON: ").Op("+").Err().Dot("Error").Call(),
			})),
		),
		jen.Line(),

		// Dispatch to instance method
		jen.List(jen.Id("result"), jen.Err()).Op(":=").Id("dispatch").Call(
			jen.Op("&").Id("instance"),
			jen.Id("req").Dot("Selector"),
			jen.Id("req").Dot("Args"),
		),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.If(jen.Qual("errors", "Is").Call(jen.Err(), jen.Id("ErrUnknownSelector"))).Block(
				jen.Return(jen.Id("ServeResponse").Values(jen.Dict{jen.Id("ExitCode"): jen.Lit(200)})),
			),
			jen.Return(jen.Id("ServeResponse").Values(jen.Dict{
				jen.Id("ExitCode"): jen.Lit(1),
				jen.Id("Error"):    jen.Err().Dot("Error").Call(),
			})),
		),
		jen.Line(),

		// Return updated instance + result
		jen.List(jen.Id("updatedJSON"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Op("&").Id("instance")),
		jen.Return(jen.Id("ServeResponse").Values(jen.Dict{
			jen.Id("Instance"): jen.String().Parens(jen.Id("updatedJSON")),
			jen.Id("Result"):   jen.Id("result"),
			jen.Id("ExitCode"): jen.Lit(0),
		})),
	)
}

func (g *generator) compileMethods() []*compiledMethod {
	var compiled []*compiledMethod

	for _, m := range g.class.Methods {
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

func (g *generator) generateClassDispatch(f *jen.File, methods []*compiledMethod) {
	cases := []jen.Code{}
	for _, m := range methods {
		var callExpr *jen.Statement
		if len(m.args) > 0 {
			// Check args length
			argCheck := jen.If(jen.Len(jen.Id("args")).Op("<").Lit(len(m.args))).Block(
				jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit(m.selector+" requires "+fmt.Sprintf("%d", len(m.args))+" argument"))),
			)

			// Build call with args - class methods are package-level functions
			callArgs := []jen.Code{}
			for i := range m.args {
				callArgs = append(callArgs, jen.Id("args").Index(jen.Lit(i)))
			}
			callExpr = jen.Id(m.goName).Call(callArgs...)

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
			// No args - direct call to package-level function
			callExpr = jen.Id(m.goName).Call()
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

	// dispatchClass takes no instance receiver
	f.Func().Id("dispatchClass").Params(
		jen.Id("selector").String(),
		jen.Id("args").Index().String(),
	).Parens(jen.List(jen.String(), jen.Error())).Block(
		jen.Switch(jen.Id("selector")).Block(cases...),
	)
}

func (g *generator) generateMethod(f *jen.File, m *compiledMethod) {
	className := g.class.Name

	// Special handling for Environment class - generate SQLite-based storage methods
	if g.class.Name == "Environment" && m.isClass {
		g.generateEnvironmentMethod(f, m)
		return
	}

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

	if m.isClass {
		// Class methods are package-level functions (no receiver)
		if returnType != nil {
			f.Func().Id(m.goName).Params(params...).Add(returnType).Block(body...)
		} else {
			f.Func().Id(m.goName).Params(params...).Block(body...)
		}
	} else {
		// Instance methods have receiver
		if returnType != nil {
			f.Func().Parens(jen.Id("c").Op("*").Id(className)).Id(m.goName).Params(params...).Add(returnType).Block(body...)
		} else {
			f.Func().Parens(jen.Id("c").Op("*").Id(className)).Id(m.goName).Params(params...).Block(body...)
		}
	}
	f.Line()
}

func (g *generator) generateMethodBody(m *compiledMethod) []jen.Code {
	var stmts []jen.Code

	// Convert string args to int if needed
	// We use argName + "Int" to avoid shadowing the original parameter
	// Also add _ = xInt to suppress "declared and not used" if arg is only passed to other methods
	for _, arg := range m.args {
		intVar := arg + "Int"
		stmts = append(stmts,
			jen.List(jen.Id(intVar), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id(arg)),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Id("_").Op("=").Id(intVar), // Suppress unused warning
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
		// Check if the return value is already a string (message sends, string literals)
		_, isMessageSend := s.Value.(*parser.MessageSend)
		_, isStringLit := s.Value.(*parser.StringLit)
		if isMessageSend || isStringLit {
			// Already a string, no conversion needed
			if m.returnsErr {
				return []jen.Code{jen.Return(expr, jen.Nil())}
			}
			return []jen.Code{jen.Return(expr)}
		}
		// Numeric values need conversion
		if m.returnsErr {
			return []jen.Code{jen.Return(jen.Qual("strconv", "Itoa").Call(expr), jen.Nil())}
		}
		return []jen.Code{jen.Return(jen.Qual("strconv", "Itoa").Call(expr))}

	case *parser.ExprStmt:
		return []jen.Code{g.generateExpr(s.Expr, m)}

	case *parser.IfExpr:
		return g.generateIfStatement(s, m)

	case *parser.WhileExpr:
		return g.generateWhileStatement(s, m)

	default:
		return []jen.Code{jen.Comment("unknown statement")}
	}
}

// generateIfStatement generates Go if/else from Trashtalk ifTrue:/ifFalse:
func (g *generator) generateIfStatement(s *parser.IfExpr, m *compiledMethod) []jen.Code {
	condition := g.generateExpr(s.Condition, m)

	// Generate true block statements
	var trueStmts []jen.Code
	for _, stmt := range s.TrueBlock {
		trueStmts = append(trueStmts, g.generateStatement(stmt, m)...)
	}

	// Generate false block statements if present
	var falseStmts []jen.Code
	for _, stmt := range s.FalseBlock {
		falseStmts = append(falseStmts, g.generateStatement(stmt, m)...)
	}

	// Build the if statement
	if len(s.TrueBlock) > 0 && len(s.FalseBlock) > 0 {
		// ifTrue: [true] ifFalse: [false]
		return []jen.Code{
			jen.If(condition).Block(trueStmts...).Else().Block(falseStmts...),
		}
	} else if len(s.TrueBlock) > 0 {
		// ifTrue: [true] only
		return []jen.Code{
			jen.If(condition).Block(trueStmts...),
		}
	} else if len(s.FalseBlock) > 0 {
		// ifFalse: [false] only - negate condition
		return []jen.Code{
			jen.If(jen.Op("!").Parens(condition)).Block(falseStmts...),
		}
	}

	return []jen.Code{jen.Comment("empty if statement")}
}

// generateWhileStatement generates Go for loop from Trashtalk whileTrue:
func (g *generator) generateWhileStatement(s *parser.WhileExpr, m *compiledMethod) []jen.Code {
	condition := g.generateExpr(s.Condition, m)

	// Generate body statements
	var bodyStmts []jen.Code
	for _, stmt := range s.Body {
		bodyStmts = append(bodyStmts, g.generateStatement(stmt, m)...)
	}

	// Go's "while" is just "for condition"
	return []jen.Code{
		jen.For(condition).Block(bodyStmts...),
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

	case *parser.ComparisonExpr:
		left := g.generateExpr(e.Left, m)
		right := g.generateExpr(e.Right, m)
		return left.Op(e.Op).Add(right)

	case *parser.Identifier:
		name := e.Name
		// Check if it's an instance variable
		if g.instanceVars[name] {
			return jen.Id("c").Dot(capitalize(name))
		}
		// Check if it's a method arg (use converted int var)
		for _, arg := range m.args {
			if arg == name {
				return jen.Id(name + "Int") // Use argName + "Int" for converted int
			}
		}
		return jen.Id(name)

	case *parser.NumberLit:
		return jen.Lit(mustAtoi(e.Value))

	case *parser.StringLit:
		return jen.Lit(e.Value)

	case *parser.MessageSend:
		if e.IsSelf {
			// Self send: direct Go method call
			goMethodName := selectorToGoName(e.Selector)
			if len(e.Args) == 0 {
				return jen.Id("c").Dot(goMethodName).Call()
			}
			// Build args - Go methods take string params
			args := []jen.Code{}
			for _, arg := range e.Args {
				// Check if the arg is a method parameter (already a string)
				if ident, ok := arg.(*parser.Identifier); ok {
					isMethodArg := false
					for _, methodArg := range m.args {
						if methodArg == ident.Name {
							isMethodArg = true
							break
						}
					}
					if isMethodArg {
						// Use original string parameter directly
						args = append(args, jen.Id(ident.Name))
						continue
					}
				}
				// For other args, generate and convert if needed
				argExpr := g.generateExpr(arg, m)
				// Wrap numeric literals in strconv.Itoa
				if _, ok := arg.(*parser.NumberLit); ok {
					argExpr = jen.Qual("strconv", "Itoa").Call(argExpr)
				}
				args = append(args, argExpr)
			}
			return jen.Id("c").Dot(goMethodName).Call(args...)
		}
		// Non-self send: shell out to bash runtime
		// Generate: sendMessage(receiver, selector, args...)
		receiverExpr := g.generateExpr(e.Receiver, m)
		args := []jen.Code{
			receiverExpr,
			jen.Lit(e.Selector),
		}
		for _, arg := range e.Args {
			args = append(args, g.generateExpr(arg, m))
		}
		return jen.Id("sendMessage").Call(args...)

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

// generateEnvironmentMethod generates specialized SQLite-based implementations
// for the Environment class storage methods.
func (g *generator) generateEnvironmentMethod(f *jen.File, m *compiledMethod) {
	switch m.selector {
	case "get_":
		// Get(instanceId string) (string, error) - retrieve instance data
		f.Func().Id("Get").Params(jen.Id("instanceId").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.Var().Id("data").String(),
			jen.Err().Op("=").Id("db").Dot("QueryRow").Call(
				jen.Lit("SELECT data FROM instances WHERE id = ?"),
				jen.Id("instanceId"),
			).Dot("Scan").Call(jen.Op("&").Id("data")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Nil()), // Return empty string if not found
			),
			jen.Return(jen.Id("data"), jen.Nil()),
		)

	case "set_to_":
		// Set_to(instanceId, data string) (string, error) - store instance data
		f.Func().Id("Set_to").Params(
			jen.Id("instanceId").String(),
			jen.Id("data").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.List(jen.Id("_"), jen.Err()).Op("=").Id("db").Dot("Exec").Call(
				jen.Lit("INSERT OR REPLACE INTO instances (id, data) VALUES (?, json(?))"),
				jen.Id("instanceId"),
				jen.Id("data"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Id("instanceId"), jen.Nil()),
		)

	case "delete_":
		// Delete(instanceId string) (string, error) - remove instance
		f.Func().Id("Delete").Params(jen.Id("instanceId").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.List(jen.Id("_"), jen.Err()).Op("=").Id("db").Dot("Exec").Call(
				jen.Lit("DELETE FROM instances WHERE id = ?"),
				jen.Id("instanceId"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Lit(""), jen.Nil()),
		)

	case "findByClass_":
		// FindByClass(className string) (string, error) - find all instances of class
		f.Func().Id("FindByClass").Params(jen.Id("className").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.List(jen.Id("rows"), jen.Err()).Op(":=").Id("db").Dot("Query").Call(
				jen.Lit("SELECT id FROM instances WHERE class = ?"),
				jen.Id("className"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("rows").Dot("Close").Call(),
			jen.Line(),
			jen.Var().Id("ids").Index().String(),
			jen.For(jen.Id("rows").Dot("Next").Call()).Block(
				jen.Var().Id("id").String(),
				jen.If(jen.Err().Op(":=").Id("rows").Dot("Scan").Call(jen.Op("&").Id("id")).Op(";").Err().Op("==").Nil()).Block(
					jen.Id("ids").Op("=").Append(jen.Id("ids"), jen.Id("id")),
				),
			),
			jen.Return(jen.Qual("strings", "Join").Call(jen.Id("ids"), jen.Lit("\n")), jen.Nil()),
		)

	case "exists_":
		// Exists(instanceId string) (string, error) - check if instance exists
		f.Func().Id("Exists").Params(jen.Id("instanceId").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.Var().Id("exists").Int(),
			jen.Err().Op("=").Id("db").Dot("QueryRow").Call(
				jen.Lit("SELECT 1 FROM instances WHERE id = ?"),
				jen.Id("instanceId"),
			).Dot("Scan").Call(jen.Op("&").Id("exists")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("0"), jen.Nil()),
			),
			jen.Return(jen.Lit("1"), jen.Nil()),
		)

	case "listAll":
		// ListAll() string - get all instance IDs
		f.Func().Id("ListAll").Params().String().Block(
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("")),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.List(jen.Id("rows"), jen.Err()).Op(":=").Id("db").Dot("Query").Call(
				jen.Lit("SELECT id FROM instances"),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("")),
			),
			jen.Defer().Id("rows").Dot("Close").Call(),
			jen.Line(),
			jen.Var().Id("ids").Index().String(),
			jen.For(jen.Id("rows").Dot("Next").Call()).Block(
				jen.Var().Id("id").String(),
				jen.If(jen.Err().Op(":=").Id("rows").Dot("Scan").Call(jen.Op("&").Id("id")).Op(";").Err().Op("==").Nil()).Block(
					jen.Id("ids").Op("=").Append(jen.Id("ids"), jen.Id("id")),
				),
			),
			jen.Return(jen.Qual("strings", "Join").Call(jen.Id("ids"), jen.Lit("\n"))),
		)

	case "countByClass_":
		// CountByClass(className string) (string, error) - count instances of class
		f.Func().Id("CountByClass").Params(jen.Id("className").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.Var().Id("count").Int(),
			jen.Err().Op("=").Id("db").Dot("QueryRow").Call(
				jen.Lit("SELECT COUNT(*) FROM instances WHERE class = ?"),
				jen.Id("className"),
			).Dot("Scan").Call(jen.Op("&").Id("count")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("0"), jen.Nil()),
			),
			jen.Return(jen.Qual("strconv", "Itoa").Call(jen.Id("count")), jen.Nil()),
		)

	default:
		// Unknown method - generate a stub
		f.Comment("// " + m.selector + " - unknown Environment method")
		f.Func().Id(m.goName).Params().String().Block(
			jen.Return(jen.Lit("")),
		)
	}
	f.Line()
}
