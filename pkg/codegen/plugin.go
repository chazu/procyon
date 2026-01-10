// Package codegen generates Go code from Trashtalk AST.
// This file contains plugin mode generation for c-shared libraries.
package codegen

import (
	"bytes"
	"fmt"

	"github.com/chazu/procyon/pkg/ast"
	"github.com/dave/jennifer/jen"
)

// GeneratePlugin produces Go source code for a c-shared plugin.
// The output can be built with: go build -buildmode=c-shared -o Class.so
func GeneratePlugin(class *ast.Class) *Result {
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

	return g.generatePlugin()
}

func (g *generator) generatePlugin() *Result {
	f := jen.NewFile("main")

	// Import "C" for c-shared exports
	f.ImportAlias("C", "")

	// Add standard imports
	f.Anon("github.com/mattn/go-sqlite3")

	// ErrUnknownSelector
	f.Var().Id("ErrUnknownSelector").Op("=").Qual("errors", "New").Call(jen.Lit("unknown selector"))
	f.Line()

	// Struct definition (same as binary mode)
	g.generateStruct(f)
	f.Line()

	// Generate exported C functions
	g.generatePluginExports(f)
	f.Line()

	// Helper functions (same as binary mode, minus main-specific ones)
	g.generatePluginHelpers(f)
	f.Line()

	// Type conversion helpers (toInt, toBool, invokeBlock)
	g.generateTypeHelpers(f)
	f.Line()

	// JSON primitive helpers (_toStr, _arrayFirst, etc.)
	g.generateJSONHelpers(f)
	f.Line()

	// gRPC helper functions for GrpcClient class
	if g.class.Name == "GrpcClient" {
		g.generateGrpcHelpers(f)
	}

	// First pass: identify which methods will be skipped (for @ self calls)
	g.preIdentifySkippedMethods()

	// Compile methods
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

	// Generate internal dispatch functions (not exported)
	g.generatePluginDispatch(f, instanceMethods)
	f.Line()
	g.generatePluginClassDispatch(f, classMethods)
	f.Line()

	// Generate method implementations
	for _, m := range compiled {
		g.generateMethod(f, m)
	}

	// Empty main (required for c-shared but unused)
	f.Func().Id("main").Params().Block()

	// Render output
	var buf bytes.Buffer
	if err := f.Render(&buf); err != nil {
		return &Result{
			Code:           "",
			Warnings:       append(g.warnings, "render error: "+err.Error()),
			SkippedMethods: g.skipped,
		}
	}

	return &Result{
		Code:           buf.String(),
		Warnings:       g.warnings,
		SkippedMethods: g.skipped,
	}
}

// generatePluginExports generates the C-exported functions
func (g *generator) generatePluginExports(f *jen.File) {
	className := g.class.Name

	// //export GetClassName
	f.Comment("//export GetClassName")
	f.Func().Id("GetClassName").Params().Op("*").Qual("C", "char").Block(
		jen.Return(jen.Qual("C", "CString").Call(jen.Lit(className))),
	)
	f.Line()

	// //export Dispatch
	// Dispatch handles all method calls for this class
	// Returns a single JSON string with result and exit_code to avoid struct return ABI issues
	f.Comment("//export Dispatch")
	f.Func().Id("Dispatch").Params(
		jen.Id("instanceJSON").Op("*").Qual("C", "char"),
		jen.Id("selector").Op("*").Qual("C", "char"),
		jen.Id("argsJSON").Op("*").Qual("C", "char"),
	).Op("*").Qual("C", "char").Block(
		// Convert C strings to Go strings
		jen.Id("instanceStr").Op(":=").Qual("C", "GoString").Call(jen.Id("instanceJSON")),
		jen.Id("selectorStr").Op(":=").Qual("C", "GoString").Call(jen.Id("selector")),
		jen.Id("argsStr").Op(":=").Qual("C", "GoString").Call(jen.Id("argsJSON")),
		jen.Line(),
		// Call internal dispatch - returns JSON with embedded exit_code
		jen.Id("result").Op(":=").Id("dispatchInternal").Call(
			jen.Id("instanceStr"),
			jen.Id("selectorStr"),
			jen.Id("argsStr"),
		),
		jen.Return(jen.Qual("C", "CString").Call(jen.Id("result"))),
	)
}

// generatePluginHelpers generates helper functions for plugin mode
func (g *generator) generatePluginHelpers(f *jen.File) {
	className := g.class.Name

	// openDB - same as binary mode
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
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(jen.Index().Byte().Parens(jen.Id("data")), jen.Op("&").Id("instance")).Op(";").Err().Op("!=").Nil()).Block(
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
		jen.List(jen.Id("_"), jen.Err()).Op("=").Id("db").Dot("Exec").Call(
			jen.Lit("INSERT OR REPLACE INTO instances (id, data) VALUES (?, json(?))"),
			jen.Id("id"),
			jen.String().Parens(jen.Id("data")),
		),
		jen.Return(jen.Err()),
	)
	f.Line()

	// dispatchInternal - main entry point for plugin calls
	// Returns a single JSON string with exit_code embedded to avoid struct return ABI issues
	f.Func().Id("dispatchInternal").Params(
		jen.Id("instanceJSON").String(),
		jen.Id("selector").String(),
		jen.Id("argsJSON").String(),
	).String().Block(
		// Parse args
		jen.Var().Id("args").Index().String(),
		jen.Qual("encoding/json", "Unmarshal").Call(jen.Index().Byte().Parens(jen.Id("argsJSON")), jen.Op("&").Id("args")),
		jen.Line(),
		// Check if this is a class method call (empty instanceJSON or class name)
		jen.If(jen.Id("instanceJSON").Op("==").Lit("").Op("||").Id("instanceJSON").Op("==").Lit(className)).Block(
			jen.List(jen.Id("result"), jen.Err()).Op(":=").Id("dispatchClass").Call(jen.Id("selector"), jen.Id("args")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.If(jen.Qual("errors", "Is").Call(jen.Err(), jen.Id("ErrUnknownSelector"))).Block(
					jen.Return(jen.Lit(`{"exit_code":200}`)),
				),
				jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit(`{"exit_code":1,"error":%q}`), jen.Err().Dot("Error").Call())),
			),
			jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit(`{"result":%q,"exit_code":0}`), jen.Id("result"))),
		),
		jen.Line(),
		// Instance method - parse instance JSON
		jen.Var().Id("instance").Id(className),
		jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(jen.Index().Byte().Parens(jen.Id("instanceJSON")), jen.Op("&").Id("instance")).Op(";").Err().Op("!=").Nil()).Block(
			jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit(`{"exit_code":1,"error":%q}`), jen.Err().Dot("Error").Call())),
		),
		jen.Line(),
		// Dispatch to instance method
		jen.List(jen.Id("result"), jen.Err()).Op(":=").Id("dispatch").Call(jen.Op("&").Id("instance"), jen.Id("selector"), jen.Id("args")),
		jen.If(jen.Err().Op("!=").Nil()).Block(
			jen.If(jen.Qual("errors", "Is").Call(jen.Err(), jen.Id("ErrUnknownSelector"))).Block(
				jen.Return(jen.Lit(`{"exit_code":200}`)),
			),
			jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit(`{"exit_code":1,"error":%q}`), jen.Err().Dot("Error").Call())),
		),
		jen.Line(),
		// Return updated instance + result with exit_code
		jen.List(jen.Id("updatedJSON"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Op("&").Id("instance")),
		jen.Return(
			jen.Qual("fmt", "Sprintf").Call(jen.Lit(`{"instance":%s,"result":%q,"exit_code":0}`), jen.String().Parens(jen.Id("updatedJSON")), jen.Id("result")),
		),
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
}

// generatePluginDispatch generates the internal dispatch switch for instance methods
func (g *generator) generatePluginDispatch(f *jen.File, methods []*compiledMethod) {
	className := g.class.Name

	cases := []jen.Code{}
	for _, m := range methods {
		// Check if method name was renamed to avoid collision with ivar
		// In Go, you can't have a struct field and method with the same name
		methodName := m.goName
		if g.instanceVars[m.selector] {
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
		jen.Id("selector").String(),
		jen.Id("args").Index().String(),
	).Parens(jen.List(jen.String(), jen.Error())).Block(
		jen.Switch(jen.Id("selector")).Block(cases...),
	)
}

// generatePluginClassDispatch generates the dispatch switch for class methods
func (g *generator) generatePluginClassDispatch(f *jen.File, methods []*compiledMethod) {
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

	// dispatchClass takes no instance receiver
	f.Func().Id("dispatchClass").Params(
		jen.Id("selector").String(),
		jen.Id("args").Index().String(),
	).Parens(jen.List(jen.String(), jen.Error())).Block(
		jen.Switch(jen.Id("selector")).Block(cases...),
	)
}
