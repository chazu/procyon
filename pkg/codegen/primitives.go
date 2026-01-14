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
	"Block": {
		"params_code_captured_": true,
		"value":                 true,
		"valueWith_":            true,
		"valueWith_and_":        true,
		"numArgs":               true,
	},
	"FIFO": {
		"at_":             true,
		"create":          true,
		"exists":          true,
		"remove":          true,
		"open":            true,
		"close":           true,
		"writeLine_":      true,
		"readLine":        true,
		"readLineTimeout_": true,
		"startWriter_":    true,
		"stopWriter":      true,
		"startReader_":    true,
		"stopReader":      true,
		"_setPath_":       true,
	},
	"Future": {
		"for_":     true,
		"start":    true,
		"await":    true,
		"poll":     true,
		"isDone":   true,
		"cancel":   true,
		"status":   true,
		"exitCode": true,
		"cleanup":  true,
		"help":     true,
	},
	"Coproc": {
		"for_":          true,
		"startReadOnly": true,
		"start":         true,
		"readLine":      true,
		"writeLine_":    true,
		"isRunning":     true,
		"terminate":     true,
		"kill":          true,
		"_cleanup":      true,
		"_setCommand_":  true,
		"_setStatus_":   true,
	},
	"String": {
		// String tests
		"isEmpty_":             true,
		"notEmpty_":            true,
		"contains_substring_":  true,
		"startsWith_prefix_":   true,
		"endsWith_suffix_":     true,
		"equals_to_":           true,
		// String manipulation
		"trimPrefix_from_":      true,
		"trimSuffix_from_":      true,
		"trimShortPrefix_from_": true,
		"trimShortSuffix_from_": true,
		"replace_with_in_":      true,
		"replaceAll_with_in_":   true,
		"substring_from_length_": true,
		"length_":               true,
		"uppercase_":            true,
		"lowercase_":            true,
		// String splitting
		"split_on_":  true,
		"before_in_": true,
		"after_in_":  true,
		// String building
		"concat_with_":       true,
		"concat_with_with_":  true,
		"join_values_":       true,
		"repeat_times_":      true,
		// Whitespace handling
		"trim_":      true,
		"trimLeft_":  true,
		"trimRight_": true,
	},
	"Shell": {
		// Simple execution
		"exec_":     true,
		"run_":      true,
		"silent_":   true,
		"exitCode_": true,
		"succeeds_": true,
		"fails_":    true,
		// Output capture
		"execAll_":  true,
		"execErr_":  true,
		"execFull_": true,
		// Background execution
		"spawn_":                   true,
		"spawn_outputTo_":          true,
		"spawn_stdoutTo_stderrTo_": true,
		"wait_":                    true,
		// Process control
		"isAlive_":     true,
		"signal_to_":   true,
		"terminate_":   true,
		"kill_":        true,
		"pause_":       true,
		"resume_":      true,
		// Piping and chaining
		"exec_pipeTo_":        true,
		"exec_pipeTo_pipeTo_": true,
		// Input/Output
		"exec_withInput_":     true,
		"exec_withInputFrom_": true,
		"exec_outputTo_":      true,
		"exec_appendTo_":      true,
		// Conditional execution
		"if_then_":       true,
		"unless_then_":   true,
		"exec_timeout_":  true,
		// Current shell state
		"pid":          true,
		"ppid":         true,
		"lastExitCode": true,
	},
	"Array": {
		"withValues_": true,
	},
	"Dictionary": {
		"keys":       true,
		"values":     true,
		"withPairs_": true,
		"do_":        true,
		"keysDo_":    true,
		"valuesDo_":  true,
		"collect_":   true,
		"select_":    true,
		"merge_":     true,
		"asJson":     true,
	},
	"Object": {
		"printString":  true,
		"class":        true,
		"id":           true,
		"isKindOf_":    true,
		"conformsTo_":  true,
		"inspect":      true,
		"edit":         true,
	},
	"Protocol": {
		"requiredMethods": true,
		"isSatisfiedBy_":  true,
	},
	"Time": {
		// Current time
		"now":           true,
		"nowMillis":     true,
		"nowFormatted_": true,
		"nowISO":        true,
		// Formatting
		"format_as_":      true,
		"formatISO_":      true,
		"formatRelative_": true,
		// Parsing
		"parse_format_": true,
		// Delays
		"sleep_":       true,
		"sleepMillis_": true,
		// Duration/Arithmetic
		"since_":          true,
		"from_to_":        true,
		"add_to_":         true,
		"subtract_from_":  true,
		// Components
		"yearOf_":    true,
		"monthOf_":   true,
		"dayOf_":     true,
		"hourOf_":    true,
		"minuteOf_":  true,
		"secondOf_":  true,
		"weekdayOf_": true,
		// Convenience
		"today":     true,
		"tomorrow":  true,
		"yesterday": true,
	},
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
	case "Block":
		return g.generatePrimitiveMethodBlock(f, m)
	case "FIFO":
		return g.generatePrimitiveMethodFIFO(f, m)
	case "Future":
		return g.generatePrimitiveMethodFuture(f, m)
	case "Coproc":
		return g.generatePrimitiveMethodCoproc(f, m)
	case "String":
		return g.generatePrimitiveMethodString(f, m)
	case "Shell":
		return g.generatePrimitiveMethodShell(f, m)
	case "Array":
		return g.generatePrimitiveMethodArray(f, m)
	case "Dictionary":
		return g.generatePrimitiveMethodDictionary(f, m)
	case "Object":
		return g.generatePrimitiveMethodObject(f, m)
	case "Protocol":
		return g.generatePrimitiveMethodProtocol(f, m)
	case "Time":
		return g.generatePrimitiveMethodTime(f, m)
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

// generatePrimitiveMethodBlock generates native Block class methods.
// Block represents closures in Trashtalk. In native Go, these become actual Go closures.
func (g *generator) generatePrimitiveMethodBlock(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	case "params_code_captured_":
		// Factory: Create a Block with params, code, and captured variables
		// In native Go, blocks are compiled inline as closures, so this is primarily
		// for runtime compatibility. The code is stored as a string but would need
		// an interpreter to execute dynamically.
		f.Func().Id(m.goName).Params(
			jen.Id("params").String(),
			jen.Id("code").String(),
			jen.Id("captured").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Generate instance ID"),
			jen.Id("id").Op(":=").Lit("block_").Op("+").Qual("strings", "ReplaceAll").Call(
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
			jen.Id("instance").Op(":=").Op("&").Id("Block").Values(jen.Dict{
				jen.Id("Class"):     jen.Lit("Block"),
				jen.Id("CreatedAt"): jen.Qual("time", "Now").Call().Dot("Format").Call(jen.Qual("time", "RFC3339")),
				jen.Id("Params"):    jen.Id("params"),
				jen.Id("Code"):      jen.Id("code"),
				jen.Id("Captured"):  jen.Id("captured"),
			}),
			jen.Line(),
			jen.If(jen.Err().Op(":=").Id("saveInstance").Call(jen.Id("db"), jen.Id("id"), jen.Id("instance")), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Id("id"), jen.Nil()),
		)
		f.Line()
		return true

	case "numArgs":
		// Return the number of parameters
		f.Func().Parens(jen.Id("c").Op("*").Id("Block")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Parse params JSON array and return length"),
			jen.Var().Id("params").Index().String(),
			jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(jen.Id("c").Dot("Params")),
				jen.Op("&").Id("params"),
			), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("0"), jen.Nil()),
			),
			jen.Return(jen.Qual("strconv", "Itoa").Call(jen.Len(jen.Id("params"))), jen.Nil()),
		)
		f.Line()
		return true

	case "value", "valueWith_", "valueWith_and_":
		// Block execution requires eval-like functionality which isn't available in native Go
		// These would need to fall back to bash or use a different approach
		// For now, return an error indicating dynamic execution isn't supported
		return false

	default:
		return false
	}
}

// generatePrimitiveMethodFIFO generates native FIFO (named pipe) class methods.
func (g *generator) generatePrimitiveMethodFIFO(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	case "at_":
		// Factory: Create a FIFO instance for a given path
		f.Func().Id(m.goName).Params(jen.Id("path").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("id").Op(":=").Lit("fifo_").Op("+").Qual("strings", "ReplaceAll").Call(
				jen.Qual("github.com/google/uuid", "New").Call().Dot("String").Call(),
				jen.Lit("-"),
				jen.Lit(""),
			),
			jen.Line(),
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.Id("instance").Op(":=").Op("&").Id("FIFO").Values(jen.Dict{
				jen.Id("Class"):     jen.Lit("FIFO"),
				jen.Id("CreatedAt"): jen.Qual("time", "Now").Call().Dot("Format").Call(jen.Qual("time", "RFC3339")),
				jen.Id("Path"):      jen.Id("path"),
			}),
			jen.Line(),
			jen.If(jen.Err().Op(":=").Id("saveInstance").Call(jen.Id("db"), jen.Id("id"), jen.Id("instance")), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Id("id"), jen.Nil()),
		)
		f.Line()
		return true

	case "create":
		// Create the named pipe on disk
		f.Func().Parens(jen.Id("c").Op("*").Id("FIFO")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Qual("os", "Remove").Call(jen.Id("c").Dot("Path")),
			jen.Err().Op(":=").Qual("syscall", "Mkfifo").Call(jen.Id("c").Dot("Path"), jen.Lit(0644)),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "exists":
		// Check if the pipe exists
		f.Func().Parens(jen.Id("c").Op("*").Id("FIFO")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
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

	case "remove":
		// Remove the pipe from disk
		f.Func().Parens(jen.Id("c").Op("*").Id("FIFO")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Qual("os", "Remove").Call(jen.Id("c").Dot("Path")),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "writeLine_":
		// Write a line to the FIFO
		f.Func().Parens(jen.Id("c").Op("*").Id("FIFO")).Id(m.goName).Params(
			jen.Id("text").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("file"), jen.Err()).Op(":=").Qual("os", "OpenFile").Call(
				jen.Id("c").Dot("Path"),
				jen.Qual("os", "O_WRONLY"),
				jen.Lit(0),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("file").Dot("Close").Call(),
			jen.List(jen.Id("_"), jen.Err()).Op("=").Id("file").Dot("WriteString").Call(jen.Id("text").Op("+").Lit("\n")),
			jen.Return(jen.Lit(""), jen.Err()),
		)
		f.Line()
		return true

	case "readLine":
		// Read a line from the FIFO
		f.Func().Parens(jen.Id("c").Op("*").Id("FIFO")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("file"), jen.Err()).Op(":=").Qual("os", "Open").Call(jen.Id("c").Dot("Path")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("file").Dot("Close").Call(),
			jen.Line(),
			jen.Id("reader").Op(":=").Qual("bufio", "NewReader").Call(jen.Id("file")),
			jen.List(jen.Id("line"), jen.Err()).Op(":=").Id("reader").Dot("ReadString").Call(jen.Lit('\n')),
			jen.If(jen.Err().Op("!=").Nil().Op("&&").Err().Op("!=").Qual("io", "EOF")).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Qual("strings", "TrimSuffix").Call(jen.Id("line"), jen.Lit("\n")), jen.Nil()),
		)
		f.Line()
		return true

	case "_setPath_":
		// Private setter for path
		f.Func().Parens(jen.Id("c").Op("*").Id("FIFO")).Id(m.goName).Params(
			jen.Id("path").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("c").Dot("Path").Op("=").Id("path"),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "open", "close", "readLineTimeout_", "startWriter_", "stopWriter", "startReader_", "stopReader":
		// These require background process management which is complex in native Go
		// Fall back to bash for now
		return false

	default:
		return false
	}
}

// generatePrimitiveMethodFuture generates native Future class methods.
func (g *generator) generatePrimitiveMethodFuture(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	case "for_":
		// Factory: Create a Future for a command
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("id").Op(":=").Lit("future_").Op("+").Qual("strings", "ReplaceAll").Call(
				jen.Qual("github.com/google/uuid", "New").Call().Dot("String").Call(),
				jen.Lit("-"),
				jen.Lit(""),
			),
			jen.Line(),
			jen.Comment("Set up result file path"),
			jen.Id("resultDir").Op(":=").Lit("/tmp/trashtalk/futures"),
			jen.Qual("os", "MkdirAll").Call(jen.Id("resultDir"), jen.Lit(0755)),
			jen.Id("resultFile").Op(":=").Id("resultDir").Op("+").Lit("/").Op("+").Id("id"),
			jen.Line(),
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.Id("instance").Op(":=").Op("&").Id("Future").Values(jen.Dict{
				jen.Id("Class"):      jen.Lit("Future"),
				jen.Id("CreatedAt"):  jen.Qual("time", "Now").Call().Dot("Format").Call(jen.Qual("time", "RFC3339")),
				jen.Id("Command"):    jen.Id("command"),
				jen.Id("ResultFile"): jen.Id("resultFile"),
				jen.Id("Status"):     jen.Lit("created"),
			}),
			jen.Line(),
			jen.If(jen.Err().Op(":=").Id("saveInstance").Call(jen.Id("db"), jen.Id("id"), jen.Id("instance")), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Id("id"), jen.Nil()),
		)
		f.Line()
		return true

	case "status":
		// Get current status
		f.Func().Parens(jen.Id("c").Op("*").Id("Future")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("c").Dot("Status"), jen.Nil()),
		)
		f.Line()
		return true

	case "exitCode":
		// Get exit code
		f.Func().Parens(jen.Id("c").Op("*").Id("Future")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("c").Dot("ExitCode"), jen.Nil()),
		)
		f.Line()
		return true

	case "cleanup":
		// Clean up result files
		f.Func().Parens(jen.Id("c").Op("*").Id("Future")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Qual("os", "Remove").Call(jen.Id("c").Dot("ResultFile")),
			jen.Qual("os", "Remove").Call(jen.Id("c").Dot("ResultFile").Op("+").Lit(".exit")),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "help":
		// Show help text
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("help").Op(":=").Lit("=== Future - Async Computation ===\n\nUsage:\n  future := @ Future for: 'command'\n  @ future start\n  result := @ future await\n"),
			jen.Return(jen.Id("help"), jen.Nil()),
		)
		f.Line()
		return true

	case "start", "await", "poll", "isDone", "cancel":
		// These require process management with os/exec
		// Complex to implement correctly - fall back to bash
		return false

	default:
		return false
	}
}

// generatePrimitiveMethodCoproc generates native Coproc class methods.
func (g *generator) generatePrimitiveMethodCoproc(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	case "for_":
		// Factory: Create a Coproc for a command
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("id").Op(":=").Lit("coproc_").Op("+").Qual("strings", "ReplaceAll").Call(
				jen.Qual("github.com/google/uuid", "New").Call().Dot("String").Call(),
				jen.Lit("-"),
				jen.Lit(""),
			),
			jen.Line(),
			jen.List(jen.Id("db"), jen.Err()).Op(":=").Id("openDB").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("db").Dot("Close").Call(),
			jen.Line(),
			jen.Id("instance").Op(":=").Op("&").Id("Coproc").Values(jen.Dict{
				jen.Id("Class"):     jen.Lit("Coproc"),
				jen.Id("CreatedAt"): jen.Qual("time", "Now").Call().Dot("Format").Call(jen.Qual("time", "RFC3339")),
				jen.Id("Command"):   jen.Id("command"),
				jen.Id("Status"):    jen.Lit("created"),
			}),
			jen.Line(),
			jen.If(jen.Err().Op(":=").Id("saveInstance").Call(jen.Id("db"), jen.Id("id"), jen.Id("instance")), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Id("id"), jen.Nil()),
		)
		f.Line()
		return true

	case "isRunning":
		// Check if process is running using stored PID
		f.Func().Parens(jen.Id("c").Op("*").Id("Coproc")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.If(jen.Id("c").Dot("Pid").Op("==").Lit("")).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.Line(),
			jen.List(jen.Id("pid"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("c").Dot("Pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.Line(),
			jen.List(jen.Id("process"), jen.Err()).Op(":=").Qual("os", "FindProcess").Call(jen.Id("pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.Line(),
			jen.Comment("On Unix, FindProcess always succeeds. Use Signal(0) to check."),
			jen.Err().Op("=").Id("process").Dot("Signal").Call(jen.Qual("syscall", "Signal").Call(jen.Lit(0))),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.Return(jen.Lit("true"), jen.Nil()),
		)
		f.Line()
		return true

	case "terminate":
		// Send SIGTERM to the process
		f.Func().Parens(jen.Id("c").Op("*").Id("Coproc")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.If(jen.Id("c").Dot("Pid").Op("==").Lit("")).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.Line(),
			jen.List(jen.Id("pid"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("c").Dot("Pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Line(),
			jen.List(jen.Id("process"), jen.Err()).Op(":=").Qual("os", "FindProcess").Call(jen.Id("pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Id("process").Dot("Signal").Call(jen.Qual("syscall", "SIGTERM")),
			jen.Id("c").Dot("Status").Op("=").Lit("terminated"),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "kill":
		// Send SIGKILL to the process
		f.Func().Parens(jen.Id("c").Op("*").Id("Coproc")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.If(jen.Id("c").Dot("Pid").Op("==").Lit("")).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.Line(),
			jen.List(jen.Id("pid"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("c").Dot("Pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Line(),
			jen.List(jen.Id("process"), jen.Err()).Op(":=").Qual("os", "FindProcess").Call(jen.Id("pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Id("process").Dot("Kill").Call(),
			jen.Id("c").Dot("Status").Op("=").Lit("killed"),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "_setCommand_":
		f.Func().Parens(jen.Id("c").Op("*").Id("Coproc")).Id(m.goName).Params(
			jen.Id("cmd").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("c").Dot("Command").Op("=").Id("cmd"),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "_setStatus_":
		f.Func().Parens(jen.Id("c").Op("*").Id("Coproc")).Id(m.goName).Params(
			jen.Id("status").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("c").Dot("Status").Op("=").Id("status"),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "_cleanup":
		// Clean up FIFOs
		f.Func().Parens(jen.Id("c").Op("*").Id("Coproc")).Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.If(jen.Id("c").Dot("FifoIn").Op("!=").Lit("")).Block(
				jen.Qual("os", "Remove").Call(jen.Id("c").Dot("FifoIn")),
			),
			jen.If(jen.Id("c").Dot("FifoOut").Op("!=").Lit("")).Block(
				jen.Qual("os", "Remove").Call(jen.Id("c").Dot("FifoOut")),
			),
			jen.Id("c").Dot("FifoIn").Op("=").Lit(""),
			jen.Id("c").Dot("FifoOut").Op("=").Lit(""),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "startReadOnly", "start", "readLine", "writeLine_", "readLinesDo_":
		// These require complex process management with exec.Cmd and pipes
		// Fall back to bash for now
		return false

	default:
		return false
	}
}

// generatePrimitiveMethodString generates native String class methods.
// All String methods are class methods for string manipulation.
func (g *generator) generatePrimitiveMethodString(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	// String tests
	case "isEmpty_":
		f.Func().Id(m.goName).Params(jen.Id("str").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.If(jen.Len(jen.Id("str")).Op("==").Lit(0)).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "notEmpty_":
		f.Func().Id(m.goName).Params(jen.Id("str").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.If(jen.Len(jen.Id("str")).Op(">").Lit(0)).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "contains_substring_":
		f.Func().Id(m.goName).Params(
			jen.Id("str").String(),
			jen.Id("sub").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.If(jen.Qual("strings", "Contains").Call(jen.Id("str"), jen.Id("sub"))).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "startsWith_prefix_":
		f.Func().Id(m.goName).Params(
			jen.Id("str").String(),
			jen.Id("prefix").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.If(jen.Qual("strings", "HasPrefix").Call(jen.Id("str"), jen.Id("prefix"))).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "endsWith_suffix_":
		f.Func().Id(m.goName).Params(
			jen.Id("str").String(),
			jen.Id("suffix").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.If(jen.Qual("strings", "HasSuffix").Call(jen.Id("str"), jen.Id("suffix"))).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "equals_to_":
		f.Func().Id(m.goName).Params(
			jen.Id("a").String(),
			jen.Id("b").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.If(jen.Id("a").Op("==").Id("b")).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	// String manipulation
	case "trimPrefix_from_":
		f.Func().Id(m.goName).Params(
			jen.Id("prefix").String(),
			jen.Id("str").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strings", "TrimPrefix").Call(jen.Id("str"), jen.Id("prefix")), jen.Nil()),
		)
		f.Line()
		return true

	case "trimSuffix_from_":
		f.Func().Id(m.goName).Params(
			jen.Id("suffix").String(),
			jen.Id("str").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strings", "TrimSuffix").Call(jen.Id("str"), jen.Id("suffix")), jen.Nil()),
		)
		f.Line()
		return true

	case "trimShortPrefix_from_":
		// Go doesn't have exact equivalent of bash ${str#pattern} - use TrimPrefix
		f.Func().Id(m.goName).Params(
			jen.Id("prefix").String(),
			jen.Id("str").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strings", "TrimPrefix").Call(jen.Id("str"), jen.Id("prefix")), jen.Nil()),
		)
		f.Line()
		return true

	case "trimShortSuffix_from_":
		// Go doesn't have exact equivalent of bash ${str%pattern} - use TrimSuffix
		f.Func().Id(m.goName).Params(
			jen.Id("suffix").String(),
			jen.Id("str").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strings", "TrimSuffix").Call(jen.Id("str"), jen.Id("suffix")), jen.Nil()),
		)
		f.Line()
		return true

	case "replace_with_in_":
		f.Func().Id(m.goName).Params(
			jen.Id("old").String(),
			jen.Id("new").String(),
			jen.Id("str").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strings", "Replace").Call(jen.Id("str"), jen.Id("old"), jen.Id("new"), jen.Lit(1)), jen.Nil()),
		)
		f.Line()
		return true

	case "replaceAll_with_in_":
		f.Func().Id(m.goName).Params(
			jen.Id("old").String(),
			jen.Id("new").String(),
			jen.Id("str").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strings", "ReplaceAll").Call(jen.Id("str"), jen.Id("old"), jen.Id("new")), jen.Nil()),
		)
		f.Line()
		return true

	case "substring_from_length_":
		f.Func().Id(m.goName).Params(
			jen.Id("str").String(),
			jen.Id("start").String(),
			jen.Id("length").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("s"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("start")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.List(jen.Id("l"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("length")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.If(jen.Id("s").Op("<").Lit(0).Op("||").Id("s").Op(">=").Len(jen.Id("str"))).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.Id("end").Op(":=").Id("s").Op("+").Id("l"),
			jen.If(jen.Id("end").Op(">").Len(jen.Id("str"))).Block(
				jen.Id("end").Op("=").Len(jen.Id("str")),
			),
			jen.Return(jen.Id("str").Index(jen.Id("s"), jen.Id("end")), jen.Nil()),
		)
		f.Line()
		return true

	case "length_":
		f.Func().Id(m.goName).Params(jen.Id("str").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strconv", "Itoa").Call(jen.Len(jen.Id("str"))), jen.Nil()),
		)
		f.Line()
		return true

	case "uppercase_":
		f.Func().Id(m.goName).Params(jen.Id("str").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strings", "ToUpper").Call(jen.Id("str")), jen.Nil()),
		)
		f.Line()
		return true

	case "lowercase_":
		f.Func().Id(m.goName).Params(jen.Id("str").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strings", "ToLower").Call(jen.Id("str")), jen.Nil()),
		)
		f.Line()
		return true

	// String splitting
	case "split_on_":
		f.Func().Id(m.goName).Params(
			jen.Id("str").String(),
			jen.Id("delim").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("parts").Op(":=").Qual("strings", "Split").Call(jen.Id("str"), jen.Id("delim")),
			jen.Return(jen.Qual("strings", "Join").Call(jen.Id("parts"), jen.Lit("\n")), jen.Nil()),
		)
		f.Line()
		return true

	case "before_in_":
		f.Func().Id(m.goName).Params(
			jen.Id("delim").String(),
			jen.Id("str").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("idx").Op(":=").Qual("strings", "Index").Call(jen.Id("str"), jen.Id("delim")),
			jen.If(jen.Id("idx").Op("<").Lit(0)).Block(
				jen.Return(jen.Id("str"), jen.Nil()),
			),
			jen.Return(jen.Id("str").Index(jen.Empty(), jen.Id("idx")), jen.Nil()),
		)
		f.Line()
		return true

	case "after_in_":
		f.Func().Id(m.goName).Params(
			jen.Id("delim").String(),
			jen.Id("str").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("idx").Op(":=").Qual("strings", "Index").Call(jen.Id("str"), jen.Id("delim")),
			jen.If(jen.Id("idx").Op("<").Lit(0)).Block(
				jen.Return(jen.Id("str"), jen.Nil()),
			),
			jen.Return(jen.Id("str").Index(jen.Id("idx").Op("+").Len(jen.Id("delim")), jen.Empty()), jen.Nil()),
		)
		f.Line()
		return true

	// String building
	case "concat_with_":
		f.Func().Id(m.goName).Params(
			jen.Id("a").String(),
			jen.Id("b").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("a").Op("+").Id("b"), jen.Nil()),
		)
		f.Line()
		return true

	case "concat_with_with_":
		f.Func().Id(m.goName).Params(
			jen.Id("a").String(),
			jen.Id("b").String(),
			jen.Id("c").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("a").Op("+").Id("b").Op("+").Id("c"), jen.Nil()),
		)
		f.Line()
		return true

	case "join_values_":
		f.Func().Id(m.goName).Params(
			jen.Id("delim").String(),
			jen.Id("values").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Split values on whitespace, rejoin with delimiter"),
			jen.Id("parts").Op(":=").Qual("strings", "Fields").Call(jen.Id("values")),
			jen.Return(jen.Qual("strings", "Join").Call(jen.Id("parts"), jen.Id("delim")), jen.Nil()),
		)
		f.Line()
		return true

	case "repeat_times_":
		f.Func().Id(m.goName).Params(
			jen.Id("str").String(),
			jen.Id("times").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("n"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("times")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Qual("strings", "Repeat").Call(jen.Id("str"), jen.Id("n")), jen.Nil()),
		)
		f.Line()
		return true

	// Whitespace handling
	case "trim_":
		f.Func().Id(m.goName).Params(jen.Id("str").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strings", "TrimSpace").Call(jen.Id("str")), jen.Nil()),
		)
		f.Line()
		return true

	case "trimLeft_":
		f.Func().Id(m.goName).Params(jen.Id("str").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strings", "TrimLeft").Call(jen.Id("str"), jen.Lit(" \t\n\r")), jen.Nil()),
		)
		f.Line()
		return true

	case "trimRight_":
		f.Func().Id(m.goName).Params(jen.Id("str").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strings", "TrimRight").Call(jen.Id("str"), jen.Lit(" \t\n\r")), jen.Nil()),
		)
		f.Line()
		return true

	default:
		return false
	}
}

// generatePrimitiveMethodShell generates native Shell class methods.
// Shell provides command execution primitives - all methods are class methods.
func (g *generator) generatePrimitiveMethodShell(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	// Simple execution
	case "exec_":
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.List(jen.Id("output"), jen.Err()).Op(":=").Id("cmd").Dot("Output").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Comment("Return output even on error (non-zero exit)"),
				jen.Return(jen.String().Call(jen.Id("output")), jen.Nil()),
			),
			jen.Return(jen.String().Call(jen.Id("output")), jen.Nil()),
		)
		f.Line()
		return true

	case "run_":
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.List(jen.Id("output"), jen.Id("_")).Op(":=").Id("cmd").Dot("Output").Call(),
			jen.Return(jen.String().Call(jen.Id("output")), jen.Nil()),
		)
		f.Line()
		return true

	case "silent_":
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Id("_").Op("=").Id("cmd").Dot("Run").Call(),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "exitCode_":
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Err().Op(":=").Id("cmd").Dot("Run").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.If(
					jen.List(jen.Id("exitErr"), jen.Id("ok")).Op(":=").Err().Dot("").Parens(jen.Op("*").Qual("os/exec", "ExitError")),
					jen.Id("ok"),
				).Block(
					jen.Return(jen.Qual("strconv", "Itoa").Call(jen.Id("exitErr").Dot("ExitCode").Call()), jen.Nil()),
				),
				jen.Return(jen.Lit("1"), jen.Nil()),
			),
			jen.Return(jen.Lit("0"), jen.Nil()),
		)
		f.Line()
		return true

	case "succeeds_":
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Err().Op(":=").Id("cmd").Dot("Run").Call(),
			jen.If(jen.Err().Op("==").Nil()).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "fails_":
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Err().Op(":=").Id("cmd").Dot("Run").Call(),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	// Output capture
	case "execAll_":
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.List(jen.Id("output"), jen.Id("_")).Op(":=").Id("cmd").Dot("CombinedOutput").Call(),
			jen.Return(jen.String().Call(jen.Id("output")), jen.Nil()),
		)
		f.Line()
		return true

	case "execErr_":
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Var().Id("stderr").Qual("bytes", "Buffer"),
			jen.Id("cmd").Dot("Stderr").Op("=").Op("&").Id("stderr"),
			jen.Id("_").Op("=").Id("cmd").Dot("Run").Call(),
			jen.Return(jen.Id("stderr").Dot("String").Call(), jen.Nil()),
		)
		f.Line()
		return true

	case "execFull_":
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Var().Id("stdout").Qual("bytes", "Buffer"),
			jen.Var().Id("stderr").Qual("bytes", "Buffer"),
			jen.Id("cmd").Dot("Stdout").Op("=").Op("&").Id("stdout"),
			jen.Id("cmd").Dot("Stderr").Op("=").Op("&").Id("stderr"),
			jen.Err().Op(":=").Id("cmd").Dot("Run").Call(),
			jen.Id("exitCode").Op(":=").Lit(0),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.If(
					jen.List(jen.Id("exitErr"), jen.Id("ok")).Op(":=").Err().Dot("").Parens(jen.Op("*").Qual("os/exec", "ExitError")),
					jen.Id("ok"),
				).Block(
					jen.Id("exitCode").Op("=").Id("exitErr").Dot("ExitCode").Call(),
				).Else().Block(
					jen.Id("exitCode").Op("=").Lit(1),
				),
			),
			jen.Comment("Return JSON object with stdout, stderr, exitCode"),
			jen.Id("result").Op(":=").Qual("fmt", "Sprintf").Call(
				jen.Lit(`{"stdout":%q,"stderr":%q,"exitCode":%d}`),
				jen.Id("stdout").Dot("String").Call(),
				jen.Id("stderr").Dot("String").Call(),
				jen.Id("exitCode"),
			),
			jen.Return(jen.Id("result"), jen.Nil()),
		)
		f.Line()
		return true

	// Background execution
	case "spawn_":
		f.Func().Id(m.goName).Params(jen.Id("command").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.If(jen.Err().Op(":=").Id("cmd").Dot("Start").Call(), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.Qual("strconv", "Itoa").Call(jen.Id("cmd").Dot("Process").Dot("Pid")), jen.Nil()),
		)
		f.Line()
		return true

	case "spawn_outputTo_":
		f.Func().Id(m.goName).Params(
			jen.Id("command").String(),
			jen.Id("filepath").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("outFile"), jen.Err()).Op(":=").Qual("os", "Create").Call(jen.Id("filepath")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Id("cmd").Dot("Stdout").Op("=").Id("outFile"),
			jen.Id("cmd").Dot("Stderr").Op("=").Id("outFile"),
			jen.If(jen.Err().Op(":=").Id("cmd").Dot("Start").Call(), jen.Err().Op("!=").Nil()).Block(
				jen.Id("outFile").Dot("Close").Call(),
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Comment("Close file when process exits (in goroutine)"),
			jen.Go().Func().Params().Block(
				jen.Id("cmd").Dot("Wait").Call(),
				jen.Id("outFile").Dot("Close").Call(),
			).Call(),
			jen.Return(jen.Qual("strconv", "Itoa").Call(jen.Id("cmd").Dot("Process").Dot("Pid")), jen.Nil()),
		)
		f.Line()
		return true

	case "spawn_stdoutTo_stderrTo_":
		f.Func().Id(m.goName).Params(
			jen.Id("command").String(),
			jen.Id("stdoutPath").String(),
			jen.Id("stderrPath").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("stdoutFile"), jen.Err()).Op(":=").Qual("os", "Create").Call(jen.Id("stdoutPath")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.List(jen.Id("stderrFile"), jen.Err()).Op(":=").Qual("os", "Create").Call(jen.Id("stderrPath")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Id("stdoutFile").Dot("Close").Call(),
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Id("cmd").Dot("Stdout").Op("=").Id("stdoutFile"),
			jen.Id("cmd").Dot("Stderr").Op("=").Id("stderrFile"),
			jen.If(jen.Err().Op(":=").Id("cmd").Dot("Start").Call(), jen.Err().Op("!=").Nil()).Block(
				jen.Id("stdoutFile").Dot("Close").Call(),
				jen.Id("stderrFile").Dot("Close").Call(),
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Comment("Close files when process exits (in goroutine)"),
			jen.Go().Func().Params().Block(
				jen.Id("cmd").Dot("Wait").Call(),
				jen.Id("stdoutFile").Dot("Close").Call(),
				jen.Id("stderrFile").Dot("Close").Call(),
			).Call(),
			jen.Return(jen.Qual("strconv", "Itoa").Call(jen.Id("cmd").Dot("Process").Dot("Pid")), jen.Nil()),
		)
		f.Line()
		return true

	case "wait_":
		f.Func().Id(m.goName).Params(jen.Id("pidStr").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("pid"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("pidStr")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("1"), jen.Err()),
			),
			jen.List(jen.Id("proc"), jen.Err()).Op(":=").Qual("os", "FindProcess").Call(jen.Id("pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("1"), jen.Nil()),
			),
			jen.List(jen.Id("state"), jen.Id("_")).Op(":=").Id("proc").Dot("Wait").Call(),
			jen.If(jen.Id("state").Op("!=").Nil()).Block(
				jen.Return(jen.Qual("strconv", "Itoa").Call(jen.Id("state").Dot("ExitCode").Call()), jen.Nil()),
			),
			jen.Return(jen.Lit("0"), jen.Nil()),
		)
		f.Line()
		return true

	// Process control
	case "isAlive_":
		f.Func().Id(m.goName).Params(jen.Id("pidStr").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("pid"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("pidStr")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.List(jen.Id("proc"), jen.Err()).Op(":=").Qual("os", "FindProcess").Call(jen.Id("pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit("false"), jen.Nil()),
			),
			jen.Comment("On Unix, FindProcess always succeeds - use Signal(0) to check"),
			jen.Err().Op("=").Id("proc").Dot("Signal").Call(jen.Qual("syscall", "Signal").Call(jen.Lit(0))),
			jen.If(jen.Err().Op("==").Nil()).Block(
				jen.Return(jen.Lit("true"), jen.Nil()),
			),
			jen.Return(jen.Lit("false"), jen.Nil()),
		)
		f.Line()
		return true

	case "signal_to_":
		f.Func().Id(m.goName).Params(
			jen.Id("signalName").String(),
			jen.Id("pidStr").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("pid"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("pidStr")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.List(jen.Id("proc"), jen.Err()).Op(":=").Qual("os", "FindProcess").Call(jen.Id("pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Comment("Map signal name to signal"),
			jen.Var().Id("sig").Qual("os", "Signal"),
			jen.Switch(jen.Id("signalName")).Block(
				jen.Case(jen.Lit("TERM"), jen.Lit("SIGTERM")).Block(
					jen.Id("sig").Op("=").Qual("syscall", "SIGTERM"),
				),
				jen.Case(jen.Lit("KILL"), jen.Lit("SIGKILL")).Block(
					jen.Id("sig").Op("=").Qual("syscall", "SIGKILL"),
				),
				jen.Case(jen.Lit("STOP"), jen.Lit("SIGSTOP")).Block(
					jen.Id("sig").Op("=").Qual("syscall", "SIGSTOP"),
				),
				jen.Case(jen.Lit("CONT"), jen.Lit("SIGCONT")).Block(
					jen.Id("sig").Op("=").Qual("syscall", "SIGCONT"),
				),
				jen.Case(jen.Lit("INT"), jen.Lit("SIGINT")).Block(
					jen.Id("sig").Op("=").Qual("syscall", "SIGINT"),
				),
				jen.Case(jen.Lit("HUP"), jen.Lit("SIGHUP")).Block(
					jen.Id("sig").Op("=").Qual("syscall", "SIGHUP"),
				),
				jen.Default().Block(
					jen.Id("sig").Op("=").Qual("syscall", "SIGTERM"),
				),
			),
			jen.Id("proc").Dot("Signal").Call(jen.Id("sig")),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "terminate_":
		f.Func().Id(m.goName).Params(jen.Id("pidStr").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("pid"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("pidStr")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.List(jen.Id("proc"), jen.Err()).Op(":=").Qual("os", "FindProcess").Call(jen.Id("pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.Id("proc").Dot("Signal").Call(jen.Qual("syscall", "SIGTERM")),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "kill_":
		f.Func().Id(m.goName).Params(jen.Id("pidStr").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("pid"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("pidStr")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.List(jen.Id("proc"), jen.Err()).Op(":=").Qual("os", "FindProcess").Call(jen.Id("pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.Id("proc").Dot("Kill").Call(),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "pause_":
		f.Func().Id(m.goName).Params(jen.Id("pidStr").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("pid"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("pidStr")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.List(jen.Id("proc"), jen.Err()).Op(":=").Qual("os", "FindProcess").Call(jen.Id("pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.Id("proc").Dot("Signal").Call(jen.Qual("syscall", "SIGSTOP")),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "resume_":
		f.Func().Id(m.goName).Params(jen.Id("pidStr").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("pid"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("pidStr")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.List(jen.Id("proc"), jen.Err()).Op(":=").Qual("os", "FindProcess").Call(jen.Id("pid")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Nil()),
			),
			jen.Id("proc").Dot("Signal").Call(jen.Qual("syscall", "SIGCONT")),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	// Piping and chaining
	case "exec_pipeTo_":
		f.Func().Id(m.goName).Params(
			jen.Id("command").String(),
			jen.Id("pipeCommand").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Execute first command and pipe to second"),
			jen.Id("fullCmd").Op(":=").Id("command").Op("+").Lit(" | ").Op("+").Id("pipeCommand"),
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("fullCmd")),
			jen.List(jen.Id("output"), jen.Id("_")).Op(":=").Id("cmd").Dot("Output").Call(),
			jen.Return(jen.String().Call(jen.Id("output")), jen.Nil()),
		)
		f.Line()
		return true

	case "exec_pipeTo_pipeTo_":
		f.Func().Id(m.goName).Params(
			jen.Id("command").String(),
			jen.Id("pipe1").String(),
			jen.Id("pipe2").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("fullCmd").Op(":=").Id("command").Op("+").Lit(" | ").Op("+").Id("pipe1").Op("+").Lit(" | ").Op("+").Id("pipe2"),
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("fullCmd")),
			jen.List(jen.Id("output"), jen.Id("_")).Op(":=").Id("cmd").Dot("Output").Call(),
			jen.Return(jen.String().Call(jen.Id("output")), jen.Nil()),
		)
		f.Line()
		return true

	// Input/Output
	case "exec_withInput_":
		f.Func().Id(m.goName).Params(
			jen.Id("command").String(),
			jen.Id("input").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Id("cmd").Dot("Stdin").Op("=").Qual("strings", "NewReader").Call(jen.Id("input")),
			jen.List(jen.Id("output"), jen.Id("_")).Op(":=").Id("cmd").Dot("Output").Call(),
			jen.Return(jen.String().Call(jen.Id("output")), jen.Nil()),
		)
		f.Line()
		return true

	case "exec_withInputFrom_":
		f.Func().Id(m.goName).Params(
			jen.Id("command").String(),
			jen.Id("filepath").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("inputFile"), jen.Err()).Op(":=").Qual("os", "Open").Call(jen.Id("filepath")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("inputFile").Dot("Close").Call(),
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Id("cmd").Dot("Stdin").Op("=").Id("inputFile"),
			jen.List(jen.Id("output"), jen.Id("_")).Op(":=").Id("cmd").Dot("Output").Call(),
			jen.Return(jen.String().Call(jen.Id("output")), jen.Nil()),
		)
		f.Line()
		return true

	case "exec_outputTo_":
		f.Func().Id(m.goName).Params(
			jen.Id("command").String(),
			jen.Id("filepath").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("outFile"), jen.Err()).Op(":=").Qual("os", "Create").Call(jen.Id("filepath")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("outFile").Dot("Close").Call(),
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Id("cmd").Dot("Stdout").Op("=").Id("outFile"),
			jen.Id("_").Op("=").Id("cmd").Dot("Run").Call(),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "exec_appendTo_":
		f.Func().Id(m.goName).Params(
			jen.Id("command").String(),
			jen.Id("filepath").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("outFile"), jen.Err()).Op(":=").Qual("os", "OpenFile").Call(
				jen.Id("filepath"),
				jen.Qual("os", "O_APPEND").Op("|").Qual("os", "O_CREATE").Op("|").Qual("os", "O_WRONLY"),
				jen.Lit(0644),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Defer().Id("outFile").Dot("Close").Call(),
			jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.Id("cmd").Dot("Stdout").Op("=").Id("outFile"),
			jen.Id("_").Op("=").Id("cmd").Dot("Run").Call(),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	// Conditional execution
	case "if_then_":
		f.Func().Id(m.goName).Params(
			jen.Id("condition").String(),
			jen.Id("command").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("condCmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("condition")),
			jen.If(jen.Err().Op(":=").Id("condCmd").Dot("Run").Call(), jen.Err().Op("==").Nil()).Block(
				jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
				jen.List(jen.Id("output"), jen.Id("_")).Op(":=").Id("cmd").Dot("Output").Call(),
				jen.Return(jen.String().Call(jen.Id("output")), jen.Nil()),
			),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "unless_then_":
		f.Func().Id(m.goName).Params(
			jen.Id("condition").String(),
			jen.Id("command").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("condCmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("condition")),
			jen.If(jen.Err().Op(":=").Id("condCmd").Dot("Run").Call(), jen.Err().Op("!=").Nil()).Block(
				jen.Id("cmd").Op(":=").Qual("os/exec", "Command").Call(jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
				jen.List(jen.Id("output"), jen.Id("_")).Op(":=").Id("cmd").Dot("Output").Call(),
				jen.Return(jen.String().Call(jen.Id("output")), jen.Nil()),
			),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "exec_timeout_":
		f.Func().Id(m.goName).Params(
			jen.Id("command").String(),
			jen.Id("secondsStr").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("seconds"), jen.Err()).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("secondsStr")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.List(jen.Id("ctx"), jen.Id("cancel")).Op(":=").Qual("context", "WithTimeout").Call(
				jen.Qual("context", "Background").Call(),
				jen.Qual("time", "Duration").Call(jen.Id("seconds")).Op("*").Qual("time", "Second"),
			),
			jen.Defer().Id("cancel").Call(),
			jen.Id("cmd").Op(":=").Qual("os/exec", "CommandContext").Call(jen.Id("ctx"), jen.Lit("bash"), jen.Lit("-c"), jen.Id("command")),
			jen.List(jen.Id("output"), jen.Id("_")).Op(":=").Id("cmd").Dot("Output").Call(),
			jen.Return(jen.String().Call(jen.Id("output")), jen.Nil()),
		)
		f.Line()
		return true

	// Current shell state
	case "pid":
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strconv", "Itoa").Call(jen.Qual("os", "Getpid").Call()), jen.Nil()),
		)
		f.Line()
		return true

	case "ppid":
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("strconv", "Itoa").Call(jen.Qual("os", "Getppid").Call()), jen.Nil()),
		)
		f.Line()
		return true

	case "lastExitCode":
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("In native Go, we don't have a global last exit code - return 0"),
			jen.Return(jen.Lit("0"), jen.Nil()),
		)
		f.Line()
		return true

	default:
		return false
	}
}

// generatePrimitiveMethodArray generates native Array class methods.
func (g *generator) generatePrimitiveMethodArray(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	case "withValues_":
		// Initialize array with space-separated values
		f.Func().Id(m.goName).Params(
			jen.Id("receiver").String(),
			jen.Id("values").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Split values by whitespace and build JSON array"),
			jen.Id("parts").Op(":=").Qual("strings", "Fields").Call(jen.Id("values")),
			jen.Id("jsonArray").Op(":=").Make(jen.Index().Interface(), jen.Len(jen.Id("parts"))),
			jen.For(jen.List(jen.Id("i"), jen.Id("v")).Op(":=").Range().Id("parts")).Block(
				jen.Id("jsonArray").Index(jen.Id("i")).Op("=").Id("v"),
			),
			jen.List(jen.Id("data"), jen.Err()).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("jsonArray")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Comment("TODO: Set ivar 'items' on receiver to string(data)"),
			jen.Id("_").Op("=").Id("data"),
			jen.Return(jen.Id("receiver"), jen.Nil()),
		)
		f.Line()
		return true

	default:
		return false
	}
}

// generatePrimitiveMethodDictionary generates native Dictionary class methods.
func (g *generator) generatePrimitiveMethodDictionary(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	case "keys":
		// Return keys of dictionary as newline-separated string
		f.Func().Id(m.goName).Params(
			jen.Id("receiver").String(),
			jen.Id("items").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Var().Id("data").Map(jen.String()).Interface(),
			jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(jen.Id("items")),
				jen.Op("&").Id("data"),
			), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Id("keys").Op(":=").Make(jen.Index().String(), jen.Lit(0), jen.Len(jen.Id("data"))),
			jen.For(jen.Id("k").Op(":=").Range().Id("data")).Block(
				jen.Id("keys").Op("=").Append(jen.Id("keys"), jen.Id("k")),
			),
			jen.Qual("sort", "Strings").Call(jen.Id("keys")),
			jen.Return(jen.Qual("strings", "Join").Call(jen.Id("keys"), jen.Lit("\n")), jen.Nil()),
		)
		f.Line()
		return true

	case "values":
		// Return values of dictionary as newline-separated string
		f.Func().Id(m.goName).Params(
			jen.Id("receiver").String(),
			jen.Id("items").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Var().Id("data").Map(jen.String()).Interface(),
			jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(jen.Id("items")),
				jen.Op("&").Id("data"),
			), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Id("vals").Op(":=").Make(jen.Index().String(), jen.Lit(0), jen.Len(jen.Id("data"))),
			jen.For(jen.List(jen.Id("_"), jen.Id("v")).Op(":=").Range().Id("data")).Block(
				jen.Id("vals").Op("=").Append(jen.Id("vals"), jen.Qual("fmt", "Sprintf").Call(jen.Lit("%v"), jen.Id("v"))),
			),
			jen.Return(jen.Qual("strings", "Join").Call(jen.Id("vals"), jen.Lit("\n")), jen.Nil()),
		)
		f.Line()
		return true

	case "asJson":
		// Return compact JSON representation
		f.Func().Id(m.goName).Params(
			jen.Id("receiver").String(),
			jen.Id("items").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Compact JSON by parsing and re-marshaling"),
			jen.Var().Id("data").Interface(),
			jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(jen.Id("items")),
				jen.Op("&").Id("data"),
			), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.List(jen.Id("compact"), jen.Err()).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("data")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Return(jen.String().Parens(jen.Id("compact")), jen.Nil()),
		)
		f.Line()
		return true

	case "withPairs_":
		// Build dictionary from "key:value key2:value2" pairs
		f.Func().Id(m.goName).Params(
			jen.Id("receiver").String(),
			jen.Id("pairs").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("data").Op(":=").Make(jen.Map(jen.String()).String()),
			jen.For(jen.List(jen.Id("_"), jen.Id("pair")).Op(":=").Range().Qual("strings", "Fields").Call(jen.Id("pairs"))).Block(
				jen.Id("parts").Op(":=").Qual("strings", "SplitN").Call(jen.Id("pair"), jen.Lit(":"), jen.Lit(2)),
				jen.If(jen.Len(jen.Id("parts")).Op("==").Lit(2)).Block(
					jen.Id("data").Index(jen.Id("parts").Index(jen.Lit(0))).Op("=").Id("parts").Index(jen.Lit(1)),
				),
			),
			jen.List(jen.Id("jsonBytes"), jen.Err()).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("data")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Comment("TODO: Set ivar 'items' on receiver to string(jsonBytes)"),
			jen.Id("_").Op("=").Id("jsonBytes"),
			jen.Return(jen.Id("receiver"), jen.Nil()),
		)
		f.Line()
		return true

	case "merge_":
		// Merge another dictionary into this one
		f.Func().Id(m.goName).Params(
			jen.Id("receiver").String(),
			jen.Id("items").String(),
			jen.Id("otherJson").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Var().Id("data").Map(jen.String()).Interface(),
			jen.Var().Id("other").Map(jen.String()).Interface(),
			jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(jen.Id("items")),
				jen.Op("&").Id("data"),
			), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.If(jen.Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
				jen.Index().Byte().Parens(jen.Id("otherJson")),
				jen.Op("&").Id("other"),
			), jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.For(jen.List(jen.Id("k"), jen.Id("v")).Op(":=").Range().Id("other")).Block(
				jen.Id("data").Index(jen.Id("k")).Op("=").Id("v"),
			),
			jen.List(jen.Id("merged"), jen.Err()).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("data")),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Err()),
			),
			jen.Comment("TODO: Set ivar 'items' on receiver to string(merged)"),
			jen.Id("_").Op("=").Id("merged"),
			jen.Return(jen.Id("receiver"), jen.Nil()),
		)
		f.Line()
		return true

	case "do_", "keysDo_", "valuesDo_", "collect_", "select_":
		// Block iteration methods - complex, fall back to bash for now
		return false

	default:
		return false
	}
}

// generatePrimitiveMethodObject generates native Object class methods.
func (g *generator) generatePrimitiveMethodObject(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	case "printString":
		// Return "<ClassName instanceId>"
		f.Func().Id(m.goName).Params(
			jen.Id("receiver").String(),
			jen.Id("className").String(),
			jen.Id("instanceId").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("<%s %s>"), jen.Id("className"), jen.Id("instanceId")), jen.Nil()),
		)
		f.Line()
		return true

	case "class":
		// Return the class name
		f.Func().Id(m.goName).Params(
			jen.Id("receiver").String(),
			jen.Id("className").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("className"), jen.Nil()),
		)
		f.Line()
		return true

	case "id":
		// Return the instance ID
		f.Func().Id(m.goName).Params(
			jen.Id("receiver").String(),
			jen.Id("instanceId").String(),
		).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(jen.Id("instanceId"), jen.Nil()),
		)
		f.Line()
		return true

	case "isKindOf_":
		// Check class hierarchy - requires runtime support
		// Fall back to bash for now
		return false

	case "conformsTo_":
		// Check protocol conformance - requires runtime support
		// Fall back to bash for now
		return false

	case "inspect":
		// Detailed inspection - requires runtime data access
		// Fall back to bash for now
		return false

	case "edit":
		// Editor integration - requires file system and process control
		// Fall back to bash for now
		return false

	default:
		return false
	}
}

// generatePrimitiveMethodProtocol generates native Protocol class methods.
func (g *generator) generatePrimitiveMethodProtocol(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	case "requiredMethods":
		// Requires runtime metadata access - fall back to bash
		return false

	case "isSatisfiedBy_":
		// Requires runtime method introspection - fall back to bash
		return false

	default:
		return false
	}
}

// generatePrimitiveMethodTime generates native Time class methods.
func (g *generator) generatePrimitiveMethodTime(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
	// ==========================================
	// Current Time
	// ==========================================

	case "now":
		// Get current Unix timestamp (seconds since epoch)
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(
				jen.Qual("strconv", "FormatInt").Call(
					jen.Qual("time", "Now").Call().Dot("Unix").Call(),
					jen.Lit(10),
				),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "nowMillis":
		// Get current time in milliseconds
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(
				jen.Qual("strconv", "FormatInt").Call(
					jen.Qual("time", "Now").Call().Dot("UnixMilli").Call(),
					jen.Lit(10),
				),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "nowFormatted_":
		// Get current time formatted with strftime-like pattern
		f.Func().Id(m.goName).Params(jen.Id("format").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Convert strftime format to Go format"),
			jen.Id("goFormat").Op(":=").Qual("strings", "NewReplacer").Call(
				jen.Lit("%Y"), jen.Lit("2006"),
				jen.Lit("%m"), jen.Lit("01"),
				jen.Lit("%d"), jen.Lit("02"),
				jen.Lit("%H"), jen.Lit("15"),
				jen.Lit("%M"), jen.Lit("04"),
				jen.Lit("%S"), jen.Lit("05"),
				jen.Lit("%b"), jen.Lit("Jan"),
				jen.Lit("%B"), jen.Lit("January"),
				jen.Lit("%a"), jen.Lit("Mon"),
				jen.Lit("%A"), jen.Lit("Monday"),
				jen.Lit("%p"), jen.Lit("PM"),
				jen.Lit("%Z"), jen.Lit("MST"),
				jen.Lit("%z"), jen.Lit("-0700"),
				jen.Lit("%%"), jen.Lit("%"),
			).Dot("Replace").Call(jen.Id("format")),
			jen.Return(
				jen.Qual("time", "Now").Call().Dot("Format").Call(jen.Id("goFormat")),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "nowISO":
		// Get ISO 8601 formatted current time
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Return(
				jen.Qual("time", "Now").Call().Dot("UTC").Call().Dot("Format").Call(jen.Qual("time", "RFC3339")),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	// ==========================================
	// Formatting
	// ==========================================

	case "format_as_":
		// Format a Unix timestamp
		f.Func().Id(m.goName).Params(jen.Id("timestamp").String(), jen.Id("format").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("ts"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Id("t").Op(":=").Qual("time", "Unix").Call(jen.Id("ts"), jen.Lit(0)),
			jen.Comment("Convert strftime format to Go format"),
			jen.Id("goFormat").Op(":=").Qual("strings", "NewReplacer").Call(
				jen.Lit("%Y"), jen.Lit("2006"),
				jen.Lit("%m"), jen.Lit("01"),
				jen.Lit("%d"), jen.Lit("02"),
				jen.Lit("%H"), jen.Lit("15"),
				jen.Lit("%M"), jen.Lit("04"),
				jen.Lit("%S"), jen.Lit("05"),
				jen.Lit("%b"), jen.Lit("Jan"),
				jen.Lit("%B"), jen.Lit("January"),
				jen.Lit("%a"), jen.Lit("Mon"),
				jen.Lit("%A"), jen.Lit("Monday"),
				jen.Lit("%p"), jen.Lit("PM"),
				jen.Lit("%Z"), jen.Lit("MST"),
				jen.Lit("%z"), jen.Lit("-0700"),
				jen.Lit("%%"), jen.Lit("%"),
			).Dot("Replace").Call(jen.Id("format")),
			jen.Return(
				jen.Id("t").Dot("Format").Call(jen.Id("goFormat")),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "formatISO_":
		// Format timestamp as ISO 8601
		f.Func().Id(m.goName).Params(jen.Id("timestamp").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("ts"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Id("t").Op(":=").Qual("time", "Unix").Call(jen.Id("ts"), jen.Lit(0)),
			jen.Return(
				jen.Id("t").Dot("UTC").Call().Dot("Format").Call(jen.Qual("time", "RFC3339")),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "formatRelative_":
		// Format timestamp as human-readable relative time
		f.Func().Id(m.goName).Params(jen.Id("timestamp").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("ts"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Id("diff").Op(":=").Qual("time", "Now").Call().Dot("Unix").Call().Op("-").Id("ts"),
			jen.If(jen.Id("diff").Op("<").Lit(60)).Block(
				jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("%d seconds ago"), jen.Id("diff")), jen.Nil()),
			).Else().If(jen.Id("diff").Op("<").Lit(3600)).Block(
				jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("%d minutes ago"), jen.Id("diff").Op("/").Lit(60)), jen.Nil()),
			).Else().If(jen.Id("diff").Op("<").Lit(86400)).Block(
				jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("%d hours ago"), jen.Id("diff").Op("/").Lit(3600)), jen.Nil()),
			).Else().Block(
				jen.Return(jen.Qual("fmt", "Sprintf").Call(jen.Lit("%d days ago"), jen.Id("diff").Op("/").Lit(86400)), jen.Nil()),
			),
		)
		f.Line()
		return true

	// ==========================================
	// Parsing
	// ==========================================

	case "parse_format_":
		// Parse a date string to Unix timestamp
		f.Func().Id(m.goName).Params(jen.Id("dateString").String(), jen.Id("format").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Comment("Convert strftime format to Go format"),
			jen.Id("goFormat").Op(":=").Qual("strings", "NewReplacer").Call(
				jen.Lit("%Y"), jen.Lit("2006"),
				jen.Lit("%m"), jen.Lit("01"),
				jen.Lit("%d"), jen.Lit("02"),
				jen.Lit("%H"), jen.Lit("15"),
				jen.Lit("%M"), jen.Lit("04"),
				jen.Lit("%S"), jen.Lit("05"),
				jen.Lit("%b"), jen.Lit("Jan"),
				jen.Lit("%B"), jen.Lit("January"),
				jen.Lit("%a"), jen.Lit("Mon"),
				jen.Lit("%A"), jen.Lit("Monday"),
				jen.Lit("%p"), jen.Lit("PM"),
				jen.Lit("%Z"), jen.Lit("MST"),
				jen.Lit("%z"), jen.Lit("-0700"),
				jen.Lit("%%"), jen.Lit("%"),
			).Dot("Replace").Call(jen.Id("format")),
			jen.List(jen.Id("t"), jen.Id("err")).Op(":=").Qual("time", "Parse").Call(
				jen.Id("goFormat"),
				jen.Id("dateString"),
			),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Nil()), // Return empty string on parse failure (matches bash behavior)
			),
			jen.Return(
				jen.Qual("strconv", "FormatInt").Call(jen.Id("t").Dot("Unix").Call(), jen.Lit(10)),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	// ==========================================
	// Delays
	// ==========================================

	case "sleep_":
		// Sleep for specified seconds (supports decimals)
		f.Func().Id(m.goName).Params(jen.Id("seconds").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("secs"), jen.Id("err")).Op(":=").Qual("strconv", "ParseFloat").Call(jen.Id("seconds"), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Qual("time", "Sleep").Call(
				jen.Qual("time", "Duration").Call(
					jen.Id("secs").Op("*").Float64().Call(jen.Qual("time", "Second")),
				),
			),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	case "sleepMillis_":
		// Sleep for specified milliseconds
		f.Func().Id(m.goName).Params(jen.Id("millis").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("ms"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("millis"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Qual("time", "Sleep").Call(
				jen.Qual("time", "Duration").Call(jen.Id("ms")).Op("*").Qual("time", "Millisecond"),
			),
			jen.Return(jen.Lit(""), jen.Nil()),
		)
		f.Line()
		return true

	// ==========================================
	// Duration/Arithmetic
	// ==========================================

	case "since_":
		// Calculate duration since a timestamp
		f.Func().Id(m.goName).Params(jen.Id("startTime").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("start"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("startTime"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Id("diff").Op(":=").Qual("time", "Now").Call().Dot("Unix").Call().Op("-").Id("start"),
			jen.Return(
				jen.Qual("strconv", "FormatInt").Call(jen.Id("diff"), jen.Lit(10)),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "from_to_":
		// Calculate duration between two timestamps
		f.Func().Id(m.goName).Params(jen.Id("startTime").String(), jen.Id("endTime").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("start"), jen.Id("err1")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("startTime"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err1").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err1")),
			),
			jen.List(jen.Id("end"), jen.Id("err2")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("endTime"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err2").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err2")),
			),
			jen.Return(
				jen.Qual("strconv", "FormatInt").Call(jen.Id("end").Op("-").Id("start"), jen.Lit(10)),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "add_to_":
		// Add seconds to a timestamp
		f.Func().Id(m.goName).Params(jen.Id("seconds").String(), jen.Id("timestamp").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("secs"), jen.Id("err1")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("seconds"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err1").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err1")),
			),
			jen.List(jen.Id("ts"), jen.Id("err2")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err2").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err2")),
			),
			jen.Return(
				jen.Qual("strconv", "FormatInt").Call(jen.Id("ts").Op("+").Id("secs"), jen.Lit(10)),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "subtract_from_":
		// Subtract seconds from a timestamp
		f.Func().Id(m.goName).Params(jen.Id("seconds").String(), jen.Id("timestamp").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("secs"), jen.Id("err1")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("seconds"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err1").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err1")),
			),
			jen.List(jen.Id("ts"), jen.Id("err2")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err2").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err2")),
			),
			jen.Return(
				jen.Qual("strconv", "FormatInt").Call(jen.Id("ts").Op("-").Id("secs"), jen.Lit(10)),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	// ==========================================
	// Components
	// ==========================================

	case "yearOf_":
		f.Func().Id(m.goName).Params(jen.Id("timestamp").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("ts"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Id("t").Op(":=").Qual("time", "Unix").Call(jen.Id("ts"), jen.Lit(0)),
			jen.Return(
				jen.Qual("strconv", "Itoa").Call(jen.Id("t").Dot("Year").Call()),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "monthOf_":
		f.Func().Id(m.goName).Params(jen.Id("timestamp").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("ts"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Id("t").Op(":=").Qual("time", "Unix").Call(jen.Id("ts"), jen.Lit(0)),
			jen.Return(
				jen.Qual("strconv", "Itoa").Call(jen.Int().Call(jen.Id("t").Dot("Month").Call())),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "dayOf_":
		f.Func().Id(m.goName).Params(jen.Id("timestamp").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("ts"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Id("t").Op(":=").Qual("time", "Unix").Call(jen.Id("ts"), jen.Lit(0)),
			jen.Return(
				jen.Qual("strconv", "Itoa").Call(jen.Id("t").Dot("Day").Call()),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "hourOf_":
		f.Func().Id(m.goName).Params(jen.Id("timestamp").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("ts"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Id("t").Op(":=").Qual("time", "Unix").Call(jen.Id("ts"), jen.Lit(0)),
			jen.Return(
				jen.Qual("strconv", "Itoa").Call(jen.Id("t").Dot("Hour").Call()),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "minuteOf_":
		f.Func().Id(m.goName).Params(jen.Id("timestamp").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("ts"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Id("t").Op(":=").Qual("time", "Unix").Call(jen.Id("ts"), jen.Lit(0)),
			jen.Return(
				jen.Qual("strconv", "Itoa").Call(jen.Id("t").Dot("Minute").Call()),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "secondOf_":
		f.Func().Id(m.goName).Params(jen.Id("timestamp").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("ts"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Id("t").Op(":=").Qual("time", "Unix").Call(jen.Id("ts"), jen.Lit(0)),
			jen.Return(
				jen.Qual("strconv", "Itoa").Call(jen.Id("t").Dot("Second").Call()),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "weekdayOf_":
		// Returns 0=Sunday, 6=Saturday (matches bash date +%w)
		f.Func().Id(m.goName).Params(jen.Id("timestamp").String()).Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.List(jen.Id("ts"), jen.Id("err")).Op(":=").Qual("strconv", "ParseInt").Call(jen.Id("timestamp"), jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Lit(""), jen.Id("err")),
			),
			jen.Id("t").Op(":=").Qual("time", "Unix").Call(jen.Id("ts"), jen.Lit(0)),
			jen.Return(
				jen.Qual("strconv", "Itoa").Call(jen.Int().Call(jen.Id("t").Dot("Weekday").Call())),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	// ==========================================
	// Convenience
	// ==========================================

	case "today":
		// Get timestamp for start of today
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("now").Op(":=").Qual("time", "Now").Call(),
			jen.Id("today").Op(":=").Qual("time", "Date").Call(
				jen.Id("now").Dot("Year").Call(),
				jen.Id("now").Dot("Month").Call(),
				jen.Id("now").Dot("Day").Call(),
				jen.Lit(0), jen.Lit(0), jen.Lit(0), jen.Lit(0),
				jen.Id("now").Dot("Location").Call(),
			),
			jen.Return(
				jen.Qual("strconv", "FormatInt").Call(jen.Id("today").Dot("Unix").Call(), jen.Lit(10)),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "tomorrow":
		// Get timestamp for start of tomorrow
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("now").Op(":=").Qual("time", "Now").Call(),
			jen.Id("tomorrow").Op(":=").Qual("time", "Date").Call(
				jen.Id("now").Dot("Year").Call(),
				jen.Id("now").Dot("Month").Call(),
				jen.Id("now").Dot("Day").Call().Op("+").Lit(1),
				jen.Lit(0), jen.Lit(0), jen.Lit(0), jen.Lit(0),
				jen.Id("now").Dot("Location").Call(),
			),
			jen.Return(
				jen.Qual("strconv", "FormatInt").Call(jen.Id("tomorrow").Dot("Unix").Call(), jen.Lit(10)),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	case "yesterday":
		// Get timestamp for start of yesterday
		f.Func().Id(m.goName).Params().Parens(jen.List(jen.String(), jen.Error())).Block(
			jen.Id("now").Op(":=").Qual("time", "Now").Call(),
			jen.Id("yesterday").Op(":=").Qual("time", "Date").Call(
				jen.Id("now").Dot("Year").Call(),
				jen.Id("now").Dot("Month").Call(),
				jen.Id("now").Dot("Day").Call().Op("-").Lit(1),
				jen.Lit(0), jen.Lit(0), jen.Lit(0), jen.Lit(0),
				jen.Id("now").Dot("Location").Call(),
			),
			jen.Return(
				jen.Qual("strconv", "FormatInt").Call(jen.Id("yesterday").Dot("Unix").Call(), jen.Lit(10)),
				jen.Nil(),
			),
		)
		f.Line()
		return true

	default:
		return false
	}
}
