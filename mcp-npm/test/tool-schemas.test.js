const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Client } = require('@modelcontextprotocol/sdk/client/index.js');
const { InMemoryTransport } = require('@modelcontextprotocol/sdk/inMemory.js');
const { AidDistillerServer, TOOL_SCHEMAS } = require('../dist/mcp-server-sdk.js');

// The real server, connected to a real SDK client in memory. A call whose
// arguments fail validation never reaches the handler, so no aid binary runs.
async function connectedClient() {
  const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair();
  await new AidDistillerServer().server.connect(serverTransport);
  const client = new Client({ name: 'schema-test', version: '0' });
  await client.connect(clientTransport);
  return client;
}

test('an unknown key is rejected by the exported schema, naming the key', () => {
  const result = TOOL_SCHEMAS.aid_hunt_bugs.safeParse({ target_path: 'src', focus_area: 'concurrency' });
  assert.equal(result.success, false);
  assert.deepEqual(result.error.issues.map((i) => [i.code, i.keys]), [['unrecognized_keys', ['focus_area']]]);
});

test('every advertised tool schema forbids additional properties', async () => {
  const client = await connectedClient();
  const { tools } = await client.listTools();
  assert.ok(tools.length > 0);
  for (const tool of tools) {
    assert.equal(tool.inputSchema.additionalProperties, false, tool.name);
  }
  await client.close();
});

test('a mistyped parameter gets a -32602 error that names it', async () => {
  const client = await connectedClient();
  const result = await client.callTool({ name: 'distill_file', arguments: { file_path: 'a.py', include_privat: true } });
  assert.equal(result.isError, true);
  const text = result.content.map((c) => c.text).join('\n');
  assert.match(text, /-32602/);
  assert.match(text, /Unrecognized key: "include_privat"/);
  await client.close();
});

test('a tool without parameters accepts a call without arguments', async () => {
  const client = await connectedClient();
  const result = await client.callTool({ name: 'get_capabilities' });
  assert.notEqual(result.isError, true);
  assert.match(result.content[0].text, /"server_name": "AI Distiller MCP"/);
  await client.close();
});
