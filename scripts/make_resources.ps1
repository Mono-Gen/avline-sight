# 実行ファイルに埋め込むWindowsリソース（アイコン・バージョン情報）を作る。
# windres（MinGW付属）が要る。生成物 cmd/avline-sight/rsrc_windows_amd64.syso は
# リポジトリに含めるので、windres の無い環境でもアイコン付きでビルドできる。
# 使い方: pwsh scripts/make_resources.ps1 -Version 0.4.0
param([Parameter(Mandatory = $true)][string]$Version)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
if ($Version -notmatch '^(\d+)\.(\d+)\.(\d+)$') { throw "バージョンは X.Y.Z の形で指定してください: $Version" }
$comma = "$($Matches[1]),$($Matches[2]),$($Matches[3]),0"
$windres = Get-Command windres -ErrorAction SilentlyContinue
if (-not $windres) { throw "windres が見つかりません（MinGW を入れるか、PATH に追加してください）" }
$dir = Join-Path $root "cmd/avline-sight"
$rc = @"
#include <winver.h>
1 ICON "assets/app.ico"
1 VERSIONINFO
FILEVERSION $comma
PRODUCTVERSION $comma
FILEOS VOS_NT_WINDOWS32
FILETYPE VFT_APP
BEGIN
  BLOCK "StringFileInfo"
  BEGIN
    BLOCK "040904B0"
    BEGIN
      VALUE "CompanyName", "Mono-Gen"
      VALUE "FileDescription", "AVLine-Sight - AV over IP network monitor"
      VALUE "FileVersion", "$Version"
      VALUE "InternalName", "avline-sight"
      VALUE "LegalCopyright", "MIT License"
      VALUE "OriginalFilename", "avline-sight.exe"
      VALUE "ProductName", "AVLine-Sight"
      VALUE "ProductVersion", "$Version"
    END
  END
  BLOCK "VarFileInfo"
  BEGIN
    VALUE "Translation", 0x0409, 1200
  END
END
"@
$rcPath = Join-Path $dir "app.rc"
Set-Content -Path $rcPath -Value $rc -Encoding ascii
Push-Location $dir
try {
    & $windres.Source --input app.rc --output-format=coff --target=pe-x86-64 --output rsrc_windows_amd64.syso
    if ($LASTEXITCODE -ne 0) { throw "windres が失敗しました" }
} finally {
    Pop-Location
    Remove-Item $rcPath
}
Write-Host "リソースを作りました: cmd/avline-sight/rsrc_windows_amd64.syso ($Version)"
