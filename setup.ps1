#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Sets up all build dependencies for dune-admin on Windows.
.DESCRIPTION
    Installs (via winget/chocolatey):
      - Go 1.26.x
      - Node.js LTS (for pnpm/vite toolchain)
      - pnpm 10.28.1
      - GNU Make 4.x (via Chocolatey)
      - kubectl (optional, for kubectl control plane)
    Then runs pnpm install in web/ and caches Go modules.
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Write-Step($msg) { Write-Host "`n>> $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "   $msg" -ForegroundColor Green }
function Write-Skip($msg) { Write-Host "   $msg (already installed)" -ForegroundColor DarkGray }

function Refresh-Path {
    $env:Path = [System.Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' +
                [System.Environment]::GetEnvironmentVariable('Path', 'User')
}

# Resolve the full path to a command — bypasses PowerShell's stale command cache
function Find-Command($name) {
    # Search PATH directories directly for the executable
    foreach ($dir in $env:Path -split ';') {
        if (-not $dir) { continue }
        foreach ($ext in '', '.exe', '.cmd', '.bat', '.ps1') {
            $candidate = Join-Path $dir "$name$ext"
            if (Test-Path $candidate) { return $candidate }
        }
    }
    return $null
}

# ---------------------------------------------------------------------------
# Go
# ---------------------------------------------------------------------------
Write-Step "Checking Go..."
$goInstalled = $false
try {
    $goVer = (go version 2>$null) -replace '.*go(\d+\.\d+\.\d+).*', '$1'
    if ($goVer -match '^\d+\.\d+') {
        Write-Ok "Go $goVer found"
        $goInstalled = $true
    }
} catch {}

if (-not $goInstalled) {
    Write-Step "Installing Go via winget..."
    winget install --id GoLang.Go --accept-source-agreements --accept-package-agreements
    Refresh-Path
    Write-Ok "Go installed: $(go version)"
}

# ---------------------------------------------------------------------------
# Node.js (supports fnm, winget, or direct install)
# ---------------------------------------------------------------------------
Write-Step "Checking Node.js..."
$nodeMinMajor = 22
$hasFnm = $null -ne (Find-Command 'fnm')

$nodeInstalled = $false
try {
    $nodeVer = (node --version 2>$null)
    if ($nodeVer -match '^v\d+') {
        $nodeMajor = [int]($nodeVer -replace '^v(\d+).*', '$1')
        if ($nodeMajor -ge $nodeMinMajor) {
            Write-Ok "Node.js $nodeVer found"
            $nodeInstalled = $true
        }
    }
} catch {}

if (-not $nodeInstalled) {
    if ($hasFnm) {
        Write-Step "Installing Node.js $nodeMinMajor via fnm..."
        $fnmCmd = Find-Command 'fnm'
        & $fnmCmd install $nodeMinMajor
        & $fnmCmd use $nodeMinMajor
        & $fnmCmd default $nodeMinMajor
        Refresh-Path
        Write-Ok "Node.js installed via fnm: $(node --version)"
        Write-Host "   Note: run 'fnm use $nodeMinMajor' in new terminals if fnm doesn't auto-switch" -ForegroundColor Yellow
    } else {
        Write-Step "Installing Node.js $nodeMinMajor via winget..."
        winget install --id OpenJS.NodeJS.LTS --accept-source-agreements --accept-package-agreements
        Refresh-Path
        Write-Ok "Node.js installed: $(node --version)"
    }
}

# ---------------------------------------------------------------------------
# pnpm
# ---------------------------------------------------------------------------
Write-Step "Checking pnpm..."
$pnpmInstalled = $false
try {
    $pnpmVer = (pnpm --version 2>$null)
    if ($pnpmVer -match '^\d+\.\d+') {
        Write-Ok "pnpm $pnpmVer found"
        $pnpmInstalled = $true
    }
} catch {}

if (-not $pnpmInstalled) {
    Write-Step "Installing pnpm via corepack..."
    corepack enable
    corepack prepare pnpm@10.28.1 --activate
    Refresh-Path
    Write-Ok "pnpm installed: $(& (Find-Command 'pnpm') --version)"
} elseif ($pnpmVer -ne '10.28.1') {
    Write-Host "   Warning: project pins pnpm@10.28.1 but found $pnpmVer" -ForegroundColor Yellow
    Write-Host "   Run: corepack prepare pnpm@10.28.1 --activate" -ForegroundColor Yellow
}

# Resolve pnpm for use later in the script (bypasses PowerShell command cache)
$pnpmCmd = Find-Command 'pnpm'
if (-not $pnpmCmd) {
    throw "pnpm not found on PATH after installation. Please restart your terminal and re-run."
}

# ---------------------------------------------------------------------------
# GNU Make (via Chocolatey — GnuWin32 Make 3.81 is too old and broken)
# ---------------------------------------------------------------------------
Write-Step "Checking Make..."

# Remove GnuWin32 Make if present — it's from 2006 and can't run modern Makefiles
$gnuWinMake = "C:\Program Files (x86)\GnuWin32\bin\make.exe"
if (Test-Path $gnuWinMake) {
    Write-Host "   Removing broken GnuWin32 Make 3.81..." -ForegroundColor Yellow
    try { winget uninstall --id GnuWin32.Make --accept-source-agreements 2>$null } catch {}
    # Clean up PATH if GnuWin32 was added
    $gnuMakePath = "C:\Program Files (x86)\GnuWin32\bin"
    $userPath = [System.Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath -like "*$gnuMakePath*") {
        $userPath = ($userPath -split ';' | Where-Object { $_ -ne $gnuMakePath }) -join ';'
        [System.Environment]::SetEnvironmentVariable('Path', $userPath, 'User')
    }
    Refresh-Path
}

$makeInstalled = $false
$makeCmd = Find-Command 'make'
if ($makeCmd) {
    $makeVer = (& $makeCmd --version 2>$null | Select-Object -First 1)
    if ($makeVer -match 'Make' -and $makeVer -notmatch '3\.81') {
        Write-Ok "$makeVer found"
        $makeInstalled = $true
    }
}

if (-not $makeInstalled) {
    # Require Chocolatey
    $chocoCmd = Find-Command 'choco'
    if (-not $chocoCmd) {
        Write-Host "   Chocolatey not found. Installing Chocolatey..." -ForegroundColor Yellow
        Set-ExecutionPolicy Bypass -Scope Process -Force
        [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
        Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
        Refresh-Path
        $chocoCmd = Find-Command 'choco'
    }
    Write-Step "Installing GNU Make via Chocolatey..."
    & $chocoCmd install make -y
    Refresh-Path
    $makeCmd = Find-Command 'make'
    Write-Ok "Make installed: $(& $makeCmd --version 2>$null | Select-Object -First 1)"
}

# ---------------------------------------------------------------------------
# kubectl (optional — needed for kubectl control plane)
# ---------------------------------------------------------------------------
Write-Step "Checking kubectl..."
$kubectlInstalled = $false
try {
    $kubectlVer = (kubectl version --client --short 2>$null)
    if (-not $kubectlVer) { $kubectlVer = (kubectl version --client 2>$null | Select-Object -First 1) }
    if ($kubectlVer -match 'v\d+\.\d+') {
        Write-Ok "kubectl found ($($kubectlVer.Trim()))"
        $kubectlInstalled = $true
    }
} catch {}

if (-not $kubectlInstalled) {
    Write-Step "Installing kubectl via winget..."
    winget install --id Kubernetes.kubectl --accept-source-agreements --accept-package-agreements
    Refresh-Path
    $kubectlCmd = Find-Command 'kubectl'
    if ($kubectlCmd) {
        Write-Ok "kubectl installed"
    } else {
        Write-Host "   Warning: kubectl installed but not on PATH — restart your terminal" -ForegroundColor Yellow
    }
}

# ---------------------------------------------------------------------------
# Frontend dependencies
# ---------------------------------------------------------------------------
Write-Step "Installing frontend dependencies (pnpm install)..."
Push-Location (Join-Path $PSScriptRoot 'web')
try {
    & $pnpmCmd install
    Write-Ok "Frontend dependencies installed"
} finally {
    Pop-Location
}

# ---------------------------------------------------------------------------
# Go tool cache
# ---------------------------------------------------------------------------
Write-Step "Downloading Go module + tool dependencies..."
Push-Location $PSScriptRoot
try {
    go mod download
    Write-Ok "Go modules cached"
} catch {
    Write-Host "   Warning: 'go mod download' failed — run it manually later" -ForegroundColor Yellow
}
Pop-Location

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
Write-Host "`n=============================" -ForegroundColor Green
Write-Host " Setup complete!" -ForegroundColor Green
Write-Host "=============================" -ForegroundColor Green
Write-Host ""
Write-Host "  Go:    $(go version 2>$null)"
Write-Host "  Node:  $(node --version 2>$null)"
Write-Host "  pnpm:  $(& $pnpmCmd --version 2>$null)"
$makeCmd2 = Find-Command 'make'
Write-Host "  Make:  $(if ($makeCmd2) { (& $makeCmd2 --version 2>$null | Select-Object -First 1) } else { 'not found (restart terminal)' })"
$kubectlCmd2 = Find-Command 'kubectl'
Write-Host "  kubectl: $(if ($kubectlCmd2) { (& $kubectlCmd2 version --client 2>$null | Select-Object -First 1) } else { 'not found (restart terminal)' })"
Write-Host ""
Write-Host "Next steps:"
Write-Host "  make setup     # configure dune-admin (DB, SSH, etc.)"
Write-Host "  make build     # build frontend + backend"
Write-Host "  make dev       # start dev servers (air + vite)"
Write-Host ""
