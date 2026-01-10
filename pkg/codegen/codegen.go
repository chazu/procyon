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
		class:          class,
		warnings:       []string{},
		skipped:        []SkippedMethod{},
		instanceVars:   map[string]bool{},
		jsonVars:       map[string]bool{},
		skippedMethods: map[string]bool{},
	}

	// Build instance var lookup and track JSON-typed vars
	for _, iv := range class.InstanceVars {
		g.instanceVars[iv.Name] = true
		// Check if default value is JSON object or array
		defaultVal := iv.Default.Value
		if len(defaultVal) > 0 && (defaultVal[0] == '{' || defaultVal[0] == '[') {
			g.jsonVars[iv.Name] = true
		}
	}

	return g.generate()
}

type generator struct {
	class        *ast.Class
	warnings     []string
	skipped      []SkippedMethod
	instanceVars    map[string]bool
	jsonVars        map[string]bool   // vars with JSON default values (use json.RawMessage)
	skippedMethods  map[string]bool   // methods that will fall back to bash (for @ self detection)
}

type compiledMethod struct {
	selector    string
	goName      string
	args        []string
	body        *parser.MethodBody
	hasReturn   bool
	isClass     bool
	returnsErr  bool
	primitive   bool                   // True if this is a primitive method with native impl
	renamedVars map[string]string      // Original name -> safe Go name
}

func (g *generator) generate() *Result {
	f := jen.NewFile("main")

	// Note: Trait handling is done at parse time via MergeTraits().
	// If traits were provided, their methods are already in g.class.Methods.

	// Add blank imports for embed and sqlite3
	f.Anon("embed")
	f.Anon("github.com/mattn/go-sqlite3")

	// Add gRPC imports for GrpcClient class
	if g.class.Name == "GrpcClient" {
		f.Anon("google.golang.org/grpc")
	}

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

	// Type conversion helpers for iteration
	g.generateTypeHelpers(f)
	f.Line()

	// First pass: identify which methods will be skipped (for @ self calls)
	g.preIdentifySkippedMethods()

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

	// Add gRPC internal fields for GrpcClient (not serialized to JSON)
	if g.class.Name == "GrpcClient" {
		fields = append(fields, jen.Id("conn").Op("*").Qual("google.golang.org/grpc", "ClientConn").Tag(map[string]string{"json": "-"}))
		// Cached file descriptors for proto file mode
		fields = append(fields, jen.Id("fileDescs").Index().Op("*").Qual("github.com/jhump/protoreflect/desc", "FileDescriptor").Tag(map[string]string{"json": "-"}))
	}

	f.Type().Id(g.class.Name).Struct(fields...)
}

func (g *generator) inferType(iv ast.InstanceVar) *jen.Statement {
	// Check if default value is a JSON object or array
	// These are stored as actual JSON in SQLite, not as strings
	defaultVal := iv.Default.Value
	if len(defaultVal) > 0 && (defaultVal[0] == '{' || defaultVal[0] == '[') {
		// Use json.RawMessage to handle JSON values that may be objects/arrays
		return jen.Qual("encoding/json", "RawMessage")
	}
	// For regular string values, use string type
	return jen.String()
}

// isJSONArrayType checks if an instance variable has a JSON array type
// Always returns false since we use string type for all instance variables
func (g *generator) isJSONArrayType(name string) bool {
	// With string-typed instance variables, we always use JSON string operations
	return false
}

// isJSONObjectType checks if an instance variable has a JSON object type
// Always returns false since we use string type for all instance variables
func (g *generator) isJSONObjectType(name string) bool {
	// With string-typed instance variables, we always use JSON string operations
	return false
}

// exprResultsInArray checks if an expression results in a native []interface{}
// This handles chained operations like: items arrayPush: x arrayPush: y
func (g *generator) exprResultsInArray(expr parser.Expr) bool {
	switch e := expr.(type) {
	case *parser.Identifier:
		return g.isJSONArrayType(e.Name)
	case *parser.JSONPrimitiveExpr:
		// If receiver results in array and operation preserves array type
		if g.exprResultsInArray(e.Receiver) {
			switch e.Operation {
			case "arrayPush", "arrayAtPut", "arrayRemoveAt":
				return true
			}
		}
	}
	return false
}

// exprResultsInObject checks if an expression results in a native map[string]interface{}
// This handles chained operations like: data objectAt: k1 put: v1 objectAt: k2 put: v2
func (g *generator) exprResultsInObject(expr parser.Expr) bool {
	switch e := expr.(type) {
	case *parser.Identifier:
		return g.isJSONObjectType(e.Name)
	case *parser.JSONPrimitiveExpr:
		// If receiver results in object and operation preserves object type
		if g.exprResultsInObject(e.Receiver) {
			switch e.Operation {
			case "objectAtPut", "objectRemoveKey":
				return true
			}
		}
	}
	return false
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

		// Dispatch to instance method (pass receiver as instanceID)
		jen.List(jen.Id("result"), jen.Err()).Op(":=").Id("dispatch").Call(jen.Id("instance"), jen.Id("receiver"), jen.Id("selector"), jen.Id("args")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.If(jen.Qual("errors", "Is").Call(jen.Err(), jen.Id("ErrUnknownSelector"))).Block(
				jen.Qual("os", "Exit").Call(jen.Lit(200)),
			),
			jen.Qual("fmt", "Fprintf").Call(jen.Qual("os", "Stderr"), jen.Lit("Error: %v\n"), jen.Err()),
			jen.Qual("os", "Exit").Call(jen.Lit(1)),
		),
		jen.Line(),

		// Save or delete instance
		jen.If(jen.Id("selector").Op("==").Lit("delete")).Block(
			jen.If(jen.Err().Op(":=").Id("deleteInstance").Call(jen.Id("db"), jen.Id("receiver")), jen.Err().Op("!=").Nil()).Block(
				jen.Qual("fmt", "Fprintf").Call(jen.Qual("os", "Stderr"), jen.Lit("Error deleting instance: %v\n"), jen.Err()),
				jen.Qual("os", "Exit").Call(jen.Lit(1)),
			),
		).Else().Block(
			jen.If(jen.Err().Op(":=").Id("saveInstance").Call(jen.Id("db"), jen.Id("receiver"), jen.Id("instance")), jen.Err().Op("!=").Nil()).Block(
				jen.Qual("fmt", "Fprintf").Call(jen.Qual("os", "Stderr"), jen.Lit("Error saving instance: %v\n"), jen.Err()),
				jen.Qual("os", "Exit").Call(jen.Lit(1)),
			),
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

	// generateInstanceID - creates a UUID-based instance ID
	f.Func().Id("generateInstanceID").Params(jen.Id("className").String()).String().Block(
		jen.Id("uuid").Op(":=").Qual("github.com/google/uuid", "New").Call().Dot("String").Call(),
		jen.Return(jen.Qual("strings", "ToLower").Call(jen.Id("className")).Op("+").Lit("_").Op("+").Id("uuid")),
	)
	f.Line()

	// createInstance - inserts a new instance into the database
	f.Func().Id("createInstance").Params(
		jen.Id("db").Op("*").Qual("database/sql", "DB"),
		jen.Id("id").String(),
		jen.Id("instance").Op("*").Id(className),
	).Error().Block(
		jen.List(jen.Id("data"), jen.Err()).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("instance")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Err()),
		),
		jen.List(jen.Id("_"), jen.Err()).Op("=").Id("db").Dot("Exec").Call(
			jen.Lit("INSERT INTO instances (id, data) VALUES (?, json(?))"),
			jen.Id("id"),
			jen.String().Parens(jen.Id("data")),
		),
		jen.Return(jen.Err()),
	)
	f.Line()

	// deleteInstance - removes an instance from the database
	f.Func().Id("deleteInstance").Params(
		jen.Id("db").Op("*").Qual("database/sql", "DB"),
		jen.Id("id").String(),
	).Error().Block(
		jen.List(jen.Id("_"), jen.Err()).Op(":=").Id("db").Dot("Exec").Call(
			jen.Lit("DELETE FROM instances WHERE id = ?"),
			jen.Id("id"),
		),
		jen.Return(jen.Err()),
	)
	f.Line()

	// sendMessage - shell out to bash runtime for non-self message sends
	// Returns just string - errors are silently ignored to match bash behavior and simplify usage in expressions
	f.Func().Id("sendMessage").Params(
		jen.Id("receiver").Interface(),
		jen.Id("selector").String(),
		jen.Id("args").Op("...").Interface(),
	).String().Block(
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
		jen.List(jen.Id("output"), jen.Id("_")).Op(":=").Id("cmd").Dot("Output").Call(),
		jen.Return(jen.Qual("strings", "TrimSpace").Call(jen.String().Parens(jen.Id("output")))),
	)
	f.Line()

	// runServeMode - daemon mode that reads JSON requests from stdin
	g.generateServeMode(f)
	f.Line()

	// JSON primitive helper functions
	g.generateJSONHelpers(f)

	// String/File primitive helper functions
	g.generateStringFileHelpers(f)

	// gRPC helper functions for GrpcClient class
	if g.class.Name == "GrpcClient" {
		g.generateGrpcHelpers(f)
	}
}

// generateTypeHelpers generates helper functions for type conversion in iteration blocks
func (g *generator) generateTypeHelpers(f *jen.File) {
	// toInt converts interface{} to int for arithmetic operations
	f.Comment("// toInt converts interface{} to int for arithmetic in iteration blocks")
	f.Func().Id("toInt").Params(jen.Id("v").Interface()).Int().Block(
		jen.Switch(jen.Id("x").Op(":=").Id("v").Assert(jen.Type())).Block(
			jen.Case(jen.Int()).Block(jen.Return(jen.Id("x"))),
			jen.Case(jen.Int64()).Block(jen.Return(jen.Int().Parens(jen.Id("x")))),
			jen.Case(jen.Float64()).Block(jen.Return(jen.Int().Parens(jen.Id("x")))),
			jen.Case(jen.String()).Block(
				jen.List(jen.Id("n"), jen.Id("_")).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("x")),
				jen.Return(jen.Id("n")),
			),
			jen.Default().Block(jen.Return(jen.Lit(0))),
		),
	)
	f.Line()

	// toBool converts interface{} to bool for predicates
	f.Comment("// toBool converts interface{} to bool for predicates in iteration blocks")
	f.Func().Id("toBool").Params(jen.Id("v").Interface()).Bool().Block(
		jen.Switch(jen.Id("x").Op(":=").Id("v").Assert(jen.Type())).Block(
			jen.Case(jen.Bool()).Block(jen.Return(jen.Id("x"))),
			jen.Case(jen.Int()).Block(jen.Return(jen.Id("x").Op("!=").Lit(0))),
			jen.Case(jen.String()).Block(jen.Return(jen.Id("x").Op("!=").Lit(""))),
			jen.Default().Block(jen.Return(jen.Id("v").Op("!=").Nil())),
		),
	)
	f.Line()

	// invokeBlock calls a Trashtalk block through the Bash runtime (Phase 2)
	// Returns just string - errors are silently ignored to match bash behavior and simplify usage in expressions
	f.Comment("// invokeBlock calls a Trashtalk block through the Bash runtime")
	f.Comment("// blockID is the instance ID of the Block object")
	f.Comment("// args are the values to pass to the block")
	f.Func().Id("invokeBlock").Params(
		jen.Id("blockID").String(),
		jen.Id("args").Op("...").Interface(),
	).String().Block(
		// Build command based on arg count
		jen.Var().Id("cmdStr").String(),
		jen.Switch(jen.Len(jen.Id("args"))).Block(
			jen.Case(jen.Lit(0)).Block(
				jen.Id("cmdStr").Op("=").Qual("fmt", "Sprintf").Call(
					jen.Lit("source ~/.trashtalk/lib/trash.bash && @ %q value"),
					jen.Id("blockID"),
				),
			),
			jen.Case(jen.Lit(1)).Block(
				jen.Id("cmdStr").Op("=").Qual("fmt", "Sprintf").Call(
					jen.Lit("source ~/.trashtalk/lib/trash.bash && @ %q valueWith: %q"),
					jen.Id("blockID"),
					jen.Qual("fmt", "Sprint").Call(jen.Id("args").Index(jen.Lit(0))),
				),
			),
			jen.Case(jen.Lit(2)).Block(
				jen.Id("cmdStr").Op("=").Qual("fmt", "Sprintf").Call(
					jen.Lit("source ~/.trashtalk/lib/trash.bash && @ %q valueWith: %q and: %q"),
					jen.Id("blockID"),
					jen.Qual("fmt", "Sprint").Call(jen.Id("args").Index(jen.Lit(0))),
					jen.Qual("fmt", "Sprint").Call(jen.Id("args").Index(jen.Lit(1))),
				),
			),
			jen.Default().Block(
				jen.Return(jen.Lit("")),
			),
		),
		jen.Line(),
		jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("cmdStr")),
		jen.List(jen.Id("output"), jen.Id("_")).Op(":=").Id("cmd").Dot("Output").Call(),
		jen.Return(jen.Qual("strings", "TrimSpace").Call(jen.String().Parens(jen.Id("output")))),
	)
	f.Line()
}

// generateServeMode generates the daemon loop that reads JSON from stdin
func (g *generator) generateServeMode(f *jen.File) {
	className := g.class.Name
	qualifiedName := g.class.QualifiedName()

	// Request/Response structs
	f.Comment("// ServeRequest is the JSON request format for --serve mode")
	f.Type().Id("ServeRequest").Struct(
		jen.Id("InstanceID").String().Tag(map[string]string{"json": "instance_id"}),
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

		// Dispatch to instance method (pass instance ID for primitives)
		jen.List(jen.Id("result"), jen.Err()).Op(":=").Id("dispatch").Call(
			jen.Op("&").Id("instance"),
			jen.Id("req").Dot("InstanceID"),
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

		// Handle delete specially
		jen.If(jen.Id("req").Dot("Selector").Op("==").Lit("delete")).Block(
			jen.If(jen.Err().Op(":=").Id("deleteInstance").Call(jen.Id("db"), jen.Id("req").Dot("InstanceID")), jen.Err().Op("!=").Nil()).Block(
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

		// Return updated instance + result
		jen.List(jen.Id("updatedJSON"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Op("&").Id("instance")),
		jen.Return(jen.Id("ServeResponse").Values(jen.Dict{
			jen.Id("Instance"): jen.String().Parens(jen.Id("updatedJSON")),
			jen.Id("Result"):   jen.Id("result"),
			jen.Id("ExitCode"): jen.Lit(0),
		})),
	)
}

// preIdentifySkippedMethods runs through all methods to identify which will be skipped.
// This is needed so that @ self calls can use sendMessage for skipped methods.
func (g *generator) preIdentifySkippedMethods() {
	for _, m := range g.class.Methods {
		willSkip := false

		// bashOnly pragma
		if m.HasPragma("bashOnly") {
			willSkip = true
		}

		// Raw methods (unless primitive or has procyon pragma)
		if m.Raw && !m.Primitive && !m.HasPragma("procyonOnly") && !m.HasPragma("procyonNative") {
			willSkip = true
		}

		// Primitive methods without native impl
		if m.Primitive && !hasPrimitiveImpl(g.class.Name, m.Selector) {
			willSkip = true
		}

		// Check for bash-specific function calls
		for _, tok := range m.Body.Tokens {
			if tok.Type == "IDENTIFIER" {
				switch tok.Value {
				case "_ivar", "_ivar_set", "_throw", "_on_error", "_ensure", "_pop_handler":
					willSkip = true
					break
				}
			}
		}

		// Try to parse - if unsupported, will skip
		if !willSkip && !m.Raw && !m.Primitive {
			result := parser.ParseMethod(m.Body.Tokens)
			if result.Unsupported {
				willSkip = true
			}
		}

		if willSkip {
			g.skippedMethods[m.Selector] = true
		}
	}
}

func (g *generator) compileMethods() []*compiledMethod {
	var compiled []*compiledMethod

	for _, m := range g.class.Methods {
		// Skip bashOnly methods - they should only run in Bash
		if m.HasPragma("bashOnly") {
			g.skipped = append(g.skipped, SkippedMethod{
				Selector: m.Selector,
				Reason:   "bashOnly pragma",
			})
			continue
		}

		// Skip raw methods unless:
		// - procyonOnly pragma: Bash gets error stub, Procyon provides impl
		// - procyonNative pragma: Bash uses rawMethod body, Procyon provides native impl
		// - primitive: method has native Procyon implementation
		if m.Raw && !m.Primitive && !m.HasPragma("procyonOnly") && !m.HasPragma("procyonNative") {
			g.skipped = append(g.skipped, SkippedMethod{
				Selector: m.Selector,
				Reason:   "raw method",
			})
			continue
		}

		// Handle primitive methods - these have native Procyon implementations
		// The bash fallback code in the body is ignored; Procyon provides the native impl
		if m.Primitive {
			if hasPrimitiveImpl(g.class.Name, m.Selector) {
				compiled = append(compiled, &compiledMethod{
					selector:    m.Selector,
					goName:      selectorToGoName(m.Selector),
					args:        m.Args,
					body:        nil, // No parsed body - native impl provided
					hasReturn:   true,
					isClass:     m.Kind == "class",
					returnsErr:  true,
					primitive:   true,
					renamedVars: make(map[string]string),
				})
			} else {
				// No native impl registered - warn but still fall back to bash
				g.warnings = append(g.warnings,
					fmt.Sprintf("primitive method %s.%s has no native implementation, using bash fallback",
						g.class.Name, m.Selector))
				g.skipped = append(g.skipped, SkippedMethod{
					Selector: m.Selector,
					Reason:   "primitive method without native implementation",
				})
			}
			continue
		}

		// For GrpcClient procyonNative methods, skip body parsing entirely -
		// these raw methods contain Bash code that won't parse, but
		// generateGrpcClientMethod() will provide native implementations
		if g.class.Name == "GrpcClient" && m.HasPragma("procyonNative") {
			compiled = append(compiled, &compiledMethod{
				selector:    m.Selector,
				goName:      selectorToGoName(m.Selector),
				args:        m.Args,
				body:        nil, // No parsed body - native impl provided
				hasReturn:   true,
				isClass:     m.Kind == "class",
				returnsErr:  true,
				renamedVars: make(map[string]string),
			})
			continue
		}

		// Check for bash-specific function calls before parsing
		hasBashRuntimeCall := false
		for _, tok := range m.Body.Tokens {
			if tok.Type == "IDENTIFIER" {
				switch tok.Value {
				case "_ivar", "_ivar_set", "_throw", "_on_error", "_ensure", "_pop_handler":
					g.skipped = append(g.skipped, SkippedMethod{
						Selector: m.Selector,
						Reason:   "uses bash runtime function: " + tok.Value,
					})
					hasBashRuntimeCall = true
					break
				}
			}
		}
		if hasBashRuntimeCall {
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

		// Check if method has return (recursively check inside if blocks too)
		hasReturn := hasReturnInStatements(result.Body.Statements)

		// Check if any args require error handling (string to int conversion)
		returnsErr := len(m.Args) > 0

		compiled = append(compiled, &compiledMethod{
			selector:    m.Selector,
			goName:      selectorToGoName(m.Selector),
			args:        m.Args,
			body:        result.Body,
			hasReturn:   hasReturn,
			isClass:     m.Kind == "class",
			returnsErr:  returnsErr,
			renamedVars: make(map[string]string),
		})
	}

	return compiled
}

func (g *generator) generateDispatch(f *jen.File, methods []*compiledMethod) {
	className := g.class.Name
	qualifiedName := g.class.QualifiedName()

	// Built-in primitive cases for Object methods
	cases := []jen.Code{
		// class - returns the class name
		jen.Case(jen.Lit("class")).Block(
			jen.Return(jen.Lit(qualifiedName), jen.Nil()),
		),
		// id - returns the instance ID
		jen.Case(jen.Lit("id")).Block(
			jen.Return(jen.Id("instanceID"), jen.Nil()),
		),
		// delete - signals deletion (actual deletion handled by caller)
		jen.Case(jen.Lit("delete")).Block(
			jen.Return(jen.Id("instanceID"), jen.Nil()),
		),
	}

	for _, m := range methods {
		// Check if method name was renamed to avoid collision with ivar
		// In Go, you can't have a struct field and method with the same name
		methodName := m.goName
		if !m.isClass && g.instanceVars[m.selector] {
			methodName = "Get" + methodName
		}

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
			callExpr = jen.Id("c").Dot(methodName).Call(callArgs...)

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
			callExpr = jen.Id("c").Dot(methodName).Call()
			if m.hasReturn {
				if m.returnsErr {
					// Method returns (string, error) - don't add extra nil
					cases = append(cases, jen.Case(jen.Lit(m.selector)).Block(
						jen.Return(callExpr),
					))
				} else {
					// Method returns string only - add nil for error
					cases = append(cases, jen.Case(jen.Lit(m.selector)).Block(
						jen.Return(callExpr, jen.Nil()),
					))
				}
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
		jen.Id("instanceID").String(),
		jen.Id("selector").String(),
		jen.Id("args").Index().String(),
	).Parens(jen.List(jen.String(), jen.Error())).Block(
		jen.Switch(jen.Id("selector")).Block(cases...),
	)
}

func (g *generator) generateClassDispatch(f *jen.File, methods []*compiledMethod) {
	className := g.class.Name
	qualifiedName := g.class.QualifiedName()

	// Build struct initialization with default values for "new" primitive
	structFields := jen.Dict{
		jen.Id("Class"):     jen.Lit(qualifiedName),
		jen.Id("CreatedAt"): jen.Qual("time", "Now").Call().Dot("Format").Call(jen.Qual("time", "RFC3339")),
	}
	for _, iv := range g.class.InstanceVars {
		goName := capitalize(iv.Name)
		val := iv.Default.Value
		// All instance variables are strings (JSON representations for arrays/objects)
		structFields[jen.Id(goName)] = jen.Lit(val)
	}

	// "new" primitive case - creates and persists a new instance
	cases := []jen.Code{
		jen.Case(jen.Lit("new")).Block(
			jen.Id("id").Op(":=").Id("generateInstanceID").Call(jen.Lit(className)),
			jen.Id("instance").Op(":=").Op("&").Id(className).Values(structFields),
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.If(jen.Err().Op(":=").Id("createInstance").Call(jen.Id("db"), jen.Id("id"), jen.Id("instance")), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Id("id"), jen.Nil()),
		),
	}

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

	// Special handling for GrpcClient class - wire methods to gRPC helpers
	if g.class.Name == "GrpcClient" && !m.isClass {
		if g.generateGrpcClientMethod(f, m) {
			return
		}
	}

	// Handle primitive methods - these have native Procyon implementations
	if m.primitive {
		if g.generatePrimitiveMethod(f, m) {
			return
		}
	}

	// Check if method name collides with an instance variable (Go doesn't allow this)
	// If it's a simple getter (no args, returns the ivar), rename to Get<Name>
	methodName := m.goName
	if !m.isClass && g.instanceVars[strings.ToLower(m.selector)] {
		// Method name matches an ivar - rename to avoid Go collision
		methodName = "Get" + methodName
	}

	// Build parameter list (sanitize Go keywords)
	params := []jen.Code{}
	for _, arg := range m.args {
		safeName := safeGoName(arg)
		if safeName != arg {
			m.renamedVars[arg] = safeName
		}
		params = append(params, jen.Id(safeName).String())
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
			f.Func().Id(methodName).Params(params...).Add(returnType).Block(body...)
		} else {
			f.Func().Id(methodName).Params(params...).Block(body...)
		}
	} else {
		// Instance methods have receiver
		if returnType != nil {
			f.Func().Parens(jen.Id("c").Op("*").Id(className)).Id(methodName).Params(params...).Add(returnType).Block(body...)
		} else {
			f.Func().Parens(jen.Id("c").Op("*").Id(className)).Id(methodName).Params(params...).Block(body...)
		}
	}
	f.Line()
}

func (g *generator) generateMethodBody(m *compiledMethod) []jen.Code {
	var stmts []jen.Code

	// Parameters come in as strings from dispatcher and are used as strings
	// Numeric conversions happen at point of use in expressions

	// Local variables - rename if they conflict with Go builtins
	// Use interface{} for dynamic typing (Trashtalk is dynamically typed)
	for _, v := range m.body.LocalVars {
		safeName := safeGoName(v)
		if safeName != v {
			m.renamedVars[v] = safeName
		}
		stmts = append(stmts, jen.Var().Id(safeName).Interface())
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
		target := s.Target
		// Check if it's an instance variable (string typed)
		if g.instanceVars[target] {
			// For instance variables, we need string values
			var expr *jen.Statement
			switch v := s.Value.(type) {
			case *parser.Identifier:
				// Check if value is a method arg - use original string param
				isMethodArg := false
				for _, arg := range m.args {
					if arg == v.Name {
						isMethodArg = true
						break
					}
				}
				if isMethodArg {
					// Use renamed parameter name if it conflicted with Go keyword
					paramName := v.Name
					if renamed, ok := m.renamedVars[v.Name]; ok {
						paramName = renamed
					}
					expr = jen.Id(paramName)
				} else if g.instanceVars[v.Name] {
					// Assigning one ivar to another - already a string
					expr = g.generateExpr(s.Value, m)
				} else {
					// Local variable - need to convert to string
					expr = jen.Id("_toStr").Call(g.generateExpr(s.Value, m))
				}
			case *parser.BinaryExpr:
				// Arithmetic expression - result is int, need to convert to string
				expr = jen.Qual("strconv", "Itoa").Call(g.generateExpr(s.Value, m))
			case *parser.StringLit:
				// String literal - already a string
				expr = g.generateExpr(s.Value, m)
			case *parser.JSONPrimitiveExpr:
				// JSON primitives return strings
				expr = g.generateExpr(s.Value, m)
			case *parser.MessageSend:
				// Message sends return strings
				expr = g.generateExpr(s.Value, m)
			default:
				// Default: wrap in _toStr for safety
				expr = jen.Id("_toStr").Call(g.generateExpr(s.Value, m))
			}
			// JSON vars need to be wrapped in json.RawMessage
			if g.jsonVars[target] {
				expr = jen.Qual("encoding/json", "RawMessage").Parens(expr)
			}
			return []jen.Code{jen.Id("c").Dot(capitalize(target)).Op("=").Add(expr)}
		}
		// For local variables
		expr := g.generateExpr(s.Value, m)
		// Check if target was renamed to avoid Go builtin conflict
		if renamed, ok := m.renamedVars[target]; ok {
			target = renamed
		}
		return []jen.Code{jen.Id(target).Op("=").Add(expr)}

	case *parser.Return:
		// Check for iteration expression as return value
		if iterVal, ok := s.Value.(*parser.IterationExprAsValue); ok {
			// Generate iteration statements (collect: or select: produce _results)
			iterStmts := g.generateIterationStatement(iterVal.Iteration, m)
			// Return the results as JSON
			returnStmt := jen.List(jen.Id("_resultJSON"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("_results"))
			if m.returnsErr {
				return append(iterStmts, returnStmt, jen.Return(jen.String().Call(jen.Id("_resultJSON")), jen.Nil()))
			}
			return append(iterStmts, returnStmt, jen.Return(jen.String().Call(jen.Id("_resultJSON"))))
		}
		if dynIterVal, ok := s.Value.(*parser.DynamicIterationExprAsValue); ok {
			// Generate dynamic iteration statements (collect: or select: produce _results)
			iterStmts := g.generateDynamicIterationStatement(dynIterVal.Iteration, m)
			// Return the results as JSON
			returnStmt := jen.List(jen.Id("_resultJSON"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("_results"))
			if m.returnsErr {
				return append(iterStmts, returnStmt, jen.Return(jen.String().Call(jen.Id("_resultJSON")), jen.Nil()))
			}
			return append(iterStmts, returnStmt, jen.Return(jen.String().Call(jen.Id("_resultJSON"))))
		}

		expr := g.generateExpr(s.Value, m)
		// Check if the return value is already a string (message sends, string literals, JSON primitives)
		_, isMessageSend := s.Value.(*parser.MessageSend)
		_, isStringLit := s.Value.(*parser.StringLit)
		jsonPrim, isJSONPrimitive := s.Value.(*parser.JSONPrimitiveExpr)
		// Check if return value is an instance variable (all are string typed)
		isIvarReturn := false
		if id, ok := s.Value.(*parser.Identifier); ok {
			isIvarReturn = g.instanceVars[id.Name]
		}
		// Check if JSON primitive returns an array type (needs JSON encoding)
		isArrayReturningPrimitive := false
		if isJSONPrimitive {
			switch jsonPrim.Operation {
			case "objectKeys", "objectValues", "arrayCollect", "arraySelect":
				isArrayReturningPrimitive = true
			}
		}
		if isArrayReturningPrimitive {
			// Array-returning primitives need JSON encoding
			stmts := []jen.Code{
				jen.List(jen.Id("_resultJSON"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(expr),
			}
			if m.returnsErr {
				stmts = append(stmts, jen.Return(jen.String().Call(jen.Id("_resultJSON")), jen.Nil()))
			} else {
				stmts = append(stmts, jen.Return(jen.String().Call(jen.Id("_resultJSON"))))
			}
			return stmts
		}
		if isMessageSend || isStringLit || isJSONPrimitive || isIvarReturn {
			// Already a string, no conversion needed
			if m.returnsErr {
				return []jen.Code{jen.Return(expr, jen.Nil())}
			}
			return []jen.Code{jen.Return(expr)}
		}
		// Other values - use _toStr for interface{} compatibility
		if m.returnsErr {
			return []jen.Code{jen.Return(jen.Id("_toStr").Call(expr), jen.Nil())}
		}
		return []jen.Code{jen.Return(jen.Id("_toStr").Call(expr))}

	case *parser.ExprStmt:
		return []jen.Code{g.generateExpr(s.Expr, m)}

	case *parser.IfExpr:
		return g.generateIfStatement(s, m)

	case *parser.WhileExpr:
		return g.generateWhileStatement(s, m)

	case *parser.IterationExpr:
		return g.generateIterationStatement(s, m)

	case *parser.DynamicIterationExpr:
		return g.generateDynamicIterationStatement(s, m)

	default:
		return []jen.Code{jen.Comment("unknown statement")}
	}
}

// generateIfStatement generates Go if/else from Trashtalk ifTrue:/ifFalse:
func (g *generator) generateIfStatement(s *parser.IfExpr, m *compiledMethod) []jen.Code {
	condition := g.generateCondition(s.Condition, m)

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

// generateCondition generates a Go boolean condition from a Trashtalk expression.
// Comparisons return bool directly, but message sends return strings.
// For message sends, we convert to bool with: result != ""
func (g *generator) generateCondition(expr parser.Expr, m *compiledMethod) *jen.Statement {
	// Check if the expression is a comparison (already returns bool)
	switch expr.(type) {
	case *parser.ComparisonExpr:
		return g.generateExpr(expr, m)
	}

	// For message sends and other expressions, wrap in truthiness check
	// In Trashtalk, non-empty string = truthy
	return g.generateExpr(expr, m).Op("!=").Lit("")
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

// generateIterationStatement generates Go for loop from Trashtalk do:/collect:/select:
func (g *generator) generateIterationStatement(s *parser.IterationExpr, m *compiledMethod) []jen.Code {
	collectionExpr := g.generateExpr(s.Collection, m)
	iterVar := s.IterVar
	rawIterVar := "_" + iterVar // Raw interface{} variable from range

	// Check if collection is a native array (from JSON primitives) vs JSON string
	isNativeArray := g.exprResultsInArray(s.Collection)

	// Type conversion at start of loop: iterVar := toInt(_iterVar)
	typeConversion := jen.Id(iterVar).Op(":=").Id("toInt").Call(jen.Id(rawIterVar))

	switch s.Kind {
	case "do":
		// For do:, generate body statements normally
		var bodyStmts []jen.Code
		for _, stmt := range s.Body {
			bodyStmts = append(bodyStmts, g.generateIterationBodyStatement(stmt, m, iterVar)...)
		}

		// Prepend type conversion
		loopBody := append([]jen.Code{typeConversion}, bodyStmts...)

		if isNativeArray {
			// Native array: iterate directly over []interface{}
			return []jen.Code{
				jen.For(jen.List(jen.Id("_"), jen.Id(rawIterVar)).Op(":=").Range().Add(collectionExpr)).Block(loopBody...),
			}
		}
		// JSON string: unmarshal first
		return []jen.Code{
			jen.Var().Id("_items").Index().Interface(),
			jen.Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(collectionExpr),
				jen.Op("&").Id("_items"),
			),
			jen.For(jen.List(jen.Id("_"), jen.Id(rawIterVar)).Op(":=").Range().Id("_items")).Block(loopBody...),
		}

	case "collect":
		// For collect:, the last statement's expression becomes the collected value
		bodyStmts, resultExpr := g.generateCollectBody(s.Body, m, iterVar)

		// Prepend type conversion, then body, then append result
		loopBody := append([]jen.Code{typeConversion}, bodyStmts...)
		loopBody = append(loopBody, jen.Id("_results").Op("=").Append(jen.Id("_results"), resultExpr))

		if isNativeArray {
			// Native array: collect directly into []interface{}
			return []jen.Code{
				jen.Id("_results").Op(":=").Make(jen.Index().Interface(), jen.Lit(0), jen.Len(collectionExpr)),
				jen.For(jen.List(jen.Id("_"), jen.Id(rawIterVar)).Op(":=").Range().Add(collectionExpr)).Block(loopBody...),
			}
		}
		// JSON string: unmarshal first
		return []jen.Code{
			jen.Var().Id("_items").Index().Interface(),
			jen.Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(collectionExpr),
				jen.Op("&").Id("_items"),
			),
			jen.Id("_results").Op(":=").Make(jen.Index().Interface(), jen.Lit(0)),
			jen.For(jen.List(jen.Id("_"), jen.Id(rawIterVar)).Op(":=").Range().Id("_items")).Block(loopBody...),
		}

	case "select":
		// For select:, the last statement's expression becomes the filter condition
		bodyStmts, conditionExpr := g.generateSelectBody(s.Body, m, iterVar)

		// For select, we need to keep the original interface{} value for appending
		// But use the typed value for the condition
		loopBody := append([]jen.Code{typeConversion}, bodyStmts...)
		loopBody = append(loopBody, jen.If(conditionExpr).Block(
			jen.Id("_results").Op("=").Append(jen.Id("_results"), jen.Id(rawIterVar)),
		))

		if isNativeArray {
			// Native array: filter directly into []interface{}
			return []jen.Code{
				jen.Id("_results").Op(":=").Make(jen.Index().Interface(), jen.Lit(0)),
				jen.For(jen.List(jen.Id("_"), jen.Id(rawIterVar)).Op(":=").Range().Add(collectionExpr)).Block(loopBody...),
			}
		}
		// JSON string: unmarshal first
		return []jen.Code{
			jen.Var().Id("_items").Index().Interface(),
			jen.Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(collectionExpr),
				jen.Op("&").Id("_items"),
			),
			jen.Id("_results").Op(":=").Make(jen.Index().Interface(), jen.Lit(0)),
			jen.For(jen.List(jen.Id("_"), jen.Id(rawIterVar)).Op(":=").Range().Id("_items")).Block(loopBody...),
		}

	default:
		return []jen.Code{jen.Comment("unknown iteration kind: " + s.Kind)}
	}
}

// generateCollectBody generates the body statements for a collect: block
// Returns the body statements (all but last) and the result expression (last statement)
func (g *generator) generateCollectBody(body []parser.Statement, m *compiledMethod, iterVar string) ([]jen.Code, *jen.Statement) {
	if len(body) == 0 {
		return nil, jen.Nil()
	}

	var stmts []jen.Code
	// Generate all statements except the last
	for i := 0; i < len(body)-1; i++ {
		stmts = append(stmts, g.generateIterationBodyStatement(body[i], m, iterVar)...)
	}

	// Last statement should be an expression - extract it as the result
	lastStmt := body[len(body)-1]
	if exprStmt, ok := lastStmt.(*parser.ExprStmt); ok {
		return stmts, g.generateExpr(exprStmt.Expr, m)
	}

	// If last statement is not an expression, generate it normally and return nil
	stmts = append(stmts, g.generateIterationBodyStatement(lastStmt, m, iterVar)...)
	return stmts, jen.Nil()
}

// generateSelectBody generates the body statements for a select: block
// Returns the body statements (all but last) and the condition expression (last statement)
func (g *generator) generateSelectBody(body []parser.Statement, m *compiledMethod, iterVar string) ([]jen.Code, *jen.Statement) {
	if len(body) == 0 {
		return nil, jen.Lit(false)
	}

	var stmts []jen.Code
	// Generate all statements except the last
	for i := 0; i < len(body)-1; i++ {
		stmts = append(stmts, g.generateIterationBodyStatement(body[i], m, iterVar)...)
	}

	// Last statement should be an expression (the predicate) - extract it
	lastStmt := body[len(body)-1]
	if exprStmt, ok := lastStmt.(*parser.ExprStmt); ok {
		return stmts, g.generateExpr(exprStmt.Expr, m)
	}

	// If last statement is not an expression, generate it normally and return false
	stmts = append(stmts, g.generateIterationBodyStatement(lastStmt, m, iterVar)...)
	return stmts, jen.Lit(false)
}

// generateIterationBodyStatement generates statements within an iteration block
// The iterVar is available as a local variable
func (g *generator) generateIterationBodyStatement(stmt parser.Statement, m *compiledMethod, iterVar string) []jen.Code {
	// For now, just generate the statement normally
	// The iterVar will be in scope from the for loop
	return g.generateStatement(stmt, m)
}

// generateDynamicIterationStatement generates shell-out iteration for dynamic blocks (Phase 2)
// When the block is a variable/parameter, we call back to Bash for each element
func (g *generator) generateDynamicIterationStatement(s *parser.DynamicIterationExpr, m *compiledMethod) []jen.Code {
	collectionExpr := g.generateExpr(s.Collection, m)
	// Block IDs are strings - don't use the Int conversion
	blockExpr := g.generateExprAsString(s.BlockVar, m)

	// Check if collection is a native array (from JSON primitives) vs JSON string
	isNativeArray := g.exprResultsInArray(s.Collection)

	switch s.Kind {
	case "do":
		if isNativeArray {
			// Native array: iterate directly, call block for each element
			return []jen.Code{
				jen.For(jen.List(jen.Id("_"), jen.Id("_elem")).Op(":=").Range().Add(collectionExpr)).Block(
					jen.Id("_").Op("=").Id("invokeBlock").Call(
						blockExpr,
						jen.Id("_elem"),
					),
				),
			}
		}
		// JSON string: unmarshal first
		return []jen.Code{
			jen.Var().Id("_items").Index().Interface(),
			jen.Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(collectionExpr),
				jen.Op("&").Id("_items"),
			),
			jen.For(jen.List(jen.Id("_"), jen.Id("_elem")).Op(":=").Range().Id("_items")).Block(
				jen.Id("_").Op("=").Id("invokeBlock").Call(
					blockExpr,
					jen.Id("_elem"),
				),
			),
		}

	case "collect":
		if isNativeArray {
			// Native array: collect results from block calls
			return []jen.Code{
				jen.Id("_results").Op(":=").Make(jen.Index().Interface(), jen.Lit(0), jen.Len(collectionExpr)),
				jen.For(jen.List(jen.Id("_"), jen.Id("_elem")).Op(":=").Range().Add(collectionExpr)).Block(
					jen.Id("_result").Op(":=").Id("invokeBlock").Call(
						blockExpr,
						jen.Id("_elem"),
					),
					jen.Id("_results").Op("=").Append(jen.Id("_results"), jen.Id("_result")),
				),
			}
		}
		// JSON string: unmarshal first
		return []jen.Code{
			jen.Var().Id("_items").Index().Interface(),
			jen.Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(collectionExpr),
				jen.Op("&").Id("_items"),
			),
			jen.Id("_results").Op(":=").Make(jen.Index().Interface(), jen.Lit(0)),
			jen.For(jen.List(jen.Id("_"), jen.Id("_elem")).Op(":=").Range().Id("_items")).Block(
				jen.Id("_result").Op(":=").Id("invokeBlock").Call(
					blockExpr,
					jen.Id("_elem"),
				),
				jen.Id("_results").Op("=").Append(jen.Id("_results"), jen.Id("_result")),
			),
		}

	case "select":
		if isNativeArray {
			// Native array: filter based on block result
			return []jen.Code{
				jen.Id("_results").Op(":=").Make(jen.Index().Interface(), jen.Lit(0)),
				jen.For(jen.List(jen.Id("_"), jen.Id("_elem")).Op(":=").Range().Add(collectionExpr)).Block(
					jen.Id("_result").Op(":=").Id("invokeBlock").Call(
						blockExpr,
						jen.Id("_elem"),
					),
					jen.Comment("Non-empty string result means true"),
					jen.If(jen.Id("_result").Op("!=").Lit("")).Block(
						jen.Id("_results").Op("=").Append(jen.Id("_results"), jen.Id("_elem")),
					),
				),
			}
		}
		// JSON string: unmarshal first
		return []jen.Code{
			jen.Var().Id("_items").Index().Interface(),
			jen.Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(collectionExpr),
				jen.Op("&").Id("_items"),
			),
			jen.Id("_results").Op(":=").Make(jen.Index().Interface(), jen.Lit(0)),
			jen.For(jen.List(jen.Id("_"), jen.Id("_elem")).Op(":=").Range().Id("_items")).Block(
				jen.Id("_result").Op(":=").Id("invokeBlock").Call(
					blockExpr,
					jen.Id("_elem"),
				),
				jen.Comment("Non-empty string result means true"),
				jen.If(jen.Id("_result").Op("!=").Lit("")).Block(
					jen.Id("_results").Op("=").Append(jen.Id("_results"), jen.Id("_elem")),
				),
			),
		}

	default:
		return []jen.Code{jen.Comment("unknown dynamic iteration kind: " + s.Kind)}
	}
}

// generateExprAsString generates an expression keeping method args as strings (no int conversion)
// Used for block IDs and other cases where we need the original string parameter
func (g *generator) generateExprAsString(expr parser.Expr, m *compiledMethod) *jen.Statement {
	switch e := expr.(type) {
	case *parser.Identifier:
		name := e.Name
		// Check if it's self (the receiver)
		if name == "self" {
			return jen.Id("c")
		}
		// Check if it's an instance variable
		if g.instanceVars[name] {
			fieldAccess := jen.Id("c").Dot(capitalize(name))
			// JSON vars are json.RawMessage, need to convert to string
			if g.jsonVars[name] {
				return jen.String().Parens(fieldAccess)
			}
			return fieldAccess
		}
		// For method args, use the string parameter directly (no Int conversion)
		return jen.Id(name)
	default:
		// For other expressions, fall back to regular generation
		return g.generateExpr(expr, m)
	}
}

func (g *generator) generateExpr(expr parser.Expr, m *compiledMethod) *jen.Statement {
	switch e := expr.(type) {
	case *parser.BinaryExpr:
		// Wrap in toInt() for interface{} compatibility
		left := jen.Id("toInt").Call(g.generateExpr(e.Left, m))
		right := jen.Id("toInt").Call(g.generateExpr(e.Right, m))
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
		// Wrap in toInt() for interface{} compatibility
		left := jen.Id("toInt").Call(g.generateExpr(e.Left, m))
		right := jen.Id("toInt").Call(g.generateExpr(e.Right, m))
		return left.Op(e.Op).Add(right)

	case *parser.Identifier:
		name := e.Name
		// Check if it's self (the receiver) - only valid for instance methods
		if name == "self" && !m.isClass {
			return jen.Id("c")
		}
		// Check if it's a method arg FIRST (params are strings, use as-is)
		// This must come before instance var check to handle cases where
		// a method param has the same name as an instance var
		for _, arg := range m.args {
			if arg == name {
				if renamed, ok := m.renamedVars[name]; ok {
					return jen.Id(renamed)
				}
				return jen.Id(name)
			}
		}
		// Check if it's an instance variable (only for instance methods)
		if !m.isClass && g.instanceVars[name] {
			fieldAccess := jen.Id("c").Dot(capitalize(name))
			// JSON vars are json.RawMessage, need to convert to string
			if g.jsonVars[name] {
				return jen.String().Parens(fieldAccess)
			}
			return fieldAccess
		}
		// Check if this variable was renamed to avoid Go builtin conflict
		if renamed, ok := m.renamedVars[name]; ok {
			return jen.Id(renamed)
		}
		return jen.Id(name)

	case *parser.QualifiedName:
		// Qualified name (Pkg::Class) - return the full name as a string literal
		return jen.Lit(e.FullName())

	case *parser.NumberLit:
		return jen.Lit(mustAtoi(e.Value))

	case *parser.StringLit:
		return jen.Lit(e.Value)

	case *parser.MessageSend:
		if e.IsSelf {
			// Check if target method is raw or skipped (will fall back to bash)
			// If so, use sendMessage to call bash runtime instead of direct Go call
			isTargetSkipped := g.skippedMethods[e.Selector]
			if !isTargetSkipped {
				// Also check if explicitly marked as raw
				for _, method := range g.class.Methods {
					if method.Selector == e.Selector && method.Raw {
						isTargetSkipped = true
						break
					}
				}
			}

			if isTargetSkipped {
				// Use sendMessage for skipped/raw methods that aren't compiled to Go
				args := []jen.Code{jen.Id("c"), jen.Lit(e.Selector)}
				for _, arg := range e.Args {
					args = append(args, g.generateExprAsString(arg, m))
				}
				return jen.Id("sendMessage").Call(args...)
			}

			// Self send to compiled method: direct Go method call
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

		// Check for block invocation pattern: @ aBlock value / valueWith: / valueWith:and:
		// When receiver is a method parameter and selector is a block invocation selector,
		// use invokeBlock() instead of sendMessage() for better performance
		if ident, ok := e.Receiver.(*parser.Identifier); ok {
			isMethodParam := false
			for _, arg := range m.args {
				if arg == ident.Name {
					isMethodParam = true
					break
				}
			}
			if isMethodParam && isBlockInvocationSelector(e.Selector) {
				// Generate: invokeBlock(blockID, args...)
				blockArgs := []jen.Code{jen.Id(ident.Name)} // Use string param directly
				for _, arg := range e.Args {
					blockArgs = append(blockArgs, g.generateExprAsString(arg, m))
				}
				return jen.Id("invokeBlock").Call(blockArgs...)
			}
		}

		// Non-self send: shell out to bash runtime
		// Generate: sendMessage(receiver, selector, args...)
		var receiverExpr *jen.Statement
		// Check if receiver is a qualified name (Pkg::Class) - use full name as string literal
		if qn, ok := e.Receiver.(*parser.QualifiedName); ok {
			receiverExpr = jen.Lit(qn.FullName())
		} else if ident, ok := e.Receiver.(*parser.Identifier); ok {
			// Check if receiver is a class name (uppercase identifier that's not a local var)
			name := ident.Name
			isLocalVar := false
			// Check instance vars, method args, and local vars
			if g.instanceVars[name] {
				isLocalVar = true
			}
			for _, arg := range m.args {
				if arg == name {
					isLocalVar = true
					break
				}
			}
			if _, ok := m.renamedVars[name]; ok {
				isLocalVar = true
			}
			// Uppercase name that's not a local var is a class name - use string literal
			if !isLocalVar && len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
				receiverExpr = jen.Lit(name)
			} else {
				receiverExpr = g.generateExpr(e.Receiver, m)
			}
		} else {
			receiverExpr = g.generateExpr(e.Receiver, m)
		}
		args := []jen.Code{
			receiverExpr,
			jen.Lit(e.Selector),
		}
		for _, arg := range e.Args {
			args = append(args, g.generateExpr(arg, m))
		}
		return jen.Id("sendMessage").Call(args...)

	case *parser.JSONPrimitiveExpr:
		return g.generateJSONPrimitive(e, m)

	case *parser.ClassPrimitiveExpr:
		return g.generateClassPrimitive(e, m)

	case *parser.BlockExpr:
		// Block used as expression (e.g., [i < len] whileTrue: [...])
		// If it has a single expression statement, extract and evaluate it
		if len(e.Statements) == 1 {
			if exprStmt, ok := e.Statements[0].(*parser.ExprStmt); ok {
				return g.generateExpr(exprStmt.Expr, m)
			}
		}
		// Complex block - can't inline as expression
		return jen.Comment("complex block expression not supported")

	default:
		return jen.Comment("unknown expr")
	}
}

// generateStringArg generates an expression that keeps the original string value
// (unlike generateExpr which converts method args to int)
func (g *generator) generateStringArg(expr parser.Expr, m *compiledMethod) *jen.Statement {
	switch e := expr.(type) {
	case *parser.Identifier:
		// For identifiers, check if it's a method arg - if so, use the original string
		for _, arg := range m.args {
			if arg == e.Name {
				return jen.Id(e.Name) // Use original string parameter
			}
		}
		// Otherwise, fall through to normal generation
		return g.generateExpr(expr, m)
	case *parser.StringLit:
		return jen.Lit(e.Value)
	default:
		// For other expressions, use normal generation
		return g.generateExpr(expr, m)
	}
}

// generateJSONPrimitive generates Go code for JSON primitive operations
func (g *generator) generateJSONPrimitive(e *parser.JSONPrimitiveExpr, m *compiledMethod) *jen.Statement {
	receiver := g.generateExpr(e.Receiver, m)

	// Check if receiver expression results in a typed array/object
	// This handles both direct ivar access and chained operations
	isArrayType := g.exprResultsInArray(e.Receiver)
	isObjectType := g.exprResultsInObject(e.Receiver)

	switch e.Operation {
	// Array operations
	case "arrayLength":
		// Return string for consistency - all JSON primitives return strings
		if isArrayType {
			return jen.Qual("strconv", "Itoa").Call(jen.Len(receiver))
		}
		// Fallback: string containing JSON - _jsonArrayLen returns int, wrap in Itoa
		return jen.Qual("strconv", "Itoa").Call(jen.Id("_jsonArrayLen").Call(receiver))

	case "arrayFirst":
		if isArrayType {
			// Return string for consistency - _arrayFirst returns interface{}, convert to string
			return jen.Id("_toStr").Call(jen.Id("_arrayFirst").Call(receiver))
		}
		// JSON array - _jsonArrayFirst returns string
		return jen.Id("_jsonArrayFirst").Call(receiver)

	case "arrayLast":
		if isArrayType {
			// Return string for consistency - _arrayLast returns interface{}, convert to string
			return jen.Id("_toStr").Call(jen.Id("_arrayLast").Call(receiver))
		}
		// JSON array - _jsonArrayLast returns string
		return jen.Id("_jsonArrayLast").Call(receiver)

	case "arrayIsEmpty":
		if isArrayType {
			return jen.Id("_boolToString").Call(jen.Len(receiver).Op("==").Lit(0))
		}
		return jen.Id("_boolToString").Call(jen.Id("_jsonArrayIsEmpty").Call(receiver))

	case "arrayPush":
		arg := g.generateExpr(e.Args[0], m)
		if isArrayType {
			// Optimization: combine chained arrayPush into single append
			// e.g., items arrayPush: x arrayPush: y -> append(c.Items, x, y)
			allArgs, baseReceiver := g.collectArrayPushArgs(e, m)
			if len(allArgs) > 1 {
				// Build append(base, arg1, arg2, ...) - all args in one slice
				codes := make([]jen.Code, 0, len(allArgs)+1)
				codes = append(codes, baseReceiver)
				for _, a := range allArgs {
					codes = append(codes, a)
				}
				return jen.Append(codes...)
			}
			// Single append
			return jen.Append(receiver, arg)
		}
		return jen.Id("_jsonArrayPush").Call(receiver, arg)

	case "arrayAt":
		idx := g.generateExpr(e.Args[0], m)
		if isArrayType {
			// Return string for consistency - element is interface{}, convert to string
			return jen.Id("_toStr").Call(receiver.Clone().Index(jen.Id("toInt").Call(idx)))
		}
		// JSON array - _jsonArrayAt returns string, wrap idx in toInt for interface{} compatibility
		return jen.Id("_jsonArrayAt").Call(receiver, jen.Id("toInt").Call(idx))

	case "arrayAtPut":
		idx := g.generateExpr(e.Args[0], m)
		val := g.generateExpr(e.Args[1], m)
		if isArrayType {
			// Return new slice with updated element (immutable style)
			return jen.Id("_arrayAtPut").Call(receiver, jen.Id("toInt").Call(idx), val)
		}
		return jen.Id("_jsonArrayAtPut").Call(receiver, jen.Id("toInt").Call(idx), val)

	case "arrayRemoveAt":
		idx := g.generateExpr(e.Args[0], m)
		if isArrayType {
			return jen.Id("_arrayRemoveAt").Call(receiver, jen.Id("toInt").Call(idx))
		}
		return jen.Id("_jsonArrayRemoveAt").Call(receiver, jen.Id("toInt").Call(idx))

	// Object operations
	case "objectLength":
		if isObjectType {
			return jen.Qual("strconv", "Itoa").Call(jen.Len(receiver))
		}
		return jen.Qual("strconv", "Itoa").Call(jen.Id("_jsonObjectLen").Call(receiver))

	case "objectKeys":
		if isObjectType {
			return jen.Id("_mapKeys").Call(receiver)
		}
		return jen.Id("_jsonObjectKeys").Call(receiver)

	case "objectValues":
		if isObjectType {
			return jen.Id("_mapValues").Call(receiver)
		}
		return jen.Id("_jsonObjectValues").Call(receiver)

	case "objectIsEmpty":
		if isObjectType {
			return jen.Id("_boolToString").Call(jen.Len(receiver).Op("==").Lit(0))
		}
		return jen.Id("_boolToString").Call(jen.Id("_jsonObjectIsEmpty").Call(receiver))

	case "objectAt":
		key := g.generateStringArg(e.Args[0], m) // Object keys are always strings
		if isObjectType {
			return jen.Id("_toStr").Call(receiver.Clone().Index(key))
		}
		return jen.Id("_jsonObjectAt").Call(receiver, key)

	case "objectAtPut":
		key := g.generateStringArg(e.Args[0], m) // Object keys are always strings
		val := g.generateExpr(e.Args[1], m)
		if isObjectType {
			// Return new map with updated key (immutable style)
			return jen.Id("_mapAtPut").Call(receiver, key, val)
		}
		return jen.Id("_jsonObjectAtPut").Call(receiver, key, val)

	case "objectHasKey":
		key := g.generateStringArg(e.Args[0], m) // Object keys are always strings
		if isObjectType {
			return jen.Id("_boolToString").Call(jen.Id("_mapHasKey").Call(receiver, key))
		}
		return jen.Id("_boolToString").Call(jen.Id("_jsonObjectHasKey").Call(receiver, key))

	case "objectRemoveKey":
		key := g.generateStringArg(e.Args[0], m) // Object keys are always strings
		if isObjectType {
			return jen.Id("_mapRemoveKey").Call(receiver, key)
		}
		return jen.Id("_jsonObjectRemoveKey").Call(receiver, key)

	default:
		return jen.Comment("unknown JSON primitive: " + e.Operation)
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

// isBlockInvocationSelector returns true if the selector is used to invoke blocks
// These are: value, valueWith:, valueWith:and:
// Note: Parser transforms "valueWith:" to "valueWith_" and "valueWith:and:" to "valueWith_and_"
func isBlockInvocationSelector(selector string) bool {
	switch selector {
	case "value", "valueWith_", "valueWith_and_":
		return true
	default:
		return false
	}
}

// generateClassPrimitive generates Go code for class primitive operations
// like @ String isEmpty: str, @ File exists: path
func (g *generator) generateClassPrimitive(e *parser.ClassPrimitiveExpr, m *compiledMethod) *jen.Statement {
	switch e.ClassName {
	case "String":
		return g.generateStringPrimitive(e, m)
	case "File":
		return g.generateFilePrimitive(e, m)
	default:
		return jen.Comment("unknown class primitive: " + e.ClassName)
	}
}

// generateStringPrimitive generates Go code for String class primitives
func (g *generator) generateStringPrimitive(e *parser.ClassPrimitiveExpr, m *compiledMethod) *jen.Statement {
	switch e.Operation {
	case "stringIsEmpty":
		arg := g.generateStringArg(e.Args[0], m)
		return jen.Id("_boolToString").Call(jen.Len(arg).Op("==").Lit(0))

	case "stringNotEmpty":
		arg := g.generateStringArg(e.Args[0], m)
		return jen.Id("_boolToString").Call(jen.Len(arg).Op(">").Lit(0))

	case "stringContains":
		str := g.generateStringArg(e.Args[0], m)
		sub := g.generateStringArg(e.Args[1], m)
		return jen.Id("_boolToString").Call(
			jen.Qual("strings", "Contains").Call(str, sub),
		)

	case "stringStartsWith":
		str := g.generateStringArg(e.Args[0], m)
		prefix := g.generateStringArg(e.Args[1], m)
		return jen.Id("_boolToString").Call(
			jen.Qual("strings", "HasPrefix").Call(str, prefix),
		)

	case "stringEndsWith":
		str := g.generateStringArg(e.Args[0], m)
		suffix := g.generateStringArg(e.Args[1], m)
		return jen.Id("_boolToString").Call(
			jen.Qual("strings", "HasSuffix").Call(str, suffix),
		)

	case "stringEquals":
		a := g.generateStringArg(e.Args[0], m)
		b := g.generateStringArg(e.Args[1], m)
		return jen.Id("_boolToString").Call(a.Op("==").Add(b))

	case "stringTrimPrefix":
		// trimPrefix: pattern from: str -> TrimPrefix(str, pattern)
		pattern := g.generateStringArg(e.Args[0], m)
		str := g.generateStringArg(e.Args[1], m)
		return jen.Qual("strings", "TrimPrefix").Call(str, pattern)

	case "stringTrimSuffix":
		// trimSuffix: pattern from: str -> TrimSuffix(str, pattern)
		pattern := g.generateStringArg(e.Args[0], m)
		str := g.generateStringArg(e.Args[1], m)
		return jen.Qual("strings", "TrimSuffix").Call(str, pattern)

	case "stringReplace":
		// replace: old with: new in: str -> Replace(str, old, new, 1)
		old := g.generateStringArg(e.Args[0], m)
		newStr := g.generateStringArg(e.Args[1], m)
		str := g.generateStringArg(e.Args[2], m)
		return jen.Qual("strings", "Replace").Call(str, old, newStr, jen.Lit(1))

	case "stringReplaceAll":
		// replaceAll: old with: new in: str -> ReplaceAll(str, old, new)
		old := g.generateStringArg(e.Args[0], m)
		newStr := g.generateStringArg(e.Args[1], m)
		str := g.generateStringArg(e.Args[2], m)
		return jen.Qual("strings", "ReplaceAll").Call(str, old, newStr)

	case "stringSubstring":
		// substring: str from: start length: len
		str := g.generateStringArg(e.Args[0], m)
		start := g.generateExpr(e.Args[1], m)
		length := g.generateExpr(e.Args[2], m)
		return jen.Id("_stringSubstring").Call(str, jen.Id("toInt").Call(start), jen.Id("toInt").Call(length))

	case "stringLength":
		arg := g.generateStringArg(e.Args[0], m)
		return jen.Qual("strconv", "Itoa").Call(jen.Len(arg))

	case "stringUppercase":
		arg := g.generateStringArg(e.Args[0], m)
		return jen.Qual("strings", "ToUpper").Call(arg)

	case "stringLowercase":
		arg := g.generateStringArg(e.Args[0], m)
		return jen.Qual("strings", "ToLower").Call(arg)

	case "stringTrim":
		arg := g.generateStringArg(e.Args[0], m)
		return jen.Qual("strings", "TrimSpace").Call(arg)

	case "stringConcat":
		a := g.generateStringArg(e.Args[0], m)
		b := g.generateStringArg(e.Args[1], m)
		return a.Op("+").Add(b)

	default:
		return jen.Comment("unknown string primitive: " + e.Operation)
	}
}

// generateFilePrimitive generates Go code for File class primitives
func (g *generator) generateFilePrimitive(e *parser.ClassPrimitiveExpr, m *compiledMethod) *jen.Statement {
	switch e.Operation {
	case "fileExists":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileExists").Call(path)

	case "fileIsFile":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileIsFile").Call(path)

	case "fileIsDirectory":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileIsDirectory").Call(path)

	case "fileIsSymlink":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileIsSymlink").Call(path)

	case "fileIsFifo":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileIsFifo").Call(path)

	case "fileIsSocket":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileIsSocket").Call(path)

	case "fileIsBlockDevice":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileIsBlockDevice").Call(path)

	case "fileIsCharDevice":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileIsCharDevice").Call(path)

	case "fileIsReadable":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileIsReadable").Call(path)

	case "fileIsWritable":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileIsWritable").Call(path)

	case "fileIsExecutable":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileIsExecutable").Call(path)

	case "fileIsEmpty":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileIsEmpty").Call(path)

	case "fileNotEmpty":
		path := g.generateStringArg(e.Args[0], m)
		return jen.Id("_fileNotEmpty").Call(path)

	case "fileIsNewer":
		path1 := g.generateStringArg(e.Args[0], m)
		path2 := g.generateStringArg(e.Args[1], m)
		return jen.Id("_fileIsNewer").Call(path1, path2)

	case "fileIsOlder":
		path1 := g.generateStringArg(e.Args[0], m)
		path2 := g.generateStringArg(e.Args[1], m)
		return jen.Id("_fileIsOlder").Call(path1, path2)

	case "fileIsSame":
		path1 := g.generateStringArg(e.Args[0], m)
		path2 := g.generateStringArg(e.Args[1], m)
		return jen.Id("_fileIsSame").Call(path1, path2)

	default:
		return jen.Comment("unknown file primitive: " + e.Operation)
	}
}

// hasReturnInStatements recursively checks if any statement contains a return
func hasReturnInStatements(stmts []parser.Statement) bool {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *parser.Return:
			return true
		case *parser.IfExpr:
			// Check inside both branches
			if hasReturnInStatements(s.TrueBlock) {
				return true
			}
			if hasReturnInStatements(s.FalseBlock) {
				return true
			}
		case *parser.WhileExpr:
			// Check inside loop body
			if hasReturnInStatements(s.Body) {
				return true
			}
		}
	}
	return false
}

// goBuiltins are Go builtin identifiers that cannot be used as variable names
var goBuiltins = map[string]bool{
	// Builtin functions
	"len": true, "cap": true, "make": true, "new": true, "append": true,
	"copy": true, "delete": true, "close": true, "panic": true, "recover": true,
	"print": true, "println": true, "complex": true, "real": true, "imag": true,
	// Constants
	"true": true, "false": true, "nil": true, "iota": true,
	// Types
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
	"byte": true, "rune": true, "string": true, "bool": true, "error": true,
	// Go keywords (cannot be used as identifiers)
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// safeGoName returns a safe Go identifier, renaming if it conflicts with builtins
func safeGoName(name string) string {
	if goBuiltins[name] {
		return name + "_"
	}
	return name
}

// collectArrayPushArgs collects all arguments from a chain of arrayPush operations
// Returns the collected args and the base receiver (the original array ivar)
// e.g., for "items arrayPush: x arrayPush: y arrayPush: z", returns ([x,y,z], c.Items)
func (g *generator) collectArrayPushArgs(e *parser.JSONPrimitiveExpr, m *compiledMethod) ([]*jen.Statement, *jen.Statement) {
	if e.Operation != "arrayPush" {
		return nil, nil
	}

	// Collect args in reverse order (innermost first)
	var args []*jen.Statement
	current := e

	for {
		// Add current arg
		arg := g.generateExpr(current.Args[0], m)
		args = append(args, arg)

		// Check if receiver is another arrayPush
		if inner, ok := current.Receiver.(*parser.JSONPrimitiveExpr); ok && inner.Operation == "arrayPush" {
			current = inner
		} else {
			// Base case: receiver is not arrayPush
			break
		}
	}

	// Get the base receiver (the original array)
	baseReceiver := g.generateExpr(current.Receiver, m)

	// Reverse args to get correct order (outermost first)
	for i, j := 0, len(args)-1; i < j; i, j = i+1, j-1 {
		args[i], args[j] = args[j], args[i]
	}

	return args, baseReceiver
}

// generateJSONHelpers generates helper functions for JSON primitive operations
func (g *generator) generateJSONHelpers(f *jen.File) {
	// Common conversion helpers
	f.Comment("// Common conversion helpers")

	// _boolToString - convert bool to "true"/"false" string
	f.Func().Id("_boolToString").Params(jen.Id("b").Bool()).String().Block(
		jen.If(jen.Id("b")).Block(
			jen.Return(jen.Lit("true")),
		),
		jen.Return(jen.Lit("false")),
	)
	f.Line()

	// _toStr - convert interface{} to string
	f.Func().Id("_toStr").Params(jen.Id("v").Interface()).String().Block(
		jen.If(jen.Id("v").Op("==").Nil()).Block(
			jen.Return(jen.Lit("")),
		),
		jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("v"))),
	)
	f.Line()

	// Array helpers for []interface{} typed fields
	f.Comment("// Array helpers for native slice operations")

	// _arrayFirst - get first element of slice
	f.Func().Id("_arrayFirst").Params(jen.Id("arr").Index().Interface()).Interface().Block(
		jen.If(jen.Len(jen.Id("arr")).Op("==").Lit(0)).Block(
			jen.Return(jen.Nil()),
		),
		jen.Return(jen.Id("arr").Index(jen.Lit(0))),
	)
	f.Line()

	// _arrayLast - get last element of slice
	f.Func().Id("_arrayLast").Params(jen.Id("arr").Index().Interface()).Interface().Block(
		jen.If(jen.Len(jen.Id("arr")).Op("==").Lit(0)).Block(
			jen.Return(jen.Nil()),
		),
		jen.Return(jen.Id("arr").Index(jen.Len(jen.Id("arr")).Op("-").Lit(1))),
	)
	f.Line()

	// _arrayAtPut - return new slice with element at index replaced
	f.Func().Id("_arrayAtPut").Params(
		jen.Id("arr").Index().Interface(),
		jen.Id("idx").Int(),
		jen.Id("val").Interface(),
	).Index().Interface().Block(
		jen.Comment("Handle negative indices"),
		jen.If(jen.Id("idx").Op("<").Lit(0)).Block(
			jen.Id("idx").Op("=").Len(jen.Id("arr")).Op("+").Id("idx"),
		),
		jen.If(jen.Id("idx").Op("<").Lit(0).Op("||").Id("idx").Op(">=").Len(jen.Id("arr"))).Block(
			jen.Return(jen.Id("arr")),
		),
		jen.Id("result").Op(":=").Make(jen.Index().Interface(), jen.Len(jen.Id("arr"))),
		jen.Copy(jen.Id("result"), jen.Id("arr")),
		jen.Id("result").Index(jen.Id("idx")).Op("=").Id("val"),
		jen.Return(jen.Id("result")),
	)
	f.Line()

	// _arrayRemoveAt - return new slice with element at index removed
	f.Func().Id("_arrayRemoveAt").Params(
		jen.Id("arr").Index().Interface(),
		jen.Id("idx").Int(),
	).Index().Interface().Block(
		jen.Comment("Handle negative indices"),
		jen.If(jen.Id("idx").Op("<").Lit(0)).Block(
			jen.Id("idx").Op("=").Len(jen.Id("arr")).Op("+").Id("idx"),
		),
		jen.If(jen.Id("idx").Op("<").Lit(0).Op("||").Id("idx").Op(">=").Len(jen.Id("arr"))).Block(
			jen.Return(jen.Id("arr")),
		),
		jen.Id("result").Op(":=").Make(jen.Index().Interface(), jen.Lit(0), jen.Len(jen.Id("arr")).Op("-").Lit(1)),
		jen.Id("result").Op("=").Append(jen.Id("result"), jen.Id("arr").Index(jen.Op(":").Id("idx")).Op("...")),
		jen.Id("result").Op("=").Append(jen.Id("result"), jen.Id("arr").Index(jen.Id("idx").Op("+").Lit(1).Op(":")).Op("...")),
		jen.Return(jen.Id("result")),
	)
	f.Line()

	// Map helpers for map[string]interface{} typed fields
	f.Comment("// Map helpers for native map operations")

	// _mapKeys - get all keys from map
	f.Func().Id("_mapKeys").Params(jen.Id("m").Map(jen.String()).Interface()).Index().String().Block(
		jen.Id("keys").Op(":=").Make(jen.Index().String(), jen.Lit(0), jen.Len(jen.Id("m"))),
		jen.For(jen.Id("k").Op(":=").Range().Id("m")).Block(
			jen.Id("keys").Op("=").Append(jen.Id("keys"), jen.Id("k")),
		),
		jen.Return(jen.Id("keys")),
	)
	f.Line()

	// _mapValues - get all values from map
	f.Func().Id("_mapValues").Params(jen.Id("m").Map(jen.String()).Interface()).Index().Interface().Block(
		jen.Id("vals").Op(":=").Make(jen.Index().Interface(), jen.Lit(0), jen.Len(jen.Id("m"))),
		jen.For(jen.List(jen.Id("_"), jen.Id("v")).Op(":=").Range().Id("m")).Block(
			jen.Id("vals").Op("=").Append(jen.Id("vals"), jen.Id("v")),
		),
		jen.Return(jen.Id("vals")),
	)
	f.Line()

	// _mapHasKey - check if key exists
	f.Func().Id("_mapHasKey").Params(
		jen.Id("m").Map(jen.String()).Interface(),
		jen.Id("key").String(),
	).Bool().Block(
		jen.List(jen.Id("_"), jen.Id("ok")).Op(":=").Id("m").Index(jen.Id("key")),
		jen.Return(jen.Id("ok")),
	)
	f.Line()

	// _mapAtPut - return new map with key set (immutable style)
	f.Func().Id("_mapAtPut").Params(
		jen.Id("m").Map(jen.String()).Interface(),
		jen.Id("key").String(),
		jen.Id("val").Interface(),
	).Map(jen.String()).Interface().Block(
		jen.Id("result").Op(":=").Make(jen.Map(jen.String()).Interface(), jen.Len(jen.Id("m")).Op("+").Lit(1)),
		jen.For(jen.List(jen.Id("k"), jen.Id("v")).Op(":=").Range().Id("m")).Block(
			jen.Id("result").Index(jen.Id("k")).Op("=").Id("v"),
		),
		jen.Id("result").Index(jen.Id("key")).Op("=").Id("val"),
		jen.Return(jen.Id("result")),
	)
	f.Line()

	// _mapRemoveKey - return new map with key removed
	f.Func().Id("_mapRemoveKey").Params(
		jen.Id("m").Map(jen.String()).Interface(),
		jen.Id("key").String(),
	).Map(jen.String()).Interface().Block(
		jen.Id("result").Op(":=").Make(jen.Map(jen.String()).Interface(), jen.Len(jen.Id("m"))),
		jen.For(jen.List(jen.Id("k"), jen.Id("v")).Op(":=").Range().Id("m")).Block(
			jen.If(jen.Id("k").Op("!=").Id("key")).Block(
				jen.Id("result").Index(jen.Id("k")).Op("=").Id("v"),
			),
		),
		jen.Return(jen.Id("result")),
	)
	f.Line()

	// JSON string parsing helpers (for string-typed fields containing JSON)
	f.Comment("// JSON string parsing helpers (for string-typed variables containing JSON)")

	// _jsonArrayLen
	f.Func().Id("_jsonArrayLen").Params(jen.Id("jsonStr").String()).Int().Block(
		jen.Var().Id("arr").Index().Interface(),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("arr"),
		).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(0)),
		),
		jen.Return(jen.Len(jen.Id("arr"))),
	)
	f.Line()

	// _jsonArrayFirst
	f.Func().Id("_jsonArrayFirst").Params(jen.Id("jsonStr").String()).String().Block(
		jen.Var().Id("arr").Index().Interface(),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("arr"),
		).Op(";").Err().Op("!=").Nil().Op("||").Len(jen.Id("arr")).Op("==").Lit(0)).Block(
			jen.Return(jen.Lit("")),
		),
		jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("arr").Index(jen.Lit(0)))),
	)
	f.Line()

	// _jsonArrayLast
	f.Func().Id("_jsonArrayLast").Params(jen.Id("jsonStr").String()).String().Block(
		jen.Var().Id("arr").Index().Interface(),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("arr"),
		).Op(";").Err().Op("!=").Nil().Op("||").Len(jen.Id("arr")).Op("==").Lit(0)).Block(
			jen.Return(jen.Lit("")),
		),
		jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("arr").Index(jen.Len(jen.Id("arr")).Op("-").Lit(1)))),
	)
	f.Line()

	// _jsonArrayIsEmpty
	f.Func().Id("_jsonArrayIsEmpty").Params(jen.Id("jsonStr").String()).Bool().Block(
		jen.Var().Id("arr").Index().Interface(),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("arr"),
		).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.True()),
		),
		jen.Return(jen.Len(jen.Id("arr")).Op("==").Lit(0)),
	)
	f.Line()

	// _jsonArrayPush - accepts interface{} for jsonStr to handle interface{} local variables
	f.Func().Id("_jsonArrayPush").Params(
		jen.Id("jsonVal").Interface(),
		jen.Id("val").Interface(),
	).String().Block(
		jen.Id("jsonStr").Op(":=").Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("jsonVal")),
		jen.Var().Id("arr").Index().Interface(),
		jen.Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("arr"),
		),
		jen.Id("arr").Op("=").Append(jen.Id("arr"), jen.Id("val")),
		jen.List(jen.Id("result"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("arr")),
		jen.Return(jen.String().Parens(jen.Id("result"))),
	)
	f.Line()

	// _jsonArrayAt
	f.Func().Id("_jsonArrayAt").Params(
		jen.Id("jsonStr").String(),
		jen.Id("idx").Int(),
	).String().Block(
		jen.Var().Id("arr").Index().Interface(),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("arr"),
		).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit("")),
		),
		jen.If(jen.Id("idx").Op("<").Lit(0)).Block(
			jen.Id("idx").Op("=").Len(jen.Id("arr")).Op("+").Id("idx"),
		),
		jen.If(jen.Id("idx").Op("<").Lit(0).Op("||").Id("idx").Op(">=").Len(jen.Id("arr"))).Block(
			jen.Return(jen.Lit("")),
		),
		jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("arr").Index(jen.Id("idx")))),
	)
	f.Line()

	// _jsonArrayAtPut
	f.Func().Id("_jsonArrayAtPut").Params(
		jen.Id("jsonStr").String(),
		jen.Id("idx").Int(),
		jen.Id("val").Interface(),
	).String().Block(
		jen.Var().Id("arr").Index().Interface(),
		jen.Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("arr"),
		),
		jen.If(jen.Id("idx").Op("<").Lit(0)).Block(
			jen.Id("idx").Op("=").Len(jen.Id("arr")).Op("+").Id("idx"),
		),
		jen.If(jen.Id("idx").Op(">=").Lit(0).Op("&&").Id("idx").Op("<").Len(jen.Id("arr"))).Block(
			jen.Id("arr").Index(jen.Id("idx")).Op("=").Id("val"),
		),
		jen.List(jen.Id("result"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("arr")),
		jen.Return(jen.String().Parens(jen.Id("result"))),
	)
	f.Line()

	// _jsonArrayRemoveAt
	f.Func().Id("_jsonArrayRemoveAt").Params(
		jen.Id("jsonStr").String(),
		jen.Id("idx").Int(),
	).String().Block(
		jen.Var().Id("arr").Index().Interface(),
		jen.Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("arr"),
		),
		jen.If(jen.Id("idx").Op("<").Lit(0)).Block(
			jen.Id("idx").Op("=").Len(jen.Id("arr")).Op("+").Id("idx"),
		),
		jen.If(jen.Id("idx").Op(">=").Lit(0).Op("&&").Id("idx").Op("<").Len(jen.Id("arr"))).Block(
			jen.Id("arr").Op("=").Append(jen.Id("arr").Index(jen.Op(":").Id("idx")), jen.Id("arr").Index(jen.Id("idx").Op("+").Lit(1).Op(":")).Op("...")),
		),
		jen.List(jen.Id("result"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("arr")),
		jen.Return(jen.String().Parens(jen.Id("result"))),
	)
	f.Line()

	// Object JSON helpers
	// _jsonObjectLen
	f.Func().Id("_jsonObjectLen").Params(jen.Id("jsonStr").String()).Int().Block(
		jen.Var().Id("m").Map(jen.String()).Interface(),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("m"),
		).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(0)),
		),
		jen.Return(jen.Len(jen.Id("m"))),
	)
	f.Line()

	// _jsonObjectKeys
	f.Func().Id("_jsonObjectKeys").Params(jen.Id("jsonStr").String()).Index().String().Block(
		jen.Var().Id("m").Map(jen.String()).Interface(),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("m"),
		).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.Nil()),
		),
		jen.Return(jen.Id("_mapKeys").Call(jen.Id("m"))),
	)
	f.Line()

	// _jsonObjectValues
	f.Func().Id("_jsonObjectValues").Params(jen.Id("jsonStr").String()).Index().Interface().Block(
		jen.Var().Id("m").Map(jen.String()).Interface(),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("m"),
		).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.Nil()),
		),
		jen.Return(jen.Id("_mapValues").Call(jen.Id("m"))),
	)
	f.Line()

	// _jsonObjectIsEmpty
	f.Func().Id("_jsonObjectIsEmpty").Params(jen.Id("jsonStr").String()).Bool().Block(
		jen.Var().Id("m").Map(jen.String()).Interface(),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("m"),
		).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.True()),
		),
		jen.Return(jen.Len(jen.Id("m")).Op("==").Lit(0)),
	)
	f.Line()

	// _jsonObjectAt - accepts any to handle both string and interface{} local vars
	f.Func().Id("_jsonObjectAt").Params(
		jen.Id("jsonVal").Any(),
		jen.Id("key").String(),
	).String().Block(
		jen.Id("jsonStr").Op(":=").Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("jsonVal")),
		jen.Var().Id("m").Map(jen.String()).Interface(),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("m"),
		).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit("")),
		),
		jen.If(jen.List(jen.Id("v"), jen.Id("ok")).Op(":=").Id("m").Index(jen.Id("key")).Op(";").Id("ok")).Block(
			jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("v"))),
		),
		jen.Return(jen.Lit("")),
	)
	f.Line()

	// _jsonObjectAtPut - accepts any to handle both string and interface{} local vars
	f.Func().Id("_jsonObjectAtPut").Params(
		jen.Id("jsonVal").Any(),
		jen.Id("key").String(),
		jen.Id("val").Any(),
	).String().Block(
		jen.Id("jsonStr").Op(":=").Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("jsonVal")),
		jen.Var().Id("m").Map(jen.String()).Interface(),
		jen.Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("m"),
		),
		jen.If(jen.Id("m").Op("==").Nil()).Block(
			jen.Id("m").Op("=").Make(jen.Map(jen.String()).Interface()),
		),
		jen.Id("m").Index(jen.Id("key")).Op("=").Id("val"),
		jen.List(jen.Id("result"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("m")),
		jen.Return(jen.String().Parens(jen.Id("result"))),
	)
	f.Line()

	// _jsonObjectHasKey - accepts any to handle both string and interface{} local vars
	f.Func().Id("_jsonObjectHasKey").Params(
		jen.Id("jsonVal").Any(),
		jen.Id("key").String(),
	).Bool().Block(
		jen.Id("jsonStr").Op(":=").Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("jsonVal")),
		jen.Var().Id("m").Map(jen.String()).Interface(),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("m"),
		).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.False()),
		),
		jen.List(jen.Id("_"), jen.Id("ok")).Op(":=").Id("m").Index(jen.Id("key")),
		jen.Return(jen.Id("ok")),
	)
	f.Line()

	// _jsonObjectRemoveKey - accepts any to handle both string and interface{} local vars
	f.Func().Id("_jsonObjectRemoveKey").Params(
		jen.Id("jsonVal").Any(),
		jen.Id("key").String(),
	).String().Block(
		jen.Id("jsonStr").Op(":=").Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("jsonVal")),
		jen.Var().Id("m").Map(jen.String()).Interface(),
		jen.Qual("encoding/json", "Unmarshal").Call(
			jen.Index().Byte().Parens(jen.Id("jsonStr")),
			jen.Op("&").Id("m"),
		),
		jen.Delete(jen.Id("m"), jen.Id("key")),
		jen.List(jen.Id("result"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("m")),
		jen.Return(jen.String().Parens(jen.Id("result"))),
	)
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

// generateGrpcClientMethod generates specialized gRPC implementations for GrpcClient methods.
// Returns true if the method was handled, false to fall through to default generation.
// Note: selectors use underscores (from AST), not colons (from source syntax).
func (g *generator) generateGrpcClientMethod(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	case "call_with_":
		// Unary call: call: method with: jsonPayload
		f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id(m.goName).Params(
			jen.Id("method").String(),
			jen.Id("jsonPayload").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("c").Dot("grpcCall").Call(jen.Id("method"), jen.Id("jsonPayload"))),
		)
		f.Line()
		return true

	case "call_":
		// Unary call with empty payload: call: method
		f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id(m.goName).Params(
			jen.Id("method").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("c").Dot("grpcCall").Call(jen.Id("method"), jen.Lit("{}"))),
		)
		f.Line()
		return true

	case "serverStream_with_handler_":
		// Server streaming: serverStream: method with: payload handler: block
		f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id(m.goName).Params(
			jen.Id("method").String(),
			jen.Id("payload").String(),
			jen.Id("handlerBlockID").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("c").Dot("serverStream").Call(
				jen.Id("method"),
				jen.Id("payload"),
				jen.Id("handlerBlockID"),
			)),
		)
		f.Line()
		return true

	case "clientStream_handler_":
		// Client streaming: clientStream: method handler: block
		f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id(m.goName).Params(
			jen.Id("method").String(),
			jen.Id("handlerBlockID").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("c").Dot("clientStream").Call(
				jen.Id("method"),
				jen.Id("handlerBlockID"),
			)),
		)
		f.Line()
		return true

	case "bidiStream_handler_":
		// Bidi streaming: bidiStream: method handler: block
		f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id(m.goName).Params(
			jen.Id("method").String(),
			jen.Id("handlerBlockID").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("c").Dot("bidiStream").Call(
				jen.Id("method"),
				jen.Id("handlerBlockID"),
			)),
		)
		f.Line()
		return true

	case "listServices":
		// List services via reflection
		f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("conn"), jen.Err()).Op(":=").Id("c").Dot("getConnection").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.If(jen.Id("c").Dot("PoolConnections").Op("!=").Lit("yes")).Block(
				jen.Defer().Id("conn").Dot("Close").Call(),
			),
			jen.Line(),
			jen.Id("ctx").Op(":=").Qual("context", "Background").Call(),
			jen.Id("refClient").Op(":=").Qual("github.com/jhump/protoreflect/grpcreflect", "NewClientAuto").Call(
				jen.Id("ctx"),
				jen.Id("conn"),
			),
			jen.Defer().Id("refClient").Dot("Reset").Call(),
			jen.Line(),
			jen.List(jen.Id("services"), jen.Err()).Op(":=").Id("refClient").Dot("ListServices").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Qual("strings", "Join").Call(jen.Id("services"), jen.Lit("\n")), jen.Nil()),
		)
		f.Line()
		return true

	case "listMethods_":
		// List methods for a service
		f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id(m.goName).Params(
			jen.Id("serviceName").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("conn"), jen.Err()).Op(":=").Id("c").Dot("getConnection").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.If(jen.Id("c").Dot("PoolConnections").Op("!=").Lit("yes")).Block(
				jen.Defer().Id("conn").Dot("Close").Call(),
			),
			jen.Line(),
			jen.Id("ctx").Op(":=").Qual("context", "Background").Call(),
			jen.Id("refClient").Op(":=").Qual("github.com/jhump/protoreflect/grpcreflect", "NewClientAuto").Call(
				jen.Id("ctx"),
				jen.Id("conn"),
			),
			jen.Defer().Id("refClient").Dot("Reset").Call(),
			jen.Line(),
			jen.List(jen.Id("svcDesc"), jen.Err()).Op(":=").Id("refClient").Dot("ResolveService").Call(jen.Id("serviceName")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Line(),
			jen.Var().Id("methods").Index().String(),
			jen.For(jen.List(jen.Id("_"), jen.Id("m")).Op(":=").Range().Id("svcDesc").Dot("GetMethods").Call()).Block(
				jen.Id("methods").Op("=").Append(jen.Id("methods"), jen.Id("m").Dot("GetName").Call()),
			),
			jen.Return(jen.Qual("strings", "Join").Call(jen.Id("methods"), jen.Lit("\n")), jen.Nil()),
		)
		f.Line()
		return true

	default:
		// Not a special GrpcClient method - fall through to default generation
		return false
	}
}

// generateGrpcHelpers generates helper functions for GrpcClient class
func (g *generator) generateGrpcHelpers(f *jen.File) {
	f.Line()
	f.Comment("// gRPC helper functions for GrpcClient")
	f.Line()

	// getConnection - lazy connection creation
	f.Comment("// getConnection returns an existing connection or creates a new one")
	f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id("getConnection").Params().Parens(jen.List(
		jen.Op("*").Qual("google.golang.org/grpc", "ClientConn"),
		jen.Error(),
	)).Block(
		jen.If(jen.Id("c").Dot("conn").Op("!=").Nil()).Block(
			jen.Return(jen.Id("c").Dot("conn"), jen.Nil()),
		),
		jen.Var().Id("opts").Index().Qual("google.golang.org/grpc", "DialOption"),
		jen.If(jen.Id("c").Dot("UsePlaintext").Op("==").Lit("yes")).Block(
			jen.Id("opts").Op("=").Append(jen.Id("opts"), jen.Qual("google.golang.org/grpc", "WithTransportCredentials").Call(
				jen.Qual("google.golang.org/grpc/credentials/insecure", "NewCredentials").Call(),
			)),
		),
		jen.List(jen.Id("conn"), jen.Err()).Op(":=").Qual("google.golang.org/grpc", "NewClient").Call(
			jen.Id("c").Dot("Address"),
			jen.Id("opts").Op("..."),
		),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Nil(), jen.Err()),
		),
		jen.If(jen.Id("c").Dot("PoolConnections").Op("==").Lit("yes")).Block(
			jen.Id("c").Dot("conn").Op("=").Id("conn"),
		),
		jen.Return(jen.Id("conn"), jen.Nil()),
	)
	f.Line()

	// closeConnection - closes the pooled connection if any
	f.Comment("// closeConnection closes the pooled connection if any")
	f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id("closeConnection").Params().Block(
		jen.If(jen.Id("c").Dot("conn").Op("!=").Nil()).Block(
			jen.Id("c").Dot("conn").Dot("Close").Call(),
			jen.Id("c").Dot("conn").Op("=").Nil(),
		),
	)
	f.Line()

	// loadProtoFile - parses and caches proto file descriptors
	f.Comment("// loadProtoFile parses a proto file and caches the descriptors")
	f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id("loadProtoFile").Params().Error().Block(
		jen.If(jen.Len(jen.Id("c").Dot("fileDescs")).Op(">").Lit(0)).Block(
			jen.Return(jen.Nil()), // Already loaded
		),
		jen.If(jen.Id("c").Dot("ProtoFile").Op("==").Lit("")).Block(
			jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("no proto file specified"))),
		),
		jen.Line(),
		jen.Id("parser").Op(":=").Qual("github.com/jhump/protoreflect/desc/protoparse", "Parser").Values(jen.Dict{
			jen.Id("ImportPaths"): jen.Index().String().Values(
				jen.Qual("path/filepath", "Dir").Call(jen.Id("c").Dot("ProtoFile")),
				jen.Lit("."),
			),
		}),
		jen.Line(),
		jen.List(jen.Id("fds"), jen.Err()).Op(":=").Id("parser").Dot("ParseFiles").Call(
			jen.Qual("path/filepath", "Base").Call(jen.Id("c").Dot("ProtoFile")),
		),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to parse proto file %s: %w"), jen.Id("c").Dot("ProtoFile"), jen.Err())),
		),
		jen.Line(),
		jen.Id("c").Dot("fileDescs").Op("=").Id("fds"),
		jen.Return(jen.Nil()),
	)
	f.Line()

	// findMethodInProto - finds a method descriptor from cached proto descriptors
	f.Comment("// findMethodInProto finds a method descriptor from parsed proto files")
	f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id("findMethodInProto").Params(
		jen.Id("serviceName").String(),
		jen.Id("methodName").String(),
	).Parens(jen.List(
		jen.Op("*").Qual("github.com/jhump/protoreflect/desc", "MethodDescriptor"),
		jen.Error(),
	)).Block(
		jen.For(jen.List(jen.Id("_"), jen.Id("fd")).Op(":=").Range().Id("c").Dot("fileDescs")).Block(
			jen.For(jen.List(jen.Id("_"), jen.Id("svc")).Op(":=").Range().Id("fd").Dot("GetServices").Call()).Block(
				jen.If(jen.Id("svc").Dot("GetFullyQualifiedName").Call().Op("==").Id("serviceName")).Block(
					jen.Id("mtd").Op(":=").Id("svc").Dot("FindMethodByName").Call(jen.Id("methodName")),
					jen.If(jen.Id("mtd").Op("!=").Nil()).Block(
						jen.Return(jen.Id("mtd"), jen.Nil()),
					),
				),
			),
		),
		jen.Return(jen.Nil(), jen.Qual("fmt", "Errorf").Call(
			jen.Lit("method %s not found in service %s"), jen.Id("methodName"), jen.Id("serviceName"),
		)),
	)
	f.Line()

	// resolveMethod - common setup for all gRPC calls
	// Returns conn, ctx, methodDescriptor, stub, cleanup function, error
	f.Comment("// resolveMethod resolves a gRPC method using server reflection or proto file")
	f.Comment("// Returns connection, context, method descriptor, stub, cleanup func, and error")
	f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id("resolveMethod").Params(
		jen.Id("method").String(),
	).Parens(jen.List(
		jen.Op("*").Qual("google.golang.org/grpc", "ClientConn"),
		jen.Qual("context", "Context"),
		jen.Op("*").Qual("github.com/jhump/protoreflect/desc", "MethodDescriptor"),
		jen.Qual("github.com/jhump/protoreflect/dynamic/grpcdynamic", "Stub"),
		jen.Func().Params(),
		jen.Error(),
	)).Block(
		// Get connection
		jen.List(jen.Id("conn"), jen.Err()).Op(":=").Id("c").Dot("getConnection").Call(),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Nil(), jen.Nil(), jen.Nil(), jen.Qual("github.com/jhump/protoreflect/dynamic/grpcdynamic", "Stub").Values(), jen.Nil(), jen.Err()),
		),
		jen.Line(),
		// Parse method name: "service.Name/Method" -> service, method
		jen.Id("parts").Op(":=").Qual("strings", "SplitN").Call(jen.Id("method"), jen.Lit("/"), jen.Lit(2)),
		jen.If(jen.Len(jen.Id("parts")).Op("!=").Lit(2)).Block(
			jen.Return(jen.Nil(), jen.Nil(), jen.Nil(), jen.Qual("github.com/jhump/protoreflect/dynamic/grpcdynamic", "Stub").Values(), jen.Nil(),
				jen.Qual("fmt", "Errorf").Call(jen.Lit("invalid method format: %s (expected service/method)"), jen.Id("method"))),
		),
		jen.Id("serviceName").Op(":=").Id("parts").Index(jen.Lit(0)),
		jen.Id("methodName").Op(":=").Id("parts").Index(jen.Lit(1)),
		jen.Line(),
		jen.Id("ctx").Op(":=").Qual("context", "Background").Call(),
		jen.Var().Id("mtdDesc").Op("*").Qual("github.com/jhump/protoreflect/desc", "MethodDescriptor"),
		jen.Var().Id("refClient").Op("*").Qual("github.com/jhump/protoreflect/grpcreflect", "Client"),
		jen.Line(),
		// Branch based on proto file vs reflection mode
		jen.If(jen.Id("c").Dot("ProtoFile").Op("!=").Lit("")).Block(
			// Proto file mode - parse file and find method
			jen.If(jen.Err().Op(":=").Id("c").Dot("loadProtoFile").Call().Op(";").Err().Op("!=").Nil()).Block(
				jen.Return(jen.Nil(), jen.Nil(), jen.Nil(), jen.Qual("github.com/jhump/protoreflect/dynamic/grpcdynamic", "Stub").Values(), jen.Nil(), jen.Err()),
			),
			jen.List(jen.Id("mtdDesc"), jen.Err()).Op("=").Id("c").Dot("findMethodInProto").Call(jen.Id("serviceName"), jen.Id("methodName")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Nil(), jen.Nil(), jen.Nil(), jen.Qual("github.com/jhump/protoreflect/dynamic/grpcdynamic", "Stub").Values(), jen.Nil(), jen.Err()),
			),
		).Else().Block(
			// Reflection mode - query server for descriptors
			jen.Id("refClient").Op("=").Qual("github.com/jhump/protoreflect/grpcreflect", "NewClientAuto").Call(
				jen.Id("ctx"),
				jen.Id("conn"),
			),
			jen.List(jen.Id("svcDesc"), jen.Err()).Op(":=").Id("refClient").Dot("ResolveService").Call(jen.Id("serviceName")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Id("refClient").Dot("Reset").Call(),
				jen.Return(jen.Nil(), jen.Nil(), jen.Nil(), jen.Qual("github.com/jhump/protoreflect/dynamic/grpcdynamic", "Stub").Values(), jen.Nil(),
					jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to resolve service %s: %w"), jen.Id("serviceName"), jen.Err())),
			),
			jen.Id("mtdDesc").Op("=").Id("svcDesc").Dot("FindMethodByName").Call(jen.Id("methodName")),
			jen.If(jen.Id("mtdDesc").Op("==").Nil()).Block(
				jen.Id("refClient").Dot("Reset").Call(),
				jen.Return(jen.Nil(), jen.Nil(), jen.Nil(), jen.Qual("github.com/jhump/protoreflect/dynamic/grpcdynamic", "Stub").Values(), jen.Nil(),
					jen.Qual("fmt", "Errorf").Call(jen.Lit("method %s not found in service %s"), jen.Id("methodName"), jen.Id("serviceName"))),
			),
		),
		jen.Line(),
		// Create stub
		jen.Id("stub").Op(":=").Qual("github.com/jhump/protoreflect/dynamic/grpcdynamic", "NewStub").Call(jen.Id("conn")),
		jen.Line(),
		// Build cleanup function
		jen.Id("cleanup").Op(":=").Func().Params().Block(
			jen.If(jen.Id("refClient").Op("!=").Nil()).Block(
				jen.Id("refClient").Dot("Reset").Call(),
			),
			jen.If(jen.Id("c").Dot("PoolConnections").Op("!=").Lit("yes")).Block(
				jen.Id("conn").Dot("Close").Call(),
			),
		),
		jen.Line(),
		jen.Return(jen.Id("conn"), jen.Id("ctx"), jen.Id("mtdDesc"), jen.Id("stub"), jen.Id("cleanup"), jen.Nil()),
	)
	f.Line()

	// grpcCall - makes a unary gRPC call using reflection
	f.Comment("// grpcCall makes a unary gRPC call using reflection")
	f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id("grpcCall").Params(
		jen.Id("method").String(),
		jen.Id("jsonPayload").String(),
	).Parens(jen.List(jen.String(), jen.Error())).Block(
		jen.List(jen.Id("_"), jen.Id("ctx"), jen.Id("mtdDesc"), jen.Id("stub"), jen.Id("cleanup"), jen.Err()).Op(":=").Id("c").Dot("resolveMethod").Call(jen.Id("method")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Err()),
		),
		jen.Defer().Id("cleanup").Call(),
		jen.Line(),
		// Create dynamic message for request
		jen.Id("reqMsg").Op(":=").Qual("github.com/jhump/protoreflect/dynamic", "NewMessage").Call(
			jen.Id("mtdDesc").Dot("GetInputType").Call(),
		),
		jen.If(jen.Err().Op(":=").Id("reqMsg").Dot("UnmarshalJSON").Call(jen.Index().Byte().Parens(jen.Id("jsonPayload"))).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to parse request JSON: %w"), jen.Err())),
		),
		jen.Line(),
		// Invoke RPC
		jen.List(jen.Id("respMsg"), jen.Err()).Op(":=").Id("stub").Dot("InvokeRpc").Call(
			jen.Id("ctx"),
			jen.Id("mtdDesc"),
			jen.Id("reqMsg"),
		),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("RPC failed: %w"), jen.Err())),
		),
		jen.Line(),
		// Marshal response to JSON - type assert to *dynamic.Message since InvokeRpc returns proto.Message
		jen.List(jen.Id("respJSON"), jen.Err()).Op(":=").Id("respMsg").Assert(jen.Op("*").Qual("github.com/jhump/protoreflect/dynamic", "Message")).Dot("MarshalJSON").Call(),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to marshal response: %w"), jen.Err())),
		),
		jen.Return(jen.String().Parens(jen.Id("respJSON")), jen.Nil()),
	)
	f.Line()

	// serverStream - makes a server streaming gRPC call
	f.Comment("// serverStream makes a server streaming gRPC call with callback")
	f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id("serverStream").Params(
		jen.Id("method").String(),
		jen.Id("jsonPayload").String(),
		jen.Id("handlerBlockID").String(),
	).Parens(jen.List(jen.String(), jen.Error())).Block(
		jen.List(jen.Id("_"), jen.Id("ctx"), jen.Id("mtdDesc"), jen.Id("stub"), jen.Id("cleanup"), jen.Err()).Op(":=").Id("c").Dot("resolveMethod").Call(jen.Id("method")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Err()),
		),
		jen.Defer().Id("cleanup").Call(),
		jen.Line(),
		// Validate streaming type
		jen.If(jen.Op("!").Id("mtdDesc").Dot("IsServerStreaming").Call()).Block(
			jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("method is not server streaming"))),
		),
		jen.Line(),
		// Create request message
		jen.Id("reqMsg").Op(":=").Qual("github.com/jhump/protoreflect/dynamic", "NewMessage").Call(
			jen.Id("mtdDesc").Dot("GetInputType").Call(),
		),
		jen.If(jen.Err().Op(":=").Id("reqMsg").Dot("UnmarshalJSON").Call(jen.Index().Byte().Parens(jen.Id("jsonPayload"))).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to parse request: %w"), jen.Err())),
		),
		jen.Line(),
		// Invoke server streaming RPC
		jen.List(jen.Id("stream"), jen.Err()).Op(":=").Id("stub").Dot("InvokeRpcServerStream").Call(
			jen.Id("ctx"),
			jen.Id("mtdDesc"),
			jen.Id("reqMsg"),
		),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to start stream: %w"), jen.Err())),
		),
		jen.Line(),
		// Read responses and invoke block for each
		jen.Id("count").Op(":=").Lit(0),
		jen.For().Block(
			jen.List(jen.Id("respMsg"), jen.Err()).Op(":=").Id("stream").Dot("RecvMsg").Call(),
			jen.If(jen.Err().Op("==").Qual("io", "EOF")).Block(
				jen.Break(),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("stream error: %w"), jen.Err())),
			),
			jen.List(jen.Id("respJSON"), jen.Err()).Op(":=").Id("respMsg").Assert(jen.Op("*").Qual("github.com/jhump/protoreflect/dynamic", "Message")).Dot("MarshalJSON").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Continue(),
			),
			jen.Id("invokeBlock").Call(jen.Id("handlerBlockID"), jen.String().Parens(jen.Id("respJSON"))),
			jen.Id("count").Op("++"),
		),
		jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("%d"), jen.Id("count")), jen.Nil()),
	)
	f.Line()

	// clientStream - makes a client streaming gRPC call
	f.Comment("// clientStream makes a client streaming gRPC call")
	f.Comment("// Block is called repeatedly to get messages; return empty string to end stream")
	f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id("clientStream").Params(
		jen.Id("method").String(),
		jen.Id("handlerBlockID").String(),
	).Parens(jen.List(jen.String(), jen.Error())).Block(
		jen.List(jen.Id("_"), jen.Id("ctx"), jen.Id("mtdDesc"), jen.Id("stub"), jen.Id("cleanup"), jen.Err()).Op(":=").Id("c").Dot("resolveMethod").Call(jen.Id("method")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Err()),
		),
		jen.Defer().Id("cleanup").Call(),
		jen.Line(),
		// Validate streaming type
		jen.If(jen.Op("!").Id("mtdDesc").Dot("IsClientStreaming").Call()).Block(
			jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("method is not client streaming"))),
		),
		jen.Line(),
		// Start client stream
		jen.List(jen.Id("stream"), jen.Err()).Op(":=").Id("stub").Dot("InvokeRpcClientStream").Call(
			jen.Id("ctx"),
			jen.Id("mtdDesc"),
		),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to start stream: %w"), jen.Err())),
		),
		jen.Line(),
		// Send messages by invoking block until it returns empty
		jen.For().Block(
			jen.Id("msgJSON").Op(":=").Id("invokeBlock").Call(jen.Id("handlerBlockID")),
			jen.If(jen.Id("msgJSON").Op("==").Lit("")).Block(
				jen.Break(),
			),
			jen.Id("reqMsg").Op(":=").Qual("github.com/jhump/protoreflect/dynamic", "NewMessage").Call(
				jen.Id("mtdDesc").Dot("GetInputType").Call(),
			),
			jen.If(jen.Err().Op(":=").Id("reqMsg").Dot("UnmarshalJSON").Call(jen.Index().Byte().Parens(jen.Id("msgJSON"))).Op(";").Err().Op("!=").Nil()).Block(
				jen.Continue(),
			),
			jen.If(jen.Err().Op(":=").Id("stream").Dot("SendMsg").Call(jen.Id("reqMsg")).Op(";").Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("send error: %w"), jen.Err())),
			),
		),
		jen.Line(),
		// Close and receive response
		jen.List(jen.Id("respMsg"), jen.Err()).Op(":=").Id("stream").Dot("CloseAndReceive").Call(),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("close error: %w"), jen.Err())),
		),
		jen.List(jen.Id("respJSON"), jen.Err()).Op(":=").Id("respMsg").Assert(jen.Op("*").Qual("github.com/jhump/protoreflect/dynamic", "Message")).Dot("MarshalJSON").Call(),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Err()),
		),
		jen.Return(jen.String().Parens(jen.Id("respJSON")), jen.Nil()),
	)
	f.Line()

	// bidiStream - makes a bidirectional streaming gRPC call
	f.Comment("// bidiStream makes a bidirectional streaming gRPC call")
	f.Comment("// Block receives responses and returns messages to send; return empty to stop sending")
	f.Func().Parens(jen.Id("c").Op("*").Id("GrpcClient")).Id("bidiStream").Params(
		jen.Id("method").String(),
		jen.Id("handlerBlockID").String(),
	).Parens(jen.List(jen.String(), jen.Error())).Block(
		jen.List(jen.Id("_"), jen.Id("ctx"), jen.Id("mtdDesc"), jen.Id("stub"), jen.Id("cleanup"), jen.Err()).Op(":=").Id("c").Dot("resolveMethod").Call(jen.Id("method")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Err()),
		),
		jen.Defer().Id("cleanup").Call(),
		jen.Line(),
		// Validate streaming type
		jen.If(jen.Op("!").Id("mtdDesc").Dot("IsClientStreaming").Call().Op("||").Op("!").Id("mtdDesc").Dot("IsServerStreaming").Call()).Block(
			jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("method is not bidirectional streaming"))),
		),
		jen.Line(),
		// Start bidi stream
		jen.List(jen.Id("stream"), jen.Err()).Op(":=").Id("stub").Dot("InvokeRpcBidiStream").Call(
			jen.Id("ctx"),
			jen.Id("mtdDesc"),
		),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("failed to start stream: %w"), jen.Err())),
		),
		jen.Line(),
		// Run send/receive loop
		jen.Id("count").Op(":=").Lit(0),
		jen.Id("doneSending").Op(":=").False(),
		jen.For().Block(
			jen.List(jen.Id("respMsg"), jen.Err()).Op(":=").Id("stream").Dot("RecvMsg").Call(),
			jen.If(jen.Err().Op("==").Qual("io", "EOF")).Block(
				jen.Break(),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("recv error: %w"), jen.Err())),
			),
			jen.List(jen.Id("respJSON"), jen.Id("_")).Op(":=").Id("respMsg").Assert(jen.Op("*").Qual("github.com/jhump/protoreflect/dynamic", "Message")).Dot("MarshalJSON").Call(),
			jen.Id("reply").Op(":=").Id("invokeBlock").Call(jen.Id("handlerBlockID"), jen.String().Parens(jen.Id("respJSON"))),
			jen.Id("count").Op("++"),
			jen.If(jen.Op("!").Id("doneSending").Op("&&").Id("reply").Op("!=").Lit("")).Block(
				jen.Id("reqMsg").Op(":=").Qual("github.com/jhump/protoreflect/dynamic", "NewMessage").Call(
					jen.Id("mtdDesc").Dot("GetInputType").Call(),
				),
				jen.If(jen.Err().Op(":=").Id("reqMsg").Dot("UnmarshalJSON").Call(jen.Index().Byte().Parens(jen.Id("reply"))).Op(";").Err().Op("==").Nil()).Block(
					jen.Id("stream").Dot("SendMsg").Call(jen.Id("reqMsg")),
				),
			).Else().If(jen.Op("!").Id("doneSending").Op("&&").Id("reply").Op("==").Lit("")).Block(
				jen.Id("stream").Dot("CloseSend").Call(),
				jen.Id("doneSending").Op("=").True(),
			),
		),
		jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("%d"), jen.Id("count")), jen.Nil()),
	)
	f.Line()
}

// generateStringFileHelpers generates helper functions for String and File class primitives
func (g *generator) generateStringFileHelpers(f *jen.File) {
	// String helpers
	f.Comment("// String primitive helpers")

	// _stringSubstring - safe substring extraction with bounds checking
	f.Func().Id("_stringSubstring").Params(
		jen.Id("s").String(),
		jen.Id("start").Int(),
		jen.Id("length").Int(),
	).String().Block(
		jen.If(jen.Id("start").Op("<").Lit(0)).Block(
			jen.Id("start").Op("=").Lit(0),
		),
		jen.If(jen.Id("start").Op(">=").Len(jen.Id("s"))).Block(
			jen.Return(jen.Lit("")),
		),
		jen.Id("end").Op(":=").Id("start").Op("+").Id("length"),
		jen.If(jen.Id("end").Op(">").Len(jen.Id("s"))).Block(
			jen.Id("end").Op("=").Len(jen.Id("s")),
		),
		jen.Return(jen.Id("s").Index(jen.Id("start").Op(":").Id("end"))),
	)
	f.Line()

	// File helpers
	f.Comment("// File primitive helpers")

	// _fileExists - check if file/directory exists
	f.Func().Id("_fileExists").Params(jen.Id("path").String()).String().Block(
		jen.List(jen.Id("_"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
		jen.Return(jen.Id("_boolToString").Call(jen.Err().Op("==").Nil())),
	)
	f.Line()

	// _fileIsFile - check if path is a regular file
	f.Func().Id("_fileIsFile").Params(jen.Id("path").String()).String().Block(
		jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(jen.Id("info").Dot("Mode").Call().Dot("IsRegular").Call())),
	)
	f.Line()

	// _fileIsDirectory - check if path is a directory
	f.Func().Id("_fileIsDirectory").Params(jen.Id("path").String()).String().Block(
		jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(jen.Id("info").Dot("IsDir").Call())),
	)
	f.Line()

	// _fileIsSymlink - check if path is a symlink
	f.Func().Id("_fileIsSymlink").Params(jen.Id("path").String()).String().Block(
		jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Lstat").Call(jen.Id("path")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(
			jen.Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeSymlink").Op("!=").Lit(0),
		)),
	)
	f.Line()

	// _fileIsFifo - check if path is a named pipe
	f.Func().Id("_fileIsFifo").Params(jen.Id("path").String()).String().Block(
		jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(
			jen.Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeNamedPipe").Op("!=").Lit(0),
		)),
	)
	f.Line()

	// _fileIsSocket - check if path is a socket
	f.Func().Id("_fileIsSocket").Params(jen.Id("path").String()).String().Block(
		jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(
			jen.Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeSocket").Op("!=").Lit(0),
		)),
	)
	f.Line()

	// _fileIsBlockDevice - check if path is a block device
	f.Func().Id("_fileIsBlockDevice").Params(jen.Id("path").String()).String().Block(
		jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(
			jen.Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeDevice").Op("!=").Lit(0).Op("&&").
				Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeCharDevice").Op("==").Lit(0),
		)),
	)
	f.Line()

	// _fileIsCharDevice - check if path is a character device
	f.Func().Id("_fileIsCharDevice").Params(jen.Id("path").String()).String().Block(
		jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(
			jen.Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeCharDevice").Op("!=").Lit(0),
		)),
	)
	f.Line()

	// _fileIsReadable - check if path is readable
	f.Func().Id("_fileIsReadable").Params(jen.Id("path").String()).String().Block(
		jen.Err().Op(":=").Qual("golang.org/x/sys/unix", "Access").Call(jen.Id("path"), jen.Qual("golang.org/x/sys/unix", "R_OK")),
		jen.Return(jen.Id("_boolToString").Call(jen.Err().Op("==").Nil())),
	)
	f.Line()

	// _fileIsWritable - check if path is writable
	f.Func().Id("_fileIsWritable").Params(jen.Id("path").String()).String().Block(
		jen.Err().Op(":=").Qual("golang.org/x/sys/unix", "Access").Call(jen.Id("path"), jen.Qual("golang.org/x/sys/unix", "W_OK")),
		jen.Return(jen.Id("_boolToString").Call(jen.Err().Op("==").Nil())),
	)
	f.Line()

	// _fileIsExecutable - check if path is executable
	f.Func().Id("_fileIsExecutable").Params(jen.Id("path").String()).String().Block(
		jen.Err().Op(":=").Qual("golang.org/x/sys/unix", "Access").Call(jen.Id("path"), jen.Qual("golang.org/x/sys/unix", "X_OK")),
		jen.Return(jen.Id("_boolToString").Call(jen.Err().Op("==").Nil())),
	)
	f.Line()

	// _fileIsEmpty - check if file has zero size
	f.Func().Id("_fileIsEmpty").Params(jen.Id("path").String()).String().Block(
		jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(jen.Id("info").Dot("Size").Call().Op("==").Lit(0))),
	)
	f.Line()

	// _fileNotEmpty - check if file has non-zero size
	f.Func().Id("_fileNotEmpty").Params(jen.Id("path").String()).String().Block(
		jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(jen.Id("info").Dot("Size").Call().Op(">").Lit(0))),
	)
	f.Line()

	// _fileIsNewer - check if path1 is newer than path2
	f.Func().Id("_fileIsNewer").Params(
		jen.Id("path1").String(),
		jen.Id("path2").String(),
	).String().Block(
		jen.List(jen.Id("info1"), jen.Id("err1")).Op(":=").Qual("os", "Stat").Call(jen.Id("path1")),
		jen.List(jen.Id("info2"), jen.Id("err2")).Op(":=").Qual("os", "Stat").Call(jen.Id("path2")),
		jen.If(jen.Id("err1").Op("!=").Nil().Op("||").Id("err2").Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(
			jen.Id("info1").Dot("ModTime").Call().Dot("After").Call(jen.Id("info2").Dot("ModTime").Call()),
		)),
	)
	f.Line()

	// _fileIsOlder - check if path1 is older than path2
	f.Func().Id("_fileIsOlder").Params(
		jen.Id("path1").String(),
		jen.Id("path2").String(),
	).String().Block(
		jen.List(jen.Id("info1"), jen.Id("err1")).Op(":=").Qual("os", "Stat").Call(jen.Id("path1")),
		jen.List(jen.Id("info2"), jen.Id("err2")).Op(":=").Qual("os", "Stat").Call(jen.Id("path2")),
		jen.If(jen.Id("err1").Op("!=").Nil().Op("||").Id("err2").Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(
			jen.Id("info1").Dot("ModTime").Call().Dot("Before").Call(jen.Id("info2").Dot("ModTime").Call()),
		)),
	)
	f.Line()

	// _fileIsSame - check if both paths refer to the same file (same inode)
	f.Func().Id("_fileIsSame").Params(
		jen.Id("path1").String(),
		jen.Id("path2").String(),
	).String().Block(
		jen.List(jen.Id("info1"), jen.Id("err1")).Op(":=").Qual("os", "Stat").Call(jen.Id("path1")),
		jen.List(jen.Id("info2"), jen.Id("err2")).Op(":=").Qual("os", "Stat").Call(jen.Id("path2")),
		jen.If(jen.Id("err1").Op("!=").Nil().Op("||").Id("err2").Op("!=").Nil()).Block(
			jen.Return(jen.Lit("false")),
		),
		jen.Return(jen.Id("_boolToString").Call(
			jen.Qual("os", "SameFile").Call(jen.Id("info1"), jen.Id("info2")),
		)),
	)
	f.Line()
}
