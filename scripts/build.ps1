# AVLine-Sight ビルドスクリプト: WebUIをビルドし、Go単一実行ファイルへ同梱する。
# 使い方: pwsh scripts/build.ps1 [-Version 0.2.0]
param([string]$Version = "0.2.0")
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Push-Location (Join-Path $root "web")
try {
    if (-not (Test-Path node_modules)) { npm ci --no-audit --no-fund }
    npm run build
} finally { Pop-Location }
Push-Location $root
try {
    $env:CGO_ENABLED = "0"
    go test ./internal/...
    if ($LASTEXITCODE -ne 0) { throw "テストが失敗しました" }
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o bin/avline-sight.exe ./cmd/avline-sight
    if ($LASTEXITCODE -ne 0) { throw "ビルドが失敗しました" }
    Write-Host "ビルド完了: bin/avline-sight.exe ($Version)"
} finally { Pop-Location }
