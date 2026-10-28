param([switch]$All)
$ErrorActionPreference = 'Stop'
$taskWorkspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
& (Join-Path $PSScriptRoot 'reference-verify.ps1') -SkipGenerated
$taskArgs = @('run', '--locked', '--manifest-path', (Join-Path $taskWorkspace 'tools\reference-artifacts\Cargo.toml'), '-j', '2')
if (-not $All) { $taskArgs += @('--', '--diagrams-only') }
& cargo @taskArgs
if ($LASTEXITCODE -ne 0) { throw 'Original vector export or text outlining failed.' }
Push-Location $taskWorkspace
try {
    & go test ./internal/assets -run 'TestDiagram|TestNativeSVG' -count=1
    if ($LASTEXITCODE -ne 0) { throw 'Native vector labels failed verification.' }
} finally { Pop-Location }
Write-Output 'Original raw Mermaid SVGs and native path-only vectors regenerated; all six labels are visible in every theme.'
