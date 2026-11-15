# AI Distiller (aid) MCP Server

## 🎯 The AI Coding Challenge: Understanding Existing Code

When coding with AI (vibe coding), the biggest challenge is **understanding existing code**. AI assistants often write broken code on the first attempt and need multiple iterations to fix it. Why? Because they don't know the actual APIs, data types, and contracts in your codebase.

To save context, AI agents prefer **searching/grepping code** and showing only a few lines around matches. This approach is slow, often misses relevant information, and provides incomplete understanding of code structure and relationships.

**AI Distiller solves this** by extracting complete, precise code structure instantly. Simply tell your AI: *"Distill and study the public interfaces from ./src/components"* - and watch your AI write correct code on the first try!

---

AI Distiller MCP server uses [AI Distiller](https://github.com/janreges/ai-distiller) - Essential code structure extractor that provides LLMs with accurate code signatures, data types, and API contracts from your actual codebase.

> **Note:** This is the very first version of this tool. We would be very grateful for any feedback in the form of a discussion or by creating an issue on [GitHub](https://github.com/janreges/ai-distiller/issues). Thank you!

## 📋 Two Main Ways to Use AI Distiller MCP

### 1. **Code Distillation for Better Context**
Extract essential code structure from files/directories to provide AI with accurate API signatures, types, and contracts. This dramatically improves AI's understanding of your codebase and leads to more accurate code generation.

**Example**: *"Distill ./src/components to understand the component APIs before we modify them"*

### 2. **AI-Powered Analysis Actions**
Generate specialized analysis prompts and workflows that guide AI agents through comprehensive code analysis tasks like security audits, refactoring suggestions, or documentation generation.

**Example**: *"Use aid to analyze security vulnerabilities in ./api"* or *"Generate refactoring suggestions for ./legacy"*

**⚠️ IMPORTANT for AI Actions**: AI Distiller **generates analysis prompts with distilled code** - it does NOT perform the actual analysis! The output is a specialized prompt + distilled code that AI agents (like Claude Code, Cursor) or users can execute. For large codebases, you can copy the output to tools like Gemini 2.5 with 1M context window.

**💡 NOTE**: When using AI actions, AI Distiller tries to instruct the AI agent to execute the prompt and analysis immediately after generation. However, the AI agent may or may not follow this instruction automatically. If the analysis doesn't start automatically, simply ask your AI agent to "execute the generated prompt" or "perform the analysis from the aid output".

> This is the first version of this tool and its possibilities of use are very extensive. Apply it to your use-cases, be playful and inventive, and send any bugs or feature requests to [GitHub issues](https://github.com/janreges/ai-distiller/issues). We'll be implementing more useful features in future versions. One of them is the possibility to define your own API token to Gemini or ChatGPT/Claude and let external LLM perform the analysis itself. MCPs have relatively low limits on I/O size and using external LLMs via API would bring additional benefits.

## 🚀 Quick Start with Claude Code

### Option 1: Project-specific installation (Recommended)
```bash
# Install for current project only
claude mcp add aid -- npx -y @janreges/ai-distiller-mcp
```

### Option 2: User-wide installation with manual configuration
```bash
# Install globally for all projects
claude mcp add --scope=user aid -- npx -y @janreges/ai-distiller-mcp
```

**⚠️ Important for user-wide installation:**
You must set `AID_ROOT` environment variable in your Claude Code configuration to point to your current project directory. Without this, the aid tool won't know which directory to analyze.

Example configuration:
```json
{
  "mcpServers": {
    "aid": {
      "command": "npx",
      "args": ["-y", "@janreges/ai-distiller-mcp"],
      "env": {
        "AID_ROOT": "/absolute/path/to/your/project"
      }
    }
  }
}
```

## ✨ Features

### 🔑 How AI Distiller Works

AI Distiller (aid) **DOES NOT perform analysis** - it generates prompts with distilled code for AI agents to use:
1. **aid extracts code structure** (distillation)
2. **Generates specialized AI prompts** for your analysis goal
3. **Outputs prompt + distilled code** to `.aid/` directory or stdout
4. **You or AI agents execute the prompts** to perform actual analysis

**Important**: The output is always a prompt with distilled code - NOT the analysis itself!

### 📋 Typical Workflow

1. **User asks**: "Find bugs in my authentication module with aid"
2. **Claude calls**: `aid_hunt_bugs({ target_path: "src/auth/" })`
3. **aid generates**: Bug-hunting prompt + distilled code → `.aid/bug-hunting.md`
4. **Claude reads**: The generated file and follows the prompt instructions
5. **Result**: Claude performs the actual bug analysis based on the prompt

**Alternative workflow for large codebases**:
- Generate the prompt with aid
- Copy the output to tools like Gemini 2.5 (1M context window)
- Let the external AI perform deep analysis on large codebases

**Note**: AI Distiller only generates prompts - the actual analysis is performed by AI agents!

### 💾 Output Formats

- **Small analyses**: Return directly via stdout
- **Large analyses**: Save to `.aid/` directory as markdown files
- **File naming**: `.aid/ACTION.TIMESTAMP.FOLDER.md`
- **Content**: AI prompt + distilled code in one file
- **Tool usage**: AI agents call these tools via 'aid' command

### 🎯 Specialized AI Analysis Tools (New!)
- **aid_hunt_bugs** - Generates bug-hunting prompt + distilled code (NOT the bugs themselves!)
- **aid_suggest_refactoring** - Creates refactoring prompt + distilled code (NOT the refactoring!)
- **aid_generate_diagram** - Produces diagram generation prompt + distilled code (NOT diagrams!)
- **aid_analyze_security** - Generates security audit prompt + distilled code (NOT the audit!)
- **aid_generate_docs** - Creates documentation prompt + distilled code (NOT the docs!)

### Core Analysis Engine
- **aid_analyze** - Direct access to all AI actions for custom workflows
- Supports 10 different AI analysis types
- Outputs to `.aid/` directory for large analyses

### Code Structure Tools
- **distill_file** - Extract code structure from a single file
- **distill_directory** - Extract structure from entire directories
- Control visibility: public, protected, internal, private
- Control detail: with or without implementation

## Configuration

### Environment Variables

- `AID_ROOT` - **REQUIRED for user-scoped installation**: Root directory for analysis. Without this, the tool cannot determine which project to analyze.
- `AID_MCP_TIMEOUT_MS` - Time limit for one `aid` run, in milliseconds (default: `120000`). When it is hit, `aid` is stopped and the tool returns an error saying the run did not finish; narrow the request or raise the limit.
- `AID_MCP_MAX_OUTPUT_BYTES` - Cap on the stdout of one `aid` run, in bytes (default: `1000000`). When it is hit, `aid` is stopped and the result is returned marked incomplete, with the notice first and last.

A value that is not a positive integer falls back to the default.

### Working Directory Behavior

⚠️ **Critical Information:**
- **Project-scoped installation** (without `--scope=user`): Automatically uses the current project directory
- **User-scoped installation** (with `--scope=user`): MUST set `AID_ROOT` to your project path

### Claude Desktop Configuration

For user-scoped installations, you MUST configure the project path:

```json
{
  "mcpServers": {
    "aid": {
      "command": "npx",
      "args": ["-y", "@janreges/ai-distiller-mcp"],
      "env": {
        "AID_ROOT": "/absolute/path/to/your/project"  // REQUIRED!
      }
    }
  }
}
```

## 🎛️ Controlling Output Size

### Visibility Levels
Control what code elements are included:
- `include_public: true/false` - Public members (default: true)
- `include_protected: true/false` - Protected members (default: false)
- `include_internal: true/false` - Internal/package-private (default: false)
- `include_private: true/false` - Private members (default: false)

### Implementation Control
- `include_implementation: false` - Only signatures (smallest output)
- `include_implementation: true` - Full method/function bodies (largest output)

### Output Size Examples

| Configuration | Output Size | Use Case |
|--------------|-------------|----------|
| Public only, no implementation | Smallest | API documentation |
| All visibility, no implementation | Medium | Architecture overview |
| Public + implementation | Large | Detailed API analysis |
| All visibility + implementation | Largest | Deep code analysis |

### Working with Large Codebases

For large projects, aid's output may exceed AI context limits. Strategies:

1. **Target specific directories**:
   ```javascript
   aid_analyze({
     target_path: "src/auth/",  // Just auth module
     ai_action: "prompt-for-bug-hunting"
   })
   ```

2. **Use file patterns**:
   ```javascript
   aid_analyze({
     target_path: "src/",
     include_patterns: "*.py",  // Python files only
     exclude_patterns: "*test*,*mock*"
   })
   ```

3. **Progressive analysis**:
   - Start with structure only (`include_implementation: false`)
   - Then analyze specific modules with full detail

4. **Output to files**:
   - Large analyses go to `.aid/` directory
   - Can be read by AI agents or copied to tools like Gemini (1M context)

## 📖 Tool Examples

### 🎯 Specialized Tools (Recommended)

#### Generate Bug-Hunting Prompt
```javascript
// Example prompts:
// 1. "Use aid to find bugs in my authentication module"
// 2. "Run aid bug hunter on src/api/ focusing on input validation"
// 3. "Check src/services/ for memory leaks with aid, include private methods"

// Claude will call:
aid_hunt_bugs({
  target_path: "src/auth/",
  focus_area: "memory leaks and race conditions",
  include_private: true,
  include_implementation: true  // Include code bodies
})
// Returns: Bug-hunting PROMPT with distilled code (NOT bug list!)
// Output: .aid/bug-hunting-2024-06-20.auth.md
// Next step: AI agent reads this file and performs the actual bug analysis

// More examples:
aid_hunt_bugs({
  target_path: "src/api/",
  focus_area: "input validation",
  include_private: false  // Public API only
})

aid_hunt_bugs({
  target_path: "src/components/",
  include_patterns: "*.tsx",
  focus_area: "React hooks usage"
})
```

#### Generate Refactoring Prompt
```javascript
// Example prompts:
// 1. "Use aid to suggest refactoring for src/utils/ to reduce complexity"
// 2. "Run aid refactoring analysis on src/services/ for better testability"
// 3. "Apply aid to modernize legacy code in src/legacy/, show all methods"

// Claude will call:
aid_suggest_refactoring({
  target_path: "src/auth/",
  refactoring_goal: "improve testability",
  include_implementation: false  // Signatures only for overview
})
// Returns: Refactoring PROMPT with distilled code (NOT refactoring suggestions!)
// Next step: Claude reads this prompt and generates actual refactoring suggestions

// More examples:
aid_suggest_refactoring({
  target_path: "src/components/",
  refactoring_goal: "reduce complexity",
  include_implementation: true,
  include_private: true  // Include all methods
})

aid_suggest_refactoring({
  target_path: "src/database/",
  refactoring_goal: "modernize code patterns",
  include_patterns: "*.js",
  exclude_patterns: "*test*"
})
```

#### Generate Diagram Creation Prompt
```javascript
// Example prompts:
// 1. "Use aid to generate architecture diagrams for src/"
// 2. "Create data flow diagrams with aid for src/api/ endpoints"
// 3. "Run aid diagram generator on src/services/ showing class relationships"

// Claude will call:
aid_generate_diagram({
  target_path: "src/",
  diagram_focus: "authentication flow",
  include_private: true,
  include_implementation: false  // Structure only
})
// Returns: Diagram generation PROMPT with distilled code (NOT diagrams!)
// Next step: AI agent reads this and creates actual Mermaid diagrams

// More examples:
aid_generate_diagram({
  target_path: "src/api/",
  diagram_focus: "API endpoints",
  include_private: false,  // Public API only
  include_protected: true
})

aid_generate_diagram({
  target_path: "src/components/",
  diagram_focus: "component hierarchy",
  include_patterns: "*.tsx,*.jsx"
})
```

#### Generate Security Analysis Prompt
```javascript
// Example prompts:
// 1. "Use aid security analyzer on src/api/ and save to security-report.md"
// 2. "Run aid OWASP check on src/auth/ with all visibility levels"
// 3. "Execute aid security audit on src/database/ and analyze immediately"

// Claude will call:
aid_analyze_security({
  target_path: "src/api/",
  security_focus: "input validation and SQL injection",
  include_private: true,
  include_implementation: true  // Need to see actual code
})
// Returns: Security audit PROMPT with distilled code (NOT vulnerabilities!)
// Output: .aid/security-analysis-2024-06-20.api.md
// Next step: AI agent analyzes code following the prompt instructions

// More examples:
aid_analyze_security({
  target_path: "src/auth/",
  security_focus: "authentication bypass",
  include_private: true,
  include_protected: true,
  include_internal: true  // Full visibility
})

aid_analyze_security({
  target_path: "src/controllers/",
  security_focus: "XSS vulnerabilities",
  include_patterns: "*.php",
  include_implementation: true
})
```

#### Generate Documentation Prompt
```javascript
// Example prompts:
// 1. "Use aid to generate API docs for src/core/ public methods only"
// 2. "Create developer documentation with aid for src/utils/ including examples"
// 3. "Run aid docs generator on src/lib/ and write to API.md"

// Claude will call:
aid_generate_docs({
  target_path: "src/core/",
  doc_type: "api-reference",
  audience: "external developers",
  include_private: false  // Public API only
})
// Returns: Documentation generation PROMPT with distilled code (NOT docs!)
// Output: .aid/docs-api-reference-2024-06-20.core.md
// Next step: AI agent generates actual documentation from the prompt

// More examples:
aid_generate_docs({
  target_path: "src/components/",
  doc_type: "single-file-docs",
  audience: "internal team",
  include_private: true,
  include_protected: true
})

aid_generate_docs({
  target_path: "src/services/",
  doc_type: "multi-file-docs",
  include_patterns: "*.ts",
  exclude_patterns: "*.test.ts"
})
```

### Core Analysis Engine

#### Custom AI Analysis Workflows
```javascript
// Example prompts:
// 1. "Use aid to analyze src/ with deep file analysis workflow"
// 2. "Run aid performance analysis on src/services/ and execute immediately"
// 3. "Apply aid best practices check to src/utils/ with custom output path"

// For advanced users who need specific AI actions:
aid_analyze({
  ai_action: "flow-for-deep-file-to-file-analysis",
  target_path: "src/",
  user_query: "Focus on authentication and authorization patterns",
  include_patterns: "*.py,*.js"
})
// Output: .aid/flow-for-deep-file-to-file-analysis-2024-06-20.src.md

// More examples:
aid_analyze({
  ai_action: "prompt-for-performance-analysis",
  target_path: "src/api/",
  include_implementation: true,
  output_format: "md"
})

aid_analyze({
  ai_action: "prompt-for-best-practices-analysis",
  target_path: "src/components/",
  include_private: true,
  exclude_patterns: "*test*,*mock*"
})
```

Available AI actions:
- `flow-for-deep-file-to-file-analysis` - Systematic file-by-file analysis workflow
- `flow-for-multi-file-docs` - Multi-file documentation generation workflow
- `prompt-for-refactoring-suggestion` - Refactoring analysis prompt
- `prompt-for-complex-codebase-analysis` - Enterprise-grade analysis prompt
- `prompt-for-security-analysis` - Security audit prompt
- `prompt-for-performance-analysis` - Performance optimization prompt
- `prompt-for-best-practices-analysis` - Best practices evaluation prompt
- `prompt-for-bug-hunting` - Bug detection prompt
- `prompt-for-single-file-docs` - Single file documentation prompt
- `prompt-for-diagrams` - Diagram generation prompt

### Code Structure Tools

#### Extract Single File Structure
```javascript
// Example prompts:
// 1. "Use aid to extract structure from src/main.py with all visibility"
// 2. "Show me the API of src/auth/service.ts using aid, public only"
// 3. "Get aid distillation of src/utils.js including implementation"

// Claude will call:
distill_file({
  file_path: "src/auth/service.ts",
  include_private: false,      // Public API only
  include_implementation: false // Signatures only
})
// Returns: Distilled code structure directly

// More examples:
distill_file({
  file_path: "src/models/user.py",
  include_private: true,
  include_protected: true,
  include_implementation: true,  // Full code
  output_format: "json-structured"
})

distill_file({
  file_path: "src/components/Button.tsx",
  include_comments: true,
  output_format: "md"
})

// Extract only methods (no fields/properties) - great for API surface analysis
distill_file({
  file_path: "src/services/UserService.java", 
  include_fields: false,     // Exclude all fields
  include_methods: true,     // Keep methods (default)
  output_format: "text"
})

// Extract only data structure (no method noise)
distill_file({
  file_path: "src/models/Config.ts",
  include_fields: true,      // Keep fields (default)
  include_methods: false,    // Exclude methods
  include_implementation: false
})
```

#### Extract Directory Structure
```javascript
// Example prompts:
// 1. "Use aid to distill entire src/services/ directory"
// 2. "Extract structure from src/api/ with aid, exclude tests"
// 3. "Run aid on src/components/ showing protected members"

// Claude will call:
distill_directory({
  directory_path: "src/services/",
  include_private: false,
  include_implementation: false,
  recursive: true
})
// Returns: Distilled structure for all files

// More examples:
distill_directory({
  directory_path: "src/api/",
  include_patterns: "*.ts,*.js",
  exclude_patterns: "*test*,*spec*",
  include_protected: true,
  include_internal: true
})

distill_directory({
  directory_path: "src/components/",
  include_patterns: "*.tsx",
  include_private: true,
  include_implementation: true,
  output_format: "jsonl"  // One JSON per file
})
```

## 🤖 AI Agent Workflows

### Quick Bug Hunt
```
User: "Use aid to check src/ for potential bugs"
1. Claude calls aid_hunt_bugs(target_path="src/", include_private=true)
2. aid generates bug-hunting PROMPT + distilled code → .aid/bug-hunting-*.md
3. Claude reads the generated prompt file
4. Claude executes the prompt instructions to find actual bugs
5. Claude provides bug findings and fixes
```

### Security Audit
```
User: "Run aid security audit on src/api/ endpoints and analyze"
1. Claude uses aid_analyze_security(target_path="src/api/", include_implementation=true)
2. Reads .aid/security-analysis-*.md file
3. Reviews OWASP-categorized findings
4. Suggests security improvements
```

### Architecture Understanding
```
User: "Help me understand src/ architecture using aid diagrams"
1. Claude uses aid_generate_diagram(target_path="src/")
2. Reads .aid/diagrams-*.md file
3. Presents 10 different architectural views
4. Explains key components and relationships
```

### Refactoring Session
```
User: "Apply aid refactoring analysis to src/services/ for complexity"
1. Claude uses aid_suggest_refactoring(target_path="src/services/", refactoring_goal="reduce complexity")
2. Reads .aid/refactoring-suggestion-*.md file
3. Provides specific refactoring suggestions
4. Shows before/after code examples
```

## 🚀 Advanced Features

- **Pattern Filtering** - Include/exclude specific file patterns
- **Visibility Control** - Include/exclude private, protected, internal members
- **Implementation Details** - Optionally include function bodies
- **Multi-format Output** - text, markdown, JSON formats
- **Caching** - Intelligent caching for repeated analyses
- **Direct CLI Integration** - Each tool directly calls the aid binary

## 📚 Supported Languages

Python, TypeScript, JavaScript, Go, Java, C#, Rust, Ruby, Swift, Kotlin, PHP, C++, C

## 🔧 Troubleshooting

If you encounter issues:

1. **Check aid binary**: Ensure `aid` is in your PATH or build directory
2. **Verify permissions**: The server needs read access to your project
3. **Check logs**: Run with verbose logging to see detailed errors
4. **File patterns**: Use comma-separated patterns like `*.go,*.py`

## 📄 License

MIT License - see [LICENSE](LICENSE) file

---

*AI Distiller (aid) - https://github.com/janreges/ai-distiller*  
*Authored by [Claude Code](https://claude.ai/code) & [Ján Regeš](https://github.com/janreges) from [SiteOne](https://www.siteone.io/) (Czech Republic)*  
*Explore the project on [GitHub](https://github.com/janreges/ai-distiller)*