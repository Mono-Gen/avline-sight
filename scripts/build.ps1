# AVLine-Sight ビルドスクリプト: WebUIをビルドし、Go単一実行ファイルへ同梱する。
# 使い方: pwsh scripts/build.ps1 [-Version 0.5.0]
param([string]$Version = "0.5.0")
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Push-Location (Join-Path $root "web")
try {
    if (-not (Test-Path node_modules)) { npm ci --no-audit --no-fund }
    npm run build
} finally { Pop-Location }
Push-Location $root
try {
    # アイコンとバージョン情報を埋め込むリソースを、このバージョンで作り直す（windres が無ければ同梱の物を使う）
    if (Get-Command windres -ErrorAction SilentlyContinue) {
        pwsh -File (Join-Path $PSScriptRoot "make_resources.ps1") -Version $Version
        if ($LASTEXITCODE -ne 0) { throw "リソースの作成に失敗しました" }
    } else {
        Write-Warning "windres が無いため、同梱のリソース（アイコン・バージョン情報）をそのまま使います"
    }
    $env:CGO_ENABLED = "0"
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw "テストが失敗しました" }
    go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$Version" -o bin/avline-sight.exe ./cmd/avline-sight
    if ($LASTEXITCODE -ne 0) { throw "ビルドが失敗しました" }
    Write-Host "ビルド完了: bin/avline-sight.exe ($Version)"
} finally { Pop-Location }
