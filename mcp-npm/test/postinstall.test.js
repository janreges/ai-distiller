const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const p = require('../scripts/postinstall.js');

test('maps supported platforms', () => {
  assert.deepEqual(p.platformInfo('linux', 'x64'), { platform: 'linux', arch: 'amd64', ext: 'tar.gz' });
  assert.deepEqual(p.platformInfo('darwin', 'arm64'), { platform: 'darwin', arch: 'arm64', ext: 'tar.gz' });
  assert.deepEqual(p.platformInfo('win32', 'x64'), { platform: 'windows', arch: 'amd64', ext: 'zip' });
});

test('windows arm64 is reported as unsupported', () => {
  assert.throws(() => p.platformInfo('win32', 'arm64'), /no prebuilt aid binary for windows-arm64/i);
  assert.throws(() => p.platformInfo('freebsd', 'x64'), /no prebuilt aid binary for freebsd-x64/i);
});

test('builds the asset name the release workflow produces', () => {
  assert.equal(p.assetName('1.4.0', { platform: 'linux', arch: 'amd64', ext: 'tar.gz' }), 'aid-linux-amd64-v1.4.0.tar.gz');
});

test('reads release and prerelease versions', () => {
  assert.equal(p.installedVersion('aid version 1.4.0\nbuilt: 2026-05-25'), '1.4.0');
  assert.equal(p.installedVersion('aid version 1.4.0-rc.3'), '1.4.0-rc.3');
  assert.equal(p.installedVersion('aid version dev'), null);
});

test('finds a checksum by asset name', () => {
  const text = 'aaa111  aid-linux-amd64-v1.4.0.tar.gz\nBBB222  aid-windows-amd64-v1.4.0.zip\n';
  assert.equal(p.checksumFor(text, 'aid-windows-amd64-v1.4.0.zip'), 'bbb222');
  assert.equal(p.checksumFor(text, 'aid-darwin-arm64-v1.4.0.tar.gz'), null);
});

test('accepts a matching checksum and rejects a wrong checksum', async () => {
  const file = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'aid-')), 'a.bin');
  fs.writeFileSync(file, 'payload');
  const good = crypto.createHash('sha256').update('payload').digest('hex');
  await p.verifyFile(file, good);
  await assert.rejects(p.verifyFile(file, '0'.repeat(64)), /checksum mismatch for a\.bin/);
});

test('a download error names the file and the URL', () => {
  const url = 'https://github.com/janreges/ai-distiller/releases/download/v1.4.0/checksums.txt';
  const err = p.downloadError(url, 'HTTP 404');
  assert.equal(err.message, `could not download checksums.txt from ${url}: HTTP 404`);
});

test('the manual-download footer is printed for download failures only', () => {
  const footer = p.manualDownloadFooter(p.downloadError('https://h/x.tar.gz', 'HTTP 404'), '1.4.0', '/pkg/bin');
  assert.match(footer, /releases\/tag\/v1\.4\.0/);
  assert.match(footer, /\/pkg\/bin/);
  let unsupported;
  try { p.platformInfo('win32', 'arm64'); } catch (e) { unsupported = e; }
  assert.ok(unsupported, 'platformInfo throws for windows-arm64');
  assert.equal(p.manualDownloadFooter(unsupported, '1.4.0', '/pkg/bin'), '');
});

test('extractTarGz extracts a real archive', async () => {
  const tar = require('tar');
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'aid-targz-'));
  try {
    const src = path.join(tmp, 'src');
    const dest = path.join(tmp, 'dest');
    fs.mkdirSync(src);
    fs.mkdirSync(dest);
    const content = '#!/bin/sh\necho aid\n';
    fs.writeFileSync(path.join(src, 'aid'), content, { mode: 0o755 });
    fs.chmodSync(path.join(src, 'aid'), 0o755);
    const archive = path.join(tmp, 'aid-linux-amd64-v0.0.0.tar.gz');
    await tar.c({ gzip: true, file: archive, cwd: src }, ['aid']);
    await p.extractTarGz(archive, dest);
    const out = path.join(dest, 'aid');
    assert.equal(fs.readFileSync(out, 'utf8'), content);
    if (process.platform !== 'win32') {
      assert.ok(fs.statSync(out).mode & 0o111, 'extracted file is executable');
    }
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});
