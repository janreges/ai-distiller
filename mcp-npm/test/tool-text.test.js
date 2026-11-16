const { test } = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const os = require('node:os');
const {
  extractSavedPath, savedPathLine, truncationNotice, spawnFailureMessage,
  aiActionText, withIncompleteNotice,
} = require('../dist/tool-text.js');

test('extracts the path of a prompt action', () => {
  const out = "✅ AI action 'prompt-for-bug-hunting' completed successfully! (0.12s)\n📄 AI prompt with distilled code saved to:\n💾 /p/.aid/BUG-HUNTING.2026-11-15.src.md (41.3 kB)\n";
  assert.equal(extractSavedPath(out), '/p/.aid/BUG-HUNTING.2026-11-15.src.md');
});

test('extracts the path of a flow action', () => {
  const out = "✅ AI flow action 'flow-for-multi-file-docs' completed successfully! (0.40s)\n📄 Output saved to: /p/.aid/docs (12 files, 88.0 kB total)\n";
  assert.equal(extractSavedPath(out), '/p/.aid/docs');
});

test('handles paths with spaces and parentheses', () => {
  const out = "📄 AI prompt with distilled code saved to:\n💾 /p/my dir (v2)/.aid/X.md (1.0 kB)\n";
  assert.equal(extractSavedPath(out), '/p/my dir (v2)/.aid/X.md');
});

test('returns null when nothing was saved', () => {
  assert.equal(extractSavedPath('<file path="a.py">\n</file>\n'), null);
});

test('savedPathLine resolves a relative flow path against the root', () => {
  const out = "📄 Output saved to: .aid (5 files, 24.6 kB total)\n";
  assert.equal(savedPathLine(out, '/proj'), `Output file: ${path.resolve('/proj', '.aid')}`);
});

test('savedPathLine says plainly when aid reported no file', () => {
  assert.equal(savedPathLine('nothing here', '/proj'), 'aid did not report an output file');
});

test('truncation notice names no parameter that a tool might lack, and never says retry', () => {
  const text = truncationNotice(1000);
  assert.match(text, /incomplete/);
  assert.match(text, /1000 bytes/);
  assert.doesNotMatch(text, /include_patterns|include_implementation/);
  assert.doesNotMatch(text, /retry|try again/i);
});

test('spawn failure blames a missing binary and says to reinstall', () => {
  const text = spawnFailureMessage('/nonexistent/bin/aid', os.tmpdir(), 'spawn /nonexistent/bin/aid ENOENT');
  assert.match(text, /\/nonexistent\/bin\/aid/);
  assert.match(text, /missing or not executable/);
  assert.match(text, /[Rr]einstall/);
  assert.doesNotMatch(text, /retry|try again/i);
});

test('spawn failure blames a missing working directory, not the binary', () => {
  const cwd = path.join(os.tmpdir(), 'aid-no-such-root-' + process.pid);
  const text = spawnFailureMessage(process.execPath, cwd, 'spawn ENOENT');
  assert.ok(text.includes(cwd));
  assert.match(text, /AID_ROOT/);
  assert.doesNotMatch(text, /[Rr]einstall/);
  assert.doesNotMatch(text, /retry|try again/i);
});

test('the server schemas no longer declare the ignored parameters', () => {
  const { TOOL_SCHEMAS } = require('../dist/mcp-server-sdk.js');
  const gone = {
    aid_hunt_bugs: 'focus_area', aid_suggest_refactoring: 'refactoring_goal',
    aid_generate_diagram: 'diagram_focus', aid_analyze_security: 'security_focus',
    aid_generate_docs: 'audience', aid_analyze: 'user_query',
  };
  for (const [tool, param] of Object.entries(gone)) {
    assert.ok(TOOL_SCHEMAS[tool], tool);
    assert.equal(param in TOOL_SCHEMAS[tool].shape, false, `${tool}.${param}`);
  }
});

// Real lines copied from `aid src --ai-action=... </dev/null` (aid built from this branch).
const DOCS_FLOW = [
  '📚 Documentation workflow generated!',
  '📋 Documentation Task List: DOCS-TASK-LIST.src.2026-11-15.md',
  '📖 Documentation Index: DOCS-INDEX.src.2026-11-15.md',
  "✅ AI flow action 'flow-for-multi-file-docs' completed successfully! (0.00s)",
  '📄 Output saved to: .aid (5 files, 24.5 kB total)',
].join('\n');
const DEEP_FLOW = [
  '✅ AI Analysis Task List generated successfully!',
  '📋 Task List: .aid/ANALYSIS-TASK-LIST.src.2026-11-15.md',
  "✅ AI flow action 'flow-for-deep-file-to-file-analysis' completed successfully! (0.00s)",
  '📄 Output saved to: .aid (3 files, 3.1 kB total)',
].join('\n');

test('docs flow: a bare task list name is resolved inside the reported .aid directory', () => {
  const line = savedPathLine(DOCS_FLOW, '/proj');
  assert.ok(line.includes(`Output file: ${path.resolve('/proj', '.aid')}`), line);
  assert.ok(line.includes(`Task list: ${path.resolve('/proj', '.aid', 'DOCS-TASK-LIST.src.2026-11-15.md')}`), line);
});

test('deep flow: a task list path relative to the root becomes absolute', () => {
  const line = savedPathLine(DEEP_FLOW, '/proj');
  assert.ok(line.includes(`Task list: ${path.resolve('/proj', '.aid/ANALYSIS-TASK-LIST.src.2026-11-15.md')}`), line);
});

test('aiActionText states the path and gives the advice only when a file was reported', () => {
  const withFile = aiActionText('Docs.', 'Read the file.', DOCS_FLOW, '/proj');
  assert.ok(withFile.includes(path.resolve('/proj', '.aid')));
  assert.ok(withFile.includes('Read the file.'));
  assert.ok(withFile.startsWith('Docs.'));
  const without = aiActionText('Docs.', 'Read the file.', 'plain output', '/proj');
  assert.ok(without.includes('aid did not report an output file'));
  assert.ok(!without.includes('Read the file.'));
});

test('an incomplete notice is first and last, also through aiActionText', () => {
  const notice = truncationNotice(10);
  const wrapped = withIncompleteNotice('body', notice);
  assert.ok(wrapped.startsWith(notice));
  assert.ok(wrapped.endsWith(notice));
  const text = aiActionText('Intro.', 'Advice.', wrapped, '/proj');
  assert.ok(text.startsWith(notice), text);
  assert.ok(text.endsWith(notice), text);
  assert.equal(text.split(notice).length - 1, 2);
});

test('spawn failure with both paths fine states that and shows the cause inline', () => {
  const text = spawnFailureMessage(process.execPath, os.tmpdir(), 'spawn EACCES');
  assert.match(text, /spawn EACCES/);
  assert.match(text, /binary and the directory exist/);
  assert.doesNotMatch(text, /above/);
});
