# Trait Method Inlining

**Status**: Deferred (trait methods fall back to Bash)
**Priority**: Low - existing traits use Bash-specific constructs

## Current Behavior

When a class includes traits (e.g., `include: Persistable`), procyon:
1. Notes the traits in warnings
2. Falls back to Bash for trait method dispatch

This works because the Bash runtime handles trait method lookup correctly.

## Why Full Inlining is Deferred

Looking at existing Trashtalk traits:

### Debuggable trait
- Uses `$_RECEIVER`, `$_SUPERCLASS` - Bash context variables
- Calls `get_fcn_list`, `receiver_path` - Bash runtime functions
- Cannot be compiled to Go

### Persistable trait
- All methods are `rawMethod:` or `rawClassMethod:`
- Uses `_env_persist`, `db_delete`, etc. - Bash runtime functions
- Cannot be compiled to Go

Since existing traits can't be compiled anyway, full inlining provides no benefit for v1.

## Future Implementation Plan

If we want full trait inlining (for traits with compilable methods):

### Option A: Modify driver.bash (Recommended)

Add a new command that resolves traits before outputting AST:

```bash
# driver.bash ast-resolved Counter.trash
# Outputs AST with trait methods inlined
```

Implementation:
1. Parse the class AST
2. For each trait in `.traits`:
   - Load the trait file from `$TRASHDIR/traits/{trait}.trash`
   - Parse the trait's AST
3. Merge trait methods into the class AST
4. Output the combined AST

Procyon receives a self-contained AST with all methods included.

### Option B: Procyon loads traits

Procyon receives trait names and loads them directly:

```go
func (g *generator) loadTraits() {
    trashtalkRoot := os.Getenv("TRASHDIR")
    for _, trait := range g.class.Traits {
        traitPath := filepath.Join(trashtalkRoot, "traits", trait+".trash")
        // Parse trait and merge methods
    }
}
```

Drawback: Breaks the clean stdin-based interface.

### Implementation Steps (Option A)

1. Add `resolve_traits()` function to driver.bash
2. Add `ast-resolved` command
3. For each trait:
   - Find trait file (check `traits/` subdirectory)
   - Parse to AST
   - Filter compilable methods (skip raw methods)
   - Add methods to class AST with trait origin marker
4. Update procyon to handle methods with trait origin
5. Add test case with compilable trait methods

### AST Format with Inlined Traits

```json
{
  "type": "class",
  "name": "Counter",
  "traits": ["Loggable"],
  "methods": [
    {
      "selector": "increment",
      "kind": "instance",
      "origin": "Counter"
    },
    {
      "selector": "log_",
      "kind": "instance",
      "origin": "Loggable",
      "traitMethod": true
    }
  ]
}
```

## When to Implement

Implement full trait inlining when:
1. Users create traits with pure Smalltalk syntax (no Bash)
2. Performance benefit is measurable
3. Demand exists for native trait compilation

Until then, the fallback to Bash works correctly.
