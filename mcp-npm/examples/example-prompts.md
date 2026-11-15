# Example Prompts for Claude with AI Distiller MCP

This guide shows how to effectively use AI Distiller (aid) MCP server. Remember: **aid generates AI prompts and distilled code** - it doesn't analyze code directly. The generated prompts can then be executed by AI agents or copied to other AI tools.

## 🔑 Understanding How aid Works

1. **aid generates AI prompts** with distilled code attached
2. **Output goes to**:
   - the tool response, for the distill tools (`distill_file`, `distill_directory`)
   - a markdown file under `.aid/`, for the AI action tools; the response states its path
3. **AI agents or users** then execute these prompts with the distilled code

Each tool accepts only the parameters listed in [`../README.md`](../README.md#all-registered-tools); any other key fails the call with an error that names it.

## 🎯 Using Specialized Tools (Recommended)

### Bug Hunting
> "Can you ask aid to generate an AI prompt that will help find potential bugs and memory leaks in my Python application?"

What happens:
- `aid_hunt_bugs` generates a specialized prompt for bug detection
- The prompt includes distilled code structure
- Claude can then execute this prompt to find actual bugs

Example with specific scope:
> "Generate a bug-hunting prompt for the src/auth module only, including private methods"

```
aid_hunt_bugs({
  target_path: "src/auth/",
  include_private: true  // Private, protected and internal members (the default)
})
```

### Refactoring Analysis
> "Can aid generate a refactoring analysis prompt for our payment module? Signatures only, to keep the context small."

```
aid_suggest_refactoring({
  target_path: "src/payment/",
  include_implementation: false  // Only signatures, no method bodies
})
```

The goal of the refactoring (for example "reduce complexity and improve testability") is not a tool parameter: tell it to the agent that executes the generated prompt.

### Architecture Visualization
> "Generate a prompt to create architecture diagrams for the services/ directory, without test files."

```
aid_generate_diagram({
  target_path: "services/",
  exclude_patterns: "*test*"
})
```

`aid_generate_diagram` takes only the path and the file patterns; a focus such as "microservice communication" goes into your request to the agent that executes the prompt.

### Security Analysis
> "Create a security analysis prompt for our API handlers. Include full implementation since we need to check for injection vulnerabilities."

```
aid_analyze_security({
  target_path: "api/handlers/",
  include_implementation: true,  // Need to see actual code
  include_patterns: "*.py",
  exclude_patterns: "*test*"
})
```

## 📊 Controlling Output Size

### Minimal Context (Public APIs Only)
> "Distill the models/ directory, public interfaces only"

```
distill_directory({
  directory_path: "models/",
  include_private: false,
  include_protected: false,
  include_internal: false,
  include_implementation: false
})
```

### Medium Context (All Visibility, No Implementation)
> "Create an analysis prompt with all class members but no method bodies"

```
aid_analyze({
  ai_action: "prompt-for-complex-codebase-analysis",
  target_path: "src/",
  include_private: true,
  include_protected: true,
  include_internal: true,
  include_implementation: false  // Signatures only
})
```

### Full Context (Everything)
> "Generate a deep analysis prompt with complete code for the auth module"

```
aid_analyze({
  ai_action: "flow-for-deep-file-to-file-analysis",
  target_path: "src/auth/",
  include_private: true,
  include_implementation: true  // Full method bodies
})
```

## 🚀 Working with Large Codebases

### Strategy 1: Narrow the Scope
> "Generate analysis prompt for ONLY the database connection module"

```
aid_analyze({
  ai_action: "prompt-for-performance-analysis",
  target_path: "src/db/connection.py",  // Single file
  include_implementation: true
})
```

### Strategy 2: Use Pattern Filtering
> "Create prompt for TypeScript files only in the frontend"

```
aid_analyze({
  ai_action: "prompt-for-refactoring-suggestion",
  target_path: "frontend/",
  include_patterns: "*.ts,*.tsx",
  exclude_patterns: "*.test.ts,*.spec.ts"
})
```

### Strategy 3: Progressive Analysis
> "Start with structure only, then deep-dive into specific modules"

First pass - structure only:
```
distill_directory({
  directory_path: "src/",
  include_implementation: false,
  output_format: "md"
})
```

Then specific module with full details:
```
aid_analyze({
  ai_action: "prompt-for-bug-hunting",
  target_path: "src/problematic_module/",
  include_private: true,
  include_implementation: true
})
```

## 💡 Understanding Visibility Options

### Public Only (Smallest Output)
```
include_private: false,
include_protected: false,
include_internal: false
```

### All Members, No Code (Medium Output)
```
include_private: true,
include_protected: true,
include_internal: true,
include_implementation: false
```

### Everything (Largest Output)
```
include_private: true,
include_protected: true,
include_internal: true,
include_implementation: true
```

## 📁 Working with Generated Files

### Reading Generated Prompts
> "aid generated a file at .aid/BUG-HUNTING-ANALYSIS.2026-11-15.10-30-00.src.md. Can you read and execute it?"

Claude will:
1. Read the generated file containing the AI prompt
2. Execute the prompt with the attached distilled code
3. Provide analysis results

### Using Prompts in Other AI Tools
> "Generate a security analysis prompt that I can copy to Gemini"

The generated file can be:
- Copied and pasted into Gemini 2.5 Pro/Flash (supports 1M context)
- Used with any AI tool that supports large contexts
- Shared with team members for collaborative analysis

## 🎯 Complete Workflow Example

> "I need to find performance issues in our order processing system:
> 1. First, show me the structure of the order/ directory
> 2. Generate a performance analysis prompt for the order/processing/ subdirectory with full implementation
> 3. If the output is too large, focus only on the OrderProcessor class"

Step 1 - Overview:
```
distill_directory({
  directory_path: "order/",
  include_implementation: false
})
```

Step 2 - Detailed analysis:
```
aid_analyze({
  ai_action: "prompt-for-performance-analysis",
  target_path: "order/processing/",
  include_implementation: true
})
```

Step 3 - If needed, narrow focus:
```
distill_file({
  file_path: "order/processing/OrderProcessor.py",
  include_implementation: true
})
```

## 📋 Quick Reference

| Goal | Visibility Settings | Context Size |
|------|-------------------|--------------|
| API Documentation | Public only, no implementation | Smallest |
| Bug Hunting | All visibility + implementation | Largest |
| Architecture Overview | All visibility, no implementation | Medium |
| Security Audit | Private + implementation | Large |
| Quick Structure Check | Public only, no implementation | Smallest |

## 🔍 Pro Tips

1. **Start small**: Begin with public APIs only, then add visibility levels as needed
2. **Target specific directories**: Instead of analyzing entire codebase, focus on specific modules
3. **Use exclude patterns**: Skip test files, mocks, and generated code
4. **Check file size**: Generated prompts in `.aid/` directory should fit within your AI's context window
5. **Iterate**: Start with structure-only analysis, then deep-dive into specific areas

---

*AI Distiller (aid) - Generates AI prompts with distilled code for large-scale analysis*  
*Learn more at [github.com/janreges/ai-distiller](https://github.com/janreges/ai-distiller)*