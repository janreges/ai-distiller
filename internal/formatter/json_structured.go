package formatter

import (
	"encoding/json"
	"io"

	"github.com/janreges/ai-distiller/internal/ir"
)

// JSONStructuredFormatter formats IR as structured JSON
type JSONStructuredFormatter struct {
	BaseFormatter
}

// NewJSONStructuredFormatter creates a new structured JSON formatter
func NewJSONStructuredFormatter(options Options) *JSONStructuredFormatter {
	return &JSONStructuredFormatter{
		BaseFormatter: NewBaseFormatter(options),
	}
}

// Extension returns the file extension for JSON
func (f *JSONStructuredFormatter) Extension() string {
	return ".json"
}

// Format writes a single file as structured JSON
func (f *JSONStructuredFormatter) Format(w io.Writer, file *ir.DistilledFile) error {
	encoder := json.NewEncoder(w)
	if !f.options.Compact {
		encoder.SetIndent("", "  ")
	}

	data := f.fileToStructured(file)
	return encoder.Encode(data)
}

// FormatMultiple writes multiple files as structured JSON
func (f *JSONStructuredFormatter) FormatMultiple(w io.Writer, files []*ir.DistilledFile) error {
	encoder := json.NewEncoder(w)
	if !f.options.Compact {
		encoder.SetIndent("", "  ")
	}

	// Create project structure
	project := map[string]interface{}{
		"type":  "project",
		"files": make([]interface{}, len(files)),
	}

	// Add statistics
	totalStats := make(map[string]int)

	for i, file := range files {
		fileData := f.fileToStructured(file)
		project["files"].([]interface{})[i] = fileData

		// Aggregate stats
		if stats, ok := fileData["stats"].(map[string]int); ok {
			for k, v := range stats {
				totalStats[k] += v
			}
		}
	}

	if len(totalStats) > 0 {
		project["total_stats"] = totalStats
	}

	return encoder.Encode(project)
}

// fileToStructured converts a file to structured representation
func (f *JSONStructuredFormatter) fileToStructured(file *ir.DistilledFile) map[string]interface{} {
	data := map[string]interface{}{
		"type":     "file",
		"path":     file.Path,
		"language": file.Language,
		"version":  file.Version,
	}

	if f.options.IncludeLocation {
		data["location"] = file.GetLocation()
	}

	if f.options.IncludeMetadata && file.Metadata != nil {
		data["metadata"] = file.Metadata
	}

	// Structure nodes by type
	structure := f.structureNodes(file.Children)
	if len(structure) > 0 {
		data["structure"] = structure
	}

	// Add errors
	if len(file.Errors) > 0 {
		data["errors"] = file.Errors
	}

	// Add statistics
	stats := f.calculateStats(file.Children)
	if len(stats) > 0 {
		data["stats"] = stats
	}

	return data
}

// structureNodes organizes nodes by type
func (f *JSONStructuredFormatter) structureNodes(nodes []ir.DistilledNode) map[string]interface{} {
	structure := make(map[string]interface{})

	// Group nodes by type
	packages := []interface{}{}
	imports := []interface{}{}
	exports := []interface{}{}
	classes := []interface{}{}
	interfaces := []interface{}{}
	functions := []interface{}{}
	variables := []interface{}{}
	types := []interface{}{}

	for _, node := range nodes {
		switch n := node.(type) {
		case *ir.DistilledPackage:
			packages = append(packages, f.packageToStructured(n))

		case *ir.DistilledImport:
			imports = append(imports, f.importToStructured(n))

		case *ir.DistilledExport:
			exports = append(exports, f.exportToStructured(n))

		case *ir.DistilledClass:
			classes = append(classes, f.classToStructured(n))

		case *ir.DistilledInterface:
			interfaces = append(interfaces, f.interfaceToStructured(n))

		case *ir.DistilledFunction:
			functions = append(functions, f.functionToStructured(n))

		case *ir.DistilledField:
			variables = append(variables, f.fieldToStructured(n))

		case *ir.DistilledTypeAlias:
			types = append(types, f.typeAliasToStructured(n))
		}
	}

	// Add non-empty sections
	if len(packages) > 0 {
		structure["packages"] = packages
	}
	if len(imports) > 0 {
		structure["imports"] = imports
	}
	if len(exports) > 0 {
		structure["exports"] = exports
	}
	if len(classes) > 0 {
		structure["classes"] = classes
	}
	if len(interfaces) > 0 {
		structure["interfaces"] = interfaces
	}
	if len(functions) > 0 {
		structure["functions"] = functions
	}
	if len(variables) > 0 {
		structure["variables"] = variables
	}
	if len(types) > 0 {
		structure["types"] = types
	}

	return structure
}

// Node conversion methods

func (f *JSONStructuredFormatter) packageToStructured(n *ir.DistilledPackage) map[string]interface{} {
	pkg := map[string]interface{}{
		"name": n.Name,
	}
	f.addCommonFields(pkg, n)
	return pkg
}

func (f *JSONStructuredFormatter) importToStructured(n *ir.DistilledImport) map[string]interface{} {
	imp := map[string]interface{}{
		"type":   n.ImportType,
		"module": n.Module,
	}

	if n.IsType {
		imp["is_type"] = true
	}
	if len(n.Symbols) > 0 {
		imp["symbols"] = symbolsToStructured(n.Symbols)
	}

	f.addCommonFields(imp, n)
	return imp
}

func (f *JSONStructuredFormatter) exportToStructured(n *ir.DistilledExport) map[string]interface{} {
	exp := map[string]interface{}{}
	if n.Module != "" {
		exp["module"] = n.Module
	}
	if len(n.Symbols) > 0 {
		exp["symbols"] = symbolsToStructured(n.Symbols)
	}
	if n.Value != "" {
		exp["value"] = n.Value
	}
	if n.IsDefault {
		exp["is_default"] = true
	}
	if n.IsType {
		exp["is_type"] = true
	}
	f.addCommonFields(exp, n)
	return exp
}

// symbolsToStructured returns the symbols of an import or export.
func symbolsToStructured(symbols []ir.ImportedSymbol) []map[string]interface{} {
	out := make([]map[string]interface{}, len(symbols))
	for i, sym := range symbols {
		s := map[string]interface{}{"name": sym.Name}
		if sym.Alias != "" {
			s["alias"] = sym.Alias
		}
		if sym.IsType {
			s["is_type"] = true
		}
		out[i] = s
	}
	return out
}

func (f *JSONStructuredFormatter) classToStructured(n *ir.DistilledClass) map[string]interface{} {
	class := map[string]interface{}{
		"name":       n.Name,
		"visibility": n.Visibility,
	}

	if len(n.Modifiers) > 0 {
		class["modifiers"] = n.Modifiers
	}

	if len(n.Extends) > 0 {
		class["extends"] = f.typeRefsToStrings(n.Extends)
	}

	if len(n.Implements) > 0 {
		class["implements"] = f.typeRefsToStrings(n.Implements)
	}

	// Structure class members
	if len(n.Children) > 0 {
		members := f.structureClassMembers(n.Children)
		if len(members) > 0 {
			class["members"] = members
		}
	}

	f.addCommonFields(class, n)
	return class
}

func (f *JSONStructuredFormatter) interfaceToStructured(n *ir.DistilledInterface) map[string]interface{} {
	iface := map[string]interface{}{
		"name":       n.Name,
		"visibility": n.Visibility,
	}

	if len(n.Extends) > 0 {
		iface["extends"] = f.typeRefsToStrings(n.Extends)
	}

	if len(n.Children) > 0 {
		members := f.structureClassMembers(n.Children)
		if len(members) > 0 {
			iface["members"] = members
		}
	}

	f.addCommonFields(iface, n)
	return iface
}

func (f *JSONStructuredFormatter) functionToStructured(n *ir.DistilledFunction) map[string]interface{} {
	fn := map[string]interface{}{
		"name":       n.Name,
		"visibility": n.Visibility,
	}

	if len(n.Modifiers) > 0 {
		fn["modifiers"] = n.Modifiers
	}

	// Parameters
	if len(n.Parameters) > 0 {
		params := make([]map[string]interface{}, len(n.Parameters))
		for i, p := range n.Parameters {
			param := map[string]interface{}{"name": p.Name}
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
			params[i] = param
		}
		fn["parameters"] = params
	}

	// Return type
	if n.Returns != nil && n.Returns.Name != "" {
		fn["returns"] = n.Returns.Name
	}

	// Implementation
	if n.Implementation != "" {
		if f.options.Compact {
			fn["has_implementation"] = true
		} else {
			fn["implementation"] = n.Implementation
		}
	}

	f.addCommonFields(fn, n)
	return fn
}

func (f *JSONStructuredFormatter) fieldToStructured(n *ir.DistilledField) map[string]interface{} {
	field := map[string]interface{}{
		"name":       n.Name,
		"visibility": n.Visibility,
	}

	if len(n.Modifiers) > 0 {
		field["modifiers"] = n.Modifiers
	}

	if n.Type != nil && n.Type.Name != "" {
		field["type"] = n.Type.Name
	}

	if n.DefaultValue != "" {
		field["default"] = n.DefaultValue
	}

	f.addCommonFields(field, n)
	return field
}

func (f *JSONStructuredFormatter) typeAliasToStructured(n *ir.DistilledTypeAlias) map[string]interface{} {
	ta := map[string]interface{}{
		"name":       n.Name,
		"visibility": n.Visibility,
		"type":       n.Type.Name,
	}

	f.addCommonFields(ta, n)
	return ta
}

// structureClassMembers structures class/interface members
func (f *JSONStructuredFormatter) structureClassMembers(nodes []ir.DistilledNode) map[string]interface{} {
	members := make(map[string]interface{})

	fields := []interface{}{}
	methods := []interface{}{}

	for _, node := range nodes {
		switch n := node.(type) {
		case *ir.DistilledField:
			fields = append(fields, f.fieldToStructured(n))
		case *ir.DistilledFunction:
			methods = append(methods, f.functionToStructured(n))
		}
	}

	if len(fields) > 0 {
		members["fields"] = fields
	}
	if len(methods) > 0 {
		members["methods"] = methods
	}

	return members
}

// Helper methods

func (f *JSONStructuredFormatter) addCommonFields(data map[string]interface{}, node ir.DistilledNode) {
	if f.options.IncludeLocation {
		loc := node.GetLocation()
		if loc.StartLine > 0 {
			data["location"] = map[string]int{
				"start_line": loc.StartLine,
				"end_line":   loc.EndLine,
			}
		}
	}
}

func (f *JSONStructuredFormatter) typeRefsToStrings(refs []ir.TypeRef) []string {
	result := make([]string, len(refs))
	for i, ref := range refs {
		result[i] = ref.Name
	}
	return result
}

func (f *JSONStructuredFormatter) calculateStats(nodes []ir.DistilledNode) map[string]int {
	stats := make(map[string]int)
	f.countNodes(nodes, stats)
	return stats
}

func (f *JSONStructuredFormatter) countNodes(nodes []ir.DistilledNode, stats map[string]int) {
	for _, node := range nodes {
		kind := string(node.GetNodeKind())
		stats[kind]++

		if children := node.GetChildren(); len(children) > 0 {
			f.countNodes(children, stats)
		}
	}
}
