#!/usr/bin/env node

const os = require('os');
const path = require('path');
const fs = require('fs');
const https = require('https');
const { execSync } = require('child_process');
const zlib = require('zlib');
const crypto = require('crypto');
const tar = require('tar');

// AI Distiller binary version to download
// This can be different from the MCP package version
const AID_VERSION = '0.0.0-dev'; // stamped by .github/workflows/release.yml at publish time
const MCP_VERSION = require('../package.json').version;

function platformInfo(platform, arch) {
  const platformMap = {
    'darwin': 'darwin',
    'linux': 'linux',
    'win32': 'windows'
  };

  const archMap = {
    'x64': 'amd64',
    'arm64': 'arm64'
  };

  const mappedPlatform = platformMap[platform];
  const mappedArch = archMap[arch];

  // Releases publish linux and darwin for amd64 and arm64, windows for amd64 only.
  const supported = mappedPlatform && mappedArch && !(mappedPlatform === 'windows' && mappedArch !== 'amd64');
  if (!supported) {
    const error = new Error(
      `No prebuilt aid binary for ${mappedPlatform || platform}-${arch}. ` +
      'Supported: linux and macOS on x64 and arm64, Windows on x64. ' +
      'Build from source: https://github.com/janreges/ai-distiller#readme'
    );
    error.unsupportedPlatform = true;
    throw error;
  }

  return {
    platform: mappedPlatform,
    arch: mappedArch,
    ext: platform === 'win32' ? 'zip' : 'tar.gz'
  };
}

function assetName(version, info) {
  return `aid-${info.platform}-${info.arch}-v${version}.${info.ext}`;
}

function installedVersion(versionOutput) {
  const match = String(versionOutput).match(/(\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?)/);
  return match ? match[1] : null;
}

function checksumFor(checksumsText, name) {
  for (const line of String(checksumsText).split(/\r?\n/)) {
    const match = line.trim().match(/^([0-9A-Fa-f]{1,128})\s+\*?(.+)$/);
    if (match && match[2] === name) {
      return match[1].toLowerCase();
    }
  }
  return null;
}

function verifyFile(filePath, expectedSha256) {
  return new Promise((resolve, reject) => {
    const hash = crypto.createHash('sha256');
    const stream = fs.createReadStream(filePath);
    stream.on('error', reject);
    stream.on('data', (chunk) => hash.update(chunk));
    stream.on('end', () => {
      if (hash.digest('hex') === String(expectedSha256).toLowerCase()) {
        resolve();
      } else {
        reject(new Error(`checksum mismatch for ${path.basename(filePath)}`));
      }
    });
  });
}

/** Error for a failed download: names the file and the URL that was asked for. */
function downloadError(url, detail) {
  const name = decodeURIComponent(String(url).split('?')[0].split('/').pop());
  return new Error(`could not download ${name} from ${url}: ${detail}`);
}

/**
 * The manual-download advice printed after a failed install. Empty for an unsupported
 * platform, where a download cannot help and the error already says what to do.
 */
function manualDownloadFooter(error, version, binDir) {
  if (error && error.unsupportedPlatform) return '';
  return '\nYou can manually download the binary from:\n' +
    `https://github.com/janreges/ai-distiller/releases/tag/v${version}\n` +
    `\nAnd place it in: ${binDir}`;
}

function downloadFile(url, dest, isStream = false, origin = url) {
  return new Promise((resolve, reject) => {
    console.log(`Downloading from ${url}...`);
    
    https.get(url, (response) => {
      if ([301, 302, 303, 307, 308].includes(response.statusCode)) {
        // Follow redirect
        return downloadFile(response.headers.location, dest, isStream, origin)
          .then(resolve)
          .catch(reject);
      }
      
      if (response.statusCode !== 200) {
        reject(downloadError(origin, `HTTP ${response.statusCode}`));
        return;
      }
      
      if (isStream) {
        resolve(response);
      } else {
        const file = fs.createWriteStream(dest);
        response.pipe(file);
        file.on('finish', () => {
          file.close(resolve);
        });
        file.on('error', (err) => {
          fs.unlink(dest, () => {}); // Delete the file on error
          reject(err);
        });
      }
    }).on('error', (err) => reject(downloadError(origin, err.message)));
  });
}

async function extractArchive(archivePath, destDir, platform) {
  console.log('Extracting archive...');
  console.log(`Archive path: ${archivePath}`);
  console.log(`Destination: ${destDir}`);
  
  if (!fs.existsSync(destDir)) {
    fs.mkdirSync(destDir, { recursive: true });
  }
  
  // Accept both the mapped value ('windows') and the raw os.platform() value ('win32')
  if (platform === 'windows' || platform === 'win32') {
    // For Windows, use built-in PowerShell or fallback to manual extraction
    try {
      // Try PowerShell first (available on all modern Windows)
      execSync(`powershell -command "Expand-Archive -Path '${archivePath}' -DestinationPath '${destDir}' -Force"`, { 
        stdio: 'pipe' 
      });
    } catch (e) {
      // Fallback to Node.js unzip implementation
      console.log('PowerShell extraction failed, using fallback method...');
      // For simplicity, we'll require unzip to be installed
      try {
        execSync(`unzip -o "${archivePath}" -d "${destDir}"`, { stdio: 'inherit' });
      } catch (e2) {
        throw new Error('Failed to extract ZIP file. Please ensure unzip is installed or extract manually.');
      }
    }
  } else {
    // For Unix systems, try system tar first (much faster)
    try {
      console.log('Using system tar command...');
      execSync(`tar -xzf "${archivePath}" -C "${destDir}"`, { 
        stdio: 'inherit' 
      });
    } catch (e) {
      console.log('System tar failed, falling back to node-tar library...');
      // Fallback to node tar library
      await tar.x({
        file: archivePath,
        cwd: destDir,
        onentry: (entry) => {
          // Log progress for debugging
          if (entry.path.includes('aid')) {
            console.log(`Extracting: ${entry.path}`);
          }
        }
      });
    }
  }
}

async function install() {
  if (AID_VERSION === '0.0.0-dev') {
    console.log('development checkout: build aid and copy it to mcp-npm/bin/');
    return;
  }
  const startTime = Date.now();
  try {
    const info = platformInfo(os.platform(), os.arch());
    const { platform, arch } = info;
    const archiveName = assetName(AID_VERSION, info);
    const releaseBase = `https://github.com/janreges/ai-distiller/releases/download/v${AID_VERSION}`;
    const url = `${releaseBase}/${archiveName}`;
    
    console.log(`Installing AI Distiller MCP for ${platform}/${arch}...`);
    console.log(`MCP Version: ${MCP_VERSION}`);
    console.log(`AI Distiller Binary Version: ${AID_VERSION}`);
    console.log(`Download URL: ${url}`);
    
    const binDir = path.join(__dirname, '..', 'bin');
    const tempFile = path.join(binDir, archiveName);
    const binaryName = platform === 'windows' ? 'aid.exe' : 'aid';
    const binaryPath = path.join(binDir, binaryName);
    
    // Create bin directory
    if (!fs.existsSync(binDir)) {
      fs.mkdirSync(binDir, { recursive: true });
    }
    
    // Check if binary already exists and is valid
    if (fs.existsSync(binaryPath)) {
      try {
        // Try to get version to verify it works
        const existingVersionOutput = execSync(`"${binaryPath}" --version`, { 
          stdio: 'pipe',
          encoding: 'utf8'
        }).trim();
        
        // Extract version number (e.g., "aid version 1.3.0" -> "1.3.0")
        const existingVersion = installedVersion(existingVersionOutput);
        
        console.log(`Found existing AI Distiller binary version: ${existingVersion}`);
        
        if (existingVersion === AID_VERSION) {
          console.log(`Version matches required version (${AID_VERSION}). Skipping download.`);
          const totalTime = Date.now() - startTime;
          console.log(`Installation check completed in ${totalTime}ms`);
          return;
        } else {
          console.log(`Version mismatch (found ${existingVersion}, need ${AID_VERSION}). Re-downloading...`);
          fs.unlinkSync(binaryPath); // Remove old binary
        }
      } catch (e) {
        console.log('Existing binary not working or version check failed, re-downloading...');
        // Attempt to remove potentially corrupted binary
        if (fs.existsSync(binaryPath)) {
          fs.unlinkSync(binaryPath);
        }
      }
    }
    
    try {
      // Download archive
      const downloadStart = Date.now();
      await downloadFile(url, tempFile);
      const downloadTime = Date.now() - downloadStart;
      console.log(`Download complete in ${downloadTime}ms.`);
      
      // Verify the archive against the release's checksums.txt before extracting
      const checksumsFile = path.join(binDir, `checksums-v${AID_VERSION}.txt`);
      try {
        await downloadFile(`${releaseBase}/checksums.txt`, checksumsFile);
        const expected = checksumFor(fs.readFileSync(checksumsFile, 'utf8'), archiveName);
        if (!expected) {
          throw new Error(`checksums.txt of v${AID_VERSION} has no entry for ${archiveName}`);
        }
        await verifyFile(tempFile, expected);
        console.log(`Checksum verified (sha256 ${expected}).`);
      } finally {
        if (fs.existsSync(checksumsFile)) {
          fs.unlinkSync(checksumsFile);
        }
      }

      // Check archive size
      const archiveStats = fs.statSync(tempFile);
      console.log(`Archive size: ${Math.round(archiveStats.size / 1024 / 1024)} MB`);
      
      // Extract archive
      const extractStart = Date.now();
      await extractArchive(tempFile, binDir, platform);
      const extractTime = Date.now() - extractStart;
      console.log(`Extraction complete in ${extractTime}ms.`);
      
      // Verify binary exists
      if (!fs.existsSync(binaryPath)) {
        throw new Error(`Binary not found after extraction. Expected at: ${binaryPath}`);
      }
      
      // Set executable permissions on Unix
      if (platform !== 'windows') {
        fs.chmodSync(binaryPath, 0o755);
      }
      
      // Verify it works
      try {
        const version = execSync(`"${binaryPath}" --version`, { 
          stdio: 'pipe',
          encoding: 'utf8'
        }).trim();
        console.log(`AI Distiller installed successfully: ${version}`);
        console.log(`Binary location: ${binaryPath}`);
        
        // Check file size to ensure it's not corrupted
        const stats = fs.statSync(binaryPath);
        console.log(`Binary size: ${Math.round(stats.size / 1024 / 1024)} MB`);
      } catch (e) {
        console.error('ERROR: Could not verify binary:', e.message);
        console.error(`Please check if binary exists at: ${binaryPath}`);
        throw new Error('Binary verification failed');
      }
      
    } finally {
      // Clean up archive file
      if (fs.existsSync(tempFile)) {
        fs.unlinkSync(tempFile);
      }
    }
    
  } catch (error) {
    const totalTime = Date.now() - startTime;
    console.error(`Installation failed after ${totalTime}ms:`, error.message);
    const footer = manualDownloadFooter(error, AID_VERSION, path.join(__dirname, '..', 'bin'));
    if (footer) console.error(footer);
    
    // Exit with error code to fail npm install
    process.exit(1);
  }
  
  const totalTime = Date.now() - startTime;
  console.log(`Total installation time: ${totalTime}ms`);
}

// Only run if called directly (not required as module)
if (require.main === module) {
  install();
}

module.exports = { platformInfo, assetName, installedVersion, checksumFor, verifyFile, downloadError, manualDownloadFooter };