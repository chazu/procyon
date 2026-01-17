Areas for Improvement
1. Test Coverage Gaps
Package	Coverage
pkg/ast	0.0%
pkg/codegen	24.2%
pkg/ir	29.9%
pkg/parser	58.1%
pkg/runtime	74.2%
pkg/bytecode	83.6%
pkg/lexer	97.0%

Recommendation: Priority should be given to pkg/ast (needs basic tests) and pkg/codegen (critical path, low coverage).
2. TODOs Left in Code

There are 6 TODOs in the codebase that represent incomplete implementations:

    lib/runtime/runtime.go:155 - "TODO: proper bytecode chunk handling"
    lib/runtime/persistence.go:198 - "TODO: proper block handling"
    pkg/codegen/primitives.go:833,933,968 - Incomplete Array primitive methods
    pkg/bytecode/vm.go:1055 - "TODO: proper replacement"

Recommendation: Track these as issues or complete them.
3. Code Duplication in Parser

In pkg/parser/parser.go, there's significant duplication in the iteration parsing methods:

    parseDoIteration (lines 558-592)
    parseCollectIteration (lines 594-629)
    parseSelectIteration (lines 631-666)

All three follow the same pattern with only the keyword name differing.

Recommendation: Extract a common parseIteration(kind string) method:

func (p *Parser) parseIteration(kind string) (Statement, error) {
    p.advance() // consume "do:", "collect:", or "select:"
    // ... common logic with kind parameter
}

4. Magic Strings

Token types are defined as constants in pkg/ast/types.go:163-206, but the parser also uses raw strings like "STRING" (parser.go:1140,1355) which bypasses the type safety.

Location: pkg/parser/parser.go:1140-1147, pkg/parser/parser.go:1355-1362
5. Large Functions

pkg/codegen/codegen.go is very large (36K+ tokens, couldn't read in one go). Consider breaking it up:

    Expression generation could be a separate file
    Statement generation could be a separate file
    The generator struct methods are spread across multiple files already, which is good

6. Unused Helper Functions

In pkg/codegen/codegen.go:124-170, several JSON-related helper methods always return false:

func (g *generator) isJSONArrayType(name string) bool {
    return false
}
func (g *generator) isJSONObjectType(name string) bool {
    return false
}

The comments say "Always returns false since we use string type" - if this is the intended behavior, these should either be removed or the calling code simplified.
7. Error Messages Could Be More Helpful

Parser errors in pkg/parser/parser.go often return generic messages like:

return nil, fmt.Errorf("expected ) after parenthesized expression, got %s", p.peek().Type)

Recommendation: Include line/column information from the token for better debugging:

return nil, fmt.Errorf("line %d, col %d: expected ) after parenthesized expression, got %s",
    tok.Line, tok.Col, tok.Type)

8. Go Version

The go.mod specifies go 1.24.5, which doesn't exist yet (current stable is 1.23.x as of my knowledge cutoff). This might be intentional for a future release but could cause build issues.
Security Considerations

The codebase is for a compiler/code generator, not a web service, so the attack surface is limited. However:

    Shell Command Construction (codegen.go:240-243): The invokeBlock function constructs shell commands. Arguments are passed through %q formatting which provides proper quoting, which is good.

    SQLite in Runtime: The runtime uses SQLite for instance storage. The persistence code should be reviewed for SQL injection if user data flows into queries.

Recommendations Summary

High Priority:

    Add tests for pkg/ast package
    Improve pkg/codegen test coverage (24% → 50%+)
    Complete or track the 6 TODOs

Medium Priority:
4. Refactor duplicate iteration parsing methods
5. Fix the go 1.24.5 version in go.mod
6. Remove or fix the always-false JSON helper methods

Low Priority:
7. Add line/column info to parser error messages
8. Replace magic strings with constants in parser
