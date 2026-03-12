# xquare CLI installer for Windows
# Usage: iwr -useb https://raw.githubusercontent.com/team-xquare/xquare-cli/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$Repo    = "team-xquare/xquare-cli"
$Binary  = "xquare"
$InstallDir = "$env:LOCALAPPDATA\xquare\bin"

# Resolve latest version
if (-not $env:VERSION) {
    $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
    $Version = $release.tag_name
} else {
    $Version = $env:VERSION
}

if (-not $Version) {
    Write-Error "Could not determine latest version"
    exit 1
}

$VersionNum = $Version.TrimStart("v")
$Archive    = "${Binary}_${VersionNum}_windows_amd64.zip"
$Url        = "https://github.com/$Repo/releases/download/$Version/$Archive"

Write-Host "Installing xquare $Version (windows/amd64)..."

# Download
$TmpDir = Join-Path $env:TEMP ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $TmpDir | Out-Null

try {
    $ZipPath = Join-Path $TmpDir $Archive
    Invoke-WebRequest -Uri $Url -OutFile $ZipPath -UseBasicParsing

    # Extract
    Expand-Archive -Path $ZipPath -DestinationPath $TmpDir -Force

    # Install
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Move-Item -Force (Join-Path $TmpDir "${Binary}.exe") (Join-Path $InstallDir "${Binary}.exe")
} finally {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}

# Add to PATH if not present
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to PATH (restart your terminal)"
}

Write-Host ""
Write-Host "xquare $Version installed to $InstallDir\${Binary}.exe"
Write-Host ""
Write-Host "Get started:"
Write-Host "  xquare login"
Write-Host "  xquare project list"
