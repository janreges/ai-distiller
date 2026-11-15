import { spawn } from "child_process";

export interface RunAidOptions {
  cwd: string;
  timeoutMs: number; // kill the child and reject after this long
  maxOutputBytes: number; // stop reading stdout and kill the child at this size
  env?: NodeJS.ProcessEnv;
}

export interface RunAidResult {
  stdout: string;
  stderr: string;
  truncated: boolean;
}

export class AidRunError extends Error {
  kind: "timeout" | "exit" | "spawn";
  exitCode?: number;
  stderr: string;

  constructor(kind: "timeout" | "exit" | "spawn", message: string, stderr = "", exitCode?: number) {
    super(message);
    this.name = "AidRunError";
    this.kind = kind;
    this.stderr = stderr;
    this.exitCode = exitCode;
  }
}

const DEFAULT_TIMEOUT_MS = 120000;
const DEFAULT_MAX_OUTPUT_BYTES = 1000000;

function positiveInt(value: string | undefined, fallback: number): number {
  if (value === undefined || !/^\s*\d+\s*$/.test(value)) return fallback;
  const n = Number(value);
  return Number.isSafeInteger(n) && n > 0 ? n : fallback;
}

export function limitsFromEnv(env: NodeJS.ProcessEnv): { timeoutMs: number; maxOutputBytes: number } {
  return {
    timeoutMs: positiveInt(env.AID_MCP_TIMEOUT_MS, DEFAULT_TIMEOUT_MS),
    maxOutputBytes: positiveInt(env.AID_MCP_MAX_OUTPUT_BYTES, DEFAULT_MAX_OUTPUT_BYTES),
  };
}

/**
 * Runs the aid binary with a wall-clock timeout and a cap on stdout size.
 * A run that hits the cap resolves with truncated=true; a run that times out rejects.
 */
export function runAid(binaryPath: string, args: string[], opts: RunAidOptions): Promise<RunAidResult> {
  return new Promise((resolve, reject) => {
    const child = spawn(binaryPath, args, {
      cwd: opts.cwd,
      env: opts.env ?? process.env,
      stdio: ["ignore", "pipe", "pipe"],
    });

    const out: Buffer[] = [];
    let outBytes = 0;
    const err: Buffer[] = [];
    let settled = false;

    const stderrText = () => Buffer.concat(err).toString();
    const stdoutText = () => Buffer.concat(out).toString();

    // Kill the child and drop its pipes so that an orphaned grandchild holding
    // them open cannot keep the process (or this promise) alive.
    const killChild = () => {
      child.kill("SIGKILL");
      child.stdout?.destroy();
      child.stderr?.destroy();
    };

    const finish = (action: () => void) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      action();
    };

    const timer = setTimeout(() => {
      finish(() => {
        killChild();
        const seconds = opts.timeoutMs / 1000;
        reject(
          new AidRunError(
            "timeout",
            `aid did not finish within ${seconds} s. Narrow the request (smaller path, include_patterns) or raise AID_MCP_TIMEOUT_MS.`,
            stderrText()
          )
        );
      });
    }, opts.timeoutMs);

    child.stdout!.on("data", (data: Buffer) => {
      if (settled) return;
      const room = opts.maxOutputBytes - outBytes;
      if (data.length >= room) {
        out.push(data.subarray(0, room));
        outBytes += room;
        finish(() => {
          killChild();
          resolve({ stdout: stdoutText(), stderr: stderrText(), truncated: true });
        });
        return;
      }
      out.push(data);
      outBytes += data.length;
    });

    child.stderr!.on("data", (data: Buffer) => {
      if (!settled) err.push(data);
    });

    child.on("error", (e) => {
      finish(() => reject(new AidRunError("spawn", e.message, stderrText())));
    });

    child.on("close", (code) => {
      finish(() => {
        if (code !== 0) {
          const text = stderrText();
          reject(new AidRunError("exit", text || `aid exited with code ${code}`, text, code ?? undefined));
        } else {
          resolve({ stdout: stdoutText(), stderr: stderrText(), truncated: false });
        }
      });
    });
  });
}

/** Runs jobs one at a time; a failing job never blocks the ones behind it. */
export class SerialQueue {
  private tail: Promise<unknown> = Promise.resolve();

  run<T>(job: () => Promise<T>): Promise<T> {
    const result = this.tail.then(job, job);
    this.tail = result.catch(() => undefined);
    return result;
  }
}
