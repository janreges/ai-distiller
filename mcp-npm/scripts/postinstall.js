#!/usr/bin/env node

const os = require('os');
const path = require('path');
const fs = require('fs');
const https = require('https');
const { execSync } = require('child_process');
const zlib = require('zlib');
const tar = require('tar');

// AI Distiller binary version to download
// This can be different from the MCP package version
const AID_VERSION = '1.3.1'; // Update this when new aid releases are available
const MCP_VERSION = require('../package.json').version;

function getPlatformInfo() {
  const platform = os.platform();
  const arch = os.arch();
  
  const platformMap = {
    'darwin': 'darwin',
    'linux': 'linux',
    'win32': 'windows'
  };
  
  const archMap = {
    'x64': 'amd64',
    'arm64': 'arm64'
  };
  
  const mapped = {
    platform: platformMap[platform],
    arch: archMap[arch],
    ext: platform === 'win32' ? 'zip' : 'tar.gz'
  };
  
  if (!mapped.platform || !mapped.arch) {
    throw new Error(`Unsupported platform: ${platform}-${arch}`);
  }
  
  return mapped;
}

function downloadFile(url, dest, isStream = false) {
  return new Promise((resolve, reject) => {
    console.log(`Downloading from ${url}...`);
    
    https.get(url, (response) => {
      if (response.statusCode === 302 || response.statusCode === 301) {
        // Follow redirect
        return downloadFile(response.headers.location, dest, isStream)
          .then(resolve)
          .catch(reject);
      }
      
      if (response.statusCode !== 200) {
        reject(new Error(`Download failed with status code: ${response.statusCode}`));
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
    }).on('error', reject);
  });
}

async function extractArchive(archivePath, destDir, platform) {
  console.log('Extracting archive...');
  console.log(`Archive path: ${archivePath}`);
  console.log(`Destination: ${destDir}`);
  
  if (!fs.existsSync(destDir)) {
    fs.mkdirSync(destDir, { recursive: true });
  }
  
  if (platform === 'windows') {
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
  const startTime = Date.now();
  try {
    const { platform, arch, ext } = getPlatformInfo();
    const archiveName = `aid-${platform}-${arch}-v${AID_VERSION}.${ext}`;
    const url = `https://github.com/janreges/ai-distiller/releases/download/v${AID_VERSION}/${archiveName}`;
    
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
        const versionMatch = existingVersionOutput.match(/(\d+\.\d+\.\d+)/);
        const existingVersion = versionMatch ? versionMatch[1] : null;
        
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
    console.error('\nYou can manually download the binary from:');
    console.error(`https://github.com/janreges/ai-distiller/releases/tag/v${AID_VERSION}`);
    console.error('\nAnd place it in:', path.join(__dirname, '..', 'bin'));
    
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