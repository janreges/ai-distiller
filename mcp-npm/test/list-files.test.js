const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { listFiles, listFilesText } = require('../dist/list-files.js');

function tmp() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'aid-list-'));
}

test('a missing path is an error naming the absolute path, not an empty project', async () => {
  const missing = path.join(tmp(), 'nope');
  await assert.rejects(listFiles(missing, {}), { message: `Path does not exist or is not a directory: ${missing}` });
});

test('a regular file is not a directory', async () => {
  const dir = tmp();
  const file = path.join(dir, 'a.py');
  fs.writeFileSync(file, 'x = 1\n');
  await assert.rejects(listFiles(file, {}), { message: `Path does not exist or is not a directory: ${file}` });
});

test('lists files with language and skips unreadable subdirectories silently', async (t) => {
  const dir = tmp();
  fs.writeFileSync(path.join(dir, 'a.py'), 'x = 1\n');
  fs.mkdirSync(path.join(dir, 'locked'));
  fs.writeFileSync(path.join(dir, 'locked', 'b.py'), 'y = 2\n');
  fs.chmodSync(path.join(dir, 'locked'), 0o000);
  t.after(() => fs.chmodSync(path.join(dir, 'locked'), 0o755));
  const r = await listFiles(dir, {});
  assert.equal(r.file_count, process.getuid && process.getuid() === 0 ? 2 : 1);
  assert.equal(r.files[0].language, 'python');
  assert.equal(r.incomplete, undefined);
});

test('a capped listing keeps incomplete inside compact JSON and the notice first and last', async () => {
  const dir = tmp();
  for (let i = 0; i < 5; i++) fs.writeFileSync(path.join(dir, `f${i}.py`), 'x\n');
  const r = await listFiles(dir, { maxEntries: 3 });
  assert.equal(r.file_count, 3);
  assert.equal(r.incomplete, true);
  const text = listFilesText(r, 3);
  const lines = text.split('\n');
  assert.match(lines[0], /^\[List incomplete: stopped at 3 entries/);
  assert.match(lines[lines.length - 1], /^\[List incomplete/);
  const json = lines.slice(1, -1).join('\n');
  assert.equal(JSON.parse(json).incomplete, true);
  assert.ok(!json.includes('\n'), 'compact JSON is one line');
});

test('a complete listing is compact JSON without a notice', async () => {
  const dir = tmp();
  fs.writeFileSync(path.join(dir, 'a.go'), 'package a\n');
  const text = listFilesText(await listFiles(dir, {}), 2000);
  assert.ok(!text.includes('\n'));
  assert.equal(JSON.parse(text).file_count, 1);
});
