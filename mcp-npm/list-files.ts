import * as fs from "fs";
import * as path from "path";

export const MAX_LIST_ENTRIES = 2000;

export interface ListedFile {
  path: string;
  size: number;
  modified: string;
  language: string;
  extension: string;
}

export interface ListResult {
  path: string;
  file_count: number;
  total_size: number;
  languages: Record<string, number>;
  files: ListedFile[];
  incomplete?: true;
}

const LANGUAGE_BY_EXTENSION: Record<string, string> = {
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

export function detectLanguage(ext: string): string | null {
  return LANGUAGE_BY_EXTENSION[ext] || null;
}

function matchesPattern(filePath: string, patternStr?: string): boolean {
  if (!patternStr) return true;
  const safePattern = patternStr
    .replace(/[.+^${}()|[\]\\]/g, '\\$&')
    .replace(/\*/g, '.*')
    .replace(/\?/g, '.');
  try {
    return new RegExp(`^${safePattern}$`).test(filePath);
  } catch {
    return false;
  }
}

/**
 * Lists files under basePath. A base path that is missing or not a directory is an
 * error (an empty project would be a wrong answer); unreadable subdirectories are skipped.
 */
export async function listFiles(
  basePath: string,
  opts: { pattern?: string; recursive?: boolean; maxEntries?: number }
): Promise<ListResult> {
  const maxEntries = opts.maxEntries ?? MAX_LIST_ENTRIES;
  const recursive = opts.recursive !== false;
  let isDir = false;
  try {
    isDir = (await fs.promises.stat(basePath)).isDirectory();
  } catch {
    isDir = false;
  }
  if (!isDir) throw new Error(`Path does not exist or is not a directory: ${basePath}`);

  const files: ListedFile[] = [];
  let incomplete = false;

  async function scan(dir: string): Promise<void> {
    try {
      const items = await fs.promises.readdir(dir, { withFileTypes: true });
      for (const item of items) {
        if (files.length >= maxEntries) {
          incomplete = true;
          return;
        }
        const fullPath = path.join(dir, item.name);
        const relativePath = path.relative(basePath, fullPath);
        if (item.isFile()) {
          if (matchesPattern(relativePath, opts.pattern)) {
            const stats = await fs.promises.stat(fullPath);
            const ext = path.extname(item.name).toLowerCase();
            files.push({
              path: relativePath,
              size: stats.size,
              modified: stats.mtime.toISOString(),
              language: detectLanguage(ext) || 'unknown',
              extension: ext
            });
          }
        } else if (item.isDirectory() && recursive && !item.name.startsWith('.')) {
          await scan(fullPath);
        }
      }
    } catch {
      // Skip directories we can't read
    }
  }

  await scan(basePath);

  const languages: Record<string, number> = {};
  for (const f of files) languages[f.language] = (languages[f.language] || 0) + 1;
  const result: ListResult = {
    path: basePath,
    file_count: files.length,
    total_size: files.reduce((sum, f) => sum + f.size, 0),
    languages,
    files
  };
  if (incomplete) result.incomplete = true;
  return result;
}

/** Tool text for a listing: compact JSON; an incomplete one gets its notice first and last. */
export function listFilesText(result: ListResult, maxEntries: number): string {
  const json = JSON.stringify(result);
  if (!result.incomplete) return json;
  const notice = `[List incomplete: stopped at ${maxEntries} entries. This is not the whole list. Pass a subdirectory or a pattern.]`;
  return `${notice}\n${json}\n${notice}`;
}
