const { test } = require('node:test');
const assert = require('node:assert/strict');
const { InMemoryTransport } = require('@modelcontextprotocol/sdk/inMemory.js');
const { LATEST_PROTOCOL_VERSION } = require('@modelcontextprotocol/sdk/types.js');
const { AidDistillerServer } = require('../dist/mcp-server-sdk.js');

// Speaks raw JSON-RPC to the real server, so the test chooses the version the
// client asks for, as a real client such as Claude Code does.
async function handshake(requested) {
  const [client, serverTransport] = InMemoryTransport.createLinkedPair();
  await new AidDistillerServer().connectTransport(serverTransport);
  const answers = new Map();
  client.onmessage = (message) => {
    if (message.id !== undefined) answers.get(message.id)?.(message);
  };
  await client.start();
  const call = (id, method, params) => new Promise((resolve) => {
    answers.set(id, resolve);
    client.send({ jsonrpc: '2.0', id, method, params });
  });
  const init = await call(1, 'initialize', {
    protocolVersion: requested, capabilities: {}, clientInfo: { name: 'version-test', version: '0' },
  });
  await client.send({ jsonrpc: '2.0', method: 'notifications/initialized' });
  const caps = await call(2, 'tools/call', { name: 'get_capabilities', arguments: {} });
  await client.close();
  return { negotiated: init.result.protocolVersion, reported: JSON.parse(caps.result.content[0].text).protocol_version };
}

test('get_capabilities reports the version negotiated in initialize, not a constant', async () => {
  for (const requested of ['2025-06-18', '2025-03-26', '2024-11-05']) {
    const { negotiated, reported } = await handshake(requested);
    assert.equal(negotiated, requested);
    assert.equal(reported, requested);
  }
});

test('for a version the SDK does not support, it reports the version the server answered with', async () => {
  const { negotiated, reported } = await handshake('1999-01-01');
  assert.equal(negotiated, LATEST_PROTOCOL_VERSION);
  assert.equal(reported, negotiated);
});

test('without an initialize answer get_capabilities has no protocol_version', async () => {
  const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair();
  await new AidDistillerServer().connectTransport(serverTransport);
  const answer = new Promise((resolve) => { clientTransport.onmessage = resolve; });
  await clientTransport.start();
  await clientTransport.send({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name: 'get_capabilities', arguments: {} } });
  const caps = JSON.parse((await answer).result.content[0].text);
  assert.equal('protocol_version' in caps, false);
  assert.equal(caps.server_name, 'AI Distiller MCP');
  await clientTransport.close();
});
