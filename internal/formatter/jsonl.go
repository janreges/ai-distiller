package formatter

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/janreges/ai-distiller/internal/ir"
)

// JSONLFormatter formats IR as JSON Lines (one JSON object per line)
type JSONLFormatter struct {
	BaseFormatter
	encoder *json.Encoder
}

// NewJSONLFormatter creates a new JSONL formatter
func NewJSONLFormatter(options Options) *JSONLFormatter {
	return &JSONLFormatter{
		BaseFormatter: NewBaseFormatter(options),
	}
}

// Extension returns the file extension for JSONL
func (f *JSONLFormatter) Extension() string {
	return ".jsonl"
}

// Format writes a single file as JSONL
func (f *JSONLFormatter) Format(w io.Writer, file *ir.DistilledFile) error {
	// JSONL format should always be one object per line, no indentation
	encoder := json.NewEncoder(w)

	// Write file as a single line
	fileObj := f.fileToJSON(file)
	if err := encoder.Encode(fileObj); err != nil {
		return err
	}

	// Write each node as a separate line
	return f.writeNodes(encoder, file.Path, file.Children, []string{})
}

// FormatMultiple writes multiple files as JSONL
func (f *JSONLFormatter) FormatMultiple(w io.Writer, files []*ir.DistilledFile) error {
	// JSONL format should always be one object per line, no indentation
	encoder := json.NewEncoder(w)

	for _, file := range files {
		// Write file object
		fileObj := f.fileToJSON(file)
		if err := encoder.Encode(fileObj); err != nil {
			return err
		}

		// Write nodes
		if err := f.writeNodes(encoder, file.Path, file.Children, []string{}); err != nil {
			return err
		}
	}

	return nil
}

// fileToJSON converts a file to JSON representation
func (f *JSONLFormatter) fileToJSON(file *ir.DistilledFile) map[string]interface{} {
	obj := map[string]interface{}{
		"type":     "file",
		"path":     file.Path,
		"language": file.Language,
		"version":  file.Version,
	}

	if f.options.IncludeLocation {
		obj["location"] = file.GetLocation()
	}

	if f.options.IncludeMetadata && file.Metadata != nil {
		obj["metadata"] = file.Metadata
	}

	if len(file.Errors) > 0 {
		obj["errors"] = file.Errors
	}

	// Summary stats
	stats := f.calculateStats(file)
	if len(stats) > 0 {
		obj["stats"] = stats
	}

	return obj
}

// writeNodes recursively writes nodes as JSONL
func (f *JSONLFormatter) writeNodes(encoder *json.Encoder, filePath string, nodes []ir.DistilledNode, path []string) error {
	for _, node := range nodes {
		nodeObj := f.nodeToJSON(filePath, node, path)
		if err := encoder.Encode(nodeObj); err != nil {
			return err
		}

		// Process children
		if children := node.GetChildren(); len(children) > 0 {
			newPath := append(path, f.getNodeName(node))
			if err := f.writeNodes(encoder, filePath, children, newPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// nodeToJSON converts a node to JSON representation
func (f *JSONLFormatter) nodeToJSON(filePath string, node ir.DistilledNode, path []string) map[string]interface{} {
	obj := map[string]interface{}{
		"type": string(node.GetNodeKind()),
		"file": filePath,
		"path": path,
		"name": f.getNodeName(node),
	}

	if f.options.IncludeLocation {
		obj["location"] = node.GetLocation()
	}

	// Add node-specific fields
	switch n := node.(type) {
	case *ir.DistilledPackage:
		// Package is already handled by name

	case *ir.DistilledImport:
		obj["import_type"] = n.ImportType
		obj["module"] = n.Module
		if n.IsType {
			obj["is_type"] = true
		}
		if len(n.Symbols) > 0 {
			obj["symbols"] = n.Symbols
		}

	case *ir.DistilledClass:
		obj["visibility"] = n.Visibility
		if len(n.Modifiers) > 0 {
			obj["modifiers"] = n.Modifiers
		}
		if len(n.Extends) > 0 {
			obj["extends"] = f.typeRefsToStrings(n.Extends)
		}
		if len(n.Implements) > 0 {
			obj["implements"] = f.typeRefsToStrings(n.Implements)
		}

	case *ir.DistilledInterface:
		obj["visibility"] = n.Visibility
		if len(n.Extends) > 0 {
			obj["extends"] = f.typeRefsToStrings(n.Extends)
		}

	case *ir.DistilledFunction:
		obj["visibility"] = n.Visibility
		if len(n.Modifiers) > 0 {
			obj["modifiers"] = n.Modifiers
		}
		obj["parameters"] = f.parametersToJSON(n.Parameters)
		if n.Returns != nil {
			obj["returns"] = n.Returns.Name
		}
		if n.Implementation != "" && f.options.Compact {
			obj["has_implementation"] = true
		} else if n.Implementation != "" {
			obj["implementation"] = n.Implementation
		}

	case *ir.DistilledField:
		obj["visibility"] = n.Visibility
		if len(n.Modifiers) > 0 {
			obj["modifiers"] = n.Modifiers
		}
		if n.Type != nil {
			obj["field_type"] = n.Type.Name
		}
		if n.DefaultValue != "" {
			obj["default"] = n.DefaultValue
		}

	case *ir.DistilledTypeAlias:
		obj["visibility"] = n.Visibility
		obj["alias_type"] = n.Type.Name

	case *ir.DistilledComment:
		obj["format"] = n.Format
		obj["text"] = n.Text

	case *ir.DistilledError:
		obj["severity"] = n.Severity
		obj["message"] = n.Message
		if n.Code != "" {
			obj["code"] = n.Code
		}
	}

	return obj
}

// getNodeName extracts the name from a node
func (f *JSONLFormatter) getNodeName(node ir.DistilledNode) string {
	switch n := node.(type) {
	case *ir.DistilledPackage:
		return n.Name
	case *ir.DistilledImport:
		return n.Module
	case *ir.DistilledClass:
		return n.Name
	case *ir.DistilledInterface:
		return n.Name
	case *ir.DistilledStruct:
		return n.Name
	case *ir.DistilledEnum:
		return n.Name
	case *ir.DistilledFunction:
		return n.Name
	case *ir.DistilledField:
		return n.Name
	case *ir.DistilledTypeAlias:
		return n.Name
	case *ir.DistilledComment:
		return "comment"
	case *ir.DistilledError:
		return fmt.Sprintf("error:%d", n.GetLocation().StartLine)
	default:
		return string(node.GetNodeKind())
	}
}

// typeRefsToStrings converts type references to strings
func (f *JSONLFormatter) typeRefsToStrings(refs []ir.TypeRef) []string {
	result := make([]string, len(refs))
	for i, ref := range refs {
		result[i] = ref.Name
	}
	return result
}

// parametersToJSON converts parameters to JSON
func (f *JSONLFormatter) parametersToJSON(params []ir.Parameter) []map[string]interface{} {
	result := make([]map[string]interface{}, len(params))
	for i, p := range params {
		param := map[string]interface{}{
			"name": p.Name,
		}
		if p.Type.Name != "" {
			param["type"] = p.Type.Name
		}
		if p.DefaultValue != "" {
			param["default"] = p.DefaultValue
		}
		if p.IsOptional {
			param["optional"] = true
		}
		if p.IsVariadic {
			param["variadic"] = true
		}
		if p.IsReference {
			param["reference"] = true
		}
		result[i] = param
	}
	return result
}

// calculateStats calculates statistics for a file
func (f *JSONLFormatter) calculateStats(file *ir.DistilledFile) map[string]int {
	stats := make(map[string]int)
	f.countNodes(file.Children, stats)
	return stats
}

// countNodes recursively counts node types
func (f *JSONLFormatter) countNodes(nodes []ir.DistilledNode, stats map[string]int) {
	for _, node := range nodes {
		kind := string(node.GetNodeKind())
		stats[kind]++

		if children := node.GetChildren(); len(children) > 0 {
			f.countNodes(children, stats)
		}
	}
}
