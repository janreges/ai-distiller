import * as fs from "fs";
import * as path from "path";

// Output lines that `aid` prints after an AI action (internal/cli/root.go):
//   prompt actions: "📄 AI prompt with distilled code saved to:" then "💾 <path> (<n> kB)"
//   flow actions:   "📄 Output saved to: <path> (<n> files, <n> kB total)"
const PROMPT_PATH = /^💾 (.+) \(\d+(?:\.\d+)? kB\)\s*$/mu;
const FLOW_PATH = /^📄 Output saved to: (.+) \(\d+ files?, \d+(?:\.\d+)? kB total\)\s*$/mu;

/** The path aid reports after an AI action, or null when the output has none. */
export function extractSavedPath(output: string): string | null {
  const match = PROMPT_PATH.exec(output) ?? FLOW_PATH.exec(output);
  return match ? match[1] : null;
}

/** Absolute directory for list_files: relative paths resolve against root. */
export function resolveBasePath(requested: string | undefined, root: string): string {
  return path.resolve(root, requested || ".");
}

const TASK_LIST = /^📋 (?:Documentation )?Task List: (.+?)\s*$/mu;

/** Absolute path of the task list a flow action reports (a bare name lives in the reported output directory). */
function taskListPath(output: string, root: string): string | null {
  const match = TASK_LIST.exec(output);
  if (!match) return null;
  const listed = match[1];
  const saved = extractSavedPath(output);
  const hasDir = listed.includes("/") || listed.includes("\\");
  return hasDir || saved === null ? path.resolve(root, listed) : path.resolve(root, saved, listed);
}

/** The tool text that tells the agent where aid wrote its output, as absolute paths. */
export function savedPathLine(output: string, root: string): string {
  const saved = extractSavedPath(output);
  if (saved === null) return "aid did not report an output file";
  const lines = [`Output file: ${path.resolve(root, saved)}`];
  const taskList = taskListPath(output, root);
  if (taskList !== null) lines.push(`Task list: ${taskList}`);
  return lines.join("\n");
}

const NOTICE_START = "[Output incomplete:";

/** Output cut at the size cap: the notice goes first, where a client that keeps only the head still shows it, and last. */
export function withIncompleteNotice(output: string, notice: string): string {
  return `${notice}\n\n${output}\n\n${notice}`;
}

/**
 * Tool text for an AI action: intro, the absolute output path, the advice (only when aid
 * reported a file) and aid's own output. A leading incomplete notice stays at the very start.
 */
export function aiActionText(intro: string, advice: string, result: string, root: string): string {
  let head = "";
  let body = result;
  if (result.startsWith(NOTICE_START)) {
    const end = result.indexOf("\n\n");
    head = result.slice(0, end) + "\n\n";
    body = result.slice(end + 2);
  }
  const reported = extractSavedPath(result) !== null;
  return `${head}${intro}\n\n${savedPathLine(result, root)}\n\n${reported ? advice + "\n\n" : ""}${body}`;
}

/** Appended to output that was cut at the size cap. True for every tool. */
export function truncationNotice(maxOutputBytes: number): string {
  return `[Output incomplete: stopped at ${maxOutputBytes} bytes. This is not the whole result. Narrow the request: pass a subdirectory or a single file, and use the file pattern parameters or turn off implementation details if this tool offers them. The limit is set by AID_MCP_MAX_OUTPUT_BYTES.]`;
}

/**
 * Message for an aid process that could not be started. Node reports ENOENT both
 * for a missing binary and for a missing working directory, so check which one is wrong.
 */
export function spawnFailureMessage(binaryPath: string, cwd: string, cause: string): string {
  if (!isDirectory(cwd)) {
    return `Could not start aid: the working directory ${cwd} does not exist (${cause}). Set AID_ROOT to an existing project directory.`;
  }
  if (!isExecutableFile(binaryPath)) {
    return `Could not start the aid binary at ${binaryPath} (${cause}). The file is missing or not executable. Reinstall the package (npm install @janreges/ai-distiller-mcp) to download it again.`;
  }
  return `Could not start the aid binary at ${binaryPath} in ${cwd} (${cause}). The binary and the directory exist, so the cause is the system error shown here.`;
}

function isDirectory(p: string): boolean {
  try {
    return fs.statSync(p).isDirectory();
  } catch {
    return false;
  }
}

function isExecutableFile(p: string): boolean {
  try {
    if (!fs.statSync(p).isFile()) return false;
    fs.accessSync(p, fs.constants.X_OK);
    return true;
  } catch {
    return false;
  }
}
