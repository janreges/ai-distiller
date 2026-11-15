//go:build cgo
// +build cgo

package language

import (
	"github.com/janreges/ai-distiller/internal/language/cpp"
	"github.com/janreges/ai-distiller/internal/language/csharp"
	"github.com/janreges/ai-distiller/internal/language/golang"
	"github.com/janreges/ai-distiller/internal/language/java"
	"github.com/janreges/ai-distiller/internal/language/javascript"
	"github.com/janreges/ai-distiller/internal/language/kotlin"
	"github.com/janreges/ai-distiller/internal/language/php"
	"github.com/janreges/ai-distiller/internal/language/python"
	"github.com/janreges/ai-distiller/internal/language/ruby"
	"github.com/janreges/ai-distiller/internal/language/rust"
	"github.com/janreges/ai-distiller/internal/language/swift"
	"github.com/janreges/ai-distiller/internal/language/typescript"
	"github.com/janreges/ai-distiller/internal/processor"
)

// RegisterAll registers all built-in language processors (CGO version)
func RegisterAll() error {
	// Register Python processor
	pythonProc := python.NewProcessor()
	if err := processor.Register(pythonProc); err != nil {
		return err
	}

	// Register PHP processor
	phpProc := php.NewProcessor()
	if err := processor.Register(phpProc); err != nil {
		return err
	}

	// Register JavaScript processor
	jsProc := javascript.NewProcessor()
	if err := processor.Register(jsProc); err != nil {
		return err
	}

	// Register TypeScript processor
	tsProc := typescript.NewProcessor()
	if err := processor.Register(tsProc); err != nil {
		return err
	}

	// Register Go processor
	goProc := golang.NewProcessor()
	if err := processor.Register(goProc); err != nil {
		return err
	}

	// Register Rust processor
	rustProc := rust.NewProcessor()
	if err := processor.Register(rustProc); err != nil {
		return err
	}

	// Register Swift processor
	swiftProc := swift.NewProcessor()
	if err := processor.Register(swiftProc); err != nil {
		return err
	}

	// Register Ruby processor
	rubyProc := ruby.NewProcessor()
	if err := processor.Register(rubyProc); err != nil {
		return err
	}

	// Register Java processor
	javaProc := java.NewProcessor()
	if err := processor.Register(javaProc); err != nil {
		return err
	}

	// Register C# processor
	csharpProc := csharp.NewProcessor()
	if err := processor.Register(csharpProc); err != nil {
		return err
	}

	// Register Kotlin processor
	kotlinProc := kotlin.NewProcessor()
	if err := processor.Register(kotlinProc); err != nil {
		return err
	}

	// Register C++ processor
	cppProc := cpp.NewProcessor()
	if err := processor.Register(cppProc); err != nil {
		return err
	}

	return nil
}
