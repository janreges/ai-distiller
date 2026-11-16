import * as fs from "fs";
import * as path from "path";

/**
 * A path parameter that names a place outside AID_ROOT. Its message names the
 * parameter, AID_ROOT and how to widen it; repeating the call cannot succeed.
 */
export class OutsideRootError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "OutsideRootError";
  }
}

/** A path parameter checked against AID_ROOT. */
export interface ConfinedPath {
  /** The real absolute path: symlinks resolved, `.` and `..` gone. */
  real: string;
  /**
   * The argument for aid, which runs in AID_ROOT: the real path relative to
   * the real AID_ROOT ("." for the root itself, "./-x" for a name that starts
   * with "-", so aid never reads it as a flag).
   */
  arg: string;
}

/**
 * A path parameter that names nothing checkable: ".." after a part that does
 * not resolve to an existing directory, or an empty value. Repeating the call
 * fails the same way.
 */
export class UncheckablePathError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "UncheckablePathError";
  }
}

/**
 * Splits the absolute path p at its longest prefix that the operating system
 * resolves: real is that prefix's real path (symlinks and every `..` in it
 * resolved by the OS), rest the components after it, unresolved, in order.
 * rest is empty when all of p resolves.
 */
function splitAtLongestPrefix(p: string): { real: string; rest: string[] } {
  let prefix = p;
  const rest: string[] = [];
  for (;;) {
    try {
      return { real: fs.realpathSync.native(prefix), rest };
    } catch {
      const parent = path.dirname(prefix);
      if (parent === prefix) return { real: prefix, rest };
      rest.unshift(path.basename(prefix));
      prefix = parent;
    }
  }
}

/**
 * The real path of p, an absolute path without `..` after a part that does
 * not resolve (a normalised AID_ROOT): the real path of its longest prefix
 * that exists, with the rest appended.
 */
export function realPathOfLongestPrefix(p: string): string {
  const { real, rest } = splitAtLongestPrefix(p);
  return path.join(real, ...rest);
}

/** Whether target lies in dir or is dir; both are absolute real paths. */
function isInside(target: string, dir: string): boolean {
  const rel = path.relative(dir, target);
  return rel === "" || !(rel === ".." || rel.startsWith(".." + path.sep) || path.isAbsolute(rel));
}

/**
 * Resolves the value of the path parameter param against root (AID_ROOT, or
 * the server's working directory when it is not set) and checks that it lies
 * inside root, both with symlinks resolved. A relative value is relative to
 * root.
 *
 * The operating system resolves the longest prefix of the value that it can;
 * the rest does not resolve for anyone (a missing name, a file, a dangling or
 * looping symlink), so aid cannot follow a symlink in it either, and when it
 * holds no "..", it can only name a place below that prefix. A ".." in the
 * rest would cancel a part that the check never resolved ("nope/../out" with
 * a symlink out), so such a value is refused (UncheckablePathError), as is an
 * empty value, which would otherwise name all of root. Throws
 * OutsideRootError for a path outside root; a path inside root that does not
 * exist passes, and aid or list_files report it.
 */
export function confinePath(value: string, root: string, param: string, rootFromEnv: boolean): ConfinedPath {
  if (value.includes("\0")) {
    throw new Error(`${param} contains a NUL character, which no file path can hold. Pass the path without it.`);
  }
  const rootAbs = path.resolve(root);
  const rootReal = realPathOfLongestPrefix(rootAbs);
  if (value === "") {
    throw new UncheckablePathError(
      `${param} is empty. Name a file or directory inside AID_ROOT (${rootReal}); a relative path is ` +
      `resolved against it, and "." is AID_ROOT itself.`);
  }
  // Not normalised before the OS resolves it: "link/.." is the parent of the
  // symlink's target, not the directory that holds the symlink.
  const joined = path.isAbsolute(value) ? value : rootAbs + path.sep + value;
  const { real: prefixReal, rest } = splitAtLongestPrefix(joined);
  const dotdot = rest.indexOf("..");
  if (dotdot >= 0) {
    const unresolved = path.join(prefixReal, ...rest.slice(0, dotdot));
    throw new UncheckablePathError(
      `${param} "${value}" has ".." after ${unresolved}, which is not an existing directory, so the server ` +
      `cannot tell where the path leads and refuses it; this call fails the same way when repeated. Pass ` +
      `the path without that "..", naming the file or directory you mean directly (a relative path is ` +
      `resolved against AID_ROOT, ${rootReal}).`);
  }
  const real = path.join(prefixReal, ...rest);
  if (!isInside(real, rootReal)) {
    const where = rootFromEnv ? `AID_ROOT (${rootDescription(rootAbs, rootReal)})` :
      `AID_ROOT, which is not set, so the server's working directory (${rootDescription(rootAbs, rootReal)})`;
    const resolved = real === value ? "" : ` resolves to ${real}, which`;
    throw new OutsideRootError(
      `${param} "${value}"${resolved} is outside ${where}. This server reads and lists only files under ` +
      `AID_ROOT, so this call fails the same way when repeated. Pass a path inside ${rootReal}; a relative ` +
      `path is resolved against it. To work with files elsewhere, the MCP server must be started with ` +
      `AID_ROOT set to a directory that contains them (an env setting of the server in the client's MCP ` +
      `configuration, then a restart).`);
  }
  const rel = path.relative(rootReal, real);
  const arg = rel === "" ? "." : rel.startsWith("-") ? `.${path.sep}${rel}` : rel;
  return { real, arg };
}

function rootDescription(rootAbs: string, rootReal: string): string {
  return rootAbs === rootReal ? rootAbs : `${rootAbs}, real path ${rootReal}`;
}
