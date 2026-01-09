// Package ir provides the IR builder that converts AST to IR with name resolution.
package ir

import (
	"strconv"
	"strings"

	"github.com/chazu/procyon/pkg/ast"
	"github.com/chazu/procyon/pkg/parser"
)

// Scope tracks variable bindings for name resolution.
type Scope struct {
	parent   *Scope
	bindings map[string]VarDecl
}

// NewScope creates a new scope with the given parent.
func NewScope(parent *Scope) *Scope {
	return &Scope{
		parent:   parent,
		bindings: make(map[string]VarDecl),
	}
}

// Define adds a variable binding to the scope.
func (s *Scope) Define(name string, decl VarDecl) {
	s.bindings[name] = decl
}

// Resolve looks up a variable in this scope and parent scopes.
func (s *Scope) Resolve(name string) (VarDecl, bool) {
	if decl, ok := s.bindings[name]; ok {
		return decl, true
	}
	if s.parent != nil {
		return s.parent.Resolve(name)
	}
	return VarDecl{}, false
}

// Builder converts AST classes to IR programs.
type Builder struct {
	class    *ast.Class
	scope    *Scope
	errors   []string
	warnings []string
}

// NewBuilder creates a new builder for the given AST class.
func NewBuilder(class *ast.Class) *Builder {
	b := &Builder{
		class:    class,
		errors:   []string{},
		warnings: []string{},
	}

	// Initialize root scope with instance variables
	b.scope = NewScope(nil)
	for _, ivar := range class.InstanceVars {
		decl := VarDecl{
			Name:   ivar.Name,
			Type:   inferTypeFromDefault(ivar.Default),
			IsIVar: true,
		}
		if ivar.Default.Value != "" {
			decl.Default = parseDefaultValue(ivar.Default)
		}
		b.scope.Define(ivar.Name, decl)
	}

	// Add class instance variables to scope
	for _, cvar := range class.ClassInstanceVars {
		decl := VarDecl{
			Name:       cvar.Name,
			Type:       inferTypeFromDefault(cvar.Default),
			IsClassVar: true,
		}
		if cvar.Default.Value != "" {
			decl.Default = parseDefaultValue(cvar.Default)
		}
		b.scope.Define(cvar.Name, decl)
	}

	return b
}

// Build converts the AST class to an IR program.
// Returns the program, warnings, and errors.
func (b *Builder) Build() (*Program, []string, []string) {
	// Create program from class metadata
	program := &Program{
		Package:       b.class.Package,
		Name:          b.class.Name,
		QualifiedName: b.class.QualifiedName(),
		Parent:        b.class.Parent,
		Traits:        b.class.Traits,
	}

	// Handle parent package (if parent is qualified like Pkg::Parent)
	if strings.Contains(b.class.Parent, "::") {
		parts := strings.Split(b.class.Parent, "::")
		program.ParentPackage = parts[0]
		program.Parent = parts[1]
	}

	// Convert instance variables to VarDecl slice
	for _, ivar := range b.class.InstanceVars {
		decl := VarDecl{
			Name:   ivar.Name,
			Type:   inferTypeFromDefault(ivar.Default),
			IsIVar: true,
		}
		if ivar.Default.Value != "" {
			decl.Default = parseDefaultValue(ivar.Default)
		}
		program.InstanceVars = append(program.InstanceVars, decl)
	}

	// Convert class instance variables to VarDecl slice
	for _, cvar := range b.class.ClassInstanceVars {
		decl := VarDecl{
			Name:       cvar.Name,
			Type:       inferTypeFromDefault(cvar.Default),
			IsClassVar: true,
		}
		if cvar.Default.Value != "" {
			decl.Default = parseDefaultValue(cvar.Default)
		}
		program.ClassVars = append(program.ClassVars, decl)
	}

	// Convert each method
	for i := range b.class.Methods {
		method := b.buildMethod(&b.class.Methods[i])
		program.Methods = append(program.Methods, method)
	}

	return program, b.warnings, b.errors
}

// buildMethod converts an AST method to an IR method.
func (b *Builder) buildMethod(m *ast.Method) Method {
	// Create method scope with parent = class scope
	methodScope := NewScope(b.scope)

	// Determine method kind
	kind := InstanceMethod
	if m.Kind == "class" {
		kind = ClassMethod
	}

	// Create method
	method := Method{
		Selector:   m.Selector,
		Kind:       kind,
		IsRaw:      m.Raw,
		CanCompile: !m.Raw, // Raw methods can't be compiled to Go
	}

	// If raw method, mark it for Bash backend
	if m.Raw {
		method.Backend = BackendBash
		method.FallbackReason = "raw method requires Bash"
		// Return early - no need to parse body
		return method
	}

	// Add parameters to scope
	for _, arg := range m.Args {
		decl := VarDecl{
			Name:    arg,
			Type:    TypeAny, // Parameters have unknown type initially
			IsParam: true,
		}
		methodScope.Define(arg, decl)
		method.Args = append(method.Args, decl)
	}

	// Parse the method body
	parseResult := parser.ParseMethod(m.Body.Tokens)
	if parseResult.Unsupported {
		method.CanCompile = false
		method.Backend = BackendBash
		method.FallbackReason = parseResult.Reason
		b.warnings = append(b.warnings, "method "+m.Selector+": "+parseResult.Reason)
		return method
	}

	if parseResult.Body != nil {
		// Add locals to scope
		for _, local := range parseResult.Body.LocalVars {
			decl := VarDecl{
				Name:    local,
				Type:    TypeAny, // Locals have unknown type initially
				IsLocal: true,
			}
			methodScope.Define(local, decl)
			method.Locals = append(method.Locals, decl)
		}

		// Build statements from method body
		stmts, backend, reason := b.buildStatements(parseResult.Body.Statements, methodScope)
		method.Body = stmts
		if backend == BackendBash {
			method.Backend = BackendBash
			method.CanCompile = false
			method.FallbackReason = reason
		} else {
			method.Backend = BackendAny
		}
	}

	return method
}

// buildStatements converts parser statements to IR statements.
func (b *Builder) buildStatements(stmts []parser.Statement, scope *Scope) ([]Statement, Backend, string) {
	var result []Statement
	backend := BackendAny
	var reason string

	for _, stmt := range stmts {
		irStmt, stmtBackend, stmtReason := b.buildStatement(stmt, scope)
		if irStmt != nil {
			result = append(result, irStmt)
		}
		if stmtBackend == BackendBash {
			backend = BackendBash
			if reason == "" {
				reason = stmtReason
			}
		}
	}

	return result, backend, reason
}

// buildStatement converts a parser statement to an IR statement.
func (b *Builder) buildStatement(stmt parser.Statement, scope *Scope) (Statement, Backend, string) {
	switch s := stmt.(type) {
	case *parser.Assignment:
		return b.buildAssignment(s, scope)
	case *parser.Return:
		return b.buildReturn(s, scope)
	case *parser.ExprStmt:
		return b.buildExprStmt(s, scope)
	case *parser.IfExpr:
		return b.buildIfStmt(s, scope)
	case *parser.WhileExpr:
		return b.buildWhileStmt(s, scope)
	case *parser.IterationExpr:
		return b.buildForEachStmt(s, scope)
	case *parser.DynamicIterationExpr:
		// Dynamic iteration requires Bash fallback
		return &BashStmt{
			Code:   "# dynamic iteration",
			Reason: "dynamic block invocation requires Bash",
		}, BackendBash, "dynamic block invocation requires Bash"
	case *parser.MessageSend:
		expr, exprBackend, reason := b.buildExpr(s, scope)
		return &ExprStmt{Expr: expr}, exprBackend, reason
	case *parser.LocalVarDecl:
		// LocalVarDecl is handled at method level, not as a statement
		return nil, BackendAny, ""
	default:
		return nil, BackendAny, ""
	}
}

// buildAssignment converts a parser assignment to an IR assignment.
func (b *Builder) buildAssignment(a *parser.Assignment, scope *Scope) (Statement, Backend, string) {
	value, backend, reason := b.buildExpr(a.Value, scope)

	// Determine assignment kind by resolving target
	kind := AssignLocal
	if decl, found := scope.Resolve(a.Target); found {
		if decl.IsIVar {
			kind = AssignIVar
		} else if decl.IsClassVar {
			kind = AssignClassVar
		}
	}

	return &AssignStmt{
		Target: a.Target,
		Value:  value,
		Kind:   kind,
	}, backend, reason
}

// buildReturn converts a parser return to an IR return.
func (b *Builder) buildReturn(r *parser.Return, scope *Scope) (Statement, Backend, string) {
	if r.Value == nil {
		return &ReturnStmt{Value: nil}, BackendAny, ""
	}
	value, backend, reason := b.buildExpr(r.Value, scope)
	return &ReturnStmt{Value: value}, backend, reason
}

// buildExprStmt wraps an expression as a statement.
func (b *Builder) buildExprStmt(e *parser.ExprStmt, scope *Scope) (Statement, Backend, string) {
	expr, backend, reason := b.buildExpr(e.Expr, scope)
	return &ExprStmt{Expr: expr}, backend, reason
}

// buildIfStmt converts a parser if expression to an IR if statement.
func (b *Builder) buildIfStmt(i *parser.IfExpr, scope *Scope) (Statement, Backend, string) {
	condition, condBackend, condReason := b.buildExpr(i.Condition, scope)

	thenBlock, thenBackend, thenReason := b.buildStatements(i.TrueBlock, scope)
	var elseBlock []Statement
	elseBackend := BackendAny
	elseReason := ""
	if len(i.FalseBlock) > 0 {
		elseBlock, elseBackend, elseReason = b.buildStatements(i.FalseBlock, scope)
	}

	// Determine overall backend requirement
	backend := condBackend
	reason := condReason
	if thenBackend == BackendBash {
		backend = BackendBash
		if reason == "" {
			reason = thenReason
		}
	}
	if elseBackend == BackendBash {
		backend = BackendBash
		if reason == "" {
			reason = elseReason
		}
	}

	return &IfStmt{
		Condition: condition,
		ThenBlock: thenBlock,
		ElseBlock: elseBlock,
	}, backend, reason
}

// buildWhileStmt converts a parser while expression to an IR while statement.
func (b *Builder) buildWhileStmt(w *parser.WhileExpr, scope *Scope) (Statement, Backend, string) {
	condition, condBackend, condReason := b.buildExpr(w.Condition, scope)
	body, bodyBackend, bodyReason := b.buildStatements(w.Body, scope)

	backend := condBackend
	reason := condReason
	if bodyBackend == BackendBash {
		backend = BackendBash
		if reason == "" {
			reason = bodyReason
		}
	}

	return &WhileStmt{
		Condition: condition,
		Body:      body,
	}, backend, reason
}

// buildForEachStmt converts a parser iteration expression to an IR foreach statement.
func (b *Builder) buildForEachStmt(i *parser.IterationExpr, scope *Scope) (Statement, Backend, string) {
	collection, collBackend, collReason := b.buildExpr(i.Collection, scope)

	// Create new scope for loop body with iteration variable
	loopScope := NewScope(scope)
	loopScope.Define(i.IterVar, VarDecl{
		Name:    i.IterVar,
		Type:    TypeAny,
		IsLocal: true,
	})

	body, bodyBackend, bodyReason := b.buildStatements(i.Body, loopScope)

	backend := collBackend
	reason := collReason
	if bodyBackend == BackendBash {
		backend = BackendBash
		if reason == "" {
			reason = bodyReason
		}
	}

	return &ForEachStmt{
		IterVar:    i.IterVar,
		Collection: collection,
		Body:       body,
	}, backend, reason
}

// buildExpr converts a parser expression to an IR expression.
func (b *Builder) buildExpr(expr parser.Expr, scope *Scope) (Expression, Backend, string) {
	switch e := expr.(type) {
	case *parser.NumberLit:
		val, _ := strconv.ParseInt(e.Value, 10, 64)
		return &LiteralExpr{
			Value: val,
			Type_: TypeInt,
		}, BackendAny, ""

	case *parser.StringLit:
		return &LiteralExpr{
			Value: e.Value,
			Type_: TypeString,
		}, BackendAny, ""

	case *parser.Identifier:
		return b.buildIdentifier(e, scope)

	case *parser.QualifiedName:
		return &ClassRefExpr{
			Package: e.Package,
			Name:    e.Name,
		}, BackendAny, ""

	case *parser.BinaryExpr:
		return b.buildBinaryExpr(e, scope)

	case *parser.ComparisonExpr:
		return b.buildComparisonExpr(e, scope)

	case *parser.MessageSend:
		return b.buildMessageSend(e, scope)

	case *parser.BlockExpr:
		return b.buildBlockExpr(e, scope)

	case *parser.JSONPrimitiveExpr:
		return b.buildJSONPrimitive(e, scope)

	case *parser.IterationExprAsValue:
		// For return statements with iteration
		return b.buildIterationAsValue(e.Iteration, scope)

	case *parser.DynamicIterationExprAsValue:
		// Dynamic iteration requires Bash
		return &SubshellExpr{Code: "# dynamic iteration"}, BackendBash, "dynamic block invocation requires Bash"

	case *parser.UnsupportedExpr:
		return &SubshellExpr{Code: "# unsupported: " + e.Reason}, BackendBash, e.Reason

	default:
		return &LiteralExpr{Value: nil, Type_: TypeAny}, BackendAny, ""
	}
}

// buildIdentifier resolves an identifier to a variable reference.
func (b *Builder) buildIdentifier(id *parser.Identifier, scope *Scope) (Expression, Backend, string) {
	// Handle special identifiers
	if id.Name == "self" {
		return &SelfExpr{}, BackendAny, ""
	}
	if id.Name == "true" {
		return &LiteralExpr{Value: true, Type_: TypeBool}, BackendAny, ""
	}
	if id.Name == "false" {
		return &LiteralExpr{Value: false, Type_: TypeBool}, BackendAny, ""
	}
	if id.Name == "nil" {
		return &LiteralExpr{Value: nil, Type_: TypeAny}, BackendAny, ""
	}

	// Check if it's a class reference (starts with uppercase)
	if len(id.Name) > 0 && id.Name[0] >= 'A' && id.Name[0] <= 'Z' {
		return &ClassRefExpr{Name: id.Name}, BackendAny, ""
	}

	// Resolve variable
	if decl, found := scope.Resolve(id.Name); found {
		kind := VarLocal
		if decl.IsParam {
			kind = VarParam
		} else if decl.IsIVar {
			kind = VarIVar
		} else if decl.IsClassVar {
			kind = VarClassVar
		}
		return &VarRefExpr{
			Name:  id.Name,
			Kind:  kind,
			Type_: decl.Type,
		}, BackendAny, ""
	}

	// Unknown variable - might be a global or error
	return &VarRefExpr{
		Name:  id.Name,
		Kind:  VarLocal,
		Type_: TypeAny,
	}, BackendAny, ""
}

// buildBinaryExpr converts a parser binary expression to IR.
func (b *Builder) buildBinaryExpr(e *parser.BinaryExpr, scope *Scope) (Expression, Backend, string) {
	left, leftBackend, leftReason := b.buildExpr(e.Left, scope)
	right, rightBackend, rightReason := b.buildExpr(e.Right, scope)

	backend := leftBackend
	reason := leftReason
	if rightBackend == BackendBash {
		backend = BackendBash
		if reason == "" {
			reason = rightReason
		}
	}

	// Determine result type based on operation
	resultType := TypeInt
	if e.Op == "+" {
		// String concatenation if either side is string
		if left.ResultType() == TypeString || right.ResultType() == TypeString {
			resultType = TypeString
		}
	}

	return &BinaryExpr{
		Left:  left,
		Op:    e.Op,
		Right: right,
		Type_: resultType,
	}, backend, reason
}

// buildComparisonExpr converts a parser comparison expression to IR.
func (b *Builder) buildComparisonExpr(e *parser.ComparisonExpr, scope *Scope) (Expression, Backend, string) {
	left, leftBackend, leftReason := b.buildExpr(e.Left, scope)
	right, rightBackend, rightReason := b.buildExpr(e.Right, scope)

	backend := leftBackend
	reason := leftReason
	if rightBackend == BackendBash {
		backend = BackendBash
		if reason == "" {
			reason = rightReason
		}
	}

	return &BinaryExpr{
		Left:  left,
		Op:    e.Op,
		Right: right,
		Type_: TypeBool,
	}, backend, reason
}

// buildMessageSend converts a parser message send to IR.
func (b *Builder) buildMessageSend(m *parser.MessageSend, scope *Scope) (Expression, Backend, string) {
	var receiver Expression
	var backend Backend = BackendAny
	var reason string

	if m.IsSelf {
		receiver = &SelfExpr{}
	} else {
		receiver, backend, reason = b.buildExpr(m.Receiver, scope)
	}

	var args []Expression
	for _, arg := range m.Args {
		argExpr, argBackend, argReason := b.buildExpr(arg, scope)
		args = append(args, argExpr)
		if argBackend == BackendBash {
			backend = BackendBash
			if reason == "" {
				reason = argReason
			}
		}
	}

	// Determine if this is a class send (receiver is a class reference)
	isClassSend := false
	targetClass := ""
	if classRef, ok := receiver.(*ClassRefExpr); ok {
		isClassSend = true
		targetClass = classRef.FullName()
	}

	return &MessageSendExpr{
		Receiver:    receiver,
		Selector:    m.Selector,
		Args:        args,
		IsSelfSend:  m.IsSelf,
		IsClassSend: isClassSend,
		TargetClass: targetClass,
		Type_:       TypeAny, // Return type unknown at this point
		Backend:     backend,
	}, backend, reason
}

// buildBlockExpr converts a parser block expression to IR.
func (b *Builder) buildBlockExpr(blk *parser.BlockExpr, scope *Scope) (Expression, Backend, string) {
	// Create block scope
	blockScope := NewScope(scope)
	for _, param := range blk.Params {
		blockScope.Define(param, VarDecl{
			Name:    param,
			Type:    TypeAny,
			IsParam: true,
		})
	}

	body, backend, reason := b.buildStatements(blk.Statements, blockScope)

	return &BlockExpr{
		Params: blk.Params,
		Body:   body,
		Type_:  TypeBlock,
	}, backend, reason
}

// buildJSONPrimitive converts a parser JSON primitive to IR.
func (b *Builder) buildJSONPrimitive(j *parser.JSONPrimitiveExpr, scope *Scope) (Expression, Backend, string) {
	receiver, backend, reason := b.buildExpr(j.Receiver, scope)

	var args []Expression
	for _, arg := range j.Args {
		argExpr, argBackend, argReason := b.buildExpr(arg, scope)
		args = append(args, argExpr)
		if argBackend == BackendBash {
			backend = BackendBash
			if reason == "" {
				reason = argReason
			}
		}
	}

	// Determine result type based on operation
	resultType := TypeAny
	switch j.Operation {
	case "arrayLength", "objectLength":
		resultType = TypeInt
	case "arrayIsEmpty", "objectIsEmpty", "objectHasKey":
		resultType = TypeBool
	case "arrayFirst", "arrayLast", "arrayAt", "objectAt":
		resultType = TypeAny
	case "arrayPush", "arrayRemoveAt", "objectRemoveKey", "arrayAtPut", "objectAtPut":
		resultType = TypeJSON
	case "objectKeys", "objectValues":
		resultType = TypeJSON
	}

	return &JSONPrimitiveExpr{
		Receiver:  receiver,
		Operation: j.Operation,
		Args:      args,
		Type_:     resultType,
	}, backend, reason
}

// buildIterationAsValue handles iteration expressions used as values (e.g., in return).
func (b *Builder) buildIterationAsValue(i *parser.IterationExpr, scope *Scope) (Expression, Backend, string) {
	// For collect: and select:, we need to return the result
	// For now, treat as a message send that returns a collection
	collection, backend, reason := b.buildExpr(i.Collection, scope)

	// Create block scope for iteration variable
	blockScope := NewScope(scope)
	blockScope.Define(i.IterVar, VarDecl{
		Name:    i.IterVar,
		Type:    TypeAny,
		IsLocal: true,
	})

	body, bodyBackend, bodyReason := b.buildStatements(i.Body, blockScope)
	if bodyBackend == BackendBash {
		backend = BackendBash
		if reason == "" {
			reason = bodyReason
		}
	}

	// Wrap as a block expression for the iteration
	block := &BlockExpr{
		Params: []string{i.IterVar},
		Body:   body,
		Type_:  TypeBlock,
	}

	// Create message send for the iteration method (collect:, select:, do:)
	selector := i.Kind + "_"
	return &MessageSendExpr{
		Receiver: collection,
		Selector: selector,
		Args:     []Expression{block},
		Type_:    TypeJSON,
		Backend:  backend,
	}, backend, reason
}

// resolve looks up a variable in the current scope chain.
func (b *Builder) resolve(name string) (VarDecl, bool) {
	return b.scope.Resolve(name)
}

// inferTypeFromDefault infers the IR Type from an AST default value.
func inferTypeFromDefault(def ast.DefaultValue) Type {
	switch def.Type {
	case "number":
		return TypeInt
	case "string":
		return TypeString
	case "bool":
		return TypeBool
	case "json", "array", "object":
		return TypeJSON
	default:
		return TypeAny
	}
}

// parseDefaultValue converts an AST default value to an IR Value.
func parseDefaultValue(def ast.DefaultValue) Value {
	v := Value{
		Type: def.Type,
		Raw:  def.Value,
	}

	switch def.Type {
	case "number":
		if val, err := strconv.ParseInt(def.Value, 10, 64); err == nil {
			v.Parsed = val
		} else if val, err := strconv.ParseFloat(def.Value, 64); err == nil {
			v.Parsed = val
		}
	case "string":
		v.Parsed = def.Value
	case "bool":
		v.Parsed = def.Value == "true"
	default:
		v.Parsed = def.Value
	}

	return v
}
