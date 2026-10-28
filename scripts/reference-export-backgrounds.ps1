param([string]$SourceRoot = 'D:\zeron')
$ErrorActionPreference = 'Stop'
$taskWorkspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
& (Join-Path $PSScriptRoot 'reference-verify.ps1') -SourceRoot $SourceRoot
$taskRelativeSource = 'crates/ui/src/new_thread_background_effects.rs'
$taskSourcePath = Join-Path $SourceRoot $taskRelativeSource
$taskOriginal = Get-Content -LiteralPath $taskSourcePath -Raw
$taskMethodStart = $taskOriginal.IndexOf('    fn scanline_pixels(')
$taskMethodEnd = $taskOriginal.IndexOf('type Source =', $taskMethodStart)
$taskDitherStart = $taskOriginal.IndexOf('fn dither_color(')
$taskDitherEnd = $taskOriginal.IndexOf('#[cfg(test)]', $taskDitherStart)
if ($taskMethodStart -lt 0 -or $taskMethodEnd -lt 0 -or $taskDitherStart -lt 0 -or $taskDitherEnd -lt 0) { throw 'Original effect methods could not be located.' }
$taskExporter = Join-Path $taskWorkspace 'reference\background-exporter'
$taskAdapter = Get-Content -LiteralPath (Join-Path $taskExporter 'src\effects.rs') -Raw
if (-not $taskAdapter.Contains($taskOriginal.Substring($taskMethodStart, $taskMethodEnd - $taskMethodStart)) -or
    -not $taskAdapter.Contains($taskOriginal.Substring($taskDitherStart, $taskDitherEnd - $taskDitherStart))) {
    throw 'Original raster methods changed in the standalone exporter.'
}
$taskOutput = Join-Path $taskWorkspace 'internal\design\testdata\background'
& cargo run --locked --quiet --manifest-path (Join-Path $taskExporter 'Cargo.toml') -j 2 -- $taskOutput
if ($LASTEXITCODE -ne 0) { throw 'Original background raster export failed.' }
$taskBaseline = Get-Content -LiteralPath (Join-Path $taskWorkspace 'reference\baseline.json') -Raw | ConvertFrom-Json
$taskFiles = foreach ($taskFile in Get-ChildItem -LiteralPath $taskOutput -File | Sort-Object Name) {
    [ordered]@{ name = $taskFile.Name; bytes = $taskFile.Length; sha256 = (Get-FileHash -LiteralPath $taskFile.FullName -Algorithm SHA256).Hash.ToLowerInvariant() }
}
[ordered]@{
    schemaVersion = 1
    sourceCommit = $taskBaseline.sourceCommit
    source = $taskRelativeSource
    sourceSha256 = (Get-FileHash -LiteralPath $taskSourcePath -Algorithm SHA256).Hash.ToLowerInvariant()
    exporterSourceSha256 = (Get-FileHash -LiteralPath (Join-Path $taskExporter 'src\effects.rs') -Algorithm SHA256).Hash.ToLowerInvariant()
    imageCrate = '0.25.10'
    moxcmsCrate = '0.8.1'
    rgbaSourceSize = @(33, 19)
    classification = 'original-raster-algorithm-unit-reference'
    nativeScreenshot = $false
    files = @($taskFiles)
} | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $taskWorkspace 'reference\background-pixels.json')
Write-Output 'Exported original raster bytes. These are algorithm references, not native UI screenshot goldens.'
