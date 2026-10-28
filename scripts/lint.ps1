[CmdletBinding()]
param([string]$ToolPath)
$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
Push-Location -LiteralPath $taskRoot
try {
    if ($ToolPath) {
        $taskVersion = & $ToolPath version
        if ($LASTEXITCODE -ne 0 -or $taskVersion -notmatch 'version 2\.15\.0\b') { throw 'Expected golangci-lint 2.15.0.' }
        & $ToolPath run --concurrency 2 --timeout 5m
    } else {
        & go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run --concurrency 2 --timeout 5m
    }
    if ($LASTEXITCODE -ne 0) { throw 'Project lint failed.' }
} finally { Pop-Location }
