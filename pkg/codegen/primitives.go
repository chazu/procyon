// Package codegen generates Go code from Trashtalk AST.
// This file contains primitive method implementations for system classes.
package codegen

import (
	"github.com/dave/jennifer/jen"
)

// primitiveRegistry maps (className, selector) to whether a native implementation exists.
// The actual implementation is in generatePrimitiveMethod.
var primitiveRegistry = map[string]map[string]bool{
	"File": {
		// Factory class methods
		"at_":            true,
		"temp":           true,
		"tempWithPrefix_": true,
		"mkfifo_":        true,
		// Instance methods
		"read":             true,
		"write_":           true,
		"append_":          true,
		"delete":           true,
		"exists":           true,
		"isFile":           true,
		"isDirectory":      true,
		"isFifo":           true,
		"size":             true,
		"path":             true,
		"directory":        true,
		"basename":         true,
		"extension":        true,
		"stem":             true,
		"writeLine_":       true,
		"appendLine_":      true,
		"copyTo_":          true,
		"moveTo_":          true,
		"touch":            true,
		"modificationTime": true,
		"readLines":        true,
		"printString":      true,
		"info":             true,
		// Class methods - file tests
		"exists_":         true,
		"isFile_":         true,
		"isDirectory_":    true,
		"isSymlink_":      true,
		"isFifo_":         true,
		"isSocket_":       true,
		"isBlockDevice_":  true,
		"isCharDevice_":   true,
		"isReadable_":     true,
		"isWritable_":     true,
		"isExecutable_":   true,
		"isEmpty_":        true,
		"notEmpty_":       true,
		"isNewer_than_":   true,
		"isOlder_than_":   true,
		"isSame_as_":      true,
		// Class methods - quick operations
		"read_":     true,
		"write_to_": true,
		"delete_":   true,
	},
	"Env": {
		"get_":    true,
		"set_to_": true,
		"unset_":  true,
		"has_":    true,
	},
	"Console": {
		"print_":  true,
		"write_":  true,
		"error_":  true,
		"newline": true,
	},
	// More classes can be added here as we implement them
}

// hasPrimitiveImpl checks if a native implementation exists for a primitive method.
func hasPrimitiveImpl(className, selector string) bool {
	if classMap, ok := primitiveRegistry[className]; ok {
		return classMap[selector]
	}
	return false
}

// generatePrimitiveMethod generates native Go code for a primitive method.
// Returns true if the method was handled, false to fall back to default behavior.
func (g *generator) generatePrimitiveMethod(f *jen.File, m *compiledMethod) bool {
	className := g.class.Name

	switch className {
	case "File":
		return g.generatePrimitiveMethodFile(f, m)
	case "Env":
		return g.generatePrimitiveMethodEnv(f, m)
	case "Console":
		return g.generatePrimitiveMethodConsole(f, m)
	default:
		return false
	}
}

// generatePrimitiveMethodFile generates native File class methods.
func (g *generator) generatePrimitiveMethodFile(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	// Factory class methods
	case "at_":
		// Create a File instance at the given path
		f.Func().Id(m.goName).Params(jen.Id("filepath").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Generate instance ID"),
			jen.Id("id").Op(":=").Lit("file_").Op("+").Qual("strings", "ReplaceAll").Call(
				jen.Qual("github.com/google/uuid", "New").Call().Dot("String").Call(),
				jen.Lit("-"),
				jen.Lit(""),
			),
			jen.Line(),
			jen.Comment("Create instance in database"),
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.Id("instance").Op(":=").Op("&").Id("File").Values(jen.Dict{
				jen.Id("Class"):     jen.Lit("File"),
				jen.Id("CreatedAt"): jen.Qual("time", "Now").Call().Dot("Format").Call(jen.Qual("time", "RFC3339")),
				jen.Id("Path"):      jen.Id("filepath"),
			}),
			jen.Line(),
			jen.If(jen.Err().Op(":=").Id("saveInstance").Call(jen.Id("db"), jen.Id("id"), jen.Id("instance")), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Id("id"), jen.Nil()),
		)
		f.Line()
		return true

	case "temp":
		// Create a temporary file and return File instance
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Create temp file"),
			jen.List(jen.Id("tmpfile"), jen.Err()).Op(":=").Qual("os", "CreateTemp").Call(jen.Lit(""), jen.Lit("trashtalk-*")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Id("tmpfile").Dot("Close").Call(),
			jen.Line(),
			jen.Comment("Generate instance ID"),
			jen.Id("id").Op(":=").Lit("file_").Op("+").Qual("strings", "ReplaceAll").Call(
				jen.Qual("github.com/google/uuid", "New").Call().Dot("String").Call(),
				jen.Lit("-"),
				jen.Lit(""),
			),
			jen.Line(),
			jen.Comment("Create instance in database"),
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.Id("instance").Op(":=").Op("&").Id("File").Values(jen.Dict{
				jen.Id("Class"):     jen.Lit("File"),
				jen.Id("CreatedAt"): jen.Qual("time", "Now").Call().Dot("Format").Call(jen.Qual("time", "RFC3339")),
				jen.Id("Path"):      jen.Id("tmpfile").Dot("Name").Call(),
			}),
			jen.Line(),
			jen.If(jen.Err().Op(":=").Id("saveInstance").Call(jen.Id("db"), jen.Id("id"), jen.Id("instance")), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Id("id"), jen.Nil()),
		)
		f.Line()
		return true

	case "tempWithPrefix_":
		// Create a temporary file with prefix and return File instance
		f.Func().Id(m.goName).Params(jen.Id("prefix").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Create temp file with prefix"),
			jen.List(jen.Id("tmpfile"), jen.Err()).Op(":=").Qual("os", "CreateTemp").Call(jen.Lit(""), jen.Id("prefix").Op("+").Lit("*")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Id("tmpfile").Dot("Close").Call(),
			jen.Line(),
			jen.Comment("Generate instance ID"),
			jen.Id("id").Op(":=").Lit("file_").Op("+").Qual("strings", "ReplaceAll").Call(
				jen.Qual("github.com/google/uuid", "New").Call().Dot("String").Call(),
				jen.Lit("-"),
				jen.Lit(""),
			),
			jen.Line(),
			jen.Comment("Create instance in database"),
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.Id("instance").Op(":=").Op("&").Id("File").Values(jen.Dict{
				jen.Id("Class"):     jen.Lit("File"),
				jen.Id("CreatedAt"): jen.Qual("time", "Now").Call().Dot("Format").Call(jen.Qual("time", "RFC3339")),
				jen.Id("Path"):      jen.Id("tmpfile").Dot("Name").Call(),
			}),
			jen.Line(),
			jen.If(jen.Err().Op(":=").Id("saveInstance").Call(jen.Id("db"), jen.Id("id"), jen.Id("instance")), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Id("id"), jen.Nil()),
		)
		f.Line()
		return true

	case "mkfifo_":
		// Create a named pipe (FIFO) and return File instance
		f.Func().Id(m.goName).Params(jen.Id("filepath").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Create FIFO (named pipe)"),
			jen.If(jen.Err().Op(":=").Qual("syscall", "Mkfifo").Call(jen.Id("filepath"), jen.Lit(0644)), jen.Err().Op("!=").Nil()).Block(
				jen.Comment("Ignore error if FIFO already exists"),
				jen.If(jen.Op("!").Qual("os", "IsExist").Call(jen.Err())).Block(
					jen.Return(jen.Lit(""), jen.Err()),
				),
			),
			jen.Line(),
			jen.Comment("Generate instance ID"),
			jen.Id("id").Op(":=").Lit("file_").Op("+").Qual("strings", "ReplaceAll").Call(
				jen.Qual("github.com/google/uuid", "New").Call().Dot("String").Call(),
				jen.Lit("-"),
				jen.Lit(""),
			),
			jen.Line(),
			jen.Comment("Create instance in database"),
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.Id("instance").Op(":=").Op("&").Id("File").Values(jen.Dict{
				jen.Id("Class"):     jen.Lit("File"),
				jen.Id("CreatedAt"): jen.Qual("time", "Now").Call().Dot("Format").Call(jen.Qual("time", "RFC3339")),
				jen.Id("Path"):      jen.Id("filepath"),
			}),
			jen.Line(),
			jen.If(jen.Err().Op(":=").Id("saveInstance").Call(jen.Id("db"), jen.Id("id"), jen.Id("instance")), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Id("id"), jen.Nil()),
		)
		f.Line()
		return true

	case "read":
		// Instance method: read file at self.path
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("data"), jen.Err()).Op(":=").Qual("os", "ReadFile").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.String().Parens(jen.Id("data")), jen.Nil()),
		)
		f.Line()
		return true

	case "write_":
		// Instance method: write contents to self.path
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params(
			jen.Id("contents").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Err().Op(":=").Qual("os", "WriteFile").Call(
				jen.Id("c").Dot("Path"),
				jen.Index().Byte().Parens(jen.Id("contents")),
				jen.Lit(0644),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "append_":
		// Instance method: append contents to self.path
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params(
			jen.Id("contents").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("file"), jen.Err()).Op(":=").Qual("os", "OpenFile").Call(
				jen.Id("c").Dot("Path"),
				jen.Qual("os", "O_APPEND").Op("|").Qual("os", "O_CREATE").Op("|").Qual("os", "O_WRONLY"),
				jen.Lit(0644),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("file").Dot("Close").Call(),
			jen.List(jen.Id("_"), jen.Err()).Op("=").Id("file").Dot("WriteString").Call(jen.Id("contents")),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "delete":
		// Instance method: delete file at self.path
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Err().Op(":=").Qual("os", "Remove").Call(jen.Id("c").Dot("Path")),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "exists":
		// Instance method: check if file exists
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("_"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("==").Nil()).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isFile":
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info").Dot("Mode").Call().Dot("IsRegular").Call()).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isDirectory":
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info").Dot("IsDir").Call()).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "size":
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("0"), jen.Nil()),
			),
			jen.Return(jen.Qual("strconv", "FormatInt").Call(jen.Id("info").Dot("Size").Call(), jen.Lit(10)), jen.Nil()),
		)
		f.Line()
		return true

	case "path":
		// Use GetPath to avoid collision with Path field
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id("GetPath").Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("c").Dot("Path"), jen.Nil()),
		)
		f.Line()
		return true

	case "directory":
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("path/filepath", "Dir").Call(jen.Id("c").Dot("Path")), jen.Nil()),
		)
		f.Line()
		return true

	case "basename":
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("path/filepath", "Base").Call(jen.Id("c").Dot("Path")), jen.Nil()),
		)
		f.Line()
		return true

	case "extension":
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("ext").Op(":=").Qual("path/filepath", "Ext").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Len(jen.Id("ext")).Op(">").Lit(0)).Block(
				jen.Return(jen.Id("ext").Index(jen.Lit(1).Op(":")), jen.Nil()), // Remove leading dot
			),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "isFifo":
		// Check if file is a named pipe (FIFO)
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeNamedPipe").Op("!=").Lit(0)).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "stem":
		// Get filename without extension
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("base").Op(":=").Qual("path/filepath", "Base").Call(jen.Id("c").Dot("Path")),
			jen.Id("ext").Op(":=").Qual("path/filepath", "Ext").Call(jen.Id("base")),
			jen.If(jen.Len(jen.Id("ext")).Op(">").Lit(0)).Block(
				jen.Return(jen.Id("base").Index(jen.Empty(), jen.Len(jen.Id("base")).Op("-").Len(jen.Id("ext"))), jen.Nil()),
			),
			jen.Return(jen.Id("base"), jen.Nil()),
		)
		f.Line()
		return true

	case "writeLine_":
		// Write contents with newline
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params(
			jen.Id("contents").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Err().Op(":=").Qual("os", "WriteFile").Call(
				jen.Id("c").Dot("Path"),
				jen.Index().Byte().Parens(jen.Id("contents").Op("+").Lit("\n")),
				jen.Lit(0644),
			),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "appendLine_":
		// Append contents with newline
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params(
			jen.Id("contents").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("file"), jen.Err()).Op(":=").Qual("os", "OpenFile").Call(
				jen.Id("c").Dot("Path"),
				jen.Qual("os", "O_APPEND").Op("|").Qual("os", "O_CREATE").Op("|").Qual("os", "O_WRONLY"),
				jen.Lit(0644),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("file").Dot("Close").Call(),
			jen.List(jen.Id("_"), jen.Err()).Op("=").Id("file").Dot("WriteString").Call(jen.Id("contents").Op("+").Lit("\n")),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "copyTo_":
		// Copy file to destination
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params(
			jen.Id("destPath").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("data"), jen.Err()).Op(":=").Qual("os", "ReadFile").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Err().Op("=").Qual("os", "WriteFile").Call(jen.Id("destPath"), jen.Id("data"), jen.Lit(0644)),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "moveTo_":
		// Move/rename file
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params(
			jen.Id("destPath").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Err().Op(":=").Qual("os", "Rename").Call(jen.Id("c").Dot("Path"), jen.Id("destPath")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Id("c").Dot("Path").Op("=").Id("destPath"),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "touch":
		// Touch file (create or update timestamp)
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("now").Op(":=").Qual("time", "Now").Call(),
			jen.Err().Op(":=").Qual("os", "Chtimes").Call(jen.Id("c").Dot("Path"), jen.Id("now"), jen.Id("now")),
			jen.If(jen.Qual("os", "IsNotExist").Call(jen.Err())).Block(
				jen.Comment("Create the file if it doesn't exist"),
				jen.List(jen.Id("f"), jen.Err()).Op(":=").Qual("os", "Create").Call(jen.Id("c").Dot("Path")),
				jen.If(jen.Err().Op("!=").Nil()).Block(
					jen.Return(jen.Lit(""), jen.Err()),
				),
				jen.Id("f").Dot("Close").Call(),
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "modificationTime":
		// Get modification time as unix timestamp
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("0"), jen.Nil()),
			),
			jen.Return(jen.Qual("strconv", "FormatInt").Call(jen.Id("info").Dot("ModTime").Call().Dot("Unix").Call(), jen.Lit(10)), jen.Nil()),
		)
		f.Line()
		return true

	case "readLines":
		// Read file as lines (returns newline-separated content)
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("!=").Nil().Op("||").Op("!").Id("info").Dot("Mode").Call().Dot("IsRegular").Call()).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.List(jen.Id("data"), jen.Err()).Op(":=").Qual("os", "ReadFile").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.String().Parens(jen.Id("data")), jen.Nil()),
		)
		f.Line()
		return true

	case "printString":
		// String representation
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Lit("<File ").Op("+").Id("c").Dot("Path").Op("+").Lit(">"), jen.Nil()),
		)
		f.Line()
		return true

	case "info":
		// Print file info
		f.Func().Parens(jen.Id("c").Op("*").Id("File")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Var().Id("result").Qual("strings", "Builder"),
			jen.Id("result").Dot("WriteString").Call(jen.Lit("Path: ").Op("+").Id("c").Dot("Path").Op("+").Lit("\n")),
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("==").Nil()).Block(
				jen.Id("result").Dot("WriteString").Call(jen.Lit("Exists: true\n")),
				jen.Id("result").Dot("WriteString").Call(jen.Lit("Size: ").Op("+").Qual("strconv", "FormatInt").Call(jen.Id("info").Dot("Size").Call(), jen.Lit(10)).Op("+").Lit(" bytes\n")),
			).Else().Block(
				jen.Id("result").Dot("WriteString").Call(jen.Lit("Exists: false\n")),
			),
			jen.Return(jen.Id("result").Dot("String").Call(), jen.Nil()),
		)
		f.Line()
		return true

	// Class methods
	case "exists_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("_"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
			jen.If(jen.Err().Op("==").Nil()).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isFile_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info").Dot("Mode").Call().Dot("IsRegular").Call()).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isDirectory_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info").Dot("IsDir").Call()).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "read_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("data"), jen.Err()).Op(":=").Qual("os", "ReadFile").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.String().Parens(jen.Id("data")), jen.Nil()),
		)
		f.Line()
		return true

	case "write_to_":
		f.Func().Id(m.goName).Params(
			jen.Id("contents").String(),
			jen.Id("path").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Err().Op(":=").Qual("os", "WriteFile").Call(
				jen.Id("path"),
				jen.Index().Byte().Parens(jen.Id("contents")),
				jen.Lit(0644),
			),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "delete_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Err().Op(":=").Qual("os", "Remove").Call(jen.Id("path")),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "isSymlink_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Lstat").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeSymlink").Op("!=").Lit(0)).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isFifo_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeNamedPipe").Op("!=").Lit(0)).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isSocket_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeSocket").Op("!=").Lit(0)).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isBlockDevice_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeDevice").Op("!=").Lit(0)).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isCharDevice_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info").Dot("Mode").Call().Op("&").Qual("os", "ModeCharDevice").Op("!=").Lit(0)).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isReadable_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("file"), jen.Err()).Op(":=").Qual("os", "Open").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.Id("file").Dot("Close").Call(),
			jen.Return(jen.Lit("true"), jen.Nil()),
		)
		f.Line()
		return true

	case "isWritable_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("file"), jen.Err()).Op(":=").Qual("os", "OpenFile").Call(
				jen.Id("path"),
				jen.Qual("os", "O_WRONLY"),
				jen.Lit(0),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.Id("file").Dot("Close").Call(),
			jen.Return(jen.Lit("true"), jen.Nil()),
		)
		f.Line()
		return true

	case "isExecutable_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.Comment("Check if any execute bit is set"),
			jen.If(jen.Id("info").Dot("Mode").Call().Dot("Perm").Call().Op("&").Lit(0111).Op("!=").Lit(0)).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isEmpty_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("true"), jen.Nil()), // Non-existent is "empty"
			),
			jen.If(jen.Id("info").Dot("Size").Call().Op("==").Lit(0)).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "notEmpty_":
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info"), jen.Err()).Op(":=").Qual("os", "Stat").Call(jen.Id("path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info").Dot("Size").Call().Op(">").Lit(0)).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isNewer_than_":
		f.Func().Id(m.goName).Params(
			jen.Id("path1").String(),
			jen.Id("path2").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info1"), jen.Id("err1")).Op(":=").Qual("os", "Stat").Call(jen.Id("path1")),
			jen.List(jen.Id("info2"), jen.Id("err2")).Op(":=").Qual("os", "Stat").Call(jen.Id("path2")),
			jen.If(jen.Id("err1").Op("!=").Nil().Op("||").Id("err2").Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info1").Dot("ModTime").Call().Dot("After").Call(jen.Id("info2").Dot("ModTime").Call())).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isOlder_than_":
		f.Func().Id(m.goName).Params(
			jen.Id("path1").String(),
			jen.Id("path2").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info1"), jen.Id("err1")).Op(":=").Qual("os", "Stat").Call(jen.Id("path1")),
			jen.List(jen.Id("info2"), jen.Id("err2")).Op(":=").Qual("os", "Stat").Call(jen.Id("path2")),
			jen.If(jen.Id("err1").Op("!=").Nil().Op("||").Id("err2").Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Id("info1").Dot("ModTime").Call().Dot("Before").Call(jen.Id("info2").Dot("ModTime").Call())).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "isSame_as_":
		f.Func().Id(m.goName).Params(
			jen.Id("path1").String(),
			jen.Id("path2").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("info1"), jen.Id("err1")).Op(":=").Qual("os", "Stat").Call(jen.Id("path1")),
			jen.List(jen.Id("info2"), jen.Id("err2")).Op(":=").Qual("os", "Stat").Call(jen.Id("path2")),
			jen.If(jen.Id("err1").Op("!=").Nil().Op("||").Id("err2").Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.If(jen.Qual("os", "SameFile").Call(jen.Id("info1"), jen.Id("info2"))).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	default:
		return false
	}
}

// generatePrimitiveMethodEnv generates native Env class methods.
func (g *generator) generatePrimitiveMethodEnv(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	case "get_":
		f.Func().Id(m.goName).Params(jen.Id("name").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("os", "Getenv").Call(jen.Id("name")), jen.Nil()),
		)
		f.Line()
		return true

	case "set_to_":
		f.Func().Id(m.goName).Params(
			jen.Id("name").String(),
			jen.Id("value").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Err().Op(":=").Qual("os", "Setenv").Call(jen.Id("name"), jen.Id("value")),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "unset_":
		f.Func().Id(m.goName).Params(jen.Id("name").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Err().Op(":=").Qual("os", "Unsetenv").Call(jen.Id("name")),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "has_":
		f.Func().Id(m.goName).Params(jen.Id("name").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("_"), jen.Id("exists")).Op(":=").Qual("os", "LookupEnv").Call(jen.Id("name")),
			jen.If(jen.Id("exists")).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	default:
		return false
	}
}

// generatePrimitiveMethodConsole generates native Console class methods.
func (g *generator) generatePrimitiveMethodConsole(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	case "print_":
		// Print message to stdout with newline
		f.Func().Id(m.goName).Params(jen.Id("message").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Qual("fmt", "Println").Call(jen.Id("message")),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "write_":
		// Print message to stdout without newline
		f.Func().Id(m.goName).Params(jen.Id("message").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Qual("fmt", "Print").Call(jen.Id("message")),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "error_":
		// Print message to stderr with newline
		f.Func().Id(m.goName).Params(jen.Id("message").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Qual("fmt", "Fprintln").Call(jen.Qual("os", "Stderr"), jen.Id("message")),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "newline":
		// Print a blank line
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Qual("fmt", "Println").Call(),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	default:
		return false
	}
}
