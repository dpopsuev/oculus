package ts

import (
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// Language accessors — one function per grammar.
// Backend: smacker/go-tree-sitter (bundled grammars).
// To swap to official tree-sitter/go-tree-sitter: change this file only.
// BLOCKED: can't mix official + smacker in one binary (C linker collision).
// Will swap when Swift + C# get official Go bindings, or when we drop smacker entirely.

func Go() Language         { return golang.GetLanguage() }
func Python() Language     { return python.GetLanguage() }
func TypeScript() Language { return typescript.GetLanguage() }
func Rust() Language       { return rust.GetLanguage() }
func Java() Language       { return java.GetLanguage() }
func C() Language          { return c.GetLanguage() }
func Cpp() Language        { return cpp.GetLanguage() }
func Kotlin() Language     { return kotlin.GetLanguage() }
func Swift() Language      { return swift.GetLanguage() }
func CSharp() Language     { return csharp.GetLanguage() }
