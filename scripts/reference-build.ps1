param([string]$SourceRoot = 'D:\zeron', [string]$Example = 'sidebar-fixture', [string]$Features = 'project-palette-fixture')
$ErrorActionPreference = 'Stop'
$taskWorkspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
& (Join-Path $PSScriptRoot 'reference-verify.ps1') -SourceRoot $SourceRoot
$taskDestination = [IO.Path]::GetFullPath((Join-Path $taskWorkspace 'output\reference-native-0.2.128'))
$taskTarget = Join-Path $taskWorkspace 'output\reference-native\target'
if (-not $taskDestination.StartsWith($taskWorkspace + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Reference build directory escapes the workspace.' }
if (-not (Test-Path -LiteralPath (Join-Path $taskDestination 'Cargo.toml'))) {
    New-Item -ItemType Directory -Path $taskDestination -Force | Out-Null
    $taskArchive = Join-Path $taskWorkspace 'output\reference-native-0.2.128-source.zip'
    & git -C $SourceRoot archive --format=zip --output=$taskArchive ab189159f10cf8e63c0502be75a59bbc487e0eaf
    if ($LASTEXITCODE -ne 0) { throw 'Could not archive reference commit.' }
    Expand-Archive -LiteralPath $taskArchive -DestinationPath $taskDestination
    [IO.File]::WriteAllText((Join-Path $taskDestination '.source-commit'), 'ab189159f10cf8e63c0502be75a59bbc487e0eaf')
}
if ((Get-Content -LiteralPath (Join-Path $taskDestination '.source-commit') -Raw).Trim() -ne 'ab189159f10cf8e63c0502be75a59bbc487e0eaf') { throw 'Extracted reference source has a different baseline.' }
$env:CARGO_TARGET_DIR = $taskTarget
& cargo build --locked -j 2 --manifest-path (Join-Path $taskDestination 'Cargo.toml') -p zeron-ui --example $Example --features $Features
if ($LASTEXITCODE -ne 0) { throw 'Native GPUI reference build failed; no visual parity may be claimed.' }
Write-Output (Join-Path $taskTarget ('debug\examples\' + $Example + '.exe'))
