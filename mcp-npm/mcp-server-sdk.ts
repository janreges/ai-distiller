#!/usr/bin/env node

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";
import { spawn } from "child_process";
import * as path from "path";
import * as os from "os";
import * as fs from "fs";

// Debug logging control
const isDebug = process.env.DEBUG === 'true' || process.env.DEBUG === '1' || process.env.AID_MCP_DEBUG === 'true';

// Load package.json dynamically
const packageJsonPath = path.join(__dirname, '..', 'package.json');
const pkg = JSON.parse(fs.readFileSync(packageJsonPath, 'utf-8'));

// Get the path to the aid binary
const binaryName = os.platform() === 'win32' ? 'aid.exe' : 'aid';
const binaryPath = path.join(__dirname, '..', 'bin', binaryName);

// Debug logging for troubleshooting
if (isDebug) {
  console.error(`[AID MCP] Binary path: ${binaryPath}`);
  console.error(`[AID MCP] __dirname: ${__dirname}`);
}

// Check if binary exists
if (!fs.existsSync(binaryPath)) {
  console.error(`Error: AI Distiller binary not found at ${binaryPath}`);
  console.error('');
  console.error('This usually means the postinstall script failed to download the binary.');
  console.error('');
  console.error('To fix this issue, try one of the following:');
  console.error('1. Check the installation logs above for any download errors');
  console.error('2. Reinstall the package: npm install @janreges/ai-distiller-mcp');
  console.error('3. Run the postinstall script manually:');
  console.error(`   cd ${path.join(__dirname, '..')} && npm run postinstall`);
  console.error('4. Download the binary manually from:');
  console.error('   https://github.com/janreges/ai-distiller/releases/latest');
  console.error(`   and place it at: ${binaryPath}`);
  console.error('');
  console.error('Platform info:', process.platform, process.arch);
  process.exit(1);
}

// Types
interface QueuedRequest {
  args: string[];
  resolve: (value: string) => void;
  reject: (error: Error) => void;
}

// Removed unused interface

/**
 * AI Distiller MCP Server
 * Executes aid commands with queuing to handle concurrency
 */
class AidDistillerServer {
  private server: McpServer;
  private requestQueue: QueuedRequest[] = [];
  private isProcessing = false;

  constructor() {
    this.server = new McpServer({
      name: "AI Distiller MCP",
      version: pkg.version
    });

    this.registerCoreTools();
    this.registerSpecializedTools();
    this.registerAIActionTools();
    this.registerLegacyTools();
    this.registerMetaTools();
  }


  /**
   * Executes aid command with queuing to handle concurrency
   */
  private async executeAidCommand(args: string[]): Promise<string> {
    if (isDebug) console.error(`[AID MCP] executeAidCommand called with args: ${JSON.stringify(args)}`);
    return new Promise((resolve, reject) => {
      this.requestQueue.push({ args, resolve, reject });
      this.processQueue();
    });
  }

  /**
   * Processes the request queue serially
   */
  private async processQueue(): Promise<void> {
    if (this.isProcessing || this.requestQueue.length === 0) {
      return;
    }

    this.isProcessing = true;
    const request = this.requestQueue.shift()!;
    if (isDebug) console.error(`[AID MCP] processQueue: Processing request with args: ${JSON.stringify(request.args)}`);

    try {
      const result = await this.runAidCommand(request.args);
      request.resolve(result);
    } catch (error) {
      if (isDebug) console.error(`[AID MCP] processQueue: Error processing request: ${error}`);
      request.reject(error as Error);
    } finally {
      this.isProcessing = false;
      // Process next in queue
      this.processQueue();
    }
  }

  /**
   * Runs aid command and returns output
   */
  private runAidCommand(args: string[]): Promise<string> {
    return new Promise((resolve, reject) => {
      // Don't add --summary-type=off anymore - we want to capture stderr
      const fullCommand = `${binaryPath} ${args.map(arg => `"${arg}"`).join(' ')}`;
      if (isDebug) {
        console.error(`[AID MCP] Running command: ${fullCommand}`);
        console.error(`[AID MCP] Binary path: ${binaryPath}`);
        console.error(`[AID MCP] Arguments: ${JSON.stringify(args)}`);
        console.error(`[AID MCP] Working directory: ${process.env.AID_ROOT || process.cwd()}`);
      }
      
      const child = spawn(binaryPath, args, {
        cwd: process.env.AID_ROOT || process.cwd(),
        env: process.env,
        stdio: ['ignore', 'pipe', 'pipe'] // Explicitly set stdio: ignore stdin, pipe stdout/stderr
      });
      
      let stdout = '';
      let stderr = '';
      
      child.stdout.on('data', (data) => {
        const chunk = data.toString();
        stdout += chunk;
        if (isDebug) console.error(`[AID MCP] Stdout chunk (${chunk.length} bytes): ${chunk.substring(0, 100)}...`);
      });
      
      child.stderr.on('data', (data) => {
        const chunk = data.toString();
        stderr += chunk;
        if (isDebug) console.error(`[AID MCP] Stderr chunk: ${chunk}`);
      });
      
      child.on('close', (code) => {
        if (isDebug) {
          console.error(`[AID MCP] Command exited with code: ${code}`);
          console.error(`[AID MCP] Total stdout length: ${stdout.length} bytes`);
          console.error(`[AID MCP] Total stderr length: ${stderr.length} bytes`);
        }
        
        if (code !== 0) {
          // If command failed, include stderr in error message
          reject(new Error(stderr || `Aid exited with code ${code}`));
        } else {
          // Command succeeded - combine stderr and stdout
          let result = '';
          
          // Put stderr (progress/info) first
          if (stderr && stderr.trim()) {
            result = stderr + '\n\n';
          }
          
          // Then append stdout (actual content)
          result += stdout;
          
          resolve(result);
        }
      });
      
      child.on('error', (err) => {
        console.error(`[AID MCP] Process spawn error: ${err.message}`);
        if (isDebug) console.error(`[AID MCP] Error stack: ${err.stack}`);
        reject(err);
      });
    });
  }

  /**
   * Register core distillation tools
   */
  private registerCoreTools(): void {
    // Core file distillation
    this.server.registerTool(
      "distill_file",
      {
        title: "Extract Code Structure from File",
        description: "Extracts essential code structure from a single file - the core functionality of AI Distiller. Returns clean, structured code signatures optimized for AI context windows.\n\nUSAGE: Essential for providing accurate code context to AI assistants. Automatically detects language and extracts API signatures, types, and structure while removing unnecessary implementation details.",
        inputSchema: {
          file_path: z.string().describe("Path to the file to distill"),
          include_private: z.boolean().optional().describe("Include private members (default: false)"),
          include_protected: z.boolean().optional().describe("Include protected members (default: false)"),
          include_internal: z.boolean().optional().describe("Include internal/package-private members (default: false)"),
          include_implementation: z.boolean().optional().describe("Include function/method bodies (default: false)"),
          include_comments: z.boolean().optional().describe("Include comments (default: false)"),
          include_fields: z.boolean().optional().describe("Include fields/properties (default: true)"),
          include_methods: z.boolean().optional().describe("Include methods/functions (default: true)"),
          expand: z.string().optional().describe("Keep full implementation ONLY for symbols whose name matches these globs (comma-separated, e.g. 'GetUser,*Service*'); matches a function/method name, or a class/struct name to expand all its methods. Signatures everywhere else."),
          output_format: z.enum(["text", "md", "jsonl", "json-structured", "xml"]).optional().describe("Output format (default: text)")
        }
      },
      async (params) => {
        if (isDebug) console.error(`[AID MCP] distill_file called with params: ${JSON.stringify(params)}`);
        const args = [params.file_path, '--stdout', '--show-ai-agent-instructions'];
        
        if (params.output_format) args.push(`--format=${params.output_format}`);
        if (params.include_private) args.push('--private=1');
        if (params.include_protected) args.push('--protected=1');
        if (params.include_internal) args.push('--internal=1');
        if (params.include_implementation) args.push('--implementation=1');
        if (params.include_comments) args.push('--comments=1');
        if (params.include_fields === false) args.push('--fields=0');
        if (params.include_methods === false) args.push('--methods=0');
        if (params.expand) args.push(`--expand=${params.expand}`);

        if (isDebug) console.error(`[AID MCP] distill_file final args: ${JSON.stringify(args)}`);
        const result = await this.executeAidCommand(args);
        
        // Check if result is empty
        if (!result || result.trim().length === 0) {
          if (isDebug) console.error(`[AID MCP] Warning: Empty result for file ${params.file_path}`);
          throw new Error(`No output received from aid command. File may not exist or aid binary may have issues.`);
        }
        
        if (isDebug) console.error(`[AID MCP] distill_file result length: ${result.length} bytes`);
        return {
          content: [{
            type: "text",
            text: result
          }]
        };
      }
    );

    // Core directory distillation
    this.server.registerTool(
      "distill_directory",
      {
        title: "Extract Code Structure from Directory",
        description: "Extracts essential code structure from entire directories - the core functionality of AI Distiller. Processes all supported programming languages and returns structured API/code signatures optimized for AI context windows.\n\nUSAGE: Perfect for understanding codebases, API discovery, and providing accurate code context to AI assistants. Supports filtering by visibility levels, file patterns, and content types.",
        inputSchema: {
          directory_path: z.string().describe("Path to the directory to distill"),
          recursive: z.boolean().optional().describe("Process subdirectories recursively (default: true)"),
          include_private: z.boolean().optional().describe("Include private members (default: false)"),
          include_protected: z.boolean().optional().describe("Include protected members (default: false)"),
          include_internal: z.boolean().optional().describe("Include internal/package-private members (default: false)"),
          include_implementation: z.boolean().optional().describe("Include function/method bodies (default: false)"),
          include_comments: z.boolean().optional().describe("Include comments (default: false)"),
          include_fields: z.boolean().optional().describe("Include fields/properties (default: true)"),
          include_methods: z.boolean().optional().describe("Include methods/functions (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include (comma-separated, e.g., '*.go,*.py')"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude (comma-separated, e.g., '*test*,vendor/**')"),
          expand: z.string().optional().describe("Keep full implementation ONLY for symbols whose name matches these globs (comma-separated, e.g. 'GetUser,*Service*'); matches a function/method name, or a class/struct name to expand all its methods. Signatures everywhere else."),
          output_format: z.enum(["text", "md", "jsonl", "json-structured", "xml"]).optional().describe("Output format (default: text)")
        }
      },
      async (params) => {
        if (isDebug) console.error(`[AID MCP] distill_directory called with params: ${JSON.stringify(params)}`);
        const args = [params.directory_path, '--stdout', '--show-ai-agent-instructions'];
        
        if (params.output_format) args.push(`--format=${params.output_format}`);
        if (params.recursive === false) args.push('--recursive=0');
        if (params.include_private) args.push('--private=1');
        if (params.include_protected) args.push('--protected=1');
        if (params.include_internal) args.push('--internal=1');
        if (params.include_implementation) args.push('--implementation=1');
        if (params.include_comments) args.push('--comments=1');
        if (params.include_fields === false) args.push('--fields=0');
        if (params.include_methods === false) args.push('--methods=0');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        if (params.expand) args.push(`--expand=${params.expand}`);

        if (isDebug) console.error(`[AID MCP] distill_directory final args: ${JSON.stringify(args)}`);
        const result = await this.executeAidCommand(args);
        
        // Check if result is empty
        if (!result || result.trim().length === 0) {
          if (isDebug) console.error(`[AID MCP] Warning: Empty result for directory ${params.directory_path}`);
          throw new Error(`No output received from aid command. Directory may not exist or aid binary may have issues.`);
        }
        
        if (isDebug) console.error(`[AID MCP] distill_directory result length: ${result.length} bytes`);
        return {
          content: [{
            type: "text",
            text: result
          }]
        };
      }
    );

    // Dependency-aware distillation
    this.server.registerTool(
      "distill_with_dependencies",
      {
        title: "Dependency-Aware Code Distillation",
        description: "Analyzes call dependencies and returns distilled code of only relevant methods/classes up to specified depth. This advanced feature traces function/method calls across files and includes only the code that is actually called from the target file, creating focused distillations for deep code analysis.\n\nUSAGE: Perfect for understanding code execution flows, impact analysis, and creating focused context for AI assistants. Particularly useful for large codebases where you need to understand how specific functionality works across multiple files.",
        inputSchema: {
          file_path: z.string().describe("Target file to analyze"),
          max_depth: z.number().default(2).describe("Maximum depth to follow dependencies (default: 2)"),
          include_private: z.boolean().optional().describe("Include private members (default: false)"),
          include_protected: z.boolean().optional().describe("Include protected members (default: false)"),
          include_internal: z.boolean().optional().describe("Include internal/package-private members (default: false)"),
          include_implementation: z.boolean().optional().describe("Include function/method bodies (default: false)"),
          include_comments: z.boolean().optional().describe("Include comments (default: false)"),
          include_fields: z.boolean().optional().describe("Include fields/properties (default: true)"),
          include_methods: z.boolean().optional().describe("Include methods/functions (default: true)"),
          expand: z.string().optional().describe("Keep full implementation ONLY for symbols whose name matches these globs (comma-separated, e.g. 'GetUser,*Service*'); matches a function/method name, or a class/struct name to expand all its methods. Signatures everywhere else."),
          output_format: z.enum(["text", "md", "jsonl", "json-structured", "xml"]).optional().describe("Output format (default: text)")
        }
      },
      async (params) => {
        if (isDebug) console.error(`[AID MCP] distill_with_dependencies called with params: ${JSON.stringify(params)}`);
        const args = [params.file_path, '--dependency-aware', '--stdout', '--show-ai-agent-instructions'];
        
        // Add max-depth parameter
        args.push(`--max-depth=${params.max_depth || 2}`);
        
        // Add standard distillation options
        if (params.output_format) args.push(`--format=${params.output_format}`);
        if (params.include_private) args.push('--private=1');
        if (params.include_protected) args.push('--protected=1');
        if (params.include_internal) args.push('--internal=1');
        if (params.include_implementation) args.push('--implementation=1');
        if (params.include_comments) args.push('--comments=1');
        if (params.include_fields === false) args.push('--fields=0');
        if (params.include_methods === false) args.push('--methods=0');
        if (params.expand) args.push(`--expand=${params.expand}`);

        if (isDebug) console.error(`[AID MCP] distill_with_dependencies final args: ${JSON.stringify(args)}`);
        const result = await this.executeAidCommand(args);
        
        // Check if result is empty
        if (!result || result.trim().length === 0) {
          if (isDebug) console.error(`[AID MCP] Warning: Empty result for dependency analysis of ${params.file_path}`);
          throw new Error(`No output received from dependency-aware distillation. File may not exist, may not be supported, or aid binary may have issues.`);
        }
        
        if (isDebug) console.error(`[AID MCP] distill_with_dependencies result length: ${result.length} bytes`);
        return {
          content: [{
            type: "text",
            text: result
          }]
        };
      }
    );
  }

  /**
   * Register specialized analysis tools
   */
  private registerSpecializedTools(): void {
    this.server.registerTool(
      "aid_hunt_bugs",
      {
        title: "Hunt for Bugs and Quality Issues",
        description: "Generates a bug hunting prompt with distilled code for AI agents to systematically identify potential bugs, logical errors, race conditions, and quality issues. Use when you need AI to analyze code for hidden bugs or perform a comprehensive code health check.\n\nOUTPUT: Generates a markdown file with bug hunting prompt and distilled code. The response includes the file path - AI agents should read this file and follow instructions to perform the actual bug analysis.",
        inputSchema: {
          target_path: z.string().describe("Path to file or directory to analyze for bugs"),
          focus_area: z.string().optional().describe("Specific area to focus on (e.g., 'concurrency', 'memory leaks', 'error handling')"),
          include_private: z.boolean().optional().describe("Include private members in analysis (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        }
      },
      async (params) => {
        const args = [
          params.target_path,
          '--ai-action=prompt-for-bug-hunting'
        ];
        
        if (params.include_private !== false) args.push('--private=1', '--protected=1', '--internal=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        // Extract file path from output
        const filePathMatch = result.match(/📋 Bug Analysis Prompt: (.+)/);
        const filePath = filePathMatch ? filePathMatch[1] : 'Check .aid/ directory';
        
        return {
          content: [{
            type: "text",
            text: `Bug hunting prompt generated successfully!\n\nPrompt file: ${filePath}\n\nAI agents should read this file and follow the instructions to perform bug analysis.\n\n${result}`
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_suggest_refactoring",
      {
        title: "Suggest Code Refactoring Opportunities",
        description: "Generates a refactoring analysis prompt with distilled code for AI agents to identify specific refactoring opportunities. Use when you need AI to suggest improvements for code quality, readability, maintainability, or performance.\n\nOUTPUT: Generates a markdown file with refactoring prompt and distilled code. The response includes the file path - AI agents should read this file and follow instructions to provide refactoring suggestions with before/after examples.",
        inputSchema: {
          target_path: z.string().describe("Path to file or directory to analyze"),
          refactoring_goal: z.string().describe("Goal of refactoring (e.g., 'improve readability', 'reduce complexity', 'modernize code', 'extract common patterns')"),
          include_implementation: z.boolean().optional().describe("Include implementation details in analysis (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        }
      },
      async (params) => {
        const args = [
          params.target_path,
          '--ai-action=prompt-for-refactoring-suggestion'
        ];
        
        if (params.include_implementation !== false) args.push('--implementation=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        const filePathMatch = result.match(/📋 Refactoring Analysis Prompt: (.+)/);
        const filePath = filePathMatch ? filePathMatch[1] : 'Check .aid/ directory';
        
        return {
          content: [{
            type: "text",
            text: `Refactoring prompt generated for goal: "${params.refactoring_goal}"\n\nPrompt file: ${filePath}\n\nAI agents should read this file and follow the instructions to provide refactoring suggestions.\n\n${result}`
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_generate_diagram",
      {
        title: "Generate Architecture Diagrams",
        description: "Generates a diagram creation prompt with distilled code for AI agents to create architectural diagrams in Mermaid format. Use when you need AI to generate flowcharts, sequence diagrams, class diagrams, and architecture overviews.\n\nOUTPUT: Generates a markdown file with diagram generation prompt and distilled code. The response includes the file path - AI agents should read this file and follow instructions to create 10+ Mermaid diagrams.",
        inputSchema: {
          target_path: z.string().describe("Path to file or directory to visualize"),
          diagram_focus: z.string().optional().describe("Specific diagram focus (e.g., 'data flow', 'class hierarchy', 'module dependencies', 'API endpoints')"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        }
      },
      async (params) => {
        const args = [
          params.target_path,
          '--ai-action=prompt-for-diagrams'
        ];
        
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        const filePathMatch = result.match(/📋 Diagram Generation Prompt: (.+)/);
        const filePath = filePathMatch ? filePathMatch[1] : 'Check .aid/ directory';
        
        return {
          content: [{
            type: "text",
            text: `Diagram generation prompt created!\n\nDiagram file: ${filePath}\n\nThe file contains prompts for generating 10 different architectural diagrams in Mermaid format.\n\n${result}`
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_analyze_security",
      {
        title: "Perform Security Analysis",
        description: "Generates a security analysis prompt with distilled code for AI agents to perform comprehensive security audits with OWASP Top 10 focus. Use when you need AI to identify vulnerabilities, security anti-patterns, and weak points.\n\nOUTPUT: Generates a markdown file with security audit prompt and distilled code. The response includes the file path - AI agents should read this file and follow instructions to analyze security vulnerabilities and suggest remediation.",
        inputSchema: {
          target_path: z.string().describe("Path to file or directory to analyze"),
          security_focus: z.string().optional().describe("Specific security concern (e.g., 'SQL injection', 'XSS', 'authentication', 'authorization', 'crypto')"),
          include_private: z.boolean().optional().describe("Include private members in analysis (default: true)"),
          include_implementation: z.boolean().optional().describe("Include implementation details (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        }
      },
      async (params) => {
        const args = [
          params.target_path,
          '--ai-action=prompt-for-security-analysis'
        ];
        
        if (params.include_private !== false) args.push('--private=1', '--protected=1', '--internal=1');
        if (params.include_implementation !== false) args.push('--implementation=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        const filePathMatch = result.match(/📋 Security Analysis Prompt: (.+)/);
        const filePath = filePathMatch ? filePathMatch[1] : 'Check .aid/ directory';
        
        return {
          content: [{
            type: "text",
            text: `Security analysis prompt generated!\n\nPrompt file: ${filePath}\n\nAI agents should read this file and follow the instructions to perform security analysis with OWASP Top 10 focus.\n\n${result}`
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_generate_docs",
      {
        title: "Generate Documentation",
        description: "Generates documentation creation prompts with distilled code for AI agents to create comprehensive documentation including API references, usage examples, and developer guides. Use when you need AI to generate technical documentation from code.\n\nOUTPUT: Generates markdown files with documentation prompts and distilled code. The response includes file paths - AI agents should read these files and follow instructions to create the actual documentation.",
        inputSchema: {
          target_path: z.string().describe("Path to file or directory to document"),
          doc_type: z.enum(["single-file-docs", "multi-file-docs", "api-reference"]).optional().describe("Type of documentation to generate"),
          audience: z.string().optional().describe("Target audience (e.g., 'developers', 'api-users', 'contributors', 'end-users')"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        }
      },
      async (params) => {
        let aiAction = 'prompt-for-single-file-docs';
        if (params.doc_type === 'multi-file-docs' || params.doc_type === 'api-reference') {
          aiAction = 'flow-for-multi-file-docs';
        }
        
        const args = [
          params.target_path,
          `--ai-action=${aiAction}`
        ];
        
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: `Documentation generation prompt created!\n\nTarget audience: ${params.audience || 'general'}\n\n${result}`
          }]
        };
      }
    );
  }

  /**
   * Register AI action tools for comprehensive analysis
   */
  private registerAIActionTools(): void {
    this.server.registerTool(
      "aid_deep_file_analysis",
      {
        title: "Deep File-by-File Analysis Workflow",
        description: "Generates task lists and prompts for systematic file-by-file analysis. Creates a structured workflow where AI agents analyze each file across multiple dimensions (Security, Performance, Maintainability, Readability). Perfect for comprehensive codebase reviews.\n\nOUTPUT: Generates task list, summary template, and directory structure for organizing analysis results. AI agents should read the task list and follow instructions systematically.",
        inputSchema: {
          target_path: z.string().describe("Path to analyze"),
          include_private: z.boolean().optional().describe("Include all visibility levels (default: true)"),
          include_implementation: z.boolean().optional().describe("Include implementation details (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        }
      },
      async (params) => {
        const args = [
          params.target_path,
          '--ai-action=flow-for-deep-file-to-file-analysis'
        ];
        
        if (params.include_private !== false) args.push('--private=1', '--protected=1', '--internal=1');
        if (params.include_implementation !== false) args.push('--implementation=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: `Deep file-by-file analysis workflow generated!\n\n${result}\n\n💡 AI agents should read the Task List file and follow all instructions to systematically analyze each file.`
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_multi_file_docs",
      {
        title: "Multi-File Documentation Workflow",
        description: "Creates documentation workflow prompts with file relationships. Generates structured prompts that guide AI agents to create interconnected documentation covering multiple files, their relationships, and overall system architecture.\n\nOUTPUT: Generates workflow files that AI agents can follow to create comprehensive documentation with proper cross-references.",
        inputSchema: {
          target_path: z.string().describe("Path to document"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        }
      },
      async (params) => {
        const args = [
          params.target_path,
          '--ai-action=flow-for-multi-file-docs'
        ];
        
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: `Multi-file documentation workflow generated!\n\n${result}\n\n📚 The workflow includes prompts for creating interconnected documentation with proper cross-references.`
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_complex_analysis",
      {
        title: "Complex Codebase Analysis",
        description: "Enterprise-grade analysis prompt with full codebase context. Generates comprehensive prompts for architecture analysis, compliance checks, and detailed findings. Best suited for large codebases requiring deep architectural insights.\n\nOUTPUT: Generates analysis prompt with distilled code that AI agents can use to create architecture diagrams, identify patterns, and provide strategic recommendations.",
        inputSchema: {
          target_path: z.string().describe("Path to analyze"),
          include_private: z.boolean().optional().describe("Include all visibility levels (default: true)"),
          include_implementation: z.boolean().optional().describe("Include implementation details (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        }
      },
      async (params) => {
        const args = [
          params.target_path,
          '--ai-action=prompt-for-complex-codebase-analysis'
        ];
        
        if (params.include_private !== false) args.push('--private=1', '--protected=1', '--internal=1');
        if (params.include_implementation !== false) args.push('--implementation=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: `Complex codebase analysis prompt generated!\n\n${result}\n\n🏗️ The prompt includes guidance for creating architecture diagrams and strategic recommendations.`
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_performance_analysis",
      {
        title: "Performance Analysis",
        description: "Performance optimization prompt with complexity focus. Generates analysis prompts that guide AI agents to identify performance bottlenecks, analyze algorithmic complexity, and suggest optimization strategies.\n\nOUTPUT: Generates performance analysis prompt focusing on scalability issues, resource usage, and optimization opportunities.",
        inputSchema: {
          target_path: z.string().describe("Path to analyze"),
          include_implementation: z.boolean().optional().describe("Include implementation details (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        }
      },
      async (params) => {
        const args = [
          params.target_path,
          '--ai-action=prompt-for-performance-analysis'
        ];
        
        if (params.include_implementation !== false) args.push('--implementation=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: `Performance analysis prompt generated!\n\n${result}\n\n⚡ The analysis focuses on identifying bottlenecks and optimization opportunities.`
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_best_practices",
      {
        title: "Best Practices Analysis",
        description: "Code quality prompt with industry standards. Generates analysis prompts that guide AI agents to assess code against best practices, design patterns, and clean code principles.\n\nOUTPUT: Generates best practices analysis prompt covering code quality metrics, pattern usage, and improvement suggestions.",
        inputSchema: {
          target_path: z.string().describe("Path to analyze"),
          include_private: z.boolean().optional().describe("Include all visibility levels (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        }
      },
      async (params) => {
        const args = [
          params.target_path,
          '--ai-action=prompt-for-best-practices-analysis'
        ];
        
        if (params.include_private !== false) args.push('--private=1', '--protected=1', '--internal=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: `Best practices analysis prompt generated!\n\n${result}\n\n✨ The analysis covers code quality, design patterns, and clean code principles.`
          }]
        };
      }
    );
  }

  /**
   * Register legacy tools for backward compatibility
   */
  private registerLegacyTools(): void {
    this.server.registerTool(
      "aid_analyze",
      {
        title: "Core AI Prompt Generation Engine",
        description: "Core AI Distiller prompt generation engine. Generates pre-configured prompts with distilled code for AI-driven analysis. Use specialized tools when available. This tool directly maps to aid --ai-action.\n\nIMPORTANT: This tool DOES NOT perform analysis - it generates prompt files that AI agents can then execute. The output includes file paths to the generated prompts. AI agents should read these files and follow the instructions within to perform the actual analysis.",
        inputSchema: {
          ai_action: z.enum([
            "flow-for-deep-file-to-file-analysis",
            "flow-for-multi-file-docs",
            "prompt-for-refactoring-suggestion",
            "prompt-for-complex-codebase-analysis",
            "prompt-for-security-analysis",
            "prompt-for-performance-analysis",
            "prompt-for-best-practices-analysis",
            "prompt-for-bug-hunting",
            "prompt-for-single-file-docs",
            "prompt-for-diagrams"
          ]).describe("AI action to perform"),
          target_path: z.string().describe("Path to analyze"),
          user_query: z.string().optional().describe("Additional context or specific query"),
          output_format: z.enum(["text", "md", "jsonl", "json-structured", "xml"]).optional().describe("Output format"),
          include_private: z.boolean().optional().describe("Include private members"),
          include_protected: z.boolean().optional().describe("Include protected members"),
          include_internal: z.boolean().optional().describe("Include internal/package-private members"),
          include_implementation: z.boolean().optional().describe("Include implementation"),
          include_comments: z.boolean().optional().describe("Include comments"),
          include_fields: z.boolean().optional().describe("Include fields/properties (default: true)"),
          include_methods: z.boolean().optional().describe("Include methods/functions (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        }
      },
      async (params) => {
        const args = [
          params.target_path,
          `--ai-action=${params.ai_action}`
        ];
        
        if (params.output_format) args.push(`--format=${params.output_format}`);
        if (params.include_private) args.push('--private=1');
        if (params.include_protected) args.push('--protected=1');
        if (params.include_internal) args.push('--internal=1');
        if (params.include_implementation) args.push('--implementation=1');
        if (params.include_comments) args.push('--comments=1');
        if (params.include_fields === false) args.push('--fields=0');
        if (params.include_methods === false) args.push('--methods=0');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: result
          }]
        };
      }
    );
  }

  /**
   * Register meta tools
   */
  private registerMetaTools(): void {
    this.server.registerTool(
      "list_files",
      {
        title: "List Project Files",
        description: "Lists project files with language detection and statistics. Useful for exploring project structure before distillation.",
        inputSchema: {
          path: z.string().optional().describe("Path to list (default: current directory)"),
          pattern: z.string().optional().describe("File pattern to match"),
          recursive: z.boolean().optional().describe("List recursively (default: true)")
        }
      },
      async (params) => {
        const result = await this.listFiles(params);
        
        return {
          content: [{
            type: "text",
            text: JSON.stringify(result, null, 2)
          }]
        };
      }
    );

    this.server.registerTool(
      "get_capabilities",
      {
        title: "Get AI Distiller Capabilities",
        description: "Returns comprehensive information about AI Distiller capabilities, supported languages, and available tools.",
        inputSchema: {}
      },
      async () => {
        const capabilities = {
          server_name: "AI Distiller MCP",
          server_version: pkg.version,
          protocol_version: "2025-03-26",
          root_path: process.env.AID_ROOT || process.cwd(),
          cache_dir: path.join(process.env.AID_PROJECT_ROOT || process.cwd(), '.aid/cache/mcp'),
          tools: {
            core: [
              "distill_file - Extract structure from single files",
              "distill_directory - Extract structure from directories",
              "distill_with_dependencies - Dependency-aware distillation with call graph analysis"
            ],
            specialized: [
              "aid_hunt_bugs - Systematic bug detection",
              "aid_suggest_refactoring - Code improvement suggestions",
              "aid_generate_diagram - Architecture visualization",
              "aid_analyze_security - Security vulnerability detection",
              "aid_generate_docs - Documentation generation"
            ],
            ai_workflows: [
              "aid_deep_file_analysis - File-by-file deep analysis",
              "aid_multi_file_docs - Multi-file documentation",
              "aid_complex_analysis - Enterprise-grade analysis",
              "aid_performance_analysis - Performance optimization",
              "aid_best_practices - Code quality assessment"
            ],
            legacy: [
              "aid_analyze - Generic AI action interface"
            ],
            meta: [
              "list_files - File exploration",
              "get_capabilities - This tool"
            ]
          },
          supported_languages: [
            "Python", "TypeScript", "JavaScript", "Go", "Java",
            "C#", "Rust", "Ruby", "Swift", "Kotlin", "PHP", "C++"
          ],
          supported_formats: ["text", "md", "jsonl", "json-structured", "xml"],
          ai_actions: {
            prompts: [
              "prompt-for-refactoring-suggestion",
              "prompt-for-complex-codebase-analysis",
              "prompt-for-security-analysis",
              "prompt-for-performance-analysis",
              "prompt-for-best-practices-analysis",
              "prompt-for-bug-hunting",
              "prompt-for-single-file-docs",
              "prompt-for-diagrams"
            ],
            workflows: [
              "flow-for-deep-file-to-file-analysis",
              "flow-for-multi-file-docs"
            ]
          },
          features: [
            "Request queuing for concurrent access",
            "Automatic language detection",
            "Granular visibility control",
            "Pattern-based file filtering",
            "Multiple output formats",
            "AI-powered analysis prompts",
            "Comprehensive documentation generation"
          ]
        };
        
        return {
          content: [{
            type: "text",
            text: JSON.stringify(capabilities, null, 2)
          }]
        };
      }
    );
  }

  /**
   * List files implementation
   */
  private async listFiles(args: any): Promise<any> {
    const fs = require('fs').promises;
    const basePath = path.resolve(args.path || '.');
    const pattern = args.pattern;
    const recursive = args.recursive !== false;
    
    const results: any[] = [];
    
    function matchesPattern(filePath: string, patternStr?: string): boolean {
      if (!patternStr) return true;
      
      const safePattern = patternStr
        .replace(/[.+^${}()|[\]\\]/g, '\\$&')
        .replace(/\*/g, '.*')
        .replace(/\?/g, '.');
      
      try {
        const regex = new RegExp(`^${safePattern}$`);
        return regex.test(filePath);
      } catch {
        return false;
      }
    }
    
    const detectLanguage = (ext: string): string | null => {
      return this.detectLanguage(ext);
    };

    async function scan(dir: string): Promise<void> {
      try {
        const items = await fs.readdir(dir, { withFileTypes: true });
        
        for (const item of items) {
          const fullPath = path.join(dir, item.name);
          const relativePath = path.relative(basePath, fullPath);
          
          if (item.isFile()) {
            if (!pattern || matchesPattern(relativePath, pattern)) {
              const stats = await fs.stat(fullPath);
              
              // Detect language
              const ext = path.extname(item.name).toLowerCase();
              const language = detectLanguage(ext);
              
              results.push({
                path: relativePath,
                size: stats.size,
                modified: stats.mtime.toISOString(),
                language: language || 'unknown',
                extension: ext
              });
            }
          } else if (item.isDirectory() && recursive && !item.name.startsWith('.')) {
            await scan(fullPath);
          }
        }
      } catch (err) {
        // Skip directories we can't read
      }
    }
    
    await scan(basePath);
    
    return {
      path: basePath,
      file_count: results.length,
      total_size: results.reduce((sum, f) => sum + f.size, 0),
      languages: this.summarizeLanguages(results),
      files: results
    };
  }

  /**
   * Detect language from file extension
   */
  private detectLanguage(ext: string): string | null {
    const languageMap: Record<string, string> = {
      '.py': 'python',
      '.pyw': 'python',
      '.pyi': 'python',
      '.js': 'javascript',
      '.mjs': 'javascript',
      '.cjs': 'javascript',
      '.jsx': 'javascript',
      '.ts': 'typescript',
      '.tsx': 'typescript',
      '.d.ts': 'typescript',
      '.go': 'go',
      '.rs': 'rust',
      '.rb': 'ruby',
      '.rake': 'ruby',
      '.gemspec': 'ruby',
      '.java': 'java',
      '.cs': 'csharp',
      '.kt': 'kotlin',
      '.kts': 'kotlin',
      '.cpp': 'cpp',
      '.cc': 'cpp',
      '.cxx': 'cpp',
      '.c++': 'cpp',
      '.h': 'cpp',
      '.hpp': 'cpp',
      '.hh': 'cpp',
      '.hxx': 'cpp',
      '.h++': 'cpp',
      '.php': 'php',
      '.phtml': 'php',
      '.php3': 'php',
      '.php4': 'php',
      '.php5': 'php',
      '.php7': 'php',
      '.phps': 'php',
      '.inc': 'php',
      '.swift': 'swift'
    };
    
    return languageMap[ext] || null;
  }

  /**
   * Summarize languages in file list
   */
  private summarizeLanguages(files: any[]): Record<string, number> {
    const summary: Record<string, number> = {};
    
    for (const file of files) {
      const lang = file.language || 'unknown';
      summary[lang] = (summary[lang] || 0) + 1;
    }
    
    return summary;
  }

  /**
   * Connect to transport and start server
   */
  async connect(): Promise<void> {
    try {
      const transport = new StdioServerTransport();
      
      // Set up error handlers before connecting
      transport.onclose = () => {
        if (isDebug) console.error('[AID MCP] Transport closed');
        process.exit(0);
      };
      
      transport.onerror = (error: Error) => {
        console.error('[AID MCP] Transport error:', error);
        process.exit(1);
      };
      
      await this.server.connect(transport);
      
      if (isDebug) {
        console.error(`[AID MCP] Server connected successfully
Binary: ${binaryPath}
Working directory: ${process.env.AID_ROOT || process.cwd()}
Protocol: 2025-03-26`);
      }
    } catch (error) {
      console.error('[AID MCP] Failed to connect transport:', error);
      process.exit(1);
    }
  }
}

// Handle process termination gracefully
process.on('SIGINT', () => {
  if (isDebug) console.error('[AID MCP] Received SIGINT, shutting down gracefully');
  process.exit(0);
});

process.on('SIGTERM', () => {
  if (isDebug) console.error('[AID MCP] Received SIGTERM, shutting down gracefully');
  process.exit(0);
});

process.on('uncaughtException', (error) => {
  console.error('[AID MCP] Uncaught exception:', error);
  process.exit(1);
});

process.on('unhandledRejection', (reason, promise) => {
  console.error('[AID MCP] Unhandled rejection at:', promise, 'reason:', reason);
  process.exit(1);
});

// Start the server
const server = new AidDistillerServer();
server.connect().catch(error => {
  console.error('[AID MCP] Failed to start server:', error);
  process.exit(1);
});