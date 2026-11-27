param(
    [string]$Version = "v1.0.0",
    [switch]$IncludeDesktop,
    [switch]$Publish
)

$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent $PSScriptRoot
$DistDir = Join-Path $RepoRoot "dist"
New-Item -ItemType Directory -Force -Path $DistDir | Out-Null

$Commit = (git -C $RepoRoot rev-parse --short HEAD 2>$null)
if (-not $Commit) { $Commit = "dev" }
$LdFlags = "-s -w -X main.Version=$Version -X main.Commit=$Commit"

$Targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Out = "mw-ww-cli-windows-amd64.exe" },
    @{ GOOS = "linux";   GOARCH = "amd64"; Out = "mw-ww-cli-linux-amd64" },
    @{ GOOS = "linux";   GOARCH = "arm64"; Out = "mw-ww-cli-linux-arm64" }
)

Push-Location $RepoRoot
try {
    foreach ($t in $Targets) {
        Write-Host "Building $($t.Out)..."
        $env:GOOS = $t.GOOS
        $env:GOARCH = $t.GOARCH
        $env:CGO_ENABLED = "0"
        $outPath = Join-Path $DistDir $t.Out
        & go build -ldflags $LdFlags -o $outPath ./cmd/mw-ww-cli
        if ($LASTEXITCODE -ne 0) { throw "Build failed for $($t.Out)" }
    }

    if ($IncludeDesktop) {
        Remove-Item Env:GOOS -ErrorAction SilentlyContinue
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
        $desktopBin = Join-Path $DistDir "mw-ww-windows-amd64.exe"
        $existingExe = Join-Path $RepoRoot "mw-ww.exe"
        if (Get-Command wails -ErrorAction SilentlyContinue) {
            & wails build -clean -ldflags "-s -w"
            if ($LASTEXITCODE -ne 0) { throw "wails build failed" }
            $wailsOut = Join-Path $RepoRoot "build\bin\mw-ww.exe"
            if (Test-Path $wailsOut) { Copy-Item -Force $wailsOut $desktopBin }
        } elseif (Test-Path $existingExe) {
            Copy-Item -Force $existingExe $desktopBin
        }
    }

    $ChecksumLines = @()
    Get-ChildItem -Path $DistDir -File | Where-Object { $_.Name -ne "checksums.txt" } | Sort-Object Name | ForEach-Object {
        $hash = (Get-FileHash -Path $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $ChecksumLines += "$hash  $($_.Name)"
    }
    Set-Content -Path (Join-Path $DistDir "checksums.txt") -Value $ChecksumLines -Encoding ascii

    if ($Publish) {
        $Assets = Get-ChildItem -Path $DistDir -File | Select-Object -ExpandProperty FullName
        & gh release create $Version $Assets --title "mw-ww $Version" --notes "Zero-card local build release ($Commit)"
        if ($LASTEXITCODE -ne 0) { throw "gh release create failed" }
    }
}
finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    Pop-Location
}
