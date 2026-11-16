package php

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/formatter"
	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

func TestProcessorBasicClass(t *testing.T) {
	source := `<?php
namespace App\Models;

use App\Core\BaseModel;

class User extends BaseModel
{
    public const TYPE_USER = 'user';
    
    public string $name;
    protected string $email;
    private string $password;
    
    public function __construct(string $name, string $email)
    {
        $this->name = $name;
        $this->email = $email;
    }
    
    public function getName(): string
    {
        return $this->name;
    }
    
    protected function getEmail(): string
    {
        return $this->email;
    }
    
    private function validatePassword(string $password): bool
    {
        return strlen($password) >= 8;
    }
}`

	proc := NewProcessor()

	ctx := context.Background()
	file, err := proc.Process(ctx, strings.NewReader(source), "test.php")

	require.NoError(t, err)
	require.NotNil(t, file)

	// Check file metadata
	assert.Equal(t, "test.php", file.Path)
	assert.Equal(t, "php", file.Language)

	// Find the User class
	var userClass *ir.DistilledClass
	for _, child := range file.Children {
		if class, ok := child.(*ir.DistilledClass); ok {
			if class.Name == "User" {
				userClass = class
				break
			}
		}
	}

	require.NotNil(t, userClass, "User class not found")
}

func TestProcessorPHP8Features(t *testing.T) {
	source := `<?php
namespace App\Modern;

enum Status: string
{
    case PENDING = 'pending';
    case APPROVED = 'approved';
}

#[Route('/api/users')]
class UserController
{
    public function __construct(
        private readonly string $apiKey,
        public string|int $version = 1
    ) {}
    
    #[Route('/api/users/{id}', methods: ['GET'])]
    public function show(string|int $id): array
    {
        return ['id' => $id];
    }
}`

	proc := NewProcessor()

	ctx := context.Background()
	file, err := proc.Process(ctx, strings.NewReader(source), "modern.php")

	require.NoError(t, err)
	require.NotNil(t, file)

	// Should parse without errors
	assert.Empty(t, file.Errors)

	// Find UserController class
	var controller *ir.DistilledClass
	for _, child := range file.Children {
		if class, ok := child.(*ir.DistilledClass); ok && class.Name == "UserController" {
			controller = class
			break
		}
	}
	require.NotNil(t, controller, "UserController class not found")

	// Check class has attribute
	assert.NotEmpty(t, controller.Decorators, "UserController should have decorators")
	assert.Contains(t, controller.Decorators[0], "Route", "UserController should have Route attribute")

	// Check constructor property promotion
	fieldCount := 0
	var apiKeyField *ir.DistilledField
	for _, child := range controller.Children {
		if field, ok := child.(*ir.DistilledField); ok {
			fieldCount++
			if field.Name == "apiKey" {
				apiKeyField = field
			}
		}
	}
	assert.Equal(t, 2, fieldCount, "Expected 2 promoted properties")
	require.NotNil(t, apiKeyField, "apiKey field not found")
	assert.Equal(t, ir.VisibilityPrivate, apiKeyField.Visibility, "apiKey should be private")

	// Check for readonly modifier
	hasReadonly := false
	for _, mod := range apiKeyField.Modifiers {
		if mod == ir.ModifierReadonly {
			hasReadonly = true
			break
		}
	}
	assert.True(t, hasReadonly, "apiKey should have readonly modifier")

	// Check method with attribute
	var showMethod *ir.DistilledFunction
	for _, child := range controller.Children {
		if fn, ok := child.(*ir.DistilledFunction); ok && fn.Name == "show" {
			showMethod = fn
			break
		}
	}
	require.NotNil(t, showMethod, "show method not found")
	assert.NotEmpty(t, showMethod.Decorators, "show method should have decorators")

	// Check union type parameter
	assert.Len(t, showMethod.Parameters, 1, "show method should have 1 parameter")
	assert.Contains(t, showMethod.Parameters[0].Type.Name, "|", "Parameter should have union type")
}

func TestProcessorNamespaceAndUse(t *testing.T) {
	source := `<?php
namespace App\Core\Services;

use App\Contracts\ServiceInterface;
use App\Traits\{Loggable, Cacheable};
use App\Events\UserCreated as UserCreatedEvent;
use function App\Helpers\sanitize_input;
use const App\Config\DEFAULT_TIMEOUT;

class UserService implements ServiceInterface
{
    use Loggable, Cacheable;
    
    public function createUser(): void
    {
        // Implementation
    }
}`

	proc := NewProcessor()

	ctx := context.Background()
	file, err := proc.Process(ctx, strings.NewReader(source), "namespace.php")

	require.NoError(t, err)
	require.NotNil(t, file)

	var imports []*ir.DistilledImport
	for _, child := range file.Children {
		if imp, ok := child.(*ir.DistilledImport); ok {
			imports = append(imports, imp)
		}
	}

	// One import per statement, as written: the group keeps its prefix as the
	// module and its members as symbols; an alias stays with its name.
	want := []ir.DistilledImport{
		{ImportType: "namespace", Module: `App\Core\Services`},
		{ImportType: "use", Module: `App\Contracts\ServiceInterface`, Symbols: []ir.ImportedSymbol{{Name: `App\Contracts\ServiceInterface`}}},
		{ImportType: "use", Module: `App\Traits\`, Symbols: []ir.ImportedSymbol{{Name: "Loggable"}, {Name: "Cacheable"}}},
		{ImportType: "use", Module: `App\Events\UserCreated`, Symbols: []ir.ImportedSymbol{{Name: `App\Events\UserCreated`, Alias: "UserCreatedEvent"}}},
		{ImportType: "use function", Module: `App\Helpers\sanitize_input`, Symbols: []ir.ImportedSymbol{{Name: `App\Helpers\sanitize_input`}}},
		{ImportType: "use const", Module: `App\Config\DEFAULT_TIMEOUT`, Symbols: []ir.ImportedSymbol{{Name: `App\Config\DEFAULT_TIMEOUT`}}},
	}
	require.Len(t, imports, len(want))
	for i, w := range want {
		assert.Equal(t, w.ImportType, imports[i].ImportType, "import %d", i)
		assert.Equal(t, w.Module, imports[i].Module, "import %d", i)
		if len(w.Symbols) > 0 {
			assert.Equal(t, w.Symbols, imports[i].Symbols, "import %d", i)
		}
	}
}

func TestPHP_UseStatementsPrintAsWritten(t *testing.T) {
	src := `<?php
namespace N;
use Foo\Bar as Baz;
use function Illuminate\Support\enum_value;
use function Foo\bar as baz2;
use const Foo\BAR;
use const Foo\QUX as QUUX;
use Foo\{A, B as C};
use function Foo\{f1, f2 as g2};
use Foo\{Sub\D, function h, const K};
use \Lead\Slash;
use X\Y, Z\W as V;
use function Foo\helper, Foo\second as secondFn;
use const A\B, A\C;
class Holder {}
`
	out := distillPHP(t, src, nil)
	for _, want := range []string{
		"use Foo\\Bar as Baz;\n",
		"use function Illuminate\\Support\\enum_value;\n",
		"use function Foo\\bar as baz2;\n",
		"use const Foo\\BAR;\n",
		"use const Foo\\QUX as QUUX;\n",
		"use Foo\\{A, B as C};\n",
		"use function Foo\\{f1, f2 as g2};\n",
		"use Foo\\{Sub\\D, function h, const K};\n",
		"use \\Lead\\Slash;\n",
		"use X\\Y;\nuse Z\\W as V;\n",
		// The keyword of a statement with several clauses applies to each.
		"use function Foo\\helper;\nuse function Foo\\second as secondFn;\n",
		"use const A\\B;\nuse const A\\C;\n",
	} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "use Foo\\second")
	assert.NotContains(t, out, "use A\\C")
}

func TestProcessorTraits(t *testing.T) {
	source := `<?php
trait Loggable
{
    public function log(string $message): void
    {
        error_log($message);
    }
}

trait Timestampable
{
    public DateTime $createdAt;
    public DateTime $updatedAt;
}

class User
{
    use Loggable, Timestampable;
    
    public string $name;
}`

	proc := NewProcessor()

	ctx := context.Background()
	file, err := proc.Process(ctx, strings.NewReader(source), "traits.php")

	require.NoError(t, err)
	require.NotNil(t, file)

	// Should have parsed traits and class
	assert.GreaterOrEqual(t, len(file.Children), 3, "Expected at least 3 top-level nodes")

	// Check for Timestampable trait
	var timestampableTrait *ir.DistilledClass
	for _, child := range file.Children {
		if class, ok := child.(*ir.DistilledClass); ok && class.Name == "Timestampable" {
			// Verify it's marked as a trait
			if class.Extensions != nil && class.Extensions.PHP != nil && class.Extensions.PHP.IsTrait {
				timestampableTrait = class
				break
			}
		}
	}
	assert.NotNil(t, timestampableTrait, "Timestampable trait not found or not marked as trait")

	// Check for User class with trait use comment
	var userClass *ir.DistilledClass
	for _, child := range file.Children {
		if class, ok := child.(*ir.DistilledClass); ok && class.Name == "User" {
			userClass = class
			break
		}
	}
	require.NotNil(t, userClass, "User class not found")

	// Check that User class uses traits
	assert.NotEmpty(t, userClass.Mixins, "User class should have mixins (traits)")
	assert.Len(t, userClass.Mixins, 2, "User class should use 2 traits")
	assert.Equal(t, "Loggable", userClass.Mixins[0].Name, "First trait should be Loggable")
	assert.Equal(t, "Timestampable", userClass.Mixins[1].Name, "Second trait should be Timestampable")
}

func TestProcessorInterfaces(t *testing.T) {
	source := `<?php
interface Jsonable
{
    public function toJson(): string;
}

interface Arrayable
{
    public function toArray(): array;
}

interface Serializable extends Jsonable, Arrayable
{
    public function serialize(): string;
}

class Model implements Serializable
{
    public function toJson(): string
    {
        return '{}';
    }
    
    public function toArray(): array
    {
        return [];
    }
    
    public function serialize(): string
    {
        return '';
    }
}`

	proc := NewProcessor()

	ctx := context.Background()
	file, err := proc.Process(ctx, strings.NewReader(source), "interfaces.php")

	require.NoError(t, err)
	require.NotNil(t, file)

	// Should have parsed interfaces and class
	assert.GreaterOrEqual(t, len(file.Children), 4, "Expected at least 4 top-level nodes")

	// Check that interfaces are properly parsed as DistilledInterface
	interfaceCount := 0
	var jsonable, arrayable, serializable *ir.DistilledInterface
	for _, child := range file.Children {
		if intf, ok := child.(*ir.DistilledInterface); ok {
			interfaceCount++
			switch intf.Name {
			case "Jsonable":
				jsonable = intf
			case "Arrayable":
				arrayable = intf
			case "Serializable":
				serializable = intf
			}
		}
	}

	assert.Equal(t, 3, interfaceCount, "Expected 3 interfaces")
	assert.NotNil(t, jsonable, "Jsonable interface not found")
	assert.NotNil(t, arrayable, "Arrayable interface not found")
	assert.NotNil(t, serializable, "Serializable interface not found")

	// Check that Serializable extends other interfaces
	assert.Equal(t, 2, len(serializable.Extends), "Serializable should extend 2 interfaces")
}

// distillPHP runs src through the processor and the text formatter with the
// CLI defaults (public members, no bodies, no comments, docstrings), changed
// by set when it is not nil.
func distillPHP(t *testing.T, src string, set func(*processor.ProcessOptions)) string {
	t.Helper()
	opts := processor.DefaultProcessOptions()
	opts.IncludePrivate = false
	opts.IncludeImplementation = false
	opts.IncludeComments = false
	if set != nil {
		set(&opts)
	}
	file, err := NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), "test.php", opts)
	require.NoError(t, err)
	var b strings.Builder
	require.NoError(t, formatter.NewLanguageAwareTextFormatter(formatter.Options{}).Format(&b, file))
	return b.String()
}

func TestPHP_MemberDocblocksPrintByDefault(t *testing.T) {
	src := `<?php
/** CLASS_DOC */
class Widget {
    /** PROP_DOC */
    public int $size = 1;

    /**
     * METHOD_DOC
     * second line
     */
    public function run(): void {}
}
`
	out := distillPHP(t, src, nil)
	for _, want := range []string{"CLASS_DOC", "PROP_DOC", "METHOD_DOC", "second line"} {
		assert.Contains(t, out, want)
	}
	assert.Less(t, strings.Index(out, "CLASS_DOC"), strings.Index(out, "class Widget"), "the class docblock precedes the class")
	assert.Less(t, strings.Index(out, "METHOD_DOC"), strings.Index(out, "run()"), "the method docblock precedes the method")

	off := distillPHP(t, src, func(o *processor.ProcessOptions) { o.IncludeDocstrings = false })
	for _, gone := range []string{"CLASS_DOC", "PROP_DOC", "METHOD_DOC"} {
		assert.NotContains(t, off, gone)
	}
}

func TestPHP_InterfaceAndEnumMemberDocblocksPrintByDefault(t *testing.T) {
	src := `<?php
/** IFACE_DOC */
interface Shape {
    /** IFACE_METHOD_DOC */
    public function area(): float;
}

/** ENUM_DOC */
enum Suit: string {
    /** ENUM_USE_DOC */
    use Labels;

    /** ENUM_CASE_DOC */
    case Hearts = 'H';

    /** ENUM_CONST_DOC */
    const DEFAULT = self::Hearts;

    /** ENUM_METHOD_DOC */
    public function label(): string { return 'x'; }
}
`
	out := distillPHP(t, src, nil)
	for _, want := range []string{"IFACE_DOC", "IFACE_METHOD_DOC", "ENUM_DOC", "ENUM_CASE_DOC", "ENUM_CONST_DOC", "ENUM_METHOD_DOC"} {
		assert.Contains(t, out, want)
	}
	assert.Less(t, strings.Index(out, "IFACE_METHOD_DOC"), strings.Index(out, "area()"))
	assert.Less(t, strings.Index(out, "ENUM_CASE_DOC"), strings.Index(out, "case Hearts"))
	assert.Less(t, strings.Index(out, "ENUM_CONST_DOC"), strings.Index(out, "const DEFAULT"))
	assert.Less(t, strings.Index(out, "ENUM_METHOD_DOC"), strings.Index(out, "label()"))
	// A trait use prints above the members, away from its place in the body.
	assert.NotContains(t, out, "ENUM_USE_DOC", "the doc of a member printed elsewhere must not move to the next one")

	off := distillPHP(t, src, func(o *processor.ProcessOptions) { o.IncludeDocstrings = false })
	assert.NotContains(t, off, "_DOC")
}

// An enum's members are its children in the IR, with or without a constant
// (a constant used to break the parse, a method was never read), and its
// header keeps attributes and implements.
func TestPHP_EnumMembersAreInsideTheEnum(t *testing.T) {
	src := `<?php
enum E: int { case A = 1; public function label(): string {} }

#[Marker]
enum Status: string implements HasName, \JsonSerializable {
    use Greets;
    case Active = 'active';
    const DEFAULT = self::Active;
    public function name(): string { return $this->value; }
    private function hidden(): void {}
}
`
	file, err := NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), "test.php", processor.DefaultProcessOptions())
	require.NoError(t, err)
	require.Len(t, file.Children, 2, "nothing of an enum may land at file level")

	members := func(c *ir.DistilledClass) []string {
		var names []string
		for _, child := range c.Children {
			switch n := child.(type) {
			case *ir.DistilledField:
				names = append(names, "field "+n.Name)
			case *ir.DistilledFunction:
				names = append(names, "function "+n.Name)
			}
		}
		return names
	}

	e := file.Children[0].(*ir.DistilledClass)
	assert.Equal(t, []string{"field A", "function label"}, members(e))

	status := file.Children[1].(*ir.DistilledClass)
	assert.Equal(t, []string{"field Active", "field DEFAULT", "function name", "function hidden"}, members(status))
	assert.Equal(t, []ir.TypeRef{{Name: "HasName"}, {Name: `\JsonSerializable`}}, status.Implements)
	assert.Equal(t, []ir.TypeRef{{Name: "Greets"}}, status.Mixins)
	assert.Equal(t, []string{"Marker"}, status.Decorators)
	assert.Equal(t, "string", status.Extensions.PHP.EnumBackingType)

	out := distillPHP(t, src, nil)
	assert.Contains(t, out, "enum E: int {\n    case A = 1;\n    public label(): string\n}\n")
	assert.Contains(t, out, "#[Marker]\nenum Status: string implements HasName, \\JsonSerializable {\n    use Greets;\n"+
		"    case Active = 'active';\n    public const DEFAULT = self::Active;\n    public name(): string\n}\n")
}

// Every parameter and return type form prints as written: variadics, by
// reference parameters and returns, `never`, DNF types and the rest.
func TestPHP_SignaturesPrintAsWritten(t *testing.T) {
	src := `<?php
class T {
    public (A&B)|null $dnfProp;
    public function a(): void {}
    public function b(): never { exit; }
    public function c(): static { return $this; }
    public function d(): ?self { return null; }
    public function e(): mixed { return 1; }
    public function f(): int|string|null { return 1; }
    public function g(): A&B { return $x; }
    public function h(): (A&B)|null { return null; }
    public function i(): ?\Foo\Bar { return null; }
    public function j(): false { return false; }
    public function &ref(array &$arr, &...$rest): array { return $arr; }
    public static function make(int ...$ids): static { return new static(); }
    public function opt(int &$x = 1, ?callable $cb = null, (A&B)|null $dnf = null, $untyped = 2) {}
    public function __construct(private int &$counter, public readonly (A&B)|C $both) {}
    /**
     * @param  string  ...$guards
     * @param  array<string, int>  &$map
     */
    public function guarded(array &$map, string ...$guards) {}
    /**
     * @param  mixed  ...$tags
     */
    public function tag($abstracts, $tags) {}
}
function &globalRef(): array { static $a = []; return $a; }
function helper(Status ...$s): never { exit; }
`
	out := distillPHP(t, src, func(o *processor.ProcessOptions) { o.IncludePrivate = true })
	for _, want := range []string{
		"    public (A&B)|null $dnfProp;\n",
		"    public a(): void\n",
		"    public b(): never\n",
		"    public c(): static\n",
		"    public d(): ?self\n",
		"    public e(): mixed\n",
		"    public f(): int|string|null\n",
		"    public g(): A&B\n",
		"    public h(): (A&B)|null\n",
		"    public i(): ?\\Foo\\Bar\n",
		"    public j(): false\n",
		"    public &ref(array &$arr, &...$rest): array\n",
		"    public static make(int ...$ids): static\n",
		"    public opt(int &$x = 1, ?callable $cb = null, (A&B)|null $dnf = null, $untyped = 2)\n",
		"    private int $counter;\n",
		"    public readonly (A&B)|C $both;\n",
		"    public __construct(int &$counter, (A&B)|C $both)\n",
		// A docblock type replaces the native one, without the `&` and `...`
		// that PHPDoc writes before the name.
		"    public guarded(array<string, int> &$map, string ...$guards)\n",
		// A docblock `...$x` makes a plain parameter variadic (func_get_args).
		"    public tag($abstracts, mixed ...$tags)\n",
		"&globalRef(): array\n",
		"helper(Status ...$s): never\n",
	} {
		assert.Contains(t, out, want)
	}

	// The IR carries the same facts, for every output format.
	file, err := NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), "test.php", processor.DefaultProcessOptions())
	require.NoError(t, err)
	var ref *ir.DistilledFunction
	for _, child := range file.Children {
		if c, ok := child.(*ir.DistilledClass); ok {
			for _, m := range c.Children {
				if fn, ok := m.(*ir.DistilledFunction); ok && fn.Name == "ref" {
					ref = fn
				}
			}
		}
	}
	require.NotNil(t, ref)
	require.Len(t, ref.Parameters, 2)
	assert.True(t, ref.Parameters[0].IsReference)
	assert.True(t, ref.Parameters[1].IsReference)
	assert.True(t, ref.Parameters[1].IsVariadic)
	require.NotNil(t, ref.Extensions)
	assert.True(t, ref.Extensions.PHP.ReturnsReference)
}

// tree-sitter-php v0.25.1 takes a class named like a cast type (Double,
// Integer, ...) before `::` in an argument list for a cast keyword: the
// attribute lost its argument and a class around such a call vanished. The
// grammar in go.mod must parse these, and print them as written.
func TestPHP_GrammarParsesCastNamedClasses(t *testing.T) {
	src := `<?php
namespace App;

use Types\Integer;

#[Cast(Integer::class)]
final class Dto {
    public function id(): int { return 1; }
}

class RoutingResponseFactoryTest extends TestCase
{
    public function testStreamDownload()
    {
        $factory = new ResponseFactory(Double::for(ViewFactory::class), Double::for(Redirector::class));
        $response = $factory->streamDownload(fn () => null, 'x');
        f(Boolean::from(1), Real::make(), Binary::of(), Object::new(), Int::max(), Float::min(), Bool::yes(), String::of(), Unset::it());
    }
}
`
	parser := sitter.NewParser()
	defer parser.Close()
	require.NoError(t, parser.SetLanguage(sitter.NewLanguage(tree_sitter_php.LanguagePHP())))
	tree := parser.Parse([]byte(src), nil)
	defer tree.Close()
	assert.False(t, tree.RootNode().HasError(), tree.RootNode().ToSexp())

	out := distillPHP(t, src, nil)
	assert.Contains(t, out, "#[Cast(Integer::class)]\nfinal class Dto {\n    public id(): int\n}\n")
	assert.Contains(t, out, "class RoutingResponseFactoryTest extends TestCase {\n    public testStreamDownload()\n}\n")
}

// A constant keeps its visibility, in a class and in an enum, so a private
// one is hidden by default; a typed constant (PHP 8.3+) keeps its type.
func TestPHP_ConstantsKeepVisibilityAndType(t *testing.T) {
	src := `<?php
class C {
    private const SECRET = 1;
    protected const PROT = 2;
    PUBLIC const PUB = 3;
    const PLAIN = 4;
    const int TYPED = 5;
    private const string|int UNION = 'a', OTHER = 'b';
}
enum E: string {
    case A = 'a';
    private const ENUM_SECRET = 'x';
    const D = self::A;
}
`
	out := distillPHP(t, src, nil)
	assert.Contains(t, out, "class C {\n    public const PUB = 3;\n    public const PLAIN = 4;\n    public const int TYPED = 5;\n}\n")
	assert.Contains(t, out, "enum E: string {\n    case A = 'a';\n    public const D = self::A;\n}\n")
	for _, hidden := range []string{"SECRET", "PROT", "UNION", "OTHER"} {
		assert.NotContains(t, out, hidden)
	}

	all := distillPHP(t, src, func(o *processor.ProcessOptions) {
		o.IncludePrivate = true
		o.IncludeProtected = true
	})
	for _, want := range []string{
		"    private const SECRET = 1;\n",
		"    protected const PROT = 2;\n",
		"    private const string|int UNION = 'a';\n",
		"    private const string|int OTHER = 'b';\n",
		"    private const ENUM_SECRET = 'x';\n",
	} {
		assert.Contains(t, all, want)
	}
}
