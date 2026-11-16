package formatter

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// XMLFormatter formats IR as XML
type XMLFormatter struct {
	BaseFormatter
}

// NewXMLFormatter creates a new XML formatter
func NewXMLFormatter(options Options) *XMLFormatter {
	return &XMLFormatter{
		BaseFormatter: NewBaseFormatter(options),
	}
}

// Extension returns the file extension for XML
func (f *XMLFormatter) Extension() string {
	return ".xml"
}

// Format writes a single file as XML
func (f *XMLFormatter) Format(w io.Writer, file *ir.DistilledFile) error {
	// Write XML declaration
	fmt.Fprintln(w, `<?xml version="1.0" encoding="UTF-8"?>`)

	// Start root element
	fmt.Fprintln(w, `<distilled>`)

	// Format the file
	if err := f.formatFile(w, file, 1); err != nil {
		return err
	}

	// Close root element
	fmt.Fprintln(w, `</distilled>`)

	return nil
}

// FormatMultiple writes multiple files as XML
func (f *XMLFormatter) FormatMultiple(w io.Writer, files []*ir.DistilledFile) error {
	// Write XML declaration
	fmt.Fprintln(w, `<?xml version="1.0" encoding="UTF-8"?>`)

	// Start root element
	fmt.Fprintln(w, `<distilled>`)

	// Format each file
	for _, file := range files {
		if err := f.formatFile(w, file, 1); err != nil {
			return err
		}
	}

	// Close root element
	fmt.Fprintln(w, `</distilled>`)

	return nil
}

// formatFile formats a file element
func (f *XMLFormatter) formatFile(w io.Writer, file *ir.DistilledFile, depth int) error {
	indent := strings.Repeat("  ", depth)

	// Start file element
	fmt.Fprintf(w, "%s<file", indent)
	f.writeAttr(w, "path", file.Path)
	f.writeAttr(w, "language", file.Language)
	f.writeAttr(w, "version", file.Version)

	if f.options.IncludeLocation {
		loc := file.GetLocation()
		f.writeLocationAttrs(w, &loc)
	}

	fmt.Fprintln(w, ">")

	// Write metadata if included
	if f.options.IncludeMetadata && file.Metadata != nil {
		f.formatMetadata(w, file.Metadata, depth+1)
	}

	// Write errors
	if len(file.Errors) > 0 {
		fmt.Fprintf(w, "%s  <errors>\n", indent)
		for _, err := range file.Errors {
			f.formatError(w, &err, depth+2)
		}
		fmt.Fprintf(w, "%s  </errors>\n", indent)
	}

	// Write nodes
	if len(file.Children) > 0 {
		fmt.Fprintf(w, "%s  <nodes>\n", indent)
		for _, node := range file.Children {
			f.formatNode(w, node, depth+2)
		}
		fmt.Fprintf(w, "%s  </nodes>\n", indent)
	}

	// Close file element
	fmt.Fprintf(w, "%s</file>\n", indent)

	return nil
}

// formatMetadata formats file metadata
func (f *XMLFormatter) formatMetadata(w io.Writer, meta *ir.FileMetadata, depth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(w, "%s<metadata", indent)
	f.writeAttr(w, "size", fmt.Sprintf("%d", meta.Size))
	if meta.Hash != "" {
		f.writeAttr(w, "hash", meta.Hash)
	}
	if meta.Encoding != "" {
		f.writeAttr(w, "encoding", meta.Encoding)
	}
	if meta.LastModified.Unix() > 0 {
		f.writeAttr(w, "modified", meta.LastModified.Format("2006-01-02T15:04:05Z"))
	}
	fmt.Fprintln(w, "/>")
}

// formatNode formats a node element
func (f *XMLFormatter) formatNode(w io.Writer, node ir.DistilledNode, depth int) {
	indent := strings.Repeat("  ", depth)

	switch n := node.(type) {
	case *ir.DistilledPackage:
		fmt.Fprintf(w, "%s<package", indent)
		f.writeAttr(w, "name", n.Name)
		f.writeNodeAttrs(w, n)
		f.formatNodeChildren(w, ">", "</package>", n.Children, depth)

	case *ir.DistilledImport:
		fmt.Fprintf(w, "%s<import", indent)
		f.writeAttr(w, "type", n.ImportType)
		f.writeAttr(w, "module", n.Module)
		if n.IsType {
			f.writeAttr(w, "is_type", "true")
		}
		if n.IsExport {
			f.writeAttr(w, "is_export", "true")
		}
		f.writeNodeAttrs(w, n)
		f.writeSymbols(w, indent, "</import>", n.Symbols)

	case *ir.DistilledExport:
		fmt.Fprintf(w, "%s<export", indent)
		f.writeAttr(w, "module", n.Module)
		f.writeAttr(w, "value", n.Value)
		if n.IsDefault {
			f.writeAttr(w, "is_default", "true")
		}
		if n.IsType {
			f.writeAttr(w, "is_type", "true")
		}
		f.writeAttr(w, "namespace", n.Namespace)
		f.writeNodeAttrs(w, n)
		f.writeSymbols(w, indent, "</export>", n.Symbols)

	case *ir.DistilledClass:
		fmt.Fprintf(w, "%s<class", indent)
		f.writeAttr(w, "name", n.Name)
		f.writeAttr(w, "visibility", string(n.Visibility))
		f.writeModifiers(w, n.Modifiers)
		f.writeNodeAttrs(w, n)
		f.formatNodeChildren(w, ">", "</class>", n.Children, depth)

	case *ir.DistilledInterface:
		fmt.Fprintf(w, "%s<interface", indent)
		f.writeAttr(w, "name", n.Name)
		f.writeAttr(w, "visibility", string(n.Visibility))
		f.writeModifiers(w, n.Modifiers)
		f.writeNodeAttrs(w, n)
		f.formatNodeChildren(w, ">", "</interface>", n.Children, depth)

	case *ir.DistilledFunction:
		fmt.Fprintf(w, "%s<function", indent)
		f.writeAttr(w, "name", n.Name)
		f.writeAttr(w, "visibility", string(n.Visibility))
		f.writeModifiers(w, n.Modifiers)
		if n.Returns != nil && n.Returns.Name != "" {
			f.writeAttr(w, "returns", n.Returns.Name)
		}
		f.writeNodeAttrs(w, n)

		hasContent := len(n.Parameters) > 0 || (n.Implementation != "" && !f.options.Compact)
		if hasContent {
			fmt.Fprintln(w, ">")

			// Write parameters
			if len(n.Parameters) > 0 {
				fmt.Fprintf(w, "%s  <parameters>\n", indent)
				for _, p := range n.Parameters {
					fmt.Fprintf(w, "%s    <parameter", indent)
					f.writeAttr(w, "name", p.Name)
					if p.Type.Name != "" {
						f.writeAttr(w, "type", p.Type.Name)
					}
					if p.DefaultValue != "" {
						f.writeAttr(w, "default", p.DefaultValue)
					}
					if p.IsOptional {
						f.writeAttr(w, "optional", "true")
					}
					if p.IsVariadic {
						f.writeAttr(w, "variadic", "true")
					}
					if p.IsReference {
						f.writeAttr(w, "reference", "true")
					}
					fmt.Fprintln(w, "/>")
				}
				fmt.Fprintf(w, "%s  </parameters>\n", indent)
			}

			// Write implementation
			if n.Implementation != "" && !f.options.Compact {
				fmt.Fprintf(w, "%s  <implementation>%s</implementation>\n",
					indent, cdata("\n"+n.Implementation+"\n"+indent+"  "))
			}

			fmt.Fprintf(w, "%s</function>\n", indent)
		} else {
			fmt.Fprintln(w, "/>")
		}

	case *ir.DistilledField:
		fmt.Fprintf(w, "%s<field", indent)
		f.writeAttr(w, "name", n.Name)
		f.writeAttr(w, "visibility", string(n.Visibility))
		f.writeModifiers(w, n.Modifiers)
		if n.Type != nil && n.Type.Name != "" {
			f.writeAttr(w, "type", n.Type.Name)
		}
		if n.DefaultValue != "" {
			f.writeAttr(w, "default", n.DefaultValue)
		}
		f.writeNodeAttrs(w, n)
		if n.Implementation != "" && !f.options.Compact {
			fmt.Fprintln(w, ">")
			fmt.Fprintf(w, "%s  <implementation>%s</implementation>\n",
				indent, cdata("\n"+n.Implementation+"\n"+indent+"  "))
			fmt.Fprintf(w, "%s</field>\n", indent)
		} else {
			fmt.Fprintln(w, "/>")
		}

	case *ir.DistilledStruct:
		fmt.Fprintf(w, "%s<struct", indent)
		f.writeAttr(w, "name", n.Name)
		f.writeAttr(w, "visibility", string(n.Visibility))
		f.writeNodeAttrs(w, n)
		f.formatNodeChildren(w, ">", "</struct>", n.Children, depth)

	case *ir.DistilledEnum:
		fmt.Fprintf(w, "%s<enum", indent)
		f.writeAttr(w, "name", n.Name)
		f.writeAttr(w, "visibility", string(n.Visibility))
		f.writeNodeAttrs(w, n)
		f.formatNodeChildren(w, ">", "</enum>", n.Children, depth)

	case *ir.DistilledTypeAlias:
		fmt.Fprintf(w, "%s<type_alias", indent)
		f.writeAttr(w, "name", n.Name)
		f.writeAttr(w, "visibility", string(n.Visibility))
		f.writeModifiers(w, n.Modifiers)
		if n.Type.Name != "" {
			f.writeAttr(w, "type", n.Type.Name)
		}
		f.writeNodeAttrs(w, n)
		fmt.Fprintln(w, "/>")

	case *ir.DistilledComment:
		fmt.Fprintf(w, "%s<comment", indent)
		f.writeAttr(w, "format", n.Format)
		f.writeNodeAttrs(w, n)
		fmt.Fprintf(w, ">%s</comment>\n", cdata(n.Text))

	case *ir.DistilledError:
		f.formatError(w, n, depth)

	default:
		// Generic node
		fmt.Fprintf(w, "%s<node", indent)
		f.writeAttr(w, "kind", string(node.GetNodeKind()))
		f.writeNodeAttrs(w, node)
		fmt.Fprintln(w, "/>")
	}
}

// formatError formats an error element
func (f *XMLFormatter) formatError(w io.Writer, err *ir.DistilledError, depth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(w, "%s<error", indent)
	f.writeAttr(w, "severity", err.Severity)
	if err.Code != "" {
		f.writeAttr(w, "code", err.Code)
	}
	f.writeNodeAttrs(w, err)
	fmt.Fprintf(w, ">%s</error>\n", escapeXMLText(err.Message))
}

// formatNodeChildren handles nodes with children
func (f *XMLFormatter) formatNodeChildren(w io.Writer, open, close string, children []ir.DistilledNode, depth int) {
	if len(children) > 0 {
		fmt.Fprintln(w, open)
		for _, child := range children {
			f.formatNode(w, child, depth+1)
		}
		fmt.Fprintf(w, "%s%s\n", strings.Repeat("  ", depth), close)
	} else {
		fmt.Fprintln(w, "/>")
	}
}

// writeAttr writes an XML attribute
// writeSymbols closes the open element of an import or export: with its
// symbols as <symbol> children and the closing tag, or as an empty element.
func (f *XMLFormatter) writeSymbols(w io.Writer, indent, closing string, symbols []ir.ImportedSymbol) {
	if len(symbols) == 0 {
		fmt.Fprintln(w, "/>")
		return
	}
	fmt.Fprintln(w, ">")
	for _, sym := range symbols {
		fmt.Fprintf(w, "%s  <symbol name=\"%s\"", indent, escapeXMLAttr(sym.Name))
		f.writeAttr(w, "alias", sym.Alias)
		if sym.IsType {
			f.writeAttr(w, "is_type", "true")
		}
		fmt.Fprintln(w, "/>")
	}
	fmt.Fprintf(w, "%s%s\n", indent, closing)
}

func (f *XMLFormatter) writeAttr(w io.Writer, name, value string) {
	if value != "" {
		fmt.Fprintf(w, ` %s="%s"`, name, escapeXMLAttr(value))
	}
}

// writeNodeAttrs writes common node attributes
func (f *XMLFormatter) writeNodeAttrs(w io.Writer, node ir.DistilledNode) {
	if f.options.IncludeLocation {
		loc := node.GetLocation()
		f.writeLocationAttrs(w, &loc)
	}
}

// writeLocationAttrs writes location attributes
func (f *XMLFormatter) writeLocationAttrs(w io.Writer, loc *ir.Location) {
	if loc.StartLine > 0 {
		f.writeAttr(w, "line", fmt.Sprintf("%d", loc.StartLine))
		if loc.EndLine > loc.StartLine {
			f.writeAttr(w, "endLine", fmt.Sprintf("%d", loc.EndLine))
		}
		if loc.StartColumn > 0 {
			f.writeAttr(w, "column", fmt.Sprintf("%d", loc.StartColumn))
			if loc.EndColumn > 0 && loc.EndColumn != loc.StartColumn {
				f.writeAttr(w, "endColumn", fmt.Sprintf("%d", loc.EndColumn))
			}
		}
	}
}

// escapeXMLAttr escapes a string for use in XML attributes
func escapeXMLAttr(s string) string {
	var buf bytes.Buffer
	xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// escapeXMLText escapes a string for use in XML text content
func escapeXMLText(s string) string {
	var buf bytes.Buffer
	xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// writeModifiers writes modifier attributes
func (f *XMLFormatter) writeModifiers(w io.Writer, mods []ir.Modifier) {
	if len(mods) > 0 {
		modStrs := make([]string, len(mods))
		for i, mod := range mods {
			modStrs[i] = string(mod)
		}
		f.writeAttr(w, "modifiers", strings.Join(modStrs, " "))
	}
}

// cdata wraps s in a CDATA section. A literal "]]>" would end the section
// early, so it is split across two sections: "]]" closes the first and ">"
// opens the second.
func cdata(s string) string {
	return "<![CDATA[" + strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>") + "]]>"
}
