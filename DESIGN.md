# Procyon - Design Document

*Procyon: the genus for raccoons - because what goes better with trash?*

## Overview

This document describes the design for **Procyon**, a tool that generates code from Trashtalk class definitions. Procyon supports two output modes:

1. **Bash backend**: Generates optimized Bash scripts
2. **Shared library mode**: Generates Go code compiled to dylibs, loaded by trashtalk-daemon

## Goals

1. **Flexibility**: Support both interpreted (Bash) and native (Go dylib) execution
2. **Interoperability**: Seamless interaction between Bash and native classes
3. **Native performance**: Shared libraries for performance-critical classes
4. **Incremental adoption**: Compile individual classes without rewriting the system

## Non-Goals (for v1)

1. Compiling raw methods containing arbitrary Bash
2. Full Smalltalk block/closure semantics (partial support via bytecode VM)
3. Replacing the Bash runtime entirely
4. Standalone binary mode (removed in favor of shared library architecture)

## Key Decisions

- **Traits**: Supported in v1 (fall back to Bash for trait methods)
- **Runtime code**: Shared libraries link against libtrashtalk
- **Code generation**: Use [jennifer](https://github.com/dave/jennifer) for Go, custom backend for Bash
- **Testing strategy**: Acceptance tests (AST → expected output), then unit tests
- **Philosophy**: This is an experiment. Keep it simple. Bash remains the primary runtime. Native compilation is an optimization, not a replacement.
- **Fallback behavior**: When a method can't be compiled, warn the user at generation time. Methods fall back to Bash at runtime.

## Architecture

### Pipeline Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         trashtalk repo                          │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────────────┐ │
│  │ .trash file │───▶│ tokenizer   │───▶│ parser.jq           │ │
│  └─────────────┘    │ (bash)      │    │ (outputs AST JSON)  │ │
│                     └─────────────┘    └──────────┬──────────┘ │
└──────────────────────────────────────────────────│─────────────┘
                                                   │
                                                   ▼ AST JSON (stdin)
┌─────────────────────────────────────────────────────────────────┐
│                         procyon CLI                             │
│  ┌──────────┐    ┌──────────────┐    ┌──────────────────────┐  │
│  │ AST      │───▶│ IR Builder   │───▶│ Backend              │  │
│  │ Parser   │    │              │    │ (Bash or Shared)     │  │
│  └──────────┘    └──────────────┘    └──────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Shared Library Runtime

```
┌─────────────────────────────────────────────────────────────────┐
│                      trashtalk-daemon                           │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │  libtrashtalk (C runtime)                                   ││
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐                  ││
│  │  │Counter.so│  │Person.so │  │Widget.so │  ...              ││
│  │  └──────────┘  └──────────┘  └──────────┘                  ││
│  └─────────────────────────────────────────────────────────────┘│
│                              │                                   │
│                              ▼ Unix socket / JSON protocol       │
│                         Bash runtime                             │
└─────────────────────────────────────────────────────────────────┘
```

The daemon:
- Loads shared libraries on demand
- Dispatches method calls via JSON protocol
- Maintains instance state through libtrashtalk
- Supports idle timeout for resource management

## Input: AST Structure

The jq parser produces JSON with this structure:

```json
{
  "type": "class",
  "name": "Counter",
  "parent": "Object",
  "package": "MyApp",
  "instanceVars": [
    {"name": "value", "default": {"type": "number", "value": "0"}},
    {"name": "step", "default": {"type": "number", "value": "1"}}
  ],
  "traits": [],
  "methods": [
    {
      "type": "method",
      "kind": "instance",
      "raw": false,
      "selector": "increment",
      "args": [],
      "body": {
        "type": "block",
        "tokens": [...]
      }
    }
  ]
}
```

### Key Observation: Method Bodies

Method bodies are **token streams**, not parsed expression trees. The bash codegen processes these tokens directly. We parse token streams in Go rather than extending jq.

## Output Modes

### Bash Backend

Generates shell functions:

```bash
Counter__increment() {
    local value step
    value=${_ivar_value:-0}
    step=${_ivar_step:-1}

    value=$((value + step))
    _ivar_value=$value
    echo "$value"
}
```

### Shared Library Mode

Generates Go code compiled to c-shared:

```go
package main

/*
#include <libtrashtalk.h>
*/
import "C"

//export Counter_increment
func Counter_increment(instanceID *C.char) *C.char {
    c := loadInstance(C.GoString(instanceID))
    c.Value += c.Step
    saveInstance(c)
    return C.CString(strconv.Itoa(c.Value))
}

func init() {
    // Register methods with runtime
    C.register_method(C.CString("Counter"), C.CString("increment"), ...)
}
```

## Translation Rules

### Instance Variables

| Trashtalk | Go | Bash |
|-----------|-----|------|
| `instanceVars: value:0` | `type Counter struct { Value int }` | `local value=0` |
| `value` (read) | `c.Value` | `$value` |
| `value := x` | `c.Value = x` | `value=$x` |

### Expressions

| Trashtalk | Go | Bash |
|-----------|-----|------|
| `a + b` | `a + b` | `$((a + b))` |
| `a - b` | `a - b` | `$((a - b))` |
| `^ value` | `return value` | `echo "$value"` |

### Control Flow

| Trashtalk | Go | Bash |
|-----------|-----|------|
| `condition ifTrue: [block]` | `if condition { block }` | `if ...; then block; fi` |
| `[condition] whileTrue: [block]` | `for condition { block }` | `while ...; do block; done` |

### Message Sends

| Trashtalk | Go | Bash |
|-----------|-----|------|
| `@ self increment` | `c.Increment()` | direct call |
| `@ OtherClass method` | JSON dispatch to daemon | `@ OtherClass method` |

## Type Inference

Trashtalk is dynamically typed; Go is statically typed. Strategy:

1. **Instance variables with numeric defaults** → `int`
2. **Instance variables with string defaults** → `string`
3. **Instance variables with no default** → `string` (shell semantics)
4. **Arithmetic expressions** → `int`
5. **String literals** → `string`
6. **Method return types** → inferred from `^` expressions

## Primitive Classes

Built-in implementations for system classes:

| Class | Purpose |
|-------|---------|
| String | String manipulation |
| File | File I/O |
| Shell | Command execution |
| Env | Environment variables |
| Console | Terminal I/O |
| Array | Array operations |
| Dictionary | Key-value operations |
| Block | Closure execution |
| GrpcClient | gRPC dynamic invocation |

These use the `primitiveClass:` declaration and have native Go implementations in the codegen package.

## Limitations and Unsupported Features

### Cannot Compile

1. **Raw methods** (`rawMethod:`) - contain arbitrary Bash
2. **Subshell expressions** (`$(...)`) - need Bash evaluation
3. **Bash-specific syntax** - heredocs, traps, etc.
4. **Dynamic method dispatch to self** - `@ self perform: selector`

### Strategy for Unsupported Code

When the codegen encounters unsupported constructs:

1. **Warn** and skip the method (fall back to Bash at runtime)
2. Or **fail** if `--strict` flag is set

## CLI Interface

```bash
# Bash mode - generate shell script
./driver.bash parse Counter.trash | procyon --mode bash > Counter.bash

# Shared mode - generate Go source for dylib
./driver.bash parse Counter.trash | procyon --mode shared > counter/main.go

# Build shared library
cd counter && go build -buildmode=c-shared -o Counter.so .

# Options
procyon --strict      # fail on unsupported constructs
procyon --dry-run     # show what would be generated
procyon --skip-vet    # skip Go validation (shared mode)
```

## Project Structure

```
procyon/
├── cmd/
│   ├── procyon/           # CLI entry point
│   ├── trashtalk-daemon/  # Shared library loader
│   └── libtrashtalk/      # C runtime library builder
├── lib/
│   └── runtime/           # libtrashtalk C headers
├── pkg/
│   ├── ast/               # Go types for AST JSON
│   ├── parser/            # Token stream → expression parser
│   ├── ir/                # Intermediate representation
│   ├── codegen/           # Code generators
│   │   ├── codegen.go     # Core generation logic
│   │   ├── codegen_grpc.go    # gRPC client generation
│   │   ├── bash_backend.go    # Bash script generator
│   │   ├── primitives.go      # Primitive class registry
│   │   ├── primitives_file.go # File class primitives
│   │   ├── primitives_string.go # String class primitives
│   │   └── primitives_shell.go  # Shell class primitives
│   ├── bytecode/          # Block VM
│   └── runtime/           # Go runtime helpers
├── testdata/              # Acceptance tests
├── go.mod
└── README.md
```

## Testing Strategy

### Acceptance Tests

Each test case is a directory in `testdata/` containing:
- `input.json` - AST from the jq parser
- `expected.go` - Expected generated Go code (shared mode)

### Unit Tests

Per-component tests in each package.

### Integration Tests

End-to-end tests running compiled classes against the daemon.

## Milestones

### M1: Minimal Viable Generator - Complete
- Parse AST JSON
- Generate struct from instanceVars
- Generate simple arithmetic methods
- Acceptance test framework

### M2: Control Flow - Complete
- ifTrue:/ifFalse: → if/else
- whileTrue: → for loops
- Comparison operators
- Early return (^)

### M3: Message Sends & Traits - Complete
- Self message sends
- External message sends
- Trait awareness (fall back to Bash)

### M4: Namespace Support - Complete
- Package declarations
- Qualified class names
- Namespaced shared library naming

### M5: Class Methods & Backends - Complete
- Class method compilation
- Bash backend
- Shared library mode
- trashtalk-daemon

### M6: Polish (Next)
- Better error messages
- --strict mode improvements
- Type inference improvements
- Documentation

## Deferred Decisions

1. **Trait method inlining**: Existing traits use Bash constructs. See docs/trait-inlining.md
2. **Full block semantics**: Partial support via bytecode VM
3. **Error handling style**: Currently uses Go conventions

---

## Appendix: Example Translation

### Input: Counter.trash

```smalltalk
Counter subclass: Object
  instanceVars: value:0 step:1

  method: increment [
    | newVal |
    newVal := value + step
    value := newVal
    ^ newVal
  ]
```

### Output: Shared Mode (excerpt)

```go
//export Counter_increment
func Counter_increment(instanceID *C.char) *C.char {
    c := loadInstance(C.GoString(instanceID))
    var newVal int
    newVal = c.Value + c.Step
    c.Value = newVal
    saveInstance(c)
    return C.CString(strconv.Itoa(newVal))
}
```

### Output: Bash Mode (excerpt)

```bash
Counter__increment() {
    local value step newVal
    value=${_ivar_value:-0}
    step=${_ivar_step:-1}

    newVal=$((value + step))
    value=$newVal
    _ivar_value=$value
    echo "$newVal"
}
```
