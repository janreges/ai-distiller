#!/usr/bin/env node

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import type { Transport, TransportSendOptions } from "@modelcontextprotocol/sdk/shared/transport.js";
import { InitializeResultSchema, type JSONRPCMessage } from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";
import * as path from "path";
import * as os from "os";
import * as fs from "fs";
import { runAid, limitsFromEnv, AidRunError, SerialQueue } from "./run-aid.js";
import { aiActionText, withIncompleteNotice, truncationNotice, spawnFailureMessage } from "./tool-text.js";
import { listFiles, listFilesText, MAX_LIST_ENTRIES } from "./list-files.js";
import { confinePath } from "./confine.js";

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

// Check if binary exists; the server cannot work without it
function requireBinary(): void {
  if (fs.existsSync(binaryPath)) return;
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

/** Ends the description of every path parameter: the server reads only under AID_ROOT (confinePath). */
const IN_ROOT = " (inside the server root AID_ROOT; a relative path is resolved against it)";

/**
 * Input schemas of the specialized tools and aid_analyze, exported for the tests.
 * Like every tool schema in this file they are strict objects: an unknown or
 * mistyped key is rejected with a -32602 error that names it, and the advertised
 * JSON schema says additionalProperties: false.
 */
export const TOOL_SCHEMAS = {
  aid_hunt_bugs: z.strictObject({
    target_path: z.string().describe("Path to file or directory to analyze for bugs" + IN_ROOT),
    include_private: z.boolean().optional().describe("Include private members in analysis (default: true)"),
    include_patterns: z.string().optional().describe("File patterns to include"),
    exclude_patterns: z.string().optional().describe("File patterns to exclude")
  }),
  aid_suggest_refactoring: z.strictObject({
    target_path: z.string().describe("Path to file or directory to analyze" + IN_ROOT),
    include_implementation: z.boolean().optional().describe("Include implementation details in analysis (default: true)"),
    include_patterns: z.string().optional().describe("File patterns to include"),
    exclude_patterns: z.string().optional().describe("File patterns to exclude")
  }),
  aid_generate_diagram: z.strictObject({
    target_path: z.string().describe("Path to file or directory to visualize" + IN_ROOT),
    include_patterns: z.string().optional().describe("File patterns to include"),
    exclude_patterns: z.string().optional().describe("File patterns to exclude")
  }),
  aid_analyze_security: z.strictObject({
    target_path: z.string().describe("Path to file or directory to analyze" + IN_ROOT),
    include_private: z.boolean().optional().describe("Include private members in analysis (default: true)"),
    include_implementation: z.boolean().optional().describe("Include implementation details (default: true)"),
    include_patterns: z.string().optional().describe("File patterns to include"),
    exclude_patterns: z.string().optional().describe("File patterns to exclude")
  }),
  aid_generate_docs: z.strictObject({
    target_path: z.string().describe("Path to file or directory to document" + IN_ROOT),
    doc_type: z.enum(["single-file-docs", "multi-file-docs", "api-reference"]).optional().describe("Type of documentation to generate"),
    include_patterns: z.string().optional().describe("File patterns to include"),
    exclude_patterns: z.string().optional().describe("File patterns to exclude")
  }),
  aid_analyze: z.strictObject({
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
    target_path: z.string().describe("Path to analyze" + IN_ROOT),
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
  }),
} satisfies Record<string, z.ZodObject>;

/**
 * AI Distiller MCP Server
 * Executes aid commands with queuing to handle concurrency
 */
export class AidDistillerServer {
  /** The SDK server with every tool registered; exported so tests can connect a client to it. */
  readonly server: McpServer;
  private queue = new SerialQueue();
  /** The protocol version of the last initialize answer sent (connectTransport). */
  private negotiatedProtocolVersion: string | undefined;

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


  /** AID_ROOT, or the server's working directory when it is not set. */
  private root(): string {
    return process.env.AID_ROOT || process.cwd();
  }

  /**
   * The aid argument for the path parameter param: value resolved against AID_ROOT with
   * symlinks resolved, relative to the real root. A path outside AID_ROOT fails the call
   * with an error that names AID_ROOT (confinePath), before aid runs.
   */
  private pathArg(param: string, value: string): string {
    return confinePath(value, this.root(), param, Boolean(process.env.AID_ROOT)).arg;
  }

  /**
   * Executes aid command with queuing to handle concurrency
   */
  private executeAidCommand(args: string[]): Promise<string> {
    if (isDebug) console.error(`[AID MCP] executeAidCommand called with args: ${JSON.stringify(args)}`);
    return this.queue.run(() => this.runAidCommand(args));
  }

  /**
   * Runs aid command and returns output
   */
  private async runAidCommand(args: string[]): Promise<string> {
    const cwd = process.env.AID_ROOT || process.cwd();
    const limits = limitsFromEnv(process.env);
    if (isDebug) {
      console.error(`[AID MCP] Running command: ${binaryPath} ${JSON.stringify(args)}`);
      console.error(`[AID MCP] Working directory: ${cwd}`);
      console.error(`[AID MCP] Limits: ${JSON.stringify(limits)}`);
    }

    try {
      const { stdout, stderr, truncated } = await runAid(binaryPath, args, { cwd, ...limits });
      // Put stderr (progress/info) first, then the actual content
      let result = stderr.trim() ? stderr + '\n\n' : '';
      result += stdout;
      return truncated ? withIncompleteNotice(result, truncationNotice(limits.maxOutputBytes)) : result;
    } catch (error) {
      if (error instanceof AidRunError) {
        console.error(`[AID MCP] aid run failed (${error.kind}): ${error.message}`);
        if (error.kind === 'exit') throw new Error(error.stderr || error.message);
        if (error.kind === 'spawn') throw new Error(spawnFailureMessage(binaryPath, cwd, error.message));
      }
      throw error;
    }
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
        inputSchema: z.strictObject({
          file_path: z.string().describe("Path to the file to distill" + IN_ROOT),
          include_private: z.boolean().optional().describe("Include private members (default: false)"),
          include_protected: z.boolean().optional().describe("Include protected members (default: false)"),
          include_internal: z.boolean().optional().describe("Include internal/package-private members (default: false)"),
          include_implementation: z.boolean().optional().describe("Include function/method bodies (default: false)"),
          include_comments: z.boolean().optional().describe("Include comments (default: false)"),
          include_fields: z.boolean().optional().describe("Include fields/properties (default: true)"),
          include_methods: z.boolean().optional().describe("Include methods/functions (default: true)"),
          expand: z.string().optional().describe("Keep full implementation ONLY for symbols whose name matches these globs (comma-separated, e.g. 'GetUser,*Service*'). Matches a function/method name anywhere; matching a type name expands the methods nested inside that type's body (works for languages that nest methods under the type, e.g. Python/TypeScript/Java/C#). For languages where methods are file-level with a receiver (e.g. Go), match the method name. Signatures everywhere else."),
          output_format: z.enum(["text", "md", "jsonl", "json-structured", "xml"]).optional().describe("Output format (default: text)")
        })
      },
      async (params) => {
        if (isDebug) console.error(`[AID MCP] distill_file called with params: ${JSON.stringify(params)}`);
        const args = [this.pathArg('file_path', params.file_path), '--stdout', '--show-ai-agent-instructions'];
        
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
        inputSchema: z.strictObject({
          directory_path: z.string().describe("Path to the directory to distill" + IN_ROOT),
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
          expand: z.string().optional().describe("Keep full implementation ONLY for symbols whose name matches these globs (comma-separated, e.g. 'GetUser,*Service*'). Matches a function/method name anywhere; matching a type name expands the methods nested inside that type's body (works for languages that nest methods under the type, e.g. Python/TypeScript/Java/C#). For languages where methods are file-level with a receiver (e.g. Go), match the method name. Signatures everywhere else."),
          output_format: z.enum(["text", "md", "jsonl", "json-structured", "xml"]).optional().describe("Output format (default: text)")
        })
      },
      async (params) => {
        if (isDebug) console.error(`[AID MCP] distill_directory called with params: ${JSON.stringify(params)}`);
        const args = [this.pathArg('directory_path', params.directory_path), '--stdout', '--show-ai-agent-instructions'];
        
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
        description: "EXPERIMENTAL (regex-based, may miss or over-include calls): follows function/method calls across files up to the specified depth and returns distilled code of only what the target file appears to use.\n\nUSAGE: Perfect for understanding code execution flows, impact analysis, and creating focused context for AI assistants. Particularly useful for large codebases where you need to understand how specific functionality works across multiple files.",
        inputSchema: z.strictObject({
          file_path: z.string().describe("Target file to analyze" + IN_ROOT),
          max_depth: z.number().default(2).describe("Maximum depth to follow dependencies (default: 2)"),
          include_private: z.boolean().optional().describe("Include private members (default: false)"),
          include_protected: z.boolean().optional().describe("Include protected members (default: false)"),
          include_internal: z.boolean().optional().describe("Include internal/package-private members (default: false)"),
          include_implementation: z.boolean().optional().describe("Include function/method bodies (default: false)"),
          include_comments: z.boolean().optional().describe("Include comments (default: false)"),
          include_fields: z.boolean().optional().describe("Include fields/properties (default: true)"),
          include_methods: z.boolean().optional().describe("Include methods/functions (default: true)"),
          expand: z.string().optional().describe("Keep full implementation ONLY for symbols whose name matches these globs (comma-separated, e.g. 'GetUser,*Service*'). Matches a function/method name anywhere; matching a type name expands the methods nested inside that type's body (works for languages that nest methods under the type, e.g. Python/TypeScript/Java/C#). For languages where methods are file-level with a receiver (e.g. Go), match the method name. Signatures everywhere else."),
          output_format: z.enum(["text", "md", "jsonl", "json-structured", "xml"]).optional().describe("Output format (default: text)")
        })
      },
      async (params) => {
        if (isDebug) console.error(`[AID MCP] distill_with_dependencies called with params: ${JSON.stringify(params)}`);
        const args = [this.pathArg('file_path', params.file_path), '--dependency-aware', '--stdout', '--show-ai-agent-instructions'];
        
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
        inputSchema: TOOL_SCHEMAS.aid_hunt_bugs
      },
      async (params) => {
        const args = [
          this.pathArg('target_path', params.target_path),
          '--ai-action=prompt-for-bug-hunting'
        ];
        
        if (params.include_private !== false) args.push('--private=1', '--protected=1', '--internal=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: aiActionText('Bug hunting prompt requested.', 'AI agents should read this file and follow the instructions to perform bug analysis.', result, process.env.AID_ROOT || process.cwd())
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_suggest_refactoring",
      {
        title: "Suggest Code Refactoring Opportunities",
        description: "Generates a refactoring analysis prompt with distilled code for AI agents to identify specific refactoring opportunities. Use when you need AI to suggest improvements for code quality, readability, maintainability, or performance.\n\nOUTPUT: Generates a markdown file with refactoring prompt and distilled code. The response includes the file path - AI agents should read this file and follow instructions to provide refactoring suggestions with before/after examples.",
        inputSchema: TOOL_SCHEMAS.aid_suggest_refactoring
      },
      async (params) => {
        const args = [
          this.pathArg('target_path', params.target_path),
          '--ai-action=prompt-for-refactoring-suggestion'
        ];
        
        if (params.include_implementation !== false) args.push('--implementation=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: aiActionText('Refactoring prompt requested.', 'AI agents should read this file and follow the instructions to provide refactoring suggestions.', result, process.env.AID_ROOT || process.cwd())
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_generate_diagram",
      {
        title: "Generate Architecture Diagrams",
        description: "Generates a diagram creation prompt with distilled code for AI agents to create architectural diagrams in Mermaid format. Use when you need AI to generate flowcharts, sequence diagrams, class diagrams, and architecture overviews.\n\nOUTPUT: Generates a markdown file with diagram generation prompt and distilled code. The response includes the file path - AI agents should read this file and follow instructions to create 10+ Mermaid diagrams.",
        inputSchema: TOOL_SCHEMAS.aid_generate_diagram
      },
      async (params) => {
        const args = [
          this.pathArg('target_path', params.target_path),
          '--ai-action=prompt-for-diagrams'
        ];
        
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: aiActionText('Diagram generation prompt requested.', 'The file contains prompts for generating 10 different architectural diagrams in Mermaid format.', result, process.env.AID_ROOT || process.cwd())
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_analyze_security",
      {
        title: "Perform Security Analysis",
        description: "Generates a security analysis prompt with distilled code for AI agents to perform comprehensive security audits with OWASP Top 10 focus. Use when you need AI to identify vulnerabilities, security anti-patterns, and weak points.\n\nOUTPUT: Generates a markdown file with security audit prompt and distilled code. The response includes the file path - AI agents should read this file and follow instructions to analyze security vulnerabilities and suggest remediation.",
        inputSchema: TOOL_SCHEMAS.aid_analyze_security
      },
      async (params) => {
        const args = [
          this.pathArg('target_path', params.target_path),
          '--ai-action=prompt-for-security-analysis'
        ];
        
        if (params.include_private !== false) args.push('--private=1', '--protected=1', '--internal=1');
        if (params.include_implementation !== false) args.push('--implementation=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: aiActionText('Security analysis prompt requested.', 'AI agents should read this file and follow the instructions to perform security analysis with OWASP Top 10 focus.', result, process.env.AID_ROOT || process.cwd())
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_generate_docs",
      {
        title: "Generate Documentation",
        description: "Generates documentation creation prompts with distilled code for AI agents to create comprehensive documentation including API references, usage examples, and developer guides. Use when you need AI to generate technical documentation from code.\n\nOUTPUT: Generates markdown files with documentation prompts and distilled code. The response includes file paths - AI agents should read these files and follow instructions to create the actual documentation.",
        inputSchema: TOOL_SCHEMAS.aid_generate_docs
      },
      async (params) => {
        let aiAction = 'prompt-for-single-file-docs';
        if (params.doc_type === 'multi-file-docs' || params.doc_type === 'api-reference') {
          aiAction = 'flow-for-multi-file-docs';
        }
        
        const args = [
          this.pathArg('target_path', params.target_path),
          `--ai-action=${aiAction}`
        ];
        
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: aiActionText('Documentation generation prompt requested.', 'AI agents should read these files and follow the instructions to create the documentation.', result, process.env.AID_ROOT || process.cwd())
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
        inputSchema: z.strictObject({
          target_path: z.string().describe("Path to analyze" + IN_ROOT),
          include_private: z.boolean().optional().describe("Include all visibility levels (default: true)"),
          include_implementation: z.boolean().optional().describe("Include implementation details (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        })
      },
      async (params) => {
        const args = [
          this.pathArg('target_path', params.target_path),
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
            text: aiActionText('Deep file-by-file analysis workflow.', 'AI agents should read the task list and follow all instructions to systematically analyze each file.', result, process.env.AID_ROOT || process.cwd())
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_multi_file_docs",
      {
        title: "Multi-File Documentation Workflow",
        description: "Creates documentation workflow prompts with file relationships. Generates structured prompts that guide AI agents to create interconnected documentation covering multiple files, their relationships, and overall system architecture.\n\nOUTPUT: Generates workflow files that AI agents can follow to create comprehensive documentation with proper cross-references.",
        inputSchema: z.strictObject({
          target_path: z.string().describe("Path to document" + IN_ROOT),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        })
      },
      async (params) => {
        const args = [
          this.pathArg('target_path', params.target_path),
          '--ai-action=flow-for-multi-file-docs'
        ];
        
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: aiActionText('Multi-file documentation workflow.', 'The workflow includes prompts for creating interconnected documentation with proper cross-references.', result, process.env.AID_ROOT || process.cwd())
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_complex_analysis",
      {
        title: "Complex Codebase Analysis",
        description: "Enterprise-grade analysis prompt with full codebase context. Generates comprehensive prompts for architecture analysis, compliance checks, and detailed findings. Best suited for large codebases requiring deep architectural insights.\n\nOUTPUT: Generates analysis prompt with distilled code that AI agents can use to create architecture diagrams, identify patterns, and provide strategic recommendations.",
        inputSchema: z.strictObject({
          target_path: z.string().describe("Path to analyze" + IN_ROOT),
          include_private: z.boolean().optional().describe("Include all visibility levels (default: true)"),
          include_implementation: z.boolean().optional().describe("Include implementation details (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        })
      },
      async (params) => {
        const args = [
          this.pathArg('target_path', params.target_path),
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
            text: aiActionText('Complex codebase analysis prompt.', 'The prompt includes guidance for creating architecture diagrams and strategic recommendations.', result, process.env.AID_ROOT || process.cwd())
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_performance_analysis",
      {
        title: "Performance Analysis",
        description: "Performance optimization prompt with complexity focus. Generates analysis prompts that guide AI agents to identify performance bottlenecks, analyze algorithmic complexity, and suggest optimization strategies.\n\nOUTPUT: Generates performance analysis prompt focusing on scalability issues, resource usage, and optimization opportunities.",
        inputSchema: z.strictObject({
          target_path: z.string().describe("Path to analyze" + IN_ROOT),
          include_implementation: z.boolean().optional().describe("Include implementation details (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        })
      },
      async (params) => {
        const args = [
          this.pathArg('target_path', params.target_path),
          '--ai-action=prompt-for-performance-analysis'
        ];
        
        if (params.include_implementation !== false) args.push('--implementation=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: aiActionText('Performance analysis prompt.', 'The analysis focuses on identifying bottlenecks and optimization opportunities.', result, process.env.AID_ROOT || process.cwd())
          }]
        };
      }
    );

    this.server.registerTool(
      "aid_best_practices",
      {
        title: "Best Practices Analysis",
        description: "Code quality prompt with industry standards. Generates analysis prompts that guide AI agents to assess code against best practices, design patterns, and clean code principles.\n\nOUTPUT: Generates best practices analysis prompt covering code quality metrics, pattern usage, and improvement suggestions.",
        inputSchema: z.strictObject({
          target_path: z.string().describe("Path to analyze" + IN_ROOT),
          include_private: z.boolean().optional().describe("Include all visibility levels (default: true)"),
          include_patterns: z.string().optional().describe("File patterns to include"),
          exclude_patterns: z.string().optional().describe("File patterns to exclude")
        })
      },
      async (params) => {
        const args = [
          this.pathArg('target_path', params.target_path),
          '--ai-action=prompt-for-best-practices-analysis'
        ];
        
        if (params.include_private !== false) args.push('--private=1', '--protected=1', '--internal=1');
        if (params.include_patterns) args.push(`--include=${params.include_patterns}`);
        if (params.exclude_patterns) args.push(`--exclude=${params.exclude_patterns}`);
        
        const result = await this.executeAidCommand(args);
        
        return {
          content: [{
            type: "text",
            text: aiActionText('Best practices analysis prompt.', 'The analysis covers code quality, design patterns, and clean code principles.', result, process.env.AID_ROOT || process.cwd())
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
        inputSchema: TOOL_SCHEMAS.aid_analyze
      },
      async (params) => {
        const args = [
          this.pathArg('target_path', params.target_path),
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
            text: aiActionText('AI action ' + params.ai_action + '.', 'AI agents should read the file and follow the instructions within to perform the analysis.', result, process.env.AID_ROOT || process.cwd())
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
        description: "Lists project files with language detection and statistics. Stops after 2000 entries and says so; pass a subdirectory or a pattern to narrow it. Useful for exploring project structure before distillation.",
        inputSchema: z.strictObject({
          path: z.string().optional().describe("Directory to list (default: the server root AID_ROOT); it must lie inside AID_ROOT, and a relative path is resolved against it"),
          pattern: z.string().optional().describe("File pattern to match"),
          recursive: z.boolean().optional().describe("List recursively (default: true)")
        })
      },
      async (params) => {
        const basePath = confinePath(params.path || ".", this.root(), 'path', Boolean(process.env.AID_ROOT)).real;
        const result = await listFiles(basePath, { pattern: params.pattern, recursive: params.recursive });
        
        return {
          content: [{
            type: "text",
            text: listFilesText(result, MAX_LIST_ENTRIES)
          }]
        };
      }
    );

    this.server.registerTool(
      "get_capabilities",
      {
        title: "Get AI Distiller Capabilities",
        description: "Returns comprehensive information about AI Distiller capabilities, supported languages, and available tools.",
        inputSchema: z.strictObject({})
      },
      async () => {
        const capabilities = {
          server_name: "AI Distiller MCP",
          server_version: pkg.version,
          // The version of this connection's initialize answer; absent before one was sent.
          protocol_version: this.negotiatedProtocolVersion,
          root_path: process.env.AID_ROOT || process.cwd(),
          tools: {
            core: [
              "distill_file - Extract structure from single files",
              "distill_directory - Extract structure from directories",
              "distill_with_dependencies - EXPERIMENTAL regex-based dependency-aware distillation"
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
   * Connects the server to transport and records the protocol version of every
   * initialize answer the server sends. The SDK negotiates the version (the
   * client's when supported, else its latest) and keeps no public record of it,
   * so the answer on the wire is the only source that cannot disagree with what
   * the client was told.
   */
  async connectTransport(transport: Transport): Promise<void> {
    const send = transport.send.bind(transport);
    transport.send = (message: JSONRPCMessage, options?: TransportSendOptions) => {
      if ("result" in message) {
        const answer = InitializeResultSchema.safeParse(message.result);
        if (answer.success) {
          this.negotiatedProtocolVersion = answer.data.protocolVersion;
          if (isDebug) console.error(`[AID MCP] Protocol version negotiated: ${answer.data.protocolVersion}`);
        }
      }
      return send(message, options);
    };
    await this.server.connect(transport);
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
      
      await this.connectTransport(transport);

      if (isDebug) {
        console.error(`[AID MCP] Server connected successfully
Binary: ${binaryPath}
Working directory: ${process.env.AID_ROOT || process.cwd()}`);
      }
    } catch (error) {
      console.error('[AID MCP] Failed to connect transport:', error);
      process.exit(1);
    }
  }
}

/** Starts the stdio server. Called when this file is run directly, never on import. */
export function startServer(): void {
  requireBinary();

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
}

if (require.main === module) {
  startServer();
}
