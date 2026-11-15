package typescript

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseTS parses src with the real TypeScript processor and returns the raw IR.
func parseTS(t *testing.T, src string) *ir.DistilledFile {
	t.Helper()
	f, err := NewProcessor().Process(context.Background(), strings.NewReader(src), "t.ts")
	require.NoError(t, err)
	return f
}

func TestTS_Namespace(t *testing.T) {
	f := parseTS(t, "export namespace NS {\n  export function f(): void {}\n  function hidden(): void {}\n}\n")
	ns, ok := f.Children[0].(*ir.DistilledPackage)
	require.True(t, ok, "namespace must be a DistilledPackage, got %T", f.Children[0])
	assert.Equal(t, "NS", ns.Name)
	require.Len(t, ns.Children, 2)
	assert.Equal(t, ir.VisibilityPublic, ns.Children[0].(*ir.DistilledFunction).Visibility)
	assert.Equal(t, ir.VisibilityInternal, ns.Children[1].(*ir.DistilledFunction).Visibility)
}

func TestTS_AmbientModule(t *testing.T) {
	f := parseTS(t, "declare module \"conf-x\" {\n  export function g(a: number): void;\n}\n")
	ns := f.Children[0].(*ir.DistilledPackage)
	assert.Equal(t, `"conf-x"`, ns.Name)
	g := ns.Children[0].(*ir.DistilledFunction)
	assert.Equal(t, "g", g.Name)
	assert.Empty(t, g.Implementation)
	assert.Len(t, g.Parameters, 1)
}
