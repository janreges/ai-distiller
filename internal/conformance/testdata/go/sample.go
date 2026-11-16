package conf

import (
	"io"
	"strings"
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
	return strings.Repeat("-", w.ConfField)
}

func (w *ConfWidget) ConfOther() string {
	b := "CONF_BODY_B"
	return b
}

func (w *ConfWidget) ConfBranch(n int) string {
	if n > w.ConfField {
		return "CONF_BODY_D"
	}
	return ""
}

// ConfReadCloser embeds two interfaces and declares a method.
type ConfReadCloser interface {
	io.Reader
	io.Closer
	ConfExtra() int
}

// ConfNumber is a type-set constraint.
type ConfNumber interface {
	~int | ~int64
}

// ConfID is a defined type, a new type.
type ConfID int

// ConfName is an alias, another name for string.
type ConfName = string
