package conf

import (
	"io" // CONF_IMPORT_TRAILING
	// CONF_IMPORT_DOC
	"strings" // CONF_DROPPED_IMPORT_NOTE
)

// CONF_LINE_COMMENT

// CONF_DOC_COMMENT
type ConfWidget struct {
	ConfField int
}

func (w *ConfWidget) ConfPublic() string {
	a := "CONF_BODY_A"
	return a
}

// CONF_PRIVATE_DOC
func (w *ConfWidget) confPrivate() string {
	// CONF_HIDDEN_BODY_NOTE
	return strings.Repeat("-", w.ConfField)
}

func (w *ConfWidget) ConfOther() string {
	b := "CONF_BODY_B"
	return b
}

func (w *ConfWidget) ConfBranch(n int) string {
	// CONF_BODY_NOTE
	if n > w.ConfField {
		return "CONF_BODY_D"
	}
	return ""
}

// ConfReadCloser embeds two interfaces and declares a method.
type ConfReadCloser interface {
	io.Reader
	io.Closer
	// CONF_METHOD_DOC
	ConfExtra() int // CONF_METHOD_TRAILING
}

// ConfNumber is a type-set constraint.
type ConfNumber interface {
	~int | ~int64
}

// ConfID is a defined type, a new type.
type ConfID int

// ConfName is an alias, another name for string.
type ConfName = string

// ConfHandler is a defined function type.
type ConfHandler func(w io.Writer, n int) error

// ConfRouter holds a middleware stack.
type ConfRouter struct {
	ConfStack []func(io.Writer) io.Writer
}

// ConfMap takes function-typed parameters.
func ConfMap(items []int, f func(int) (string, error), opts ...func(*ConfWidget)) []string {
	return nil
}

// ConfRecord has commented fields.
type ConfRecord struct {
	// CONF_FIELD_DOC
	ConfNote   string // CONF_FIELD_TRAILING
	confSecret int    // CONF_HIDDEN_FIELD_NOTE
}

// ConfPair is an alias of a struct literal.
type ConfPair = struct {
	ConfLeft int
}

const (
	ConfLimit     = 10 // CONF_CONST_TRAILING
	confHiddenMax = 20 // CONF_HIDDEN_CONST_NOTE
)

type ConfKind int // CONF_TYPE_TRAILING
