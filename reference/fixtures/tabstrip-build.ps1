[CmdletBinding()]
param(
    [string]$SourceRoot = 'output/reference-sources/zeron',
    [string]$BuildDirectory
)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$taskExport = Join-Path $taskRoot 'output/reference-native-0.2.128'
$taskCommit = 'ab189159f10cf8e63c0502be75a59bbc487e0eaf'
if (-not [IO.Path]::IsPathRooted($SourceRoot)) { $SourceRoot = Join-Path $taskRoot $SourceRoot }
$SourceRoot = [IO.Path]::GetFullPath($SourceRoot)

function Resolve-TabstripPhysical([string]$Path) {
    $taskPhysical = [IO.Path]::GetPathRoot($Path)
    foreach ($taskPart in $Path.Substring($taskPhysical.Length).Split([IO.Path]::DirectorySeparatorChar, [StringSplitOptions]::RemoveEmptyEntries)) {
        $taskPhysical = Join-Path $taskPhysical $taskPart
        if (Test-Path -LiteralPath $taskPhysical) {
            $taskItem = Get-Item -LiteralPath $taskPhysical -Force
            if ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint) {
                $taskTarget = $taskItem.ResolveLinkTarget($true)
                if (-not $taskTarget) { throw 'Cannot resolve frozen source alias.' }
                $taskPhysical = $taskTarget.FullName
            }
        }
    }
    $taskPhysical.TrimEnd([IO.Path]::DirectorySeparatorChar)
}
$taskFrozenPhysical = Resolve-TabstripPhysical $SourceRoot
function Assert-TabstripWrite([string]$Path) {
    if (-not [IO.Path]::IsPathRooted($Path)) { $Path = Join-Path $taskRoot $Path }
    $taskResolved = [IO.Path]::GetFullPath($Path)
    if (-not $taskResolved.StartsWith($taskRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'TabStrip writes must remain inside this checkout.' }
    if ($taskResolved.Equals($taskFrozenPhysical, [StringComparison]::OrdinalIgnoreCase) -or $taskResolved.StartsWith($taskFrozenPhysical + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'TabStrip writes must not enter frozen source.' }
    $taskAncestor = $taskResolved
    while ($taskAncestor -and $taskAncestor.StartsWith($taskRoot, [StringComparison]::OrdinalIgnoreCase)) {
        $taskItem = Get-Item -LiteralPath $taskAncestor -Force -ErrorAction SilentlyContinue
        if ($taskItem -and ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "Write path contains a reparse point: $taskAncestor" }
        $taskParent = [IO.Path]::GetDirectoryName($taskAncestor)
        if ($taskParent -eq $taskAncestor) { break }
        $taskAncestor = $taskParent
    }
    $taskResolved
}
function Get-TabstripHash([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Copy-TabstripOwned([string]$From, [string]$To) {
    $null = Assert-TabstripWrite $To
    if (-not (Test-Path -LiteralPath $To) -or (Get-TabstripHash $From) -ne (Get-TabstripHash $To)) { [IO.File]::Copy($From, $To, $true) }
}
function Assert-TabstripByteEqual([byte[]]$Actual, [byte[]]$Expected) {
    if ($Actual.Length -ne $Expected.Length) { return $false }
    for ($taskIx = 0; $taskIx -lt $Actual.Length; $taskIx++) { if ($Actual[$taskIx] -ne $Expected[$taskIx]) { return $false } }
    return $true
}

$taskActualCommit = (& git -C $SourceRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $taskActualCommit -ne $taskCommit) { throw 'Frozen source commit mismatch.' }
& (Join-Path $taskRoot 'scripts/reference-verify.ps1') -SourceRoot $SourceRoot
if ((Get-Content -LiteralPath (Join-Path $taskExport '.source-commit') -Raw).Trim() -ne $taskCommit) { throw 'Export source identity stamp mismatch.' }
if (-not $BuildDirectory) { $BuildDirectory = Join-Path $taskRoot ('output/tabstrip-reference-build-' + [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss-fff')) }
$BuildDirectory = Assert-TabstripWrite $BuildDirectory
if (-not $BuildDirectory.StartsWith((Join-Path $taskRoot 'output/tabstrip-reference-build-'), [StringComparison]::OrdinalIgnoreCase)) { throw 'Use a new output/tabstrip-reference-build-* path.' }
if (Test-Path -LiteralPath $BuildDirectory) { throw 'Preserve existing build evidence; choose a new path.' }
$taskTarget = Assert-TabstripWrite (Join-Path $taskRoot 'output/reference-native/target')
foreach ($taskChild in @('debug','debug/build','debug/deps','debug/.fingerprint','debug/incremental','debug/examples','debug/examples/tabstrip-native.exe')) { $null = Assert-TabstripWrite (Join-Path $taskTarget $taskChild) }
if (Test-Path -LiteralPath $taskTarget) {
    if (@(Get-ChildItem -LiteralPath $taskTarget -Recurse -Force -Attributes ReparsePoint).Count -ne 0) { throw 'Shared Cargo target contains reparse points; preserve it.' }
}

$taskShell = Assert-TabstripWrite (Join-Path $taskExport 'crates/ui/src/shell.rs')
$taskGuard = "`n" + '#[cfg(feature = "compact-picker-fixture")]' + "`n" + 'pub mod tabstrip_fixture;' + "`n"
$taskGuardBytes = [Text.UTF8Encoding]::new($false).GetBytes($taskGuard)
$taskFrozenShellBytes = [IO.File]::ReadAllBytes((Join-Path $SourceRoot 'crates/ui/src/shell.rs'))
$taskExpectedShellBytes = [byte[]]::new($taskFrozenShellBytes.Length + $taskGuardBytes.Length)
[Array]::Copy($taskFrozenShellBytes, 0, $taskExpectedShellBytes, 0, $taskFrozenShellBytes.Length)
[Array]::Copy($taskGuardBytes, 0, $taskExpectedShellBytes, $taskFrozenShellBytes.Length, $taskGuardBytes.Length)
$taskShellBefore = [IO.File]::ReadAllBytes($taskShell)
$taskHasGuard = Assert-TabstripByteEqual $taskShellBefore $taskExpectedShellBytes
if (-not $taskHasGuard -and -not (Assert-TabstripByteEqual $taskShellBefore $taskFrozenShellBytes)) { throw 'Export Shell has an unexpected delta; preserve it.' }

$taskPreview = Assert-TabstripWrite (Join-Path $taskExport 'crates/ui/src/files/preview.rs')
$taskPreviewGuard = "`n" + '#[cfg(feature = "compact-picker-fixture")]' + "`n" + 'mod tabstrip_fixture;' + "`n"
$taskPreviewGuardBytes = [Text.UTF8Encoding]::new($false).GetBytes($taskPreviewGuard)
$taskFrozenPreviewBytes = [IO.File]::ReadAllBytes((Join-Path $SourceRoot 'crates/ui/src/files/preview.rs'))
$taskExpectedPreviewBytes = [byte[]]::new($taskFrozenPreviewBytes.Length + $taskPreviewGuardBytes.Length)
[Array]::Copy($taskFrozenPreviewBytes, 0, $taskExpectedPreviewBytes, 0, $taskFrozenPreviewBytes.Length)
[Array]::Copy($taskPreviewGuardBytes, 0, $taskExpectedPreviewBytes, $taskFrozenPreviewBytes.Length, $taskPreviewGuardBytes.Length)
$taskPreviewBefore = [IO.File]::ReadAllBytes($taskPreview)
$taskPreviewHasGuard = Assert-TabstripByteEqual $taskPreviewBefore $taskExpectedPreviewBytes
if (-not $taskPreviewHasGuard -and -not (Assert-TabstripByteEqual $taskPreviewBefore $taskFrozenPreviewBytes)) { throw 'Export preview.rs has an unexpected delta; preserve it.' }
if ((Get-TabstripHash (Join-Path $PSScriptRoot 'tabstrip-contract.json')) -ne '16f54432c962f447798cd40f5429107b425093224f28a8e7d4c021336ce5468c') { throw 'R14 baseline contract must remain byte-identical; style contract is independent.' }

$taskProductionPaths = @('crates/ui/src/shell.rs','crates/ui/src/shell/tabs.rs','crates/ui/src/shell/side_chats.rs','crates/ui/src/surface_chrome.rs','crates/ui/src/state.rs','crates/ui/src/theme.rs','crates/ui/src/typography.rs','crates/ui/src/browser/mod.rs','crates/proto/src/entities.rs','crates/ui/src/files/preview.rs','crates/ui/src/files/mod.rs','crates/ui/src/files/document.rs','crates/ui/src/files/editor.rs','crates/ui/src/files/client.rs','crates/ui/src/file_icons.rs')
$taskProduction = foreach ($taskPath in $taskProductionPaths) {
    $taskFrozenHash = Get-TabstripHash (Join-Path $SourceRoot $taskPath)
    $taskExportHash = Get-TabstripHash (Join-Path $taskExport $taskPath)
    $taskAppendAllowed = $taskPath -in @('crates/ui/src/shell.rs','crates/ui/src/files/preview.rs')
    if (-not $taskAppendAllowed -and $taskFrozenHash -ne $taskExportHash) { throw "Production component differs: $taskPath" }
    [ordered]@{path=$taskPath;frozenSHA256=$taskFrozenHash;exportBeforeSHA256=$taskExportHash;allowedDelta=$(if($taskAppendAllowed){'exact frozen bytes plus one feature-guarded module declaration'}else{'none'})}
}
$taskProtectedPaths = @('Cargo.toml','Cargo.lock','crates/ui/Cargo.toml','crates/ui/src/lib.rs','crates/ui/src/footer_usage_fixture.rs','crates/ui/examples/footer-native.rs','crates/ui/examples/footer-wgc-transport.rs','crates/ui/examples/compact-picker-fixture.rs','crates/ui/examples/compact-picker-windows-lossless.rs','crates/ui/examples/tabstrip-contract.json')
$taskProtectedBefore = [ordered]@{}
foreach ($taskPath in $taskProtectedPaths) { $taskProtectedBefore[$taskPath] = Get-TabstripHash (Join-Path $taskExport $taskPath) }

New-Item -ItemType Directory -Path $BuildDirectory | Out-Null
[IO.File]::Copy($taskShell, (Join-Path $BuildDirectory 'shell.rs.before'), $false)
[IO.File]::Copy($taskPreview, (Join-Path $BuildDirectory 'preview.rs.before'), $false)
[IO.File]::Copy($PSCommandPath, (Join-Path $BuildDirectory 'tabstrip-build.ps1'), $false)
foreach ($taskPath in @('Cargo.toml','Cargo.lock','crates/ui/Cargo.toml','crates/ui/src/lib.rs')) { [IO.File]::Copy((Join-Path $taskExport $taskPath), (Join-Path $BuildDirectory ($taskPath.Replace('/','__') + '.before')), $false) }
$taskEvidence = [ordered]@{
    schemaVersion=1;stage='R17';referenceCommit=$taskCommit;sourceRoot=$SourceRoot;physicalSourceRoot=$taskFrozenPhysical;exportRoot=$taskExport
    productionSources=@($taskProduction);protectedFilesBefore=$taskProtectedBefore;shellOriginalPrefixSHA256=Get-TabstripHash (Join-Path $SourceRoot 'crates/ui/src/shell.rs')
    shellGuardAlreadyPresent=$taskHasGuard;shellGuardText=$taskGuard;shellSourceSnapshotBefore=Join-Path $BuildDirectory 'shell.rs.before'
    inheritedExportDependencies='Existing Windows dev windows-capture=1.5.0 and prior lock differences are preserved byte-for-byte. .source-commit is an origin stamp, not whole-tree byte equality.'
    previewOriginalPrefixSHA256=Get-TabstripHash (Join-Path $SourceRoot 'crates/ui/src/files/preview.rs');previewGuardAlreadyPresent=$taskPreviewHasGuard;previewGuardText=$taskPreviewGuard
    permittedExistingFileDelta='shell.rs and files/preview.rs each exact append-only feature guard; original prefixes byte-identical; no production method body edits'
    manifestAndLockModified=$false;libModified=$false;oldFixturesModified=$false;productionMethodBodiesModified=$false
    buildCommand='cargo build --offline --locked -j 2 --manifest-path output/reference-native-0.2.128/Cargo.toml -p zeron-ui --example tabstrip-native --features compact-picker-fixture'
    sharedCargoTarget=$taskTarget;buildStatus='not-started';runtimeStatus='not-run; parent owns native slot';expectedScenes=36
    visualParityAcceptance='pending actual independent native capture and strict metadata/pixel comparison'
}
function Save-TabstripEvidence { [IO.File]::WriteAllText((Join-Path $BuildDirectory 'provenance.json'), (($taskEvidence | ConvertTo-Json -Depth 16)+"`n"), [Text.UTF8Encoding]::new($false)) }
Save-TabstripEvidence

$taskModule = Assert-TabstripWrite (Join-Path $taskExport 'crates/ui/src/shell/tabstrip_fixture.rs')
$taskExample = Assert-TabstripWrite (Join-Path $taskExport 'crates/ui/examples/tabstrip-native.rs')
$taskTransport = Assert-TabstripWrite (Join-Path $taskExport 'crates/ui/examples/tabstrip-wgc-transport.rs')
$taskContract = Assert-TabstripWrite (Join-Path $taskExport 'crates/ui/examples/tabstrip-contract.json')
$taskSourceManifest = Assert-TabstripWrite (Join-Path $taskExport 'crates/ui/examples/tabstrip-source-manifest.json')
$taskStyleContract = Assert-TabstripWrite (Join-Path $taskExport 'crates/ui/examples/tabstrip-style-contract.json')
$taskStyleNative = Assert-TabstripWrite (Join-Path $taskExport 'crates/ui/examples/tabstrip-style-native.rs')
$taskFileBridge = Assert-TabstripWrite (Join-Path $taskExport 'crates/ui/src/files/preview/tabstrip_fixture.rs')
$taskPreviousTarget = $env:CARGO_TARGET_DIR
try {
    if (-not $taskHasGuard) { [IO.File]::WriteAllBytes($taskShell, $taskExpectedShellBytes) }
    if (-not $taskPreviewHasGuard) { [IO.File]::WriteAllBytes($taskPreview, $taskExpectedPreviewBytes) }
    $taskFileBridgeDirectory = Assert-TabstripWrite ([IO.Path]::GetDirectoryName($taskFileBridge))
    if (-not (Test-Path -LiteralPath $taskFileBridgeDirectory)) { New-Item -ItemType Directory -Path $taskFileBridgeDirectory | Out-Null }
    Copy-TabstripOwned (Join-Path $PSScriptRoot 'tabstrip-view.rs') $taskModule
    Copy-TabstripOwned (Join-Path $PSScriptRoot 'tabstrip-native.rs') $taskExample
    Copy-TabstripOwned (Join-Path $PSScriptRoot 'tabstrip-contract.json') $taskContract
    Copy-TabstripOwned (Join-Path $PSScriptRoot 'tabstrip-style-contract.json') $taskStyleContract
    Copy-TabstripOwned (Join-Path $PSScriptRoot 'tabstrip-style-native.rs') $taskStyleNative
    Copy-TabstripOwned (Join-Path $PSScriptRoot 'tabstrip-file-bridge.rs') $taskFileBridge
    $taskTemplate = Join-Path $PSScriptRoot 'native-capture-lossless.rs'
    $taskTemplateText = [IO.File]::ReadAllText($taskTemplate)
    $taskStart = $taskTemplateText.IndexOf('mod lossless_transport {')
    $taskEnd = $taskTemplateText.IndexOf('async fn capture(', $taskStart)
    if ($taskStart -lt 0 -or $taskEnd -le $taskStart) { throw 'Original transport module boundary changed.' }
    $taskTransportText = $taskTemplateText.Substring($taskStart, $taskEnd-$taskStart)
    if (-not (Test-Path -LiteralPath $taskTransport) -or [IO.File]::ReadAllText($taskTransport) -cne $taskTransportText) { [IO.File]::WriteAllText($taskTransport, $taskTransportText, [Text.UTF8Encoding]::new($false)) }
    $taskSource = [ordered]@{
        schemaVersion=1;referenceCommit=$taskCommit;productionSources=@($taskProduction);protectedExportSHA256=$taskProtectedBefore
        shellBeforeSHA256=Get-TabstripHash (Join-Path $BuildDirectory 'shell.rs.before');shellAfterSHA256=Get-TabstripHash $taskShell
        shellGuardText=$taskGuard;productionMethodBodiesChanged=$false;originalPrefixByteIdentical=$true
        previewBeforeSHA256=Get-TabstripHash (Join-Path $BuildDirectory 'preview.rs.before');previewAfterSHA256=Get-TabstripHash $taskPreview
        previewGuardText=$taskPreviewGuard;previewOriginalPrefixByteIdentical=$true
        adapterModuleSHA256=Get-TabstripHash $taskModule;adapterExampleSHA256=Get-TabstripHash $taskExample
        fileBridgeSHA256=Get-TabstripHash $taskFileBridge;styleNativeSHA256=Get-TabstripHash $taskStyleNative;styleContractSHA256=Get-TabstripHash $taskStyleContract
        contractSHA256=Get-TabstripHash $taskContract;transportSHA256=Get-TabstripHash $taskTransport;transportTemplateSHA256=Get-TabstripHash $taskTemplate;buildScriptSHA256=Get-TabstripHash $PSCommandPath
        measurement='public ScrollHandle bounds, before-scroll item bounds plus public pixel_snap_point(offset); source-proven wrapper canvas and on_children_prepainted; no Inspector'
    }
    [IO.File]::WriteAllText($taskSourceManifest, (($taskSource | ConvertTo-Json -Depth 14)+"`n"), [Text.UTF8Encoding]::new($false))
    foreach ($taskFile in @($taskModule,$taskExample,$taskContract,$taskTransport,$taskSourceManifest,$taskShell,$taskPreview,$taskStyleNative,$taskStyleContract)) { [IO.File]::Copy($taskFile, (Join-Path $BuildDirectory ([IO.Path]::GetFileName($taskFile))), $false) }
    [IO.File]::Copy($taskFileBridge,(Join-Path $BuildDirectory 'tabstrip-file-bridge.rs'),$false)
    $taskEvidence.sourceManifestSHA256=Get-TabstripHash $taskSourceManifest
    $taskEvidence.shellAfterSHA256=Get-TabstripHash $taskShell
    $taskEvidence.buildStatus='running'
    Save-TabstripEvidence
    $env:CARGO_TARGET_DIR=$taskTarget
    & cargo build --offline --locked -j 2 --manifest-path (Join-Path $taskExport 'Cargo.toml') -p zeron-ui --example tabstrip-native --features compact-picker-fixture 2>&1 | Tee-Object -FilePath (Join-Path $BuildDirectory 'build.log')
    $taskEvidence.buildExitCode=$LASTEXITCODE
    if ($LASTEXITCODE -ne 0) { throw 'TabStrip reference build failed; preserve diagnostics.' }
    if (-not (Assert-TabstripByteEqual ([IO.File]::ReadAllBytes($taskShell)) $taskExpectedShellBytes)) { throw 'Build changed Shell beyond the exact append-only guard.' }
    if (-not (Assert-TabstripByteEqual ([IO.File]::ReadAllBytes($taskPreview)) $taskExpectedPreviewBytes)) { throw 'Build changed preview.rs beyond the exact append-only guard.' }
    foreach ($taskPath in $taskProductionPaths) {
        if ($taskPath -in @('crates/ui/src/shell.rs','crates/ui/src/files/preview.rs')) { continue }
        if ((Get-TabstripHash (Join-Path $taskExport $taskPath)) -ne (Get-TabstripHash (Join-Path $SourceRoot $taskPath))) { throw "Build changed production source: $taskPath" }
    }
    $taskEvidence.protectedFilesAfter=[ordered]@{}
    foreach ($taskPath in $taskProtectedPaths) {
        $taskAfter=Get-TabstripHash (Join-Path $taskExport $taskPath)
        $taskEvidence.protectedFilesAfter[$taskPath]=$taskAfter
        if ($taskAfter -ne $taskProtectedBefore[$taskPath]) { throw "Build changed protected file: $taskPath" }
    }
    $taskBinary=Assert-TabstripWrite (Join-Path $BuildDirectory 'tabstrip-native.exe')
    [IO.File]::Copy((Join-Path $taskTarget 'debug/examples/tabstrip-native.exe'), $taskBinary, $false)
    $taskEvidence.binaryPath=$taskBinary;$taskEvidence.binarySHA256=Get-TabstripHash $taskBinary;$taskEvidence.buildStatus='built';$taskEvidence.sourceManifest=$taskSource
    $taskEvidence.runtimeCommand='tabstrip-native.exe --out-dir NEW_CAPTURE_DIRECTORY --expected-scale 1_OR_2'
    $taskEvidence.styleRuntimeCommand='tabstrip-native.exe --out-dir NEW_STYLE_CAPTURE_DIRECTORY --expected-scale 1_OR_2 --style-case inactive-hover_OR_dirty-file_OR_file-hover_OR_file-hover-edge [--require-native-cursor] [--hover-dwell-ms 1000]'
    $taskEvidence.styleExpectedScenes=2
    Save-TabstripEvidence
    Write-Output $taskBinary
} catch {
    $taskEvidence.error=$_.Exception.Message
    $taskEvidence.buildStatus='failed-or-pending'
    Save-TabstripEvidence
    throw
} finally {
    $env:CARGO_TARGET_DIR=$taskPreviousTarget
}
