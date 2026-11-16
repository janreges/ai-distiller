const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { Client } = require('@modelcontextprotocol/sdk/client/index.js');
const { InMemoryTransport } = require('@modelcontextprotocol/sdk/inMemory.js');
const { confinePath, OutsideRootError } = require('../dist/confine.js');
const { AidDistillerServer } = require('../dist/mcp-server-sdk.js');

// A real project directory with a real outside directory, a symlink inside
// the root that points outside, one that points inside, and the real path of
// each (os.tmpdir() itself is a symlink on macOS).
function fixture() {
  const base = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'aid-confine-')));
  const root = path.join(base, 'root');
  const outside = path.join(base, 'outside');
  fs.mkdirSync(path.join(root, 'src', 'deep'), { recursive: true });
  fs.mkdirSync(outside);
  fs.writeFileSync(path.join(root, 'src', 'a.py'), 'x = 1\n');
  fs.writeFileSync(path.join(root, '-dash.py'), 'y = 2\n');
  fs.writeFileSync(path.join(outside, 'secret.py'), 'token = 1\n');
  fs.symlinkSync(outside, path.join(root, 'out'), 'dir');
  fs.symlinkSync(path.join(root, 'src'), path.join(root, 'in'), 'dir');
  fs.symlinkSync(path.join(root, 'src', 'deep'), path.join(root, 'deeplink'), 'dir');
  fs.symlinkSync(root, path.join(base, 'rootlink'), 'dir');
  return { base, root, outside };
}

const confine = (value, root, fromEnv = true) => confinePath(value, root, 'file_path', fromEnv);

test('a path inside the root becomes a path relative to the real root', () => {
  const { root } = fixture();
  assert.deepEqual(confine('src/a.py', root), { real: path.join(root, 'src', 'a.py'), arg: path.join('src', 'a.py') });
  assert.equal(confine(path.join(root, 'src', 'a.py'), root).arg, path.join('src', 'a.py'));
  assert.equal(confine('src/../src/a.py', root).arg, path.join('src', 'a.py'));
  assert.equal(confine('.', root).arg, '.');
  assert.equal(confine('', root).arg, '.');
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
  for (const value of ['out/secret.py', 'out', 'out/missing/deeper.py', 'in/../out/secret.py']) {
    assert.throws(() => confine(value, root), (e) => {
      assert.ok(e instanceof OutsideRootError, value);
      assert.ok(e.message.includes(`resolves to ${outside}`), e.message);
      return true;
    });
  }
  // A missing name followed by ".." cannot climb out either.
  assert.throws(() => confine('nope/../../outside/secret.py', root), OutsideRootError);
});

test('a root given through a symlink accepts its real paths and names both', () => {
  const { base, root } = fixture();
  const rootLink = path.join(base, 'rootlink');
  assert.equal(confine('src/a.py', rootLink).arg, path.join('src', 'a.py'));
  assert.equal(confine(path.join(root, 'src', 'a.py'), rootLink).arg, path.join('src', 'a.py'));
  assert.throws(() => confine('../outside', rootLink), (e) => {
    assert.ok(e.message.includes(`AID_ROOT (${rootLink}, real path ${root})`), e.message);
    return true;
  });
});

test('without AID_ROOT the message says the working directory stands for it', () => {
  const { root } = fixture();
  assert.throws(() => confine('/etc', root, false), (e) => {
    assert.ok(e.message.includes(`outside AID_ROOT, which is not set, so the server's working directory (${root})`), e.message);
    return true;
  });
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
        const args = { [p]: 'out/secret.py' };
        if (tool.name === 'aid_analyze') args.ai_action = 'prompt-for-bug-hunting';
        const result = await client.callTool({ name: tool.name, arguments: args });
        assert.equal(result.isError, true, `${tool.name}.${p}`);
        assert.ok(textOf(result).includes(`${p} "out/secret.py" resolves to`), `${tool.name}: ${textOf(result)}`);
        assert.ok(textOf(result).includes(`outside AID_ROOT (${root})`), tool.name);
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
    assert.deepEqual(listing.files.map((f) => f.path).sort(), ['-dash.py', path.join('src', 'a.py')]);

    const sub = JSON.parse(textOf(await client.callTool({ name: 'list_files', arguments: { path: 'in' } })));
    assert.equal(sub.path, path.join(root, 'src'));

    for (const p of ['/etc', '..', 'out']) {
      const result = await client.callTool({ name: 'list_files', arguments: { path: p } });
      assert.equal(result.isError, true, p);
      assert.match(textOf(result), /is outside AID_ROOT/);
    }
  } finally {
    await close();
  }
});
