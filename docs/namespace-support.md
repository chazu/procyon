# Procyon Namespace Support

**Status**: Complete (M4)
**Related**: `~/.trashtalk/docs/namespaces-design.md`

## Overview

This document describes Procyon's namespace support. With namespaces, a class like `Counter` in package `MyApp` has:

- Qualified name: `MyApp::Counter`
- Compiled file: `MyApp__Counter.so` (shared library)
- Instance ID format: `myapp_counter_uuid`

## AST Format

Namespaced classes include package information in the AST:

```json
{
  "type": "class",
  "name": "Counter",
  "package": "MyApp",
  "qualifiedName": "MyApp::Counter",
  "parent": "Object",
  "instanceVars": [...],
  "methods": [...]
}
```

## AST Type Fields

```go
type Class struct {
    Type               string        `json:"type"`
    Name               string        `json:"name"`
    Parent             string        `json:"parent"`
    Package            string        `json:"package"`       // "MyApp" or ""
    Imports            []string      `json:"imports"`       // ["Logging", "Utils"]
    QualifiedName      string        `json:"qualifiedName"` // "MyApp::Counter" or ""
    // ... rest unchanged
}
```

## Helper Functions

```go
// QualifiedNameOf returns the fully qualified name of a class.
// Returns "MyApp::Counter" for namespaced, "Counter" for non-namespaced.
func (c *Class) QualifiedNameOf() string

// CompiledName returns the name for the compiled shared library.
// Returns "MyApp__Counter" for namespaced, "Counter" for non-namespaced.
func (c *Class) CompiledName() string

// IsNamespaced returns true if the class belongs to a package.
func (c *Class) IsNamespaced() bool
```

## Generated Output

### Bash Mode

```bash
# Function naming includes namespace
MyApp__Counter__increment() {
    # ...
}
```

### Shared Library Mode

```go
// Export names include namespace
//export MyApp__Counter_increment
func MyApp__Counter_increment(instanceID *C.char) *C.char {
    // ...
}
```

## Build Integration

```bash
# Generate shared library for namespaced class
./driver.bash parse MyApp/Counter.trash | procyon --mode shared > myapp_counter/main.go

# Build with correct naming
go build -buildmode=c-shared -o MyApp__Counter.so myapp_counter/main.go

# Install to plugins directory
cp MyApp__Counter.so ~/.trashtalk/plugins/
```

## Test Cases

### Namespaced Class Test

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

### Test Checklist

- [x] Non-namespaced class still works (backward compat)
- [x] Namespaced class generates correct export names
- [x] Shared library name uses `__` separator
- [x] Generated code compiles and runs

## Non-Goals

- No changes to method dispatch (already works)
- No changes to instance variable handling
- No multi-package compilation (single file at a time)
- No import resolution (uses qualified refs per design decision)
