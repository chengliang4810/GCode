[CmdletBinding()]
param([string]$Compiler = 'output/tools/w64devkit-v2.10.1/w64devkit/bin/gcc.exe')
$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$taskCompiler = if ([IO.Path]::IsPathRooted($Compiler)) { [IO.Path]::GetFullPath($Compiler) } else { [IO.Path]::GetFullPath((Join-Path $taskRoot $Compiler)) }
if (-not (Test-Path -LiteralPath $taskCompiler)) { throw 'Provide a compatible Windows x64 GCC using -Compiler.' }
$taskOriginalCgo,$taskOriginalCc,$taskOriginalPath = $env:CGO_ENABLED,$env:CC,$env:PATH
Push-Location -LiteralPath $taskRoot
try {
    $env:CGO_ENABLED='1'
    $env:CC=$taskCompiler
    $env:PATH=(Split-Path -Parent $taskCompiler)+';'+$taskOriginalPath
    & go test -race -count=1 -p 1 ./internal/contracts ./internal/mock ./internal/desktop ./internal/workbench
    if ($LASTEXITCODE -ne 0) { throw 'Race checks failed.' }
} finally {
    $env:CGO_ENABLED,$env:CC,$env:PATH = $taskOriginalCgo,$taskOriginalCc,$taskOriginalPath
    Pop-Location
}
