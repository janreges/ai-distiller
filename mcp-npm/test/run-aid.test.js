const { test } = require('node:test');
const assert = require('node:assert/strict');
const { runAid, AidRunError, limitsFromEnv } = require('../dist/run-aid.js');

const node = process.execPath;
const opts = (over = {}) => ({ cwd: process.cwd(), timeoutMs: 5000, maxOutputBytes: 1_000_000, ...over });

test('returns stdout and stderr on success', async () => {
  const r = await runAid(node, ['-e', "console.log('out'); console.error('err')"], opts());
  assert.equal(r.stdout.trim(), 'out');
  assert.equal(r.stderr.trim(), 'err');
  assert.equal(r.truncated, false);
});

test('rejects with the exit code and stderr', async () => {
  await assert.rejects(runAid(node, ['-e', "console.error('boom'); process.exit(3)"], opts()), (e) => {
    assert.ok(e instanceof AidRunError);
    assert.equal(e.kind, 'exit');
    assert.equal(e.exitCode, 3);
    assert.match(e.stderr, /boom/);
    return true;
  });
});

test('times out and reports it', async () => {
  const started = Date.now();
  await assert.rejects(runAid(node, ['-e', 'setTimeout(() => {}, 60000)'], opts({ timeoutMs: 300 })), (e) => {
    assert.equal(e.kind, 'timeout');
    assert.match(e.message, /did not finish within 0\.3 s/);
    return true;
  });
  assert.ok(Date.now() - started < 3000, 'the child must be killed, not awaited');
});

test('stops at the output cap', async () => {
  const r = await runAid(node, ['-e', "process.stdout.write('x'.repeat(200000)); setTimeout(() => {}, 60000)"], opts({ maxOutputBytes: 1000 }));
  assert.equal(r.truncated, true);
  assert.equal(Buffer.byteLength(r.stdout), 1000);
});

test('reports a missing binary as a spawn error', async () => {
  await assert.rejects(runAid('/nonexistent/aid', [], opts()), (e) => e.kind === 'spawn');
});

test('limitsFromEnv falls back to defaults', () => {
  assert.deepEqual(limitsFromEnv({}), { timeoutMs: 120000, maxOutputBytes: 1000000 });
  assert.deepEqual(limitsFromEnv({ AID_MCP_TIMEOUT_MS: 'abc', AID_MCP_MAX_OUTPUT_BYTES: '-5' }), { timeoutMs: 120000, maxOutputBytes: 1000000 });
  assert.deepEqual(limitsFromEnv({ AID_MCP_TIMEOUT_MS: '5000', AID_MCP_MAX_OUTPUT_BYTES: '2048' }), { timeoutMs: 5000, maxOutputBytes: 2048 });
});

test('queue continues after a timeout', async () => {
  const { SerialQueue } = require('../dist/run-aid.js');
  const q = new SerialQueue();
  const first = q.run(() => runAid(node, ['-e', 'setTimeout(() => {}, 60000)'], opts({ timeoutMs: 200 })));
  const second = q.run(() => runAid(node, ['-e', "console.log('second')"], opts()));
  await assert.rejects(first);
  assert.equal((await second).stdout.trim(), 'second');
});
