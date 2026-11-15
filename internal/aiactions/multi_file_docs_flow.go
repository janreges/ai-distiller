package aiactions

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/janreges/ai-distiller/internal/ai"
)

// MultiFileDocsFlowAction generates comprehensive multi-file documentation workflow
type MultiFileDocsFlowAction struct{}

// Ensure MultiFileDocsFlowAction implements FlowAction
var _ ai.FlowAction = (*MultiFileDocsFlowAction)(nil)

func (a *MultiFileDocsFlowAction) Name() string {
	return "flow-for-multi-file-docs"
}

func (a *MultiFileDocsFlowAction) Description() string {
	return "Generate structured documentation workflow for comprehensive multi-file documentation"
}

func (a *MultiFileDocsFlowAction) Type() ai.ActionType {
	return ai.ActionTypeFlow
}

func (a *MultiFileDocsFlowAction) DefaultOutput() string {
	return "./.aid"
}

func (a *MultiFileDocsFlowAction) Validate() error {
	return nil
}

func (a *MultiFileDocsFlowAction) ExecuteFlow(ctx *ai.ActionContext) (*ai.FlowResult, error) {
	// Get project basename and current date
	basename := ctx.BaseName
	currentDate := fmt.Sprintf("%04d-%02d-%02d",
		ctx.Timestamp.Year(), ctx.Timestamp.Month(), ctx.Timestamp.Day())

	// Collect source files from the project
	sourceFiles, err := collectSourceFiles(ctx.ProjectPath, ctx.IncludePatterns, ctx.ExcludePatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to collect source files: %w", err)
	}

	// Generate file paths
	docsTaskListPath := fmt.Sprintf("DOCS-TASK-LIST.%s.%s.md", basename, currentDate)
	docsIndexPath := fmt.Sprintf("DOCS-INDEX.%s.%s.md", basename, currentDate)
	docsDir := fmt.Sprintf("docs.%s/%s", basename, currentDate)

	// Create the file map
	files := map[string]string{
		docsTaskListPath: a.generateDocsTaskList(basename, currentDate, sourceFiles, docsDir, ctx.ProjectPath),
		docsIndexPath:    a.generateDocsIndex(basename, currentDate, sourceFiles, ctx.ProjectPath),
		filepath.Join(docsDir, "API-REFERENCE.md"): a.generateAPIReferenceTemplate(basename, currentDate),
		filepath.Join(docsDir, "README.md"):        a.generateREADMETemplate(basename, currentDate),
	}

	// Generate individual file documentation templates
	for _, file := range sourceFiles {
		relativePath := strings.TrimPrefix(file, ctx.ProjectPath)
		relativePath = strings.TrimPrefix(relativePath, "/")

		// Create documentation file path
		docFileName := strings.ReplaceAll(relativePath, "/", "_")
		docFileName = strings.ReplaceAll(docFileName, ".", "_")
		docFileName = docFileName + ".md"

		docPath := filepath.Join(docsDir, "files", docFileName)
		docContent := a.generateFileDocTemplate(relativePath, basename)

		files[docPath] = docContent
	}

	// Generate messages
	messages := []string{
		fmt.Sprintf("\n📚 Documentation workflow generated!"),
		fmt.Sprintf("📋 Documentation Task List: %s", docsTaskListPath),
		fmt.Sprintf("📖 Documentation Index: %s", docsIndexPath),
		fmt.Sprintf("📁 Documentation Files Directory: %s", docsDir),
		fmt.Sprintf("📂 Files to document: %d", len(sourceFiles)),
		fmt.Sprintf("🤖 Ready for systematic documentation generation!\n"),
		fmt.Sprintf("💡 If you are an AI agent, please read the Documentation Task List above and follow all instructions to systematically document each file."),
	}

	return &ai.FlowResult{
		Files:    files,
		Messages: messages,
	}, nil
}

func (a *MultiFileDocsFlowAction) generateDocsTaskList(basename, date string, sourceFiles []string, docsDir string, projectPath string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`# 📚 Multi-File Documentation Task List

**Project:** %s
**Generated:** %s
**Files to Document:** %d

## 🎯 Documentation Workflow Overview

This workflow guides you through creating comprehensive documentation for the entire codebase. Each task builds upon the previous ones to create a complete documentation suite.

## 📋 Phase 1: Foundation Documentation (Priority 1)

### Task 1.1: Project Overview Documentation
- [ ] **Objective:** Create high-level project documentation
- [ ] **Input:** Use the main distilled codebase output
- [ ] **Action:** Update README.md template in %s/
- [ ] **Focus Areas:**
  - Project purpose and goals
  - Architecture overview
  - Key features and capabilities
  - Getting started guide
- [ ] **Estimated Time:** 2-3 hours
- [ ] **Dependencies:** None

### Task 1.2: API Reference Generation
- [ ] **Objective:** Create comprehensive API reference
- [ ] **Input:** Public APIs from all distilled files
- [ ] **Action:** Complete API-REFERENCE.md template in %s/
- [ ] **Focus Areas:**
  - All public functions, classes, interfaces
  - Parameters, return values, exceptions
  - Usage examples for each API
  - Integration patterns
- [ ] **Estimated Time:** 4-6 hours
- [ ] **Dependencies:** Task 1.1

### Task 1.3: Architecture Documentation
- [ ] **Objective:** Document system architecture and design
- [ ] **Input:** Structural analysis from complex-codebase analysis
- [ ] **Action:** Create ARCHITECTURE.md
- [ ] **Focus Areas:**
  - Component relationships and dependencies
  - Data flow diagrams
  - Design patterns used
  - Key architectural decisions
- [ ] **Estimated Time:** 3-4 hours
- [ ] **Dependencies:** Task 1.1

## 📋 Phase 2: Individual File Documentation (Priority 2)

`, basename, date, len(sourceFiles), docsDir, docsDir))

	for i, file := range sourceFiles {
		relativePath := strings.TrimPrefix(file, projectPath)
		relativePath = strings.TrimPrefix(relativePath, "/")

		sb.WriteString(fmt.Sprintf(`### Task 2.%d: Document %s
- [ ] **Objective:** Create comprehensive documentation for this file
- [ ] **Input:** Single-file distillation of %s
- [ ] **Action:** Complete documentation template for this file
- [ ] **Focus Areas:**
  - Purpose and responsibility
  - Public API documentation
  - Usage examples
  - Integration notes
- [ ] **Estimated Time:** 1-2 hours
- [ ] **Dependencies:** Phase 1 complete

`, i+1, relativePath, relativePath))
	}

	sb.WriteString(`## 📋 Phase 3: Integration Documentation (Priority 3)

### Task 3.1: Module Integration Guide
- [ ] **Objective:** Document how modules work together
- [ ] **Input:** All completed file documentation
- [ ] **Action:** Create MODULE-INTEGRATION.md
- [ ] **Focus Areas:**
  - Module dependencies and relationships
  - Data flow between components
  - Configuration and setup
  - Common integration patterns
- [ ] **Estimated Time:** 2-3 hours
- [ ] **Dependencies:** Phase 2 complete

### Task 3.2: Developer Guide
- [ ] **Objective:** Create comprehensive developer onboarding
- [ ] **Input:** All documentation created so far
- [ ] **Action:** Create DEVELOPER-GUIDE.md
- [ ] **Focus Areas:**
  - Development setup instructions
  - Code organization and conventions
  - Testing and debugging guidance
  - Contributing guidelines
- [ ] **Estimated Time:** 3-4 hours
- [ ] **Dependencies:** Task 3.1

### Task 3.3: Troubleshooting Documentation
- [ ] **Objective:** Document common issues and solutions
- [ ] **Input:** Code analysis for potential issues
- [ ] **Action:** Create TROUBLESHOOTING.md
- [ ] **Focus Areas:**
  - Common error scenarios
  - Debugging techniques
  - Performance considerations
  - FAQ section
- [ ] **Estimated Time:** 2-3 hours
- [ ] **Dependencies:** Task 3.2

## 📋 Phase 4: Quality Assurance (Priority 4)

### Task 4.1: Documentation Review
- [ ] **Objective:** Ensure documentation completeness and accuracy
- [ ] **Input:** All generated documentation
- [ ] **Action:** Systematic review and validation
- [ ] **Focus Areas:**
  - Accuracy against actual code
  - Completeness of coverage
  - Clarity and readability
  - Consistency across documents
- [ ] **Estimated Time:** 3-4 hours
- [ ] **Dependencies:** Phase 3 complete

### Task 4.2: Example Validation
- [ ] **Objective:** Verify all code examples work correctly
- [ ] **Input:** All documentation with code examples
- [ ] **Action:** Test and validate examples
- [ ] **Focus Areas:**
  - Code example accuracy
  - Runnable examples
  - Error handling examples
  - Performance examples
- [ ] **Estimated Time:** 2-3 hours
- [ ] **Dependencies:** Task 4.1

### Task 4.3: Documentation Index Update
- [ ] **Objective:** Create comprehensive documentation navigation
- [ ] **Input:** All completed documentation
- [ ] **Action:** Update DOCS-INDEX.md with final structure
- [ ] **Focus Areas:**
  - Complete table of contents
  - Cross-references between documents
  - Search tags and keywords
  - Maintenance schedule
- [ ] **Estimated Time:** 1-2 hours
- [ ] **Dependencies:** Task 4.2

## 🎯 Success Criteria

### Documentation Completeness
- [ ] All public APIs documented with examples
- [ ] All modules have purpose and integration documentation
- [ ] Architecture and design decisions documented
- [ ] Getting started guide complete and tested
- [ ] Troubleshooting section covers common issues

### Quality Standards
- [ ] All code examples tested and working
- [ ] Documentation is clear and accessible to new developers
- [ ] Cross-references and navigation work correctly
- [ ] Documentation is up-to-date with current code
- [ ] Consistent formatting and style throughout

### Maintenance Plan
- [ ] Documentation update process defined
- [ ] Responsibility for maintenance assigned
- [ ] Review schedule established
- [ ] Automation for detecting outdated docs

## 📚 Recommended AI Tools for Each Phase

### Phase 1 (Foundation)
- **Best Tool:** Gemini 2.5 Pro (1M context) for comprehensive overview
- **Prompt:** Use complex-codebase-analysis output as input
- **Focus:** High-level architecture and project understanding

### Phase 2 (Individual Files)
- **Best Tool:** ChatGPT-o3 or Claude for detailed analysis
- **Prompt:** Use single-file-docs prompt for each file
- **Focus:** Detailed API documentation and usage examples

### Phase 3 (Integration)
- **Best Tool:** Gemini 2.5 Pro for cross-component analysis
- **Prompt:** Use distilled output with integration focus
- **Focus:** Component relationships and integration patterns

### Phase 4 (Quality Assurance)
- **Best Tool:** Any AI tool for review and validation
- **Prompt:** Review documentation for accuracy and completeness
- **Focus:** Quality control and consistency checking

---

## 🚀 Getting Started

1. Begin with Phase 1 tasks to establish the foundation
2. Use AI Distiller to generate appropriate inputs for each task
3. Follow the templates provided in the docs directory
4. Complete phases sequentially for best results
5. Use the recommended AI tools for optimal output quality

**Total Estimated Time:** 25-35 hours
**Recommended Team Size:** 1-2 technical writers or developers
**Timeline:** 1-2 weeks with dedicated focus
`)

	return sb.String()
}

func (a *MultiFileDocsFlowAction) generateDocsIndex(basename, date string, sourceFiles []string, projectPath string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`# 📖 Documentation Index

**Project:** %s
**Last Updated:** %s
**Total Files:** %d

## 📚 Documentation Structure

This index provides a complete overview of all documentation generated for this project. Use this as your navigation hub for all project documentation.

### 🏗️ Foundation Documentation

| Document | Purpose | Status | Last Updated |
|----------|---------|--------|--------------|
| README.md | Project overview and getting started | 🔄 In Progress | %s |
| API-REFERENCE.md | Complete API documentation | 📝 Template Ready | %s |
| ARCHITECTURE.md | System architecture and design | 📋 Planned | TBD |
| MODULE-INTEGRATION.md | Component integration guide | 📋 Planned | TBD |
| DEVELOPER-GUIDE.md | Developer onboarding | 📋 Planned | TBD |
| TROUBLESHOOTING.md | Common issues and solutions | 📋 Planned | TBD |

### 📁 File-Level Documentation

The following files have individual documentation:

`, basename, date, len(sourceFiles), date, date))

	for _, file := range sourceFiles {
		relativePath := strings.TrimPrefix(file, projectPath)
		relativePath = strings.TrimPrefix(relativePath, "/")

		docFileName := strings.ReplaceAll(relativePath, "/", "_")
		docFileName = strings.ReplaceAll(docFileName, ".", "_")
		docFileName = docFileName + ".md"

		sb.WriteString(fmt.Sprintf("| %s | [files/%s](files/%s) | 📝 Template Ready | %s |\n",
			relativePath, docFileName, docFileName, date))
	}

	sb.WriteString(`

### 🎯 Documentation Categories

#### API Documentation
- **Public APIs:** All publicly available interfaces
- **Internal APIs:** Module-to-module interfaces
- **Configuration:** Setup and configuration options
- **Integration:** How to integrate with external systems

#### Technical Documentation
- **Architecture:** High-level system design
- **Design Patterns:** Patterns and practices used
- **Performance:** Performance characteristics and optimization
- **Security:** Security considerations and best practices

#### User Documentation
- **Getting Started:** Quick start guide
- **Tutorials:** Step-by-step learning guides
- **Examples:** Code examples and use cases
- **Troubleshooting:** Common problems and solutions

## 🔍 How to Use This Documentation

### For New Developers
1. Start with README.md for project overview
2. Review ARCHITECTURE.md to understand system design
3. Follow DEVELOPER-GUIDE.md for setup instructions
4. Use API-REFERENCE.md as your development reference

### For API Users
1. Check API-REFERENCE.md for available interfaces
2. Review code examples in individual file documentation
3. Check MODULE-INTEGRATION.md for integration patterns
4. Refer to TROUBLESHOOTING.md if issues arise

### For Contributors
1. Read DEVELOPER-GUIDE.md for contribution guidelines
2. Review ARCHITECTURE.md to understand design principles
3. Check file-level documentation for implementation details
4. Use TROUBLESHOOTING.md for debugging guidance

## 📋 Documentation Status Legend

| Symbol | Status | Description |
|--------|--------|-------------|
| ✅ | Complete | Documentation is complete and reviewed |
| 🔄 | In Progress | Currently being developed |
| 📝 | Template Ready | Template created, needs completion |
| 📋 | Planned | Scheduled for future development |
| ⚠️ | Needs Update | Outdated and requires revision |
| ❌ | Missing | Critical documentation gap |

## 🔧 Maintenance Information

### Update Schedule
- **Code Changes:** Documentation updated with each significant code change
- **Weekly Review:** Check for outdated information
- **Monthly Audit:** Comprehensive review of all documentation
- **Quarterly Planning:** Assess documentation strategy and gaps

### Responsibility Matrix
- **Developers:** Maintain file-level and API documentation
- **Technical Writers:** Maintain user guides and tutorials
- **Architects:** Maintain architecture and design documentation
- **Product Team:** Maintain project overview and getting started guides

### Quality Standards
- All code examples must be tested and working
- Documentation must be updated within 1 week of code changes
- New features require documentation before release
- All public APIs must have complete documentation

---

## 🚀 Quick Links

### Essential Documents
- [Project README](README.md)
- [API Reference](API-REFERENCE.md)
- [Developer Guide](DEVELOPER-GUIDE.md)
- [Architecture Overview](ARCHITECTURE.md)

### Development Resources
- [Module Integration Guide](MODULE-INTEGRATION.md)
- [Troubleshooting Guide](TROUBLESHOOTING.md)
- [File Documentation Directory](files/)

### External Resources
- [Project Repository](../)
- [Issue Tracker](../issues)
- [Contribution Guidelines](../CONTRIBUTING.md)
- [License Information](../LICENSE)
`)

	return sb.String()
}

func (a *MultiFileDocsFlowAction) generateAPIReferenceTemplate(basename, date string) string {
	return fmt.Sprintf(`# 📚 API Reference

**Project:** %s
**Last Updated:** %s

## 🎯 Overview

This document provides a comprehensive reference for all public APIs in %s. Each API is documented with usage examples, parameters, return values, and integration guidance.

## 📋 API Categories

### Core APIs
- **Description:** Fundamental APIs that provide core functionality
- **Stability:** Stable - breaking changes will include migration path
- **Usage Pattern:** Primary interfaces for most use cases

### Utility APIs
- **Description:** Helper functions and utilities
- **Stability:** Stable - minor changes may occur
- **Usage Pattern:** Supporting functionality and convenience methods

### Configuration APIs
- **Description:** Configuration and setup interfaces
- **Stability:** Evolving - may change with new features
- **Usage Pattern:** Initialization and customization

### Integration APIs
- **Description:** Interfaces for external system integration
- **Stability:** Evolving - dependent on external system changes
- **Usage Pattern:** Connecting with external services and data sources

## 🔍 API Documentation Format

Each API is documented using this standard format:

### APIName
**Purpose:** Brief description of what this API does
**Stability:** Stable | Evolving | Experimental
**Category:** Core | Utility | Configuration | Integration

#### Signature
`+"```language"+`
functionName(param1: Type1, param2: Type2) -> ReturnType
`+"```"+`

#### Parameters
- **param1** (Type1): Description of parameter 1
- **param2** (Type2): Description of parameter 2

#### Return Value
- **ReturnType**: Description of what is returned

#### Exceptions
- **ExceptionType1**: When this exception occurs
- **ExceptionType2**: When this exception occurs

#### Example Usage
`+"```language"+`
// Example showing how to use this API
const result = APIName(value1, value2);
console.log(result);
`+"```"+`

#### Related APIs
- [RelatedAPI1](#relatedapi1): How they work together
- [RelatedAPI2](#relatedapi2): Alternative approaches

---

## 🚀 Getting Started with APIs

### Basic Usage Pattern
`+"```language"+`
// 1. Initialize
const instance = new MainAPI(config);

// 2. Configure
instance.configure(options);

// 3. Use core functionality
const result = instance.process(data);

// 4. Handle results
if (result.success) {
    console.log('Success:', result.data);
} else {
    console.error('Error:', result.error);
}
`+"```"+`

### Error Handling Pattern
`+"```language"+`
try {
    const result = api.dangerousOperation(data);
    return result;
} catch (APIException e) {
    console.error('API Error:', e.message);
    return fallbackValue;
} catch (Exception e) {
    console.error('Unexpected Error:', e.message);
    throw e;
}
`+"```"+`

---

## 📚 Complete API Listing

> **Note:** This section should be populated with actual API documentation
> generated from the distilled codebase. Each public function, class, and
> interface should be documented here following the format above.

### [TO BE COMPLETED: Add actual API documentation here]

Use the AI Distiller output to generate detailed documentation for each
public API found in the codebase. Focus on:

1. **Public Functions:** All functions accessible from outside modules
2. **Public Classes:** Class constructors, public methods, and properties
3. **Interfaces:** Contract definitions and implementation requirements
4. **Types:** Custom types and their usage patterns
5. **Constants:** Public constants and configuration options

---

## 🔗 Integration Examples

### Common Integration Patterns
`+"```language"+`
// Pattern 1: Simple integration
const api = new SimpleAPI();
api.process(data);

// Pattern 2: Event-driven integration
api.on('data', (result) => handleResult(result));
api.on('error', (error) => handleError(error));

// Pattern 3: Promise-based integration
api.processAsync(data)
   .then(result => handleSuccess(result))
   .catch(error => handleError(error));
`+"```"+`

### External System Integration
`+"```language"+`
// Example: Database integration
const dbAPI = new DatabaseAPI(connectionString);
const result = await dbAPI.query(sql, parameters);

// Example: HTTP API integration
const httpAPI = new HttpAPI(baseUrl);
const response = await httpAPI.post('/endpoint', data);
`+"```"+`

---

## 📋 API Best Practices

### Usage Guidelines
- Always handle errors appropriately
- Use proper resource cleanup patterns
- Follow the recommended initialization sequence
- Check API stability before using in production

### Performance Considerations
- Batch operations when possible
- Use async APIs for I/O operations
- Cache results when appropriate
- Monitor resource usage

### Security Best Practices
- Validate all input parameters
- Use secure configuration methods
- Handle sensitive data appropriately
- Follow authentication best practices

---

## 🔄 API Versioning and Changes

### Version Policy
- **Major Versions:** Breaking changes, migration required
- **Minor Versions:** New features, backward compatible
- **Patch Versions:** Bug fixes, no API changes

### Deprecation Process
1. **Announcement:** Deprecation announced in release notes
2. **Warning Period:** Warnings added to deprecated APIs
3. **Migration Path:** Alternative APIs documented
4. **Removal:** Deprecated APIs removed after suitable period

### Migration Guides
- [Version 1.x to 2.x Migration](MIGRATION-1x-2x.md)
- [Breaking Changes Log](BREAKING-CHANGES.md)
- [Compatibility Matrix](COMPATIBILITY.md)

---

## 📞 Support and Resources

### Getting Help
- **Documentation Issues:** Report inaccuracies or gaps
- **API Questions:** Community forums and support channels
- **Bug Reports:** Issue tracker for API bugs
- **Feature Requests:** Enhancement requests and proposals

### Additional Resources
- [Developer Guide](DEVELOPER-GUIDE.md)
- [Architecture Documentation](ARCHITECTURE.md)
- [Troubleshooting Guide](TROUBLESHOOTING.md)
- [Code Examples Repository](examples/)
`, basename, date, basename)
}

func (a *MultiFileDocsFlowAction) generateREADMETemplate(basename, date string) string {
	return fmt.Sprintf(`# %s

> **Generated:** %s

## 🎯 Project Overview

[TO BE COMPLETED: Add project description, purpose, and key features]

Brief description of what this project does and why it exists. Include the main value proposition and target audience.

### Key Features
- Feature 1: Description
- Feature 2: Description  
- Feature 3: Description

### Use Cases
- Use case 1: Who would use this and why
- Use case 2: Problem this solves
- Use case 3: Integration scenarios

## 🚀 Quick Start

### Prerequisites
- Requirement 1 (version X.Y+)
- Requirement 2
- Requirement 3

### Installation
`+"```bash"+`
# Installation command
npm install %s
# or
git clone https://github.com/user/%s.git
`+"```"+`

### Basic Usage
`+"```language"+`
// Quick example showing basic usage
import { MainAPI } from '%s';

const api = new MainAPI();
const result = api.process(data);
console.log(result);
`+"```"+`

## 📚 Documentation

### Core Documentation
- [API Reference](docs/API-REFERENCE.md) - Complete API documentation
- [Developer Guide](docs/DEVELOPER-GUIDE.md) - Development setup and guidelines
- [Architecture Overview](docs/ARCHITECTURE.md) - System design and structure

### Getting Started Guides
- [Installation Guide](docs/INSTALLATION.md) - Detailed setup instructions
- [Configuration Guide](docs/CONFIGURATION.md) - Configuration options
- [Tutorial](docs/TUTORIAL.md) - Step-by-step learning guide

### Advanced Topics
- [Module Integration](docs/MODULE-INTEGRATION.md) - Component integration patterns
- [Performance Guide](docs/PERFORMANCE.md) - Optimization best practices
- [Security Guide](docs/SECURITY.md) - Security considerations

## 🏗️ Architecture

### High-Level Structure
`+"```"+`
%s/
├── core/           # Core functionality
├── utils/          # Utility functions
├── config/         # Configuration
├── integrations/   # External integrations
└── examples/       # Usage examples
`+"```"+`

### Key Components
- **Core Module:** Primary functionality and APIs
- **Utilities:** Helper functions and common operations
- **Configuration:** Setup and customization options
- **Integrations:** External system connections

## 🔧 Development

### Setup Development Environment
`+"```bash"+`
# Clone repository
git clone https://github.com/user/%s.git
cd %s

# Install dependencies
npm install

# Run tests
npm test

# Start development server
npm run dev
`+"```"+`

### Development Workflow
1. Create feature branch from main
2. Implement changes with tests
3. Run full test suite
4. Submit pull request
5. Code review and merge

### Testing
`+"```bash"+`
# Run all tests
npm test

# Run specific test suite
npm test -- --grep "component"

# Run with coverage
npm run test:coverage
`+"```"+`

## 📖 Examples

### Basic Example
`+"```language"+`
// Example 1: Basic usage
const result = api.basicOperation(input);
console.log('Result:', result);
`+"```"+`

### Advanced Example
`+"```language"+`
// Example 2: Advanced configuration
const api = new API({
    option1: 'value1',
    option2: true,
    option3: {
        nested: 'configuration'
    }
});

const result = await api.advancedOperation(complexInput);
console.log('Advanced result:', result);
`+"```"+`

### Integration Example
`+"```language"+`
// Example 3: Integration with external system
const integration = new ExternalIntegration(api);
integration.on('data', (data) => {
    console.log('Received:', data);
});

await integration.connect();
`+"```"+`

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Contribution Process
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

### Development Guidelines
- Follow existing code style
- Write comprehensive tests
- Update documentation
- Add examples for new features

## 📝 License

[License Type] - see [LICENSE](LICENSE) file for details.

## 📞 Support

### Getting Help
- **Documentation:** Check [docs/](docs/) directory
- **Issues:** Report bugs via [GitHub Issues](issues)
- **Discussions:** Community discussions and questions
- **Email:** [Contact email if available]

### Resources
- [Official Documentation](docs/)
- [API Reference](docs/API-REFERENCE.md)
- [Troubleshooting Guide](docs/TROUBLESHOOTING.md)
- [FAQ](docs/FAQ.md)

### Community
- [GitHub Repository](https://github.com/user/%s)
- [Issue Tracker](https://github.com/user/%s/issues)
- [Discussions](https://github.com/user/%s/discussions)

---

## 📊 Project Status

### Current Version
- **Version:** X.Y.Z
- **Status:** [Alpha/Beta/Stable]
- **Last Updated:** %s

### Roadmap
- [ ] Feature 1 (Version X.Y)
- [ ] Feature 2 (Version X.Z)
- [ ] Feature 3 (Future)

### Metrics
- **Test Coverage:** XX%%
- **Documentation Coverage:** XX%%
- **Performance Benchmarks:** [Link to benchmarks]

---

> **Note:** This README is generated from the AI Distiller multi-file documentation workflow. 
> Please update the placeholders with actual project information and maintain this document 
> as the project evolves.
`, basename, date, basename, basename, basename, basename, basename, basename, basename, basename, basename, date)
}

func (a *MultiFileDocsFlowAction) generateFileDocTemplate(filePath, basename string) string {
	return fmt.Sprintf(`# 📄 File Documentation: %s

**Project:** %s
**File Path:** %s
**Documentation Type:** Individual File Analysis

## 📋 File Overview

### Purpose
[TO BE COMPLETED: Describe what this file does and its role in the project]

### Responsibilities
[TO BE COMPLETED: List the main responsibilities of this file]
- Responsibility 1
- Responsibility 2
- Responsibility 3

### Dependencies
[TO BE COMPLETED: List key dependencies and why they're needed]
- Dependency 1: Purpose
- Dependency 2: Purpose
- Dependency 3: Purpose

## 🔍 Public API

### Classes
[TO BE COMPLETED: Document all public classes]

#### ClassName
**Purpose:** What this class does
**Usage Pattern:** How it's typically used

`+"```language"+`
// Constructor and basic usage
const instance = new ClassName(parameters);
`+"```"+`

**Public Methods:**
- `+"`"+`methodName(params)`+"`"+`: Description
- `+"`"+`methodName2(params)`+"`"+`: Description

### Functions
[TO BE COMPLETED: Document all public functions]

#### functionName
**Purpose:** What this function does
**Parameters:** 
- param1 (Type): Description
- param2 (Type): Description

**Returns:** Return type and description

`+"```language"+`
// Usage example
const result = functionName(value1, value2);
`+"```"+`

### Constants/Variables
[TO BE COMPLETED: Document public constants]

- `+"`"+`CONSTANT_NAME`+"`"+`: Description and usage
- `+"`"+`VARIABLE_NAME`+"`"+`: Description and usage

## 🔧 Implementation Details

### Internal Architecture
[TO BE COMPLETED: Describe internal structure]

### Key Algorithms
[TO BE COMPLETED: Explain important algorithms]

### Design Patterns
[TO BE COMPLETED: Note any design patterns used]

## 🧪 Testing

### Test Coverage
[TO BE COMPLETED: Document testing approach]

### Example Tests
`+"```language"+`
// Example test case
test('should handle valid input', () => {
    const result = functionName(validInput);
    expect(result).toBe(expectedOutput);
});
`+"```"+`

## 🔗 Integration

### How This File Integrates
[TO BE COMPLETED: Explain how this fits with other components]

### Usage Patterns
[TO BE COMPLETED: Common ways this file is used]

`+"```language"+`
// Common usage pattern 1
import { MainClass } from './%s';
const instance = new MainClass();
`+"```"+`

### Dependencies Required
[TO BE COMPLETED: What other files/modules this depends on]

## ⚠️ Important Notes

### Performance Considerations
[TO BE COMPLETED: Performance implications]

### Security Considerations
[TO BE COMPLETED: Security aspects to be aware of]

### Known Limitations
[TO BE COMPLETED: Current limitations or constraints]

## 📚 Related Documentation

- [API Reference](../API-REFERENCE.md)
- [Architecture Overview](../ARCHITECTURE.md)
- [Integration Guide](../MODULE-INTEGRATION.md)

---

> **Note:** This documentation should be completed using the single-file-docs AI action
> with the distilled content of %s as input. Use the template above as a guide for
> comprehensive file documentation.
`, filePath, basename, filePath, filePath, filePath)
}
