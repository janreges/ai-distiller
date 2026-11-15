package conf

import "strings"

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
