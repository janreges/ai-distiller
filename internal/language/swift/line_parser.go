package swift

import (
	"regexp"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// LineParser implements a line-based Swift parser
type LineParser struct {
	source   []byte
	filename string
	lines    []string

	// Pre-compiled regular expressions
	importRe           *regexp.Regexp
	classRe            *regexp.Regexp
	structRe           *regexp.Regexp
	protocolRe         *regexp.Regexp
	extensionRe        *regexp.Regexp
	enumRe             *regexp.Regexp
	functionRe         *regexp.Regexp
	propertyRe         *regexp.Regexp
	typeAliasRe        *regexp.Regexp
	enumCaseRe         *regexp.Regexp
	protocolMethodRe   *regexp.Regexp
	protocolPropertyRe *regexp.Regexp
	actorRe            *regexp.Regexp
}

// parserState tracks current parsing context
type parserState struct {
	insideClass     bool
	insideStruct    bool
	insideProtocol  bool
	insideExtension bool
	insideEnum      bool
	insideActor     bool

	currentClass     *ir.DistilledClass
	currentInterface *ir.DistilledInterface
	currentEnum      *ir.DistilledEnum

	braceLevel   int
	lastNodeLine int
}

// NewLineParser creates a new line-based parser
func NewLineParser(source []byte, filename string) *LineParser {
	return &LineParser{
		source:   source,
		filename: filename,
		lines:    strings.Split(strings.ReplaceAll(string(source), "\r\n", "\n"), "\n"),

		// Compile regular expressions
		importRe:           regexp.MustCompile(`^\s*import\s+(\S+)`),
		classRe:            regexp.MustCompile(`^\s*(open\s+|public\s+|internal\s+|fileprivate\s+|private\s+)?(final\s+)?class\s+(\w+)(\s*:\s*([^{]+))?`),
		structRe:           regexp.MustCompile(`^\s*(public\s+|internal\s+|fileprivate\s+|private\s+)?struct\s+(\w+)(\s*:\s*([^{]+))?`),
		protocolRe:         regexp.MustCompile(`^\s*(public\s+|internal\s+|fileprivate\s+|private\s+)?protocol\s+(\w+)(\s*:\s*([^{]+))?`),
		extensionRe:        regexp.MustCompile(`^\s*(public\s+|internal\s+|fileprivate\s+|private\s+)?extension\s+(\w+)(\s*:\s*([^{]+))?`),
		enumRe:             regexp.MustCompile(`^\s*(public\s+|internal\s+|fileprivate\s+|private\s+)?enum\s+(\w+)(\s*:\s*([^{]+))?`),
		functionRe:         regexp.MustCompile(`^\s*(public\s+|internal\s+|fileprivate\s+|private\s+|open\s+)?(static\s+|class\s+|final\s+|override\s+|mutating\s+)*func\s+(\w+)\s*\((.*?)\)(\s*(async\s+)?(throws\s+)?->\s*(.+))?`),
		propertyRe:         regexp.MustCompile(`^\s*(@\w+\s+)*(public\s+|internal\s+|fileprivate\s+|private\s+)?(static\s+|class\s+)?(let|var)\s+(\w+)\s*:\s*(.+?)(\s*=.*)?$`),
		typeAliasRe:        regexp.MustCompile(`^\s*(public\s+|internal\s+|fileprivate\s+|private\s+)?typealias\s+(\w+)\s*=\s*(.+)`),
		enumCaseRe:         regexp.MustCompile(`^\s*case\s+(\w+)(\((.*?)\))?`),
		protocolMethodRe:   regexp.MustCompile(`^\s*func\s+(\w+)\s*\((.*?)\)(\s*(async\s+)?(throws\s+)?->\s*(.+))?`),
		protocolPropertyRe: regexp.MustCompile(`^\s*var\s+(\w+)\s*:\s*(.+?)(?:\s*\{\s*(get|set|get\s+set).*\})?$`),
		actorRe:            regexp.MustCompile(`^\s*(public\s+|internal\s+|fileprivate\s+|private\s+)?actor\s+(\w+)`),
	}
}

// Parse parses the Swift source code
func (p *LineParser) Parse() *ir.DistilledFile {
	file := &ir.DistilledFile{
		Path:     p.filename,
		Language: "swift",
		Version:  "1.0",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	state := &parserState{}

	for i := 0; i < len(p.lines); i++ {
		lineNum := i + 1
		line := p.lines[i]
		trimmedLine := strings.TrimSpace(line)

		// DEBUG
		// if strings.Contains(trimmedLine, "enum") || strings.Contains(trimmedLine, "func") {
		// 	fmt.Printf("DEBUG: Line %d: %s\n", lineNum, trimmedLine)
		// }

		// Skip empty lines but handle comments separately
		if trimmedLine == "" {
			continue
		}

		// Handle comments
		if strings.HasPrefix(trimmedLine, "//") {
			// Parse doc comments
			if strings.HasPrefix(trimmedLine, "///") {
				comment := &ir.DistilledComment{
					BaseNode: ir.BaseNode{
						Location: ir.Location{StartLine: lineNum, EndLine: lineNum},
					},
					Text:   strings.TrimSpace(strings.TrimPrefix(trimmedLine, "///")),
					Format: ir.CommentDoc,
				}
				p.addToParent(file, state, comment)
			} else if strings.HasPrefix(trimmedLine, "// MARK:") || strings.HasPrefix(trimmedLine, "//MARK:") {
				comment := &ir.DistilledComment{
					BaseNode: ir.BaseNode{
						Location: ir.Location{StartLine: lineNum, EndLine: lineNum},
					},
					Text:   strings.TrimPrefix(strings.TrimPrefix(trimmedLine, "// MARK:"), "//MARK:"),
					Format: ir.CommentLine,
				}
				p.addToParent(file, state, comment)
			}
			continue
		}

		// Track brace levels
		state.braceLevel += braceDelta(line)

		// Parse different constructs
		if matches := p.importRe.FindStringSubmatch(line); matches != nil {
			p.parseImport(matches, lineNum, file)
		} else if matches := p.classRe.FindStringSubmatch(line); matches != nil {
			p.parseClass(matches, lineNum, file, state)
		} else if matches := p.structRe.FindStringSubmatch(line); matches != nil {
			p.parseStruct(matches, lineNum, file, state)
		} else if matches := p.protocolRe.FindStringSubmatch(line); matches != nil {
			p.parseProtocol(matches, lineNum, file, state)
		} else if matches := p.extensionRe.FindStringSubmatch(line); matches != nil {
			p.parseExtension(matches, lineNum, file, state)
		} else if matches := p.enumRe.FindStringSubmatch(line); matches != nil {
			p.parseEnum(matches, lineNum, file, state)
		} else if matches := p.actorRe.FindStringSubmatch(line); matches != nil {
			p.parseActor(matches, lineNum, file, state)
		} else if state.insideEnum {
			if matches := p.enumCaseRe.FindStringSubmatch(trimmedLine); matches != nil {
				p.parseEnumCase(matches, lineNum, state)
			}
		} else if state.insideProtocol {
			// Inside protocol, look for method/property requirements
			if matches := p.protocolMethodRe.FindStringSubmatch(trimmedLine); matches != nil {
				p.parseProtocolMethod(matches, lineNum, state)
			} else if matches := p.protocolPropertyRe.FindStringSubmatch(trimmedLine); matches != nil {
				p.parseProtocolProperty(matches, lineNum, state)
			}
		} else if matches := p.functionRe.FindStringSubmatch(line); matches != nil {
			i = p.parseFunction(matches, i, file, state)
		} else if matches := p.propertyRe.FindStringSubmatch(line); matches != nil {
			p.parseProperty(matches, lineNum, file, state)
		} else if matches := p.typeAliasRe.FindStringSubmatch(line); matches != nil {
			p.parseTypeAlias(matches, lineNum, file, state)
		}

		// Check if we're exiting a scope
		if state.braceLevel == 0 {
			state.insideClass = false
			state.insideStruct = false
			state.insideProtocol = false
			state.insideExtension = false
			state.insideEnum = false
			state.insideActor = false
			state.currentClass = nil
			state.currentInterface = nil
			state.currentEnum = nil
		}
	}

	return file
}

// parseImport parses import declarations
func (p *LineParser) parseImport(matches []string, lineNum int, file *ir.DistilledFile) {
	imp := &ir.DistilledImport{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum, EndLine: lineNum},
		},
		ImportType: "import",
		Module:     matches[1],
		Symbols: []ir.ImportedSymbol{
			{Name: matches[1]},
		},
	}
	file.Children = append(file.Children, imp)
}

// parseClass parses class declarations
func (p *LineParser) parseClass(matches []string, lineNum int, file *ir.DistilledFile, state *parserState) {
	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum},
		},
		Name:       matches[3],
		Visibility: p.parseVisibility(matches[1]),
		Modifiers:  []ir.Modifier{},
		Children:   []ir.DistilledNode{},
	}

	// Check for final modifier
	if matches[2] != "" {
		class.Modifiers = append(class.Modifiers, ir.ModifierFinal)
	}

	// Parse inheritance
	if matches[5] != "" {
		p.parseInheritance(matches[5], class)
	}

	state.insideClass = true
	state.currentClass = class
	state.lastNodeLine = lineNum

	file.Children = append(file.Children, class)
}

// parseStruct parses struct declarations
func (p *LineParser) parseStruct(matches []string, lineNum int, file *ir.DistilledFile, state *parserState) {
	// Structs are represented as classes in the IR
	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum},
		},
		Name:       matches[2],
		Visibility: p.parseVisibility(matches[1]),
		Children:   []ir.DistilledNode{},
	}

	// Parse protocols
	if matches[4] != "" {
		p.parseInheritance(matches[4], class)
	}

	state.insideStruct = true
	state.currentClass = class
	state.lastNodeLine = lineNum

	file.Children = append(file.Children, class)
}

// parseProtocol parses protocol declarations
func (p *LineParser) parseProtocol(matches []string, lineNum int, file *ir.DistilledFile, state *parserState) {
	protocol := &ir.DistilledInterface{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum},
		},
		Name:       matches[2],
		Visibility: p.parseVisibility(matches[1]),
		Children:   []ir.DistilledNode{},
	}

	// Parse protocol inheritance
	if matches[4] != "" {
		protocols := strings.Split(matches[4], ",")
		for _, proto := range protocols {
			protocol.Extends = append(protocol.Extends, ir.TypeRef{
				Name: strings.TrimSpace(proto),
			})
		}
	}

	state.insideProtocol = true
	state.currentInterface = protocol
	state.lastNodeLine = lineNum

	file.Children = append(file.Children, protocol)
}

// parseExtension parses extension declarations
func (p *LineParser) parseExtension(matches []string, lineNum int, file *ir.DistilledFile, state *parserState) {
	ext := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum},
		},
		Visibility: p.parseVisibility(matches[1]),
		Children:   []ir.DistilledNode{},
	}

	// Name the extension
	typeName := matches[2]
	if matches[4] != "" {
		ext.Name = "extension " + typeName + ": " + matches[4]
		// Parse protocol conformances
		protocols := strings.Split(matches[4], ",")
		for _, proto := range protocols {
			ext.Implements = append(ext.Implements, ir.TypeRef{
				Name: strings.TrimSpace(proto),
			})
		}
	} else {
		ext.Name = "extension " + typeName
	}

	state.insideExtension = true
	state.currentClass = ext
	state.lastNodeLine = lineNum

	file.Children = append(file.Children, ext)
}

// parseEnum parses enum declarations
func (p *LineParser) parseEnum(matches []string, lineNum int, file *ir.DistilledFile, state *parserState) {
	enum := &ir.DistilledEnum{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum},
		},
		Name:       matches[2],
		Visibility: p.parseVisibility(matches[1]),
		Children:   []ir.DistilledNode{},
	}

	state.insideEnum = true
	state.currentEnum = enum
	state.lastNodeLine = lineNum

	file.Children = append(file.Children, enum)
}

// parseActor parses actor declarations
func (p *LineParser) parseActor(matches []string, lineNum int, file *ir.DistilledFile, state *parserState) {
	// Actors are represented as classes with a special modifier
	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum},
		},
		Name:       matches[2],
		Visibility: p.parseVisibility(matches[1]),
		Modifiers:  []ir.Modifier{ir.ModifierAsync}, // Mark as async to indicate actor
		Children:   []ir.DistilledNode{},
	}

	state.insideActor = true
	state.currentClass = class
	state.lastNodeLine = lineNum

	file.Children = append(file.Children, class)
}

// parseFunction parses the function declared on line index i. A body that
// opens on that line is stored, braces included, in Implementation, and its
// lines are consumed so that they are never parsed as members. It returns the
// index of the last line it consumed.
func (p *LineParser) parseFunction(matches []string, i int, file *ir.DistilledFile, state *parserState) int {
	lineNum := i + 1
	fn := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum, EndLine: lineNum},
		},
		Name:       matches[3], // Changed from 4 to 3 after removing optional func group
		Visibility: p.parseVisibility(matches[1]),
		Parameters: p.parseParameters(matches[4]), // Changed from 5 to 4
		Modifiers:  []ir.Modifier{},
	}

	// Check modifiers
	if strings.Contains(matches[2], "static") || strings.Contains(matches[2], "class") {
		fn.Modifiers = append(fn.Modifiers, ir.ModifierStatic)
	}
	if strings.Contains(matches[0], "async") {
		fn.Modifiers = append(fn.Modifiers, ir.ModifierAsync)
	}

	// Parse return type; the capture runs to the end of the line, so cut the
	// body that may follow it.
	if matches[5] != "" && matches[8] != "" { // Changed from 6,9 to 5,8
		ret, _, _ := strings.Cut(matches[8], "{")
		if ret = strings.TrimSpace(ret); ret != "" {
			fn.Returns = &ir.TypeRef{Name: ret}
		}
	}

	last := i
	if body, end, ok := p.functionBody(i); ok {
		fn.Implementation = body
		fn.Location.EndLine = end + 1
		// Line i is already counted by the caller.
		for _, l := range p.lines[i+1 : end+1] {
			state.braceLevel += braceDelta(l)
		}
		last = end
	}

	p.addToParent(file, state, fn)
	return last
}

// functionBody returns the body that opens on line index start: the text from
// the first '{' outside parentheses, string literals and comments up to its
// matching '}', with trailing whitespace trimmed from every line, and the index
// of the line holding that '}'. ok is false when no body opens on the line or
// the body never closes.
func (p *LineParser) functionBody(start int) (body string, end int, ok bool) {
	open, parens := -1, 0
	scanCode(p.lines[start], func(i int, c byte) bool {
		switch c {
		case '(':
			parens++
		case ')':
			parens--
		case '{':
			if parens == 0 {
				open = i
				return false
			}
		}
		return true
	})
	if open < 0 {
		return "", 0, false
	}

	var lines []string
	depth := 0
	for n := start; n < len(p.lines); n++ {
		line := p.lines[n]
		if n == start {
			line = line[open:]
		}
		closeAt := -1
		scanCode(line, func(i int, c byte) bool {
			switch c {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					closeAt = i
					return false
				}
			}
			return true
		})
		if closeAt >= 0 {
			lines = append(lines, strings.TrimRight(line[:closeAt+1], " \t"))
			return strings.Join(lines, "\n"), n, true
		}
		lines = append(lines, strings.TrimRight(line, " \t"))
	}
	return "", 0, false
}

// braceDelta returns the number of '{' minus the number of '}' on line,
// ignoring braces inside double-quoted string literals and after a //
// comment.
func braceDelta(line string) int {
	delta := 0
	scanCode(line, func(_ int, c byte) bool {
		switch c {
		case '{':
			delta++
		case '}':
			delta--
		}
		return true
	})
	return delta
}

// scanCode calls visit with the offset of every byte of line that is code:
// outside double-quoted string literals (whose quotes are not visited either)
// and before a // comment. Scanning stops early when visit returns false.
func scanCode(line string, visit func(i int, c byte) bool) {
	inString := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inString:
			if c == '\\' {
				i++ // skip the escaped character
			} else if c == '"' {
				inString = false
			}
		case c == '"':
			inString = true
		case c == '/' && i+1 < len(line) && line[i+1] == '/':
			return
		default:
			if !visit(i, c) {
				return
			}
		}
	}
}

// parseProperty parses property declarations
func (p *LineParser) parseProperty(matches []string, lineNum int, file *ir.DistilledFile, state *parserState) {
	field := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum, EndLine: lineNum},
		},
		Name:       matches[5],
		Visibility: p.parseVisibility(matches[2]),
		Type:       &ir.TypeRef{Name: strings.TrimSpace(matches[6])},
		Modifiers:  []ir.Modifier{},
	}

	// Check for property wrappers
	if matches[1] != "" {
		field.Decorators = append(field.Decorators, strings.TrimSpace(matches[1]))
	}

	// Check if it's let (constant)
	if matches[4] == "let" {
		field.Modifiers = append(field.Modifiers, ir.ModifierFinal)
	}

	// Check for static
	if strings.Contains(matches[3], "static") || strings.Contains(matches[3], "class") {
		field.Modifiers = append(field.Modifiers, ir.ModifierStatic)
	}

	// Extract default value
	if matches[7] != "" {
		field.DefaultValue = strings.TrimSpace(strings.TrimPrefix(matches[7], "="))
	}

	p.addToParent(file, state, field)
}

// parseTypeAlias parses type alias declarations
func (p *LineParser) parseTypeAlias(matches []string, lineNum int, file *ir.DistilledFile, state *parserState) {
	alias := &ir.DistilledTypeAlias{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum, EndLine: lineNum},
		},
		Name:       matches[2],
		Visibility: p.parseVisibility(matches[1]),
		Type:       ir.TypeRef{Name: strings.TrimSpace(matches[3])},
	}

	p.addToParent(file, state, alias)
}

// parseEnumCase parses enum case declarations
func (p *LineParser) parseEnumCase(matches []string, lineNum int, state *parserState) {
	if state.currentEnum == nil {
		return
	}

	field := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum, EndLine: lineNum},
		},
		Name:       matches[1],
		Visibility: ir.VisibilityPublic, // Enum cases are always public
	}

	// Check for associated values
	if matches[3] != "" {
		field.Type = &ir.TypeRef{Name: "(" + matches[3] + ")"}
	}

	state.currentEnum.Children = append(state.currentEnum.Children, field)
}

// parseProtocolMethod parses protocol method requirements
func (p *LineParser) parseProtocolMethod(matches []string, lineNum int, state *parserState) {
	if state.currentInterface == nil {
		return
	}

	fn := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum, EndLine: lineNum},
		},
		Name:       matches[1],
		Visibility: ir.VisibilityPublic,
		Parameters: p.parseParameters(matches[2]),
		Modifiers:  []ir.Modifier{ir.ModifierAbstract},
	}

	// Parse return type
	if matches[3] != "" && matches[6] != "" {
		fn.Returns = &ir.TypeRef{Name: strings.TrimSpace(matches[6])}
	}

	state.currentInterface.Children = append(state.currentInterface.Children, fn)
}

// parseProtocolProperty parses protocol property requirements
func (p *LineParser) parseProtocolProperty(matches []string, lineNum int, state *parserState) {
	if state.currentInterface == nil {
		return
	}

	field := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum, EndLine: lineNum},
		},
		Name:       matches[1],
		Visibility: ir.VisibilityPublic,
		Type:       &ir.TypeRef{Name: strings.TrimSpace(matches[2])},
		Modifiers:  []ir.Modifier{ir.ModifierAbstract},
	}

	state.currentInterface.Children = append(state.currentInterface.Children, field)
}

// Helper methods

// parseVisibility extracts visibility from modifier string
func (p *LineParser) parseVisibility(modifier string) ir.Visibility {
	modifier = strings.TrimSpace(modifier)
	switch {
	case strings.Contains(modifier, "public") || strings.Contains(modifier, "open"):
		return ir.VisibilityPublic
	case strings.Contains(modifier, "private"):
		return ir.VisibilityPrivate
	case strings.Contains(modifier, "fileprivate"):
		return ir.VisibilityPrivate
	case strings.Contains(modifier, "internal"):
		return ir.VisibilityInternal
	default:
		return ir.VisibilityInternal // Default in Swift
	}
}

// parseParameters extracts function parameters
func (p *LineParser) parseParameters(paramStr string) []ir.Parameter {
	if paramStr == "" {
		return []ir.Parameter{}
	}

	var params []ir.Parameter
	// Simple parameter parsing - doesn't handle all edge cases
	parts := strings.Split(paramStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Try to extract name and type
		colonIdx := strings.Index(part, ":")
		if colonIdx > 0 {
			name := strings.TrimSpace(part[:colonIdx])
			// Handle external/internal parameter names
			nameParts := strings.Fields(name)
			if len(nameParts) > 0 {
				name = nameParts[len(nameParts)-1]
			}

			typeStr := strings.TrimSpace(part[colonIdx+1:])
			params = append(params, ir.Parameter{
				Name: name,
				Type: ir.TypeRef{Name: typeStr},
			})
		}
	}

	return params
}

// parseInheritance parses class/struct inheritance and protocol conformances
func (p *LineParser) parseInheritance(inheritanceStr string, class *ir.DistilledClass) {
	parts := strings.Split(inheritanceStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			// In Swift, we can't easily distinguish between superclass and protocols
			// For simplicity, treat them all as implements
			class.Implements = append(class.Implements, ir.TypeRef{Name: part})
		}
	}
}

// addToParent adds a node to its parent or to the file
func (p *LineParser) addToParent(file *ir.DistilledFile, state *parserState, node ir.DistilledNode) {
	if state.insideClass || state.insideStruct || state.insideExtension || state.insideActor {
		if state.currentClass != nil {
			state.currentClass.Children = append(state.currentClass.Children, node)
			return
		}
	} else if state.insideProtocol {
		if state.currentInterface != nil {
			state.currentInterface.Children = append(state.currentInterface.Children, node)
			return
		}
	} else if state.insideEnum {
		if state.currentEnum != nil {
			state.currentEnum.Children = append(state.currentEnum.Children, node)
			return
		}
	}

	file.Children = append(file.Children, node)
}
