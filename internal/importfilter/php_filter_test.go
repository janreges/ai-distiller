package importfilter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The input is what the PHP text formatter prints: top-level use statements
// at column 0, class members (trait uses among them) indented.
func TestPHPFilter(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		removed []string
	}{
		{
			name: "an alias is the name the code uses",
			code: `<file path="src/Models/Model.php">
namespace App\Models;
use Illuminate\Database\ConnectionResolverInterface as Resolver;
use JsonSerializable as JS;
use Unused\Thing as Gone;
abstract class Model implements JS {
    public static setResolver(Resolver $resolver): void
}
</file>`,
			removed: []string{"use Unused\\Thing as Gone;"},
		},
		{
			name: "a name only in implements is used, also when the class uses traits",
			code: `<file path="e.php">
use Illuminate\Support\Collection;
use App\Contracts\HasName;
abstract class User implements HasName {
    use Greets;
    use \Illuminate\Support\Traits\Macroable;
    public __construct(Collection|array $roles = [])
}
</file>`,
		},
		{
			name: "trait uses inside a class body are not imports",
			code: `<file path="t.php">
use App\Traits\Unused;
class Post {
    use SoftDeletes;
}
</file>`,
			removed: []string{"use App\\Traits\\Unused;"},
		},
		{
			name: "an attribute is a usage",
			code: `<file path="a.php">
use Override;
use App\Attributes\Marker;
#[Marker]
class C {
    #[Override]
    public name(): string
}
</file>`,
		},
		{
			name: "function and const imports, case rules",
			code: `<file path="f.php">
use function Illuminate\Support\enum_value;
use function Foo\bar as baz;
use const Foo\LIMIT;
use const Foo\OTHER;
use Foo\Widget;
f(): void { baz(Enum_Value(LIMIT)); new widget(); return other; }
</file>`,
			removed: []string{"use const Foo\\OTHER;"},
		},
		{
			name: "a group stays when one member is used and goes when none is",
			code: `<file path="g.php">
use Foo\{A, B as C};
use Bar\{D, E};
use function Baz\{f1, f2 as g2};
x(C $c): void
y(): void { g2(); }
</file>`,
			removed: []string{"use Bar\\{D, E};"},
		},
		{
			name: "a usage between two namespaces counts",
			code: `<file path="n.php">
namespace A;
use Foo\First;
class One extends First {
}
namespace B;
use Foo\Second;
class Two extends Second {
}
</file>`,
		},
		{
			name: "comments, the file path and the namespace line are not usages",
			code: `<file path="src/Reporter.php">
namespace App\Reporter;
use Foo\Reporter;
use Foo\Noted;
// Noted is mentioned only here
class Report {
}
</file>`,
			removed: []string{"use Foo\\Reporter;", "use Foo\\Noted;"},
		},
		{
			name: "a group written over several lines",
			code: `<?php
use App\Services\{
    PdfGenerator,
    EmailService as Mailer
};
use App\Models\{User, Order};
class SalesReport {
    private Mailer $mailer;
}
`,
			removed: []string{"use App\\Models\\{User, Order};"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered, removed, err := NewPHPFilter().FilterUnusedImports(tt.code, 0)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.removed, removed)
			for _, r := range tt.removed {
				assert.NotContains(t, filtered, r)
			}
		})
	}
}
