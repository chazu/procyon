// Package codegen generates Go code from Trashtalk AST.
// This file contains shared runtime plugin generation that links against libtrashtalk.dylib.
package codegen

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/chazu/procyon/pkg/ast"
	"github.com/dave/jennifer/jen"
)

// GenerateSharedPlugin produces Go source code for a plugin that links against libtrashtalk.
// The output registers itself with the shared runtime at load time via init().
func GenerateSharedPlugin(class *ast.Class) *Result {
	// Skip primitive classes entirely - they contain raw bash that can't be natively compiled
	if class.IsPrimitiveClass() {
		return &Result{
			Code:           "",
			Warnings:       []string{},
			SkippedMethods: []SkippedMethod{},
		}
	}

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
		defaultVal := iv.Default.Value
		isNumeric := false
		if len(defaultVal) > 0 {
			isNumeric = true
			for i, c := range defaultVal {
				if !((c >= '0' && c <= '9') || (i == 0 && c == '-')) {
					isNumeric = false
					break
				}
			}
		}
		if !isNumeric {
			g.jsonVars[iv.Name] = true
		}
	}

	return g.generateSharedPlugin()
}

func (g *generator) generateSharedPlugin() *Result {
	f := jen.NewFile("main")

	// CGO preamble with libtrashtalk linking
	// Note: LDFLAGS uses simple -L path. Runtime library path should be set via
	// DYLD_LIBRARY_PATH or by installing libtrashtalk.dylib to a standard location.
	f.CgoPreamble(`
#cgo CFLAGS: -I${SRCDIR}/../../include
#cgo LDFLAGS: -L${SRCDIR}/../../lib -ltrashtalk
#include <libtrashtalk.h>
#include <stdlib.h>
`)

	// Import "C" for c-shared exports
	f.ImportAlias("C", "")

	// Add standard imports
	f.Anon("github.com/mattn/go-sqlite3")

	// ErrUnknownSelector
	f.Var().Id("ErrUnknownSelector").Op("=").Qual("errors", "New").Call(jen.Lit("unknown selector"))
	f.Line()

	// Generate init() for class registration
	g.generateSharedInit(f)
	f.Line()

	// Struct definition (same as binary mode)
	g.generateStruct(f)
	f.Line()

	// Helper functions
	g.generateSharedPluginHelpers(f)
	f.Line()

	// Type conversion helpers (toInt, toBool, etc.)
	g.generateTypeHelpers(f)
	f.Line()

	// JSON primitive helpers
	g.generateJSONHelpers(f)
	f.Line()

	// gRPC helper functions for GrpcClient class
	if g.class.Name == "GrpcClient" {
		g.generateGrpcHelpers(f)
	}

	// First pass: identify which methods will be skipped
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

	// Generate method dispatch tables (internal, for runtime callbacks)
	g.generateSharedDispatch(f, instanceMethods)
	f.Line()
	g.generateSharedClassDispatch(f, classMethods)
	f.Line()

	// Generate CGO-exported method wrappers
	g.generateSharedMethodExports(f, instanceMethods, classMethods)
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

// generateSharedInit generates the init() function that registers the class with the shared runtime
func (g *generator) generateSharedInit(f *jen.File) {
	className := g.class.Name
	qualifiedName := g.class.QualifiedName()
	superclass := g.class.Parent

	// Build instance variable names array
	varNames := make([]string, 0, len(g.class.InstanceVars))
	for _, iv := range g.class.InstanceVars {
		varNames = append(varNames, iv.Name)
	}

	f.Comment("// init registers this class with the shared runtime")
	f.Func().Id("init").Params().Block(
		// Call TT_RegisterClass with class metadata
		// We'll build this as a series of statements

		// Convert class name to C string
		jen.Id("className").Op(":=").Qual("C", "CString").Call(jen.Lit(qualifiedName)),
		jen.Defer().Qual("C", "free").Call(jen.Qual("unsafe", "Pointer").Call(jen.Id("className"))),

		// Convert superclass to C string (nil if none)
		jen.Id("superclass").Op(":=").Func().Params().Op("*").Qual("C", "char").Block(
			jen.If(jen.Lit(superclass).Op("==").Lit("")).Block(
				jen.Return(jen.Nil()),
			),
			jen.Return(jen.Qual("C", "CString").Call(jen.Lit(superclass))),
		).Call(),

		// Build instance vars array
		g.generateInstanceVarsArray(varNames),

		// Register the class with the runtime
		jen.Qual("C", "TT_RegisterClass").Call(
			jen.Id("className"),
			jen.Id("superclass"),
			jen.Id("instanceVars"),
			jen.Qual("C", "int").Call(jen.Lit(len(varNames))),
			jen.Nil(), // Method table (we use exported functions instead)
		),
	)
	f.Line()

	// Also add a GetClassName export for backward compatibility
	f.Comment("//export GetClassName")
	f.Func().Id("GetClassName").Params().Op("*").Qual("C", "char").Block(
		jen.Return(jen.Qual("C", "CString").Call(jen.Lit(className))),
	)
}

// generateInstanceVarsArray generates code to create a C array of instance var names
func (g *generator) generateInstanceVarsArray(varNames []string) *jen.Statement {
	if len(varNames) == 0 {
		return jen.Id("instanceVars").Op(":=").Parens(jen.Op("**").Qual("C", "char")).Parens(jen.Nil())
	}

	// Build the block statements
	blockStmts := []jen.Code{
		jen.Id("vars").Op(":=").Make(jen.Index().Op("*").Qual("C", "char"), jen.Lit(len(varNames))),
	}
	for i, name := range varNames {
		blockStmts = append(blockStmts,
			jen.Id("vars").Index(jen.Lit(i)).Op("=").Qual("C", "CString").Call(jen.Lit(name)),
		)
	}
	blockStmts = append(blockStmts, jen.Return(jen.Op("&").Id("vars").Index(jen.Lit(0))))

	// Build array of C strings
	return jen.Id("instanceVars").Op(":=").Func().Params().Op("**").Qual("C", "char").Block(
		blockStmts...,
	).Call()
}

// generateSharedPluginHelpers generates helper functions that use the shared runtime
func (g *generator) generateSharedPluginHelpers(f *jen.File) {
	className := g.class.Name

	// openDB - delegates to shared runtime's database
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

	// sendMessage - uses shared runtime's TT_Send
	f.Func().Id("sendMessage").Params(
		jen.Id("receiver").Interface(),
		jen.Id("selector").String(),
		jen.Id("args").Op("...").Interface(),
	).String().Block(
		// Convert receiver to string
		jen.Id("receiverStr").Op(":=").Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("receiver")),

		// Convert to C strings
		jen.Id("cReceiver").Op(":=").Qual("C", "CString").Call(jen.Id("receiverStr")),
		jen.Defer().Qual("C", "free").Call(jen.Qual("unsafe", "Pointer").Call(jen.Id("cReceiver"))),
		jen.Id("cSelector").Op(":=").Qual("C", "CString").Call(jen.Id("selector")),
		jen.Defer().Qual("C", "free").Call(jen.Qual("unsafe", "Pointer").Call(jen.Id("cSelector"))),

		// Build args array
		jen.Id("cArgs").Op(":=").Make(jen.Index().Qual("C", "TTValue"), jen.Len(jen.Id("args"))),
		jen.For(jen.List(jen.Id("i"), jen.Id("arg")).Op(":=").Range().Id("args")).Block(
			jen.Id("argStr").Op(":=").Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("arg")),
			jen.Id("cStr").Op(":=").Qual("C", "CString").Call(jen.Id("argStr")),
			jen.Id("cArgs").Index(jen.Id("i")).Op("=").Qual("C", "TT_MakeString").Call(jen.Id("cStr")),
		),

		// Call TT_Send
		jen.Var().Id("argsPtr").Op("*").Qual("C", "TTValue"),
		jen.If(jen.Len(jen.Id("cArgs")).Op(">").Lit(0)).Block(
			jen.Id("argsPtr").Op("=").Op("&").Id("cArgs").Index(jen.Lit(0)),
		),
		jen.Id("result").Op(":=").Qual("C", "TT_Send").Call(
			jen.Id("cReceiver"),
			jen.Id("cSelector"),
			jen.Id("argsPtr"),
			jen.Qual("C", "int").Call(jen.Len(jen.Id("args"))),
		),

		// Convert result to Go string
		jen.Id("cResult").Op(":=").Qual("C", "TT_ValueAsString").Call(jen.Id("result")),
		jen.If(jen.Id("cResult").Op("==").Nil()).Block(
			jen.Return(jen.Lit("")),
		),
		jen.Defer().Qual("C", "free").Call(jen.Qual("unsafe", "Pointer").Call(jen.Id("cResult"))),
		jen.Return(jen.Qual("C", "GoString").Call(jen.Id("cResult"))),
	)
	f.Line()

	// invokeBlock - uses shared runtime's TT_InvokeBlock
	f.Comment("// invokeBlock calls a Trashtalk block through the shared runtime")
	f.Func().Id("invokeBlock").Params(
		jen.Id("blockID").String(),
		jen.Id("args").Op("...").Interface(),
	).String().Block(
		// Convert block ID to C string
		jen.Id("cBlockID").Op(":=").Qual("C", "CString").Call(jen.Id("blockID")),
		jen.Defer().Qual("C", "free").Call(jen.Qual("unsafe", "Pointer").Call(jen.Id("cBlockID"))),

		// Build args array
		jen.Id("cArgs").Op(":=").Make(jen.Index().Qual("C", "TTValue"), jen.Len(jen.Id("args"))),
		jen.For(jen.List(jen.Id("i"), jen.Id("arg")).Op(":=").Range().Id("args")).Block(
			jen.Id("argStr").Op(":=").Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("arg")),
			jen.Id("cStr").Op(":=").Qual("C", "CString").Call(jen.Id("argStr")),
			jen.Id("cArgs").Index(jen.Id("i")).Op("=").Qual("C", "TT_MakeString").Call(jen.Id("cStr")),
		),

		// Call TT_InvokeBlock
		jen.Var().Id("argsPtr").Op("*").Qual("C", "TTValue"),
		jen.If(jen.Len(jen.Id("cArgs")).Op(">").Lit(0)).Block(
			jen.Id("argsPtr").Op("=").Op("&").Id("cArgs").Index(jen.Lit(0)),
		),
		jen.Id("result").Op(":=").Qual("C", "TT_InvokeBlock").Call(
			jen.Id("cBlockID"),
			jen.Id("argsPtr"),
			jen.Qual("C", "int").Call(jen.Len(jen.Id("args"))),
		),

		// Convert result to Go string
		jen.Id("cResult").Op(":=").Qual("C", "TT_ValueAsString").Call(jen.Id("result")),
		jen.If(jen.Id("cResult").Op("==").Nil()).Block(
			jen.Return(jen.Lit("")),
		),
		jen.Defer().Qual("C", "free").Call(jen.Qual("unsafe", "Pointer").Call(jen.Id("cResult"))),
		jen.Return(jen.Qual("C", "GoString").Call(jen.Id("cResult"))),
	)
}

// generateSharedDispatch generates the internal dispatch switch for instance methods
func (g *generator) generateSharedDispatch(f *jen.File, methods []*compiledMethod) {
	className := g.class.Name

	cases := []jen.Code{}
	for _, m := range methods {
		methodName := m.goName
		if g.instanceVars[m.selector] {
			methodName = "Get" + methodName
		}

		var callExpr *jen.Statement
		if len(m.args) > 0 {
			argCheck := jen.If(jen.Len(jen.Id("args")).Op("<").Lit(len(m.args))).Block(
				jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit(m.selector+" requires "+fmt.Sprintf("%d", len(m.args))+" argument"))),
			)

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
					cases = append(cases, jen.Case(jen.Lit(m.selector)).Block(
						jen.Return(callExpr),
					))
				} else {
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

// generateSharedClassDispatch generates the dispatch switch for class methods
func (g *generator) generateSharedClassDispatch(f *jen.File, methods []*compiledMethod) {
	cases := []jen.Code{}
	for _, m := range methods {
		var callExpr *jen.Statement
		if len(m.args) > 0 {
			argCheck := jen.If(jen.Len(jen.Id("args")).Op("<").Lit(len(m.args))).Block(
				jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit(m.selector+" requires "+fmt.Sprintf("%d", len(m.args))+" argument"))),
			)

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
			callExpr = jen.Id(m.goName).Call()
			if m.hasReturn {
				if m.returnsErr {
					cases = append(cases, jen.Case(jen.Lit(m.selector)).Block(
						jen.Return(callExpr),
					))
				} else {
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

	cases = append(cases, jen.Default().Block(
		jen.Return(jen.Lit(""), jen.Qual("fmt", "Errorf").Call(jen.Lit("%w: %s"), jen.Id("ErrUnknownSelector"), jen.Id("selector"))),
	))

	f.Func().Id("dispatchClass").Params(
		jen.Id("selector").String(),
		jen.Id("args").Index().String(),
	).Parens(jen.List(jen.String(), jen.Error())).Block(
		jen.Switch(jen.Id("selector")).Block(cases...),
	)
}

// generateSharedMethodExports generates CGO-exported wrappers for methods
// These allow the shared runtime to call into this plugin's methods
func (g *generator) generateSharedMethodExports(f *jen.File, instanceMethods, classMethods []*compiledMethod) {
	className := g.class.Name
	// Make a valid C identifier from the class name
	cClassName := strings.ReplaceAll(className, "::", "__")

	// Generate a single Dispatch export that the runtime can call
	// This is simpler than exporting every method individually
	f.Comment("//export " + cClassName + "_Dispatch")
	f.Func().Id(cClassName + "_Dispatch").Params(
		jen.Id("instanceJSON").Op("*").Qual("C", "char"),
		jen.Id("selector").Op("*").Qual("C", "char"),
		jen.Id("argsJSON").Op("*").Qual("C", "char"),
	).Op("*").Qual("C", "char").Block(
		jen.Id("instanceStr").Op(":=").Qual("C", "GoString").Call(jen.Id("instanceJSON")),
		jen.Id("selectorStr").Op(":=").Qual("C", "GoString").Call(jen.Id("selector")),
		jen.Id("argsStr").Op(":=").Qual("C", "GoString").Call(jen.Id("argsJSON")),
		jen.Line(),
		jen.Id("result").Op(":=").Id("dispatchInternal").Call(
			jen.Id("instanceStr"),
			jen.Id("selectorStr"),
			jen.Id("argsStr"),
		),
		jen.Return(jen.Qual("C", "CString").Call(jen.Id("result"))),
	)
	f.Line()

	// Generate dispatchInternal that handles both class and instance methods
	f.Func().Id("dispatchInternal").Params(
		jen.Id("instanceJSON").String(),
		jen.Id("selector").String(),
		jen.Id("argsJSON").String(),
	).String().Block(
		jen.Var().Id("args").Index().String(),
		jen.Qual("encoding/json", "Unmarshal").Call(jen.Index().Byte().Parens(jen.Id("argsJSON")), jen.Op("&").Id("args")),
		jen.Line(),
		// Check if this is a class method call
		jen.If(jen.Id("instanceJSON").Op("==").Lit("").Op("||").Id("instanceJSON").Op("==").Lit(className).Op("||").Id("instanceJSON").Op("==").Lit(g.class.QualifiedName())).Block(
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
		// Return updated instance + result
		jen.List(jen.Id("updatedJSON"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Op("&").Id("instance")),
		jen.Return(
			jen.Qual("fmt", "Sprintf").Call(jen.Lit(`{"instance":%s,"result":%q,"exit_code":0}`), jen.String().Parens(jen.Id("updatedJSON")), jen.Id("result")),
		),
	)
}
