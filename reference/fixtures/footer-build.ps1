[CmdletBinding()]
param(
    [string]$SourceRoot = 'output/reference-sources/zeron',
    [string]$BuildDirectory,
    [switch]$Run,
    [string]$OutputDirectory
)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$taskExport = Join-Path $taskRoot 'output/reference-native-0.2.128'
if (-not [IO.Path]::IsPathRooted($SourceRoot)) { $SourceRoot = Join-Path $taskRoot $SourceRoot }
$SourceRoot = [IO.Path]::GetFullPath($SourceRoot)
function Resolve-FooterSourcePhysical([string]$Path) {
    $taskPhysical = [IO.Path]::GetPathRoot($Path)
    foreach ($taskPart in $Path.Substring($taskPhysical.Length).Split([IO.Path]::DirectorySeparatorChar, [StringSplitOptions]::RemoveEmptyEntries)) {
        $taskPhysical = Join-Path $taskPhysical $taskPart
        if (Test-Path -LiteralPath $taskPhysical) {
            $taskSourceItem = Get-Item -LiteralPath $taskPhysical -Force
            if ($taskSourceItem.Attributes -band [IO.FileAttributes]::ReparsePoint) {
                $taskSourceTarget = $taskSourceItem.ResolveLinkTarget($true)
                if (-not $taskSourceTarget) { throw 'Could not resolve the read-only SourceRoot alias.' }
                $taskPhysical = $taskSourceTarget.FullName
            }
        }
    }
    return $taskPhysical.TrimEnd([IO.Path]::DirectorySeparatorChar)
}
$taskFrozenPhysical = Resolve-FooterSourcePhysical $SourceRoot
function Assert-FooterPath([string]$Path) {
    if (-not [IO.Path]::IsPathRooted($Path)) { $Path = Join-Path $taskRoot $Path }
    $taskResolved = [IO.Path]::GetFullPath($Path)
    if (-not $taskResolved.StartsWith($taskRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Footer writes must remain inside this checkout.' }
    if ($taskResolved.Equals($taskFrozenPhysical, [StringComparison]::OrdinalIgnoreCase) -or $taskResolved.StartsWith($taskFrozenPhysical + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Footer writes must not enter the frozen source checkout.' }
    $taskAncestor = $taskResolved
    while ($taskAncestor -and $taskAncestor.StartsWith($taskRoot, [StringComparison]::OrdinalIgnoreCase)) {
        $taskItem = Get-Item -LiteralPath $taskAncestor -Force -ErrorAction SilentlyContinue
        if ($taskItem -and ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "Footer write path contains a reparse point: $taskAncestor" }
        $taskParent = [IO.Path]::GetDirectoryName($taskAncestor)
        if ($taskParent -eq $taskAncestor) { break }
        $taskAncestor = $taskParent
    }
    return $taskResolved
}
function Resolve-FooterOutput([string]$Path, [string]$DefaultPrefix) {
    if (-not $Path) { $Path = Join-Path $taskRoot ('output/footer-' + $DefaultPrefix + '-' + [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss-fff')) }
    $taskResolved = Assert-FooterPath $Path
    $taskAllowed = Join-Path $taskRoot 'output/footer-'
    if (-not $taskResolved.StartsWith($taskAllowed, [StringComparison]::OrdinalIgnoreCase)) { throw 'New footer artifacts must use output/footer-*.' }
    if (Test-Path -LiteralPath $taskResolved) { throw 'Footer evidence requires a fresh output path.' }
    return $taskResolved
}
function Get-FooterHash([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Copy-FooterOwnedSource([string]$From, [string]$To) {
    if (-not (Test-Path -LiteralPath $To) -or (Get-FooterHash $From) -ne (Get-FooterHash $To)) { [IO.File]::Copy($From, $To, $true) }
}
function Assert-FooterCargoCache([string]$Path) {
    $taskCache = Assert-FooterPath $Path
    foreach ($taskCargoChild in @('debug', 'debug/build', 'debug/deps', 'debug/.fingerprint', 'debug/incremental', 'debug/examples', 'debug/examples/footer-native.exe')) { $null = Assert-FooterPath (Join-Path $taskCache $taskCargoChild) }
    if (Test-Path -LiteralPath $taskCache) {
        $taskCargoLinks = @(Get-ChildItem -LiteralPath $taskCache -Recurse -Force -Attributes ReparsePoint -ErrorAction Stop)
        if ($taskCargoLinks.Count -ne 0) { throw 'The shared Cargo cache contains a reparse point; preserve it and leave this build pending.' }
    }
}
$taskCommit = 'ab189159f10cf8e63c0502be75a59bbc487e0eaf'
& (Join-Path $taskRoot 'scripts/reference-verify.ps1') -SourceRoot $SourceRoot
if ((Get-Content -LiteralPath (Join-Path $taskExport '.source-commit') -Raw).Trim() -ne $taskCommit) { throw 'Frozen reference export identity differs.' }
$BuildDirectory = Resolve-FooterOutput $BuildDirectory 'reference-build'
if ($Run) { $OutputDirectory = Resolve-FooterOutput $OutputDirectory 'reference-capture' }
$taskLib = Assert-FooterPath (Join-Path $taskExport 'crates/ui/src/lib.rs')
$taskModule = Assert-FooterPath (Join-Path $taskExport 'crates/ui/src/footer_usage_fixture.rs')
$taskExample = Assert-FooterPath (Join-Path $taskExport 'crates/ui/examples/footer-native.rs')
$taskTransport = Assert-FooterPath (Join-Path $taskExport 'crates/ui/examples/footer-wgc-transport.rs')
$taskTarget = Assert-FooterPath (Join-Path $taskRoot 'output/reference-native/target')
Assert-FooterCargoCache $taskTarget
$taskProduction = @('crates/ui/src/account_usage.rs', 'crates/ui/src/context_usage.rs', 'crates/ui/src/settings/accounts.rs', 'crates/ui/src/state.rs', 'crates/proto/src/entities.rs')
$taskProductionHashes = foreach ($taskFile in $taskProduction) {
    $taskFrozenHash = Get-FooterHash (Join-Path $SourceRoot $taskFile)
    $taskExportHash = Get-FooterHash (Join-Path $taskExport $taskFile)
    if ($taskFrozenHash -ne $taskExportHash) { throw "Production source differs from the frozen clone: $taskFile" }
    @{ path = $taskFile; frozenSHA256 = $taskFrozenHash; exportSHA256 = $taskExportHash }
}
$taskImmutablePaths = @('Cargo.toml', 'Cargo.lock', 'crates/ui/Cargo.toml', 'crates/ui/examples/compact-picker-fixture.rs', 'crates/ui/examples/compact-picker-windows-lossless.rs')
$taskBefore = @{}
foreach ($taskFile in $taskImmutablePaths) { $taskBefore[$taskFile] = Get-FooterHash (Join-Path $taskExport $taskFile) }
New-Item -ItemType Directory -Path $BuildDirectory | Out-Null
[IO.File]::Copy($PSCommandPath, (Join-Path $BuildDirectory 'footer-build.ps1'), $false)
[IO.File]::Copy($taskLib, (Join-Path $BuildDirectory 'lib.rs.before'), $false)
$taskGuard = '#[cfg(feature = "compact-picker-fixture")]' + "`n" + 'pub mod footer_usage_fixture;'
$taskLibText = [IO.File]::ReadAllText($taskLib)
if (-not $taskLibText.Replace("`r`n", "`n").Contains($taskGuard)) {
    if ($taskLibText.Contains('footer_usage_fixture')) { throw 'Unrecognized footer module guard; preserving the export.' }
    [IO.File]::WriteAllText($taskLib, $taskLibText.TrimEnd() + "`n`n" + $taskGuard + "`n", [Text.UTF8Encoding]::new($false))
}
Copy-FooterOwnedSource (Join-Path $PSScriptRoot 'footer-view.rs') $taskModule
Copy-FooterOwnedSource (Join-Path $PSScriptRoot 'footer-native.rs') $taskExample
$taskTransportTemplate = Join-Path $taskRoot 'reference/fixtures/native-capture-lossless.rs'
$taskTransportText = [IO.File]::ReadAllText($taskTransportTemplate)
$taskTransportStart = $taskTransportText.IndexOf('mod lossless_transport {')
$taskTransportEnd = $taskTransportText.IndexOf('async fn capture(', $taskTransportStart)
if ($taskTransportStart -lt 0 -or $taskTransportEnd -le $taskTransportStart) { throw 'Existing transport module boundary changed.' }
$taskGeneratedTransport = $taskTransportText.Substring($taskTransportStart, $taskTransportEnd - $taskTransportStart)
if (-not (Test-Path -LiteralPath $taskTransport) -or [IO.File]::ReadAllText($taskTransport) -cne $taskGeneratedTransport) { [IO.File]::WriteAllText($taskTransport, $taskGeneratedTransport, [Text.UTF8Encoding]::new($false)) }
foreach ($taskFile in @($taskModule, $taskExample, $taskTransport, $taskLib)) { [IO.File]::Copy($taskFile, (Join-Path $BuildDirectory ([IO.Path]::GetFileName($taskFile))), $false) }
$taskEvidence = [ordered]@{
    schemaVersion = 1; referenceCommit = $taskCommit; sourceRoot = $SourceRoot; physicalSourceRoot = $taskFrozenPhysical
    scaffold = @{ clientSize = @(840,560); padding = 24; footerHeight = 24; alignment = 'end'; nativeTitle = ''; nativeAppearance = 'system'; surface = 'opaque' }
    view = 'frozen AccountUsage::new + track(Some(HarnessId::Codex),None); actual shared AccountsSnapshotCache and AppState.context_usage'
    productionSources = @($taskProductionHashes); manifestAndLockModified = $false; existingCompactFixturesModified = $false
    protectedFilesBefore = $taskBefore
    adapterModuleSHA256 = Get-FooterHash $taskModule; adapterExampleSHA256 = Get-FooterHash $taskExample; exportedLibSHA256 = Get-FooterHash $taskLib
    transportTemplateSHA256 = Get-FooterHash $taskTransportTemplate; generatedTransportSHA256 = Get-FooterHash $taskTransport
    buildScriptSHA256 = Get-FooterHash $PSCommandPath; sourceSnapshots = $BuildDirectory
    buildCommand = 'cargo build --offline --locked -j 2 --manifest-path output/reference-native-0.2.128/Cargo.toml -p zeron-ui --example footer-native --features compact-picker-fixture'
    sharedCargoCache = $taskTarget; buildStatus = 'pending'; runtimeStatus = 'not-run'; expectedScenes = 28
    capture = 'existing in-process windows-capture 1.5.0 exact own-PID HWND WGC PNG; whole frame; no crop/resize/mask or privacy override'
    interaction = 'GPUI dispatch_event synthetic mouse down/up invokes actual production on_click; no OS mouse/focus acceptance'
    acceptance = 'instrumentation adapter and reference evidence only; pixel/platform/timing acceptance pending'
}
function Save-FooterEvidence { [IO.File]::WriteAllText((Join-Path $BuildDirectory 'provenance.json'), (($taskEvidence | ConvertTo-Json -Depth 14) + "`n"), [Text.UTF8Encoding]::new($false)) }
Save-FooterEvidence
$taskPreviousTarget = $env:CARGO_TARGET_DIR
$taskPreviousSurface = $env:ZERON_PARITY_SURFACE
try {
    $env:CARGO_TARGET_DIR = $taskTarget
    & cargo build --offline --locked -j 2 --manifest-path (Join-Path $taskExport 'Cargo.toml') -p zeron-ui --example footer-native --features compact-picker-fixture 2>&1 | Tee-Object -FilePath (Join-Path $BuildDirectory 'build.log')
    if ($LASTEXITCODE -ne 0) { throw 'Footer native example build failed.' }
    foreach ($taskFile in $taskImmutablePaths) { if ((Get-FooterHash (Join-Path $taskExport $taskFile)) -ne $taskBefore[$taskFile]) { throw "Build changed a protected reference file: $taskFile" } }
    foreach ($taskSource in $taskProductionHashes) { if ((Get-FooterHash (Join-Path $taskExport $taskSource.path)) -ne $taskSource.exportSHA256) { throw 'Build changed a production component.' } }
    $taskEvidence.protectedFilesAfter = @{}
    foreach ($taskFile in $taskImmutablePaths) { $taskEvidence.protectedFilesAfter[$taskFile] = Get-FooterHash (Join-Path $taskExport $taskFile) }
    $taskBinary = Assert-FooterPath (Join-Path $BuildDirectory 'footer-native.exe')
    [IO.File]::Copy((Join-Path $taskTarget 'debug/examples/footer-native.exe'), $taskBinary, $false)
    $taskEvidence.buildStatus = 'built'; $taskEvidence.binarySHA256 = Get-FooterHash $taskBinary; $taskEvidence.binaryPath = $taskBinary
    Save-FooterEvidence
    if ($Run) {
        $OutputDirectory = Resolve-FooterOutput $OutputDirectory 'reference-capture'
        $env:ZERON_PARITY_SURFACE = 'opaque'
        & $taskBinary $OutputDirectory 2>&1 | Tee-Object -FilePath (Join-Path $BuildDirectory 'run.log')
        $taskEvidence.runtimeExitCode = $LASTEXITCODE; $taskEvidence.runtimeOutputDirectory = $OutputDirectory
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath (Join-Path $OutputDirectory 'result.json'))) { throw 'Footer capture remains pending or failed; do not change privacy settings.' }
        $taskCaptures = @(Get-ChildItem -LiteralPath $OutputDirectory -Filter '*.capture.json')
        $taskExpectedNames = foreach ($taskTheme in @('dark', 'light')) {
            foreach ($taskScene in @('known', 'capacity', 'zero', 'over', 'warning', 'danger', 'unknown')) {
                "footer-$taskScene-$taskTheme.png"
                if ($taskScene -ne 'unknown') { "footer-$taskScene-context-$taskTheme.png" }
                if ($taskScene -eq 'known') { "footer-$taskScene-accounts-$taskTheme.png" }
            }
        }
        $taskActualNames = @(Get-ChildItem -LiteralPath $OutputDirectory -Filter '*.png' | ForEach-Object Name)
        if ($taskCaptures.Count -ne 28 -or $taskActualNames.Count -ne 28 -or (Compare-Object ($taskExpectedNames | Sort-Object) ($taskActualNames | Sort-Object))) { throw 'Footer capture did not produce its exact 28 declared scenes.' }
        $taskArtifacts = foreach ($taskCapture in $taskCaptures) {
            $taskRecord = Get-Content -LiteralPath $taskCapture.FullName -Raw | ConvertFrom-Json
            $taskPng = Join-Path $OutputDirectory $taskRecord.name
            $taskBytes = [IO.File]::ReadAllBytes($taskPng)
            if ($taskBytes.Length -lt 24) { throw 'Footer PNG is incomplete.' }
            if ([Convert]::ToHexString($taskBytes[0..7]) -ne '89504E470D0A1A0A') { throw 'Footer capture is not a raw PNG.' }
            $taskWidth = [uint32]$taskBytes[16]*16777216 + [uint32]$taskBytes[17]*65536 + [uint32]$taskBytes[18]*256 + [uint32]$taskBytes[19]
            $taskHeight = [uint32]$taskBytes[20]*16777216 + [uint32]$taskBytes[21]*65536 + [uint32]$taskBytes[22]*256 + [uint32]$taskBytes[23]
            if ($taskWidth -ne $taskRecord.pixels.frameSizePx[0] -or $taskHeight -ne $taskRecord.pixels.frameSizePx[1] -or $taskWidth -ne $taskRecord.pixels.pngSizePx[0] -or $taskHeight -ne $taskRecord.pixels.pngSizePx[1]) { throw 'Original PNG dimensions do not match the complete WGC frame.' }
            if ($taskRecord.status -ne 'captured' -or (Get-FooterHash $taskPng) -ne $taskRecord.pixels.pngSHA256) { throw 'Footer capture hash/status mismatch.' }
            @{ name = $taskRecord.name; pngSHA256 = Get-FooterHash $taskPng; captureMetadataSHA256 = Get-FooterHash $taskCapture.FullName; frameSizePx = $taskRecord.pixels.frameSizePx; clientSizePx = $taskRecord.clientSizePx; scale = $taskRecord.scale }
        }
        $taskEvidence.artifacts = @($taskArtifacts); $taskEvidence.runtimeStatus = '28-production-footer-scenes-captured; parity pending'
        Save-FooterEvidence
        Write-Output $OutputDirectory
    }
    Write-Output $taskBinary
} catch {
    $taskEvidence.error = $_.Exception.Message
    if ($taskEvidence.buildStatus -eq 'pending') { $taskEvidence.buildStatus = 'failed-or-pending' }
    if ($Run) { $taskEvidence.runtimeStatus = 'pending-or-failed' }
    Save-FooterEvidence
    throw
} finally {
    $env:CARGO_TARGET_DIR = $taskPreviousTarget
    $env:ZERON_PARITY_SURFACE = $taskPreviousSurface
}
