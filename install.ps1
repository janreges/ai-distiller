# PowerShell installer for the 'aid' CLI
#
# Usage:
#   iwr https://raw.githubusercontent.com/janreges/ai-distiller/main/install.ps1 -useb | iex
#   irm https://raw.githubusercontent.com/janreges/ai-distiller/main/install.ps1 | iex
#
# Or to install a specific version:
#   $env:AID_VERSION="1.0.0"; iwr https://raw.githubusercontent.com/janreges/ai-distiller/main/install.ps1 -useb | iex

$ErrorActionPreference = "Stop"

# Configuration
$Version = if ($env:AID_VERSION) { $env:AID_VERSION } else { "1.3.1" }
$Repo = "janreges/ai-distiller"
$InstallRoot = if ($env:AID_INSTALL_ROOT) { $env:AID_INSTALL_ROOT } else { "$env:USERPROFILE\.aid" }
$BinDir = "$InstallRoot\bin"

# Helper functions
function Write-Host-Colored {
    param($Text, $Color = "White")
    Write-Host "aid-installer: $Text" -ForegroundColor $Color
}

function Test-CommandExists {
    param($Command)
    $null = Get-Command $Command -ErrorAction SilentlyContinue
    return $?
}

# Main installation
try {
    # Detect architecture.
    # Note: (Get-CimInstance Win32_OperatingSystem).OSArchitecture is *localized*
    # (e.g. "64-bit", "64 bits", "64 位"), so matching it by string is unreliable.
    # Rely on locale-independent sources instead. PROCESSOR_ARCHITEW6432 is set when
    # running inside a 32-bit (WOW64) process on a 64-bit OS and holds the real arch.
    $ProcArch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }

    if (-not [Environment]::Is64BitOperatingSystem) {
        Write-Host-Colored "Error: 32-bit Windows is not supported" "Red"
        exit 1
    }

    $Arch = if ($ProcArch -match 'ARM') { "arm64" } else { "amd64" }

    # Construct URLs
    $ArchiveName = "aid-windows-$Arch-v$Version.zip"
    $DownloadUrl = "https://github.com/$Repo/releases/download/v$Version/$ArchiveName"
    $ChecksumUrl = "https://github.com/$Repo/releases/download/v$Version/checksums.txt"

    # Create temp directory
    $TempDir = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path $_ }

    try {
        # Download archive
        Write-Host-Colored "Downloading aid v$Version for Windows/$Arch..."
        $ArchivePath = Join-Path $TempDir $ArchiveName
        Invoke-WebRequest -Uri $DownloadUrl -OutFile $ArchivePath -UseBasicParsing

        # Download and verify checksum
        Write-Host-Colored "Verifying checksum..."
        $ChecksumPath = Join-Path $TempDir "checksums.txt"
        try {
            Invoke-WebRequest -Uri $ChecksumUrl -OutFile $ChecksumPath -UseBasicParsing
            
            # Calculate hash of downloaded file
            $ActualHash = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLower()
            
            # Find expected hash in checksums file
            $ChecksumContent = Get-Content $ChecksumPath
            $ExpectedLine = $ChecksumContent | Where-Object { $_ -match [regex]::Escape($ArchiveName) }
            
            if ($ExpectedLine) {
                $ExpectedHash = ($ExpectedLine -split '\s+')[0].ToLower()
                if ($ActualHash -ne $ExpectedHash) {
                    Write-Host-Colored "Error: Checksum verification failed" "Red"
                    Write-Host-Colored "Expected: $ExpectedHash" "Red"
                    Write-Host-Colored "Actual: $ActualHash" "Red"
                    exit 1
                }
                Write-Host-Colored "Checksum verified successfully" "Green"
            } else {
                Write-Host-Colored "Warning: Could not find checksum for $ArchiveName in checksums.txt" "Yellow"
            }
        } catch {
            Write-Host-Colored "Warning: Could not download checksums.txt. Skipping verification." "Yellow"
        }

        # Extract archive
        Write-Host-Colored "Extracting archive..."
        Expand-Archive -Path $ArchivePath -DestinationPath $TempDir -Force

        # Create installation directory
        if (!(Test-Path $BinDir)) {
            New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
        }

        # Install binary
        Write-Host-Colored "Installing 'aid.exe' to $BinDir..."
        $SourceExe = Join-Path $TempDir "aid.exe"
        $DestExe = Join-Path $BinDir "aid.exe"
        
        # Stop any running aid.exe processes
        Get-Process -Name "aid" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
        
        Copy-Item -Path $SourceExe -Destination $DestExe -Force

        # Update PATH if needed
        $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
        if ($UserPath -notlike "*$BinDir*") {
            Write-Host-Colored "Adding $BinDir to PATH..."
            
            # Check PATH length (Windows limit is ~2048 chars)
            $NewPath = "$UserPath;$BinDir"
            if ($NewPath.Length -gt 2000) {
                Write-Host-Colored "Warning: PATH is near Windows length limit. Manual PATH configuration may be needed." "Yellow"
            } else {
                [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
                $env:Path = "$env:Path;$BinDir"
            }
        }

        # Success message
        Write-Host-Colored "`nInstallation successful!" "Green"
        Write-Host-Colored "The 'aid' command was installed to: $DestExe" "Green"
        
        # Check if we need to restart shell
        if ($env:Path -notlike "*$BinDir*") {
            Write-Host-Colored "`n⚠ IMPORTANT: You need to restart your terminal for PATH changes to take effect." "Yellow"
            Write-Host-Colored "  After restarting, verify the installation with: aid --version" "Yellow"
        } else {
            Write-Host-Colored "`n✓ You can now use 'aid' from any directory." "Green"
            Write-Host-Colored "  Verify the installation by running: aid --version" "Green"
        }

        # Add uninstall info to registry (optional but nice)
        $UninstallKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Aid"
        if (!(Test-Path $UninstallKey)) {
            New-Item -Path $UninstallKey -Force | Out-Null
        }
        Set-ItemProperty -Path $UninstallKey -Name "DisplayName" -Value "AI Distiller (aid)"
        Set-ItemProperty -Path $UninstallKey -Name "UninstallString" -Value "powershell -Command `"Remove-Item -Recurse -Force '$InstallRoot'; [Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') -replace [regex]::Escape(';$BinDir'), '', 'User')`""
        Set-ItemProperty -Path $UninstallKey -Name "InstallLocation" -Value $InstallRoot
        Set-ItemProperty -Path $UninstallKey -Name "DisplayVersion" -Value $Version

    } finally {
        # Cleanup
        Remove-Item -Recurse -Force $TempDir -ErrorAction SilentlyContinue
    }

} catch {
    Write-Host-Colored "Installation failed: $_" "Red"
    exit 1
}