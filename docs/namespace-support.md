# Procyon Namespace Support

**Status**: ✅ Complete (M4)
**Related**: `~/.trashtalk/docs/namespaces-design.md`

## Overview

This document describes the changes needed for Procyon to support Trashtalk's namespace system. With namespaces, a class like `Counter` in package `MyApp` has:

- Qualified name: `MyApp::Counter`
- Compiled file: `MyApp__Counter.native`
- Instance ID format: `myapp_counter_uuid`

## Current State

Procyon currently assumes non-namespaced classes:

```go
// pkg/ast/types.go
type Class struct {
    Name   string  // "Counter"
    Parent string  // "Object"
    // ... no package field
}
```

```go
// Generated binary naming
// Counter.native
```

## Required Changes

### 1. AST Type Updates (`pkg/ast/types.go`)

Add namespace fields to the `Class` struct:

```go
type Class struct {
    Type               string        `json:"type"`
    Name               string        `json:"name"`
    Parent             string        `json:"parent"`
    Package            string        `json:"package"`       // NEW: "MyApp" or ""
    Imports            []string      `json:"imports"`       // NEW: ["Logging", "Utils"]
    QualifiedName      string        `json:"qualifiedName"` // NEW: "MyApp::Counter" or ""
    // ... rest unchanged
}
```

### 2. Helper Functions (`pkg/ast/helpers.go` - new file)

```go
package ast

import "strings"

// QualifiedNameOf returns the fully qualified name of a class.
// Returns "MyApp::Counter" for namespaced, "Counter" for non-namespaced.
func (c *Class) QualifiedNameOf() string {
    if c.Package != "" {
        return c.Package + "::" + c.Name
    }
    return c.Name
}

// CompiledName returns the name for the compiled binary.
// Returns "MyApp__Counter" for namespaced, "Counter" for non-namespaced.
func (c *Class) CompiledName() string {
    if c.Package != "" {
        return c.Package + "__" + c.Name
    }
    return c.Name
}

// IsNamespaced returns true if the class belongs to a package.
func (c *Class) IsNamespaced() bool {
    return c.Package != ""
}
```

### 3. Code Generator Updates (`pkg/codegen/codegen.go`)

#### 3.1 Embed Directive

```go
// Current:
f.Comment("//go:embed " + g.class.Name + ".trash")

// Change to:
f.Comment("//go:embed " + g.class.CompiledName() + ".trash")
```

#### 3.2 Binary Usage String

```go
// Current:
jen.Lit("Usage: "+className+".native <instance_id> <selector> [args...]")

// Change to:
compiledName := g.class.CompiledName()
jen.Lit("Usage: "+compiledName+".native <instance_id> <selector> [args...]")
```

#### 3.3 --info Output

```go
// Current:
jen.Qual("fmt", "Printf").Call(jen.Lit("Class: "+className+"\nHash: %s\n..."))

// Change to:
qualName := g.class.QualifiedNameOf()
jen.Qual("fmt", "Printf").Call(jen.Lit("Class: "+qualName+"\nPackage: "+g.class.Package+"\nHash: %s\n..."))
```

#### 3.4 Struct Name (optional)

The Go struct name can remain as the simple class name since it's internal:

```go
// This is fine:
type Counter struct { ... }

// No need for:
type MyApp__Counter struct { ... }  // Unnecessary complexity
```

### 4. Instance Class Field Handling

When loading instances from SQLite, the `class` field may now contain qualified names:

```json
{"class": "MyApp::Counter", "created_at": "...", "value": 0}
```

The `loadInstance` function should handle this:

```go
func (g *generator) generateLoadInstance(f *jen.File) {
    // Instance loading doesn't need to change - we match on instance ID
    // The class field in JSON is just metadata
}
```

### 5. Trashtalk Runtime Integration

The Bash runtime already handles namespaced native binaries correctly:

```bash
# lib/trash.bash (already implemented):
local compiled_name=$(_to_compiled_name "$class_name")  # MyApp::Counter → MyApp__Counter
local native_binary="$TRASHDIR/.compiled/${compiled_name}.native"
```

**Status**: ✓ Already implemented as part of Milestone 3.

### 6. Build Integration (Makefile)

Update the Trashtalk Makefile to generate namespaced binaries:

```makefile
# Current:
$(PROCYON) < ast.json > $(CLASS)/main.go
go build -o ~/.trashtalk/trash/.compiled/$(CLASS).native

# Change to:
COMPILED_NAME = $(subst ::,__,$(CLASS))
$(PROCYON) < ast.json > $(COMPILED_NAME)/main.go
go build -o ~/.trashtalk/trash/.compiled/$(COMPILED_NAME).native
```

## Test Cases

### 7. New Test Case: Namespaced Class

Create `testdata/myapp_counter/`:

**input.json**:
```json
{
  "type": "class",
  "name": "Counter",
  "package": "MyApp",
  "qualifiedName": "MyApp::Counter",
  "parent": "Object",
  "instanceVars": [{"name": "value", "default": {"type": "number", "value": "0"}}],
  "methods": [...]
}
```

**expected.go**:
```go
//go:embed MyApp__Counter.trash
var _sourceCode string

// ... rest with correct naming
```

### 8. Test Checklist

- [ ] Non-namespaced class still works (backward compat)
- [ ] Namespaced class generates correct embed directive
- [ ] --info shows package and qualified name
- [ ] Binary name uses `__` separator
- [ ] Generated code compiles and runs

## Implementation Order

1. **AST changes** - Add fields, add helper methods
2. **Codegen updates** - Use helpers for naming
3. **Add test case** - Namespaced class test
4. **Verify backward compat** - Existing tests still pass
5. **Update CLAUDE.md** - Document namespace support

## Estimated Effort

| Task | Effort |
|------|--------|
| AST type updates | Small |
| Helper functions | Small |
| Codegen changes | Medium |
| Test case | Small |
| Integration testing | Medium |

**Total**: ~2-4 hours of focused work

## Non-Goals for This Milestone

- No changes to method dispatch (already works)
- No changes to instance variable handling
- No multi-package compilation (single file at a time)
- No import resolution (uses qualified refs per design decision)
