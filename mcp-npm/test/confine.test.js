const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { Client } = require('@modelcontextprotocol/sdk/client/index.js');
const { InMemoryTransport } = require('@modelcontextprotocol/sdk/inMemory.js');
const { confinePath, confinedEnv, OutsideRootError, UncheckablePathError } = require('../dist/confine.js');
const { runAid } = require('../dist/run-aid.js');
const { AidDistillerServer } = require('../dist/mcp-server-sdk.js');

// A real project directory with a real outside directory, symlinks inside
// the root that point outside (a directory, a file, a missing name), one that
// loops, ones that point inside, and the real path of each (os.tmpdir()
// itself is a symlink on macOS).
function fixture() {
  const base = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'aid-confine-')));
  const root = path.join(base, 'root');
  const outside = path.join(base, 'outside');
  fs.mkdirSync(path.join(root, 'src', 'deep'), { recursive: true });
  fs.mkdirSync(path.join(outside, 'inner'), { recursive: true });
  fs.writeFileSync(path.join(root, 'src', 'a.py'), 'x = 1\n');
  fs.writeFileSync(path.join(root, '-dash.py'), 'y = 2\n');
  fs.writeFileSync(path.join(root, 'main.go'), 'package main\n');
  fs.writeFileSync(path.join(outside, 'secret.py'), 'token = 1\n');
  fs.writeFileSync(path.join(outside, 'secret.js'), 'export const token = 1\n');
  fs.symlinkSync(outside, path.join(root, 'out'), 'dir');
  fs.symlinkSync(path.join(outside, 'secret.py'), path.join(root, 'leak.py'), 'file');
  fs.symlinkSync(path.join(outside, 'nothere'), path.join(root, 'dangling'), 'file');
  fs.symlinkSync('loop', path.join(root, 'loop'), 'file');
  fs.symlinkSync(path.join(root, 'src'), path.join(root, 'in'), 'dir');
  fs.symlinkSync(path.join(root, 'src', 'deep'), path.join(root, 'deeplink'), 'dir');
  fs.symlinkSync(root, path.join(base, 'rootlink'), 'dir');
  return { base, root, outside };
}

// The values of the review that cancelled a part the check never resolved
// with "..": a missing name, a file, a symlink loop, a name too long for the
// file system, a dangling symlink.
const BYPASSES = ['nope/../leak.py', 'nope/../out/secret.py', 'main.go/../out/secret.py', 'loop/../out/secret.py',
  `${'a'.repeat(300)}/../out/secret.py`, 'dangling/../out/secret.py', 'nope/../out/inner', 'nope/../out/secret.js',
  'nope/../out', 'nope/../../outside/secret.py',
  // Other spellings of the same: an empty or "." part, a trailing slash, a
  // ".." below a missing part, a "." in front, ".." last.
  'nope//../out', 'nope/./../out', 'nope/../out/', './nope/../out', 'nope/x/../../out', 'leak.py/..'];

const confine = (value, root, fromEnv = true) => confinePath(value, root, 'file_path', fromEnv);

test('a path inside the root becomes a path relative to the real root', () => {
  const { root } = fixture();
  assert.deepEqual(confine('src/a.py', root), { real: path.join(root, 'src', 'a.py'), arg: path.join('src', 'a.py') });
  assert.equal(confine(path.join(root, 'src', 'a.py'), root).arg, path.join('src', 'a.py'));
  assert.equal(confine('src/../src/a.py', root).arg, path.join('src', 'a.py'));
  assert.equal(confine('.', root).arg, '.');
  assert.equal(confine(root, root).arg, '.');
});

test('a path inside the root that does not exist passes, for aid to report', () => {
  const { root } = fixture();
  assert.equal(confine('src/missing/b.py', root).arg, path.join('src', 'missing', 'b.py'));
});

test('a symlink inside the root that points inside passes as its target', () => {
  const { root } = fixture();
  assert.equal(confine('in/a.py', root).arg, path.join('src', 'a.py'));
  // The OS resolves ".." after a symlink from its target: deeplink/.. is src.
  assert.equal(confine('deeplink/../a.py', root).arg, path.join('src', 'a.py'));
});

test('a name that starts with "-" never reaches aid as a flag', () => {
  const { root } = fixture();
  assert.equal(confine('-dash.py', root).arg, `.${path.sep}-dash.py`);
});

test('a path outside the root is rejected with an error that names AID_ROOT and how to widen it', () => {
  const { root, outside } = fixture();
  for (const value of ['../outside/secret.py', path.join(outside, 'secret.py'), '/etc', '..', 'src/../../outside']) {
    assert.throws(() => confine(value, root), (e) => {
      assert.ok(e instanceof OutsideRootError, value);
      assert.match(e.message, /^file_path "/);
      assert.ok(e.message.includes(`is outside AID_ROOT (${root})`), e.message);
      assert.match(e.message, /fails the same way when repeated/);
      assert.match(e.message, /started with AID_ROOT set to a directory that contains them/);
      return true;
    });
  }
});

test('a real symlink inside the root that points outside is rejected, also below a missing name', () => {
  const { root, outside } = fixture();
  for (const value of ['out/secret.py', 'out', 'out/missing/deeper.py', 'in/../out/secret.py', 'leak.py']) {
    assert.throws(() => confine(value, root), (e) => {
      assert.ok(e instanceof OutsideRootError, value);
      assert.ok(e.message.includes(`resolves to ${outside}`), e.message);
      return true;
    });
  }
});

test('".." after a part that does not resolve is refused, so it cannot cancel that part', () => {
  const { root } = fixture();
  for (const value of BYPASSES) {
    assert.throws(() => confine(value, root), (e) => {
      assert.ok(e instanceof UncheckablePathError, `${value}: ${e.message}`);
      assert.match(e.message, /^file_path ".*" has "\.\." after .*, which is not an existing directory/);
      assert.match(e.message, /fails the same way when repeated/);
      return true;
    });
  }
  // An absolute value, and one outside the root, the same way.
  assert.throws(() => confine(`${root}/nope/../out/secret.py`, root), UncheckablePathError);
  assert.throws(() => confine('/nope/../etc', root), UncheckablePathError);
  // Also when the rest would stay inside: the check cannot tell.
  assert.throws(() => confine('src/missing/../a.py', root), UncheckablePathError);
  // ".." among parts that exist is resolved by the operating system.
  assert.equal(confine('src/../src/a.py', root).arg, path.join('src', 'a.py'));
  assert.equal(confine('src/..', root).arg, '.');
  assert.equal(confine('deeplink/../a.py', root).arg, path.join('src', 'a.py'));
  assert.throws(() => confine('..', root), OutsideRootError);
});

test('a symlink that resolves for nobody (dangling, looping) passes only without ".." after it', () => {
  const { root } = fixture();
  // aid follows them as the check did not and fails: the target does not exist.
  assert.equal(confine('dangling', root).arg, 'dangling');
  assert.equal(confine('loop', root).arg, 'loop');
  assert.throws(() => confine('dangling/../leak.py', root), UncheckablePathError);
});

test('a root given through a symlink accepts its real paths and names both', () => {
  const { base, root } = fixture();
  const rootLink = path.join(base, 'rootlink');
  assert.equal(confine('src/a.py', rootLink).arg, path.join('src', 'a.py'));
  assert.equal(confine(path.join(root, 'src', 'a.py'), rootLink).arg, path.join('src', 'a.py'));
  assert.equal(confine('../root/main.go', rootLink).arg, 'main.go');
  assert.throws(() => confine('../outside', rootLink), (e) => {
    assert.ok(e.message.includes(`AID_ROOT (${rootLink}, real path ${root})`), e.message);
    return true;
  });
  assert.throws(() => confine('../rootlink/../outside/secret.py', rootLink), OutsideRootError);
  for (const value of ['zz/../leak.py', 'zz/../out']) {
    assert.throws(() => confine(value, rootLink), UncheckablePathError);
  }
});

test('without AID_ROOT the message says the working directory stands for it', () => {
  const { root } = fixture();
  assert.throws(() => confine('/etc', root, false), (e) => {
    assert.ok(e.message.includes(`outside AID_ROOT, which is not set, so the server's working directory (${root})`), e.message);
    return true;
  });
});

test('an empty value is an input error, not all of the root', () => {
  const { root } = fixture();
  assert.throws(() => confine('', root), (e) => {
    assert.ok(e instanceof UncheckablePathError);
    assert.equal(e.message, `file_path is empty. Name a file or directory inside AID_ROOT (${root}); a relative ` +
      'path is resolved against it, and "." is AID_ROOT itself.');
    return true;
  });
});

test('an aid child gets AID_CONFINE_ROOT, the real path of AID_ROOT, and the rest of the environment', async () => {
  const { base, root } = fixture();
  const env = confinedEnv({ ...process.env, AID_CONFINE_ROOT: '/', AID_TEST_KEEP: 'kept' }, path.join(base, 'rootlink'));
  // A real child process reads its environment, as aid does.
  const script = "console.log(process.env.AID_CONFINE_ROOT + '|' + process.env.AID_TEST_KEEP)";
  const { stdout } = await runAid(process.execPath, ['-e', script],
    { cwd: root, env, timeoutMs: 5000, maxOutputBytes: 100000 });
  assert.equal(stdout.trim(), `${root}|kept`);
});

test('a NUL character is an input error, not a path', () => {
  const { root } = fixture();
  assert.throws(() => confine('src/a.py\0x', root), /contains a NUL character/);
});

// The real server and SDK client in memory, with AID_ROOT set to the fixture.
// A rejected call never starts aid, so no binary is needed.
async function connected(root) {
  const previous = process.env.AID_ROOT;
  process.env.AID_ROOT = root;
  const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair();
  await new AidDistillerServer().server.connect(serverTransport);
  const client = new Client({ name: 'confine-test', version: '0' });
  await client.connect(clientTransport);
  return {
    client,
    async close() {
      await client.close();
      if (previous === undefined) delete process.env.AID_ROOT;
      else process.env.AID_ROOT = previous;
    },
  };
}

const textOf = (result) => result.content.map((c) => c.text).join('\n');

test('every tool parameter that names a path rejects a path outside AID_ROOT', async () => {
  const { root } = fixture();
  const { client, close } = await connected(root);
  try {
    const { tools } = await client.listTools();
    const pathParams = ['file_path', 'directory_path', 'target_path', 'path'];
    let checked = 0;
    for (const tool of tools) {
      const props = Object.keys(tool.inputSchema.properties || {});
      // A new parameter that names a path must be added to pathParams (and confined).
      for (const p of props) {
        if (/path|file|dir/i.test(p)) assert.ok(pathParams.includes(p), `${tool.name}.${p} is not confined`);
      }
      for (const p of props.filter((name) => pathParams.includes(name))) {
        const call = (value) => {
          const args = { [p]: value };
          if (tool.name === 'aid_analyze') args.ai_action = 'prompt-for-bug-hunting';
          return client.callTool({ name: tool.name, arguments: args });
        };
        const result = await call('out/secret.py');
        assert.equal(result.isError, true, `${tool.name}.${p}`);
        assert.ok(textOf(result).includes(`${p} "out/secret.py" resolves to`), `${tool.name}: ${textOf(result)}`);
        assert.ok(textOf(result).includes(`outside AID_ROOT (${root})`), tool.name);
        for (const value of ['nope/../out/secret.py', 'nope/../out']) {
          const bypass = await call(value);
          assert.equal(bypass.isError, true, `${tool.name}.${p} ${value}`);
          assert.ok(textOf(bypass).startsWith(`${p} "${value}" has ".." after `), `${tool.name}: ${textOf(bypass)}`);
        }
        const empty = await call('');
        assert.equal(empty.isError, true, `${tool.name}.${p} empty`);
        assert.ok(textOf(empty).startsWith(`${p} is empty.`), `${tool.name}: ${textOf(empty)}`);
        checked++;
      }
    }
    assert.equal(checked, 15, 'fourteen tools that run aid and list_files');
  } finally {
    await close();
  }
});

test('list_files lists inside AID_ROOT and refuses /etc and a symlink out of it', async () => {
  const { root } = fixture();
  const { client, close } = await connected(root);
  try {
    const inside = await client.callTool({ name: 'list_files', arguments: {} });
    assert.notEqual(inside.isError, true, textOf(inside));
    const listing = JSON.parse(textOf(inside));
    assert.equal(listing.path, root);
    // Symlinked files (leak.py, dangling, loop) are not listed.
    assert.deepEqual(listing.files.map((f) => f.path).sort(), ['-dash.py', 'main.go', path.join('src', 'a.py')]);

    const sub = JSON.parse(textOf(await client.callTool({ name: 'list_files', arguments: { path: 'in' } })));
    assert.equal(sub.path, path.join(root, 'src'));

    for (const p of ['/etc', '..', 'out']) {
      const result = await client.callTool({ name: 'list_files', arguments: { path: p } });
      assert.equal(result.isError, true, p);
      assert.match(textOf(result), /is outside AID_ROOT/);
    }
    for (const p of ['nope/../out', 'nope/../out/inner', 'main.go/../out']) {
      const result = await client.callTool({ name: 'list_files', arguments: { path: p } });
      assert.equal(result.isError, true, p);
      assert.match(textOf(result), /has "\.\." after .*, which is not an existing directory/);
    }
  } finally {
    await close();
  }
});
