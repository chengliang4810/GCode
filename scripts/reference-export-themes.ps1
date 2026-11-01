param([string]$SourceRoot = 'D:\zeron')
$ErrorActionPreference = 'Stop'
$taskWorkspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
& (Join-Path $PSScriptRoot 'reference-verify.ps1') -SourceRoot $SourceRoot
if ($LASTEXITCODE -ne 0) { throw 'Reference verification failed.' }
$taskExporter = Join-Path $taskWorkspace 'reference\theme-exporter'
foreach ($taskName in @('lib.rs', 'builtins.rs', 'library.rs', 'vscode.rs')) {
    $taskSourceHash = (Get-FileHash -LiteralPath (Join-Path $SourceRoot ('crates\theme\src\' + $taskName)) -Algorithm SHA256).Hash
    $taskExporterHash = (Get-FileHash -LiteralPath (Join-Path $taskExporter ('src\' + $taskName)) -Algorithm SHA256).Hash
    if ($taskSourceHash -ne $taskExporterHash) { throw "Exporter source mismatch: $taskName" }
}
$taskJSON = & cargo run --locked --quiet --manifest-path (Join-Path $taskExporter 'Cargo.toml')
if ($LASTEXITCODE -ne 0) { throw 'Original theme registry export failed.' }
$taskRegistry = ($taskJSON -join "`n") | ConvertFrom-Json
foreach ($taskFamily in $taskRegistry.families) {
    if ($taskFamily.id -ne 'zeron') { continue }
    $taskFamily.id = 'rcode'
    $taskFamily.name = 'GCode'
    foreach ($taskVariant in $taskFamily.variants) {
        $taskVariant.id = $taskVariant.id.Replace('zeron-', 'rcode-')
        $taskVariant.familyId = 'rcode'
        $taskVariant.name = $taskVariant.name.Replace('Zeron', 'GCode')
    }
}
$taskJSON = $taskRegistry | ConvertTo-Json -Depth 100
$taskCount = @($taskRegistry.families | ForEach-Object { $_.variants }).Count
if ($taskCount -ne 30) { throw "Expected 30 original variants; got $taskCount." }
[IO.File]::WriteAllText((Join-Path $taskWorkspace 'internal\design\themes.json'), ($taskJSON -join "`n") + "`n", [Text.UTF8Encoding]::new($false))
Write-Output 'Exported 30 resolved palettes with GCode theme identifiers; upstream source and provenance are unchanged.'
