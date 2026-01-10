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
		"read":        true,
		"write_":      true,
		"append_":     true,
		"delete":      true,
		"exists":      true,
		"isFile":      true,
		"isDirectory": true,
		"size":        true,
		"path":        true,
		"directory":   true,
		"basename":    true,
		"extension":   true,
		// Class methods
		"exists_":      true,
		"isFile_":      true,
		"isDirectory_": true,
		"read_":        true,
		"write_to_":    true,
		"delete_":      true,
	},
	"Env": {
		"get_":    true,
		"set_to_": true,
		"unset_":  true,
		"has_":    true,
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
	default:
		return false
	}
}

// generatePrimitiveMethodFile generates native File class methods.
func (g *generator) generatePrimitiveMethodFile(f *jen.File, m *compiledMethod) bool {
	switch m.selector {
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
