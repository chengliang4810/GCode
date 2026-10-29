[CmdletBinding()]
param(
    [string]$SourceRoot = 'D:/zeron',
    [switch]$Run,
    [string]$OutputDirectory,
    [ValidateSet('opaque', 'theme')][string]$Surface = 'opaque',
    [switch]$IncludeKeyboardAssertions
)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (-not [IO.Path]::IsPathRooted($SourceRoot)) { $SourceRoot = Join-Path $taskRoot $SourceRoot }
$SourceRoot = [IO.Path]::GetFullPath($SourceRoot)
# Resolve aliases on the read-only source side as well as rejecting aliases on
# every controlled write path. SourceRoot itself remains read-only.
$taskFrozenPhysical = [IO.Path]::GetPathRoot($SourceRoot)
foreach ($taskPart in $SourceRoot.Substring($taskFrozenPhysical.Length).Split([IO.Path]::DirectorySeparatorChar, [StringSplitOptions]::RemoveEmptyEntries)) {
    $taskFrozenPhysical = Join-Path $taskFrozenPhysical $taskPart
    if (Test-Path -LiteralPath $taskFrozenPhysical) {
        $taskSourceItem = Get-Item -LiteralPath $taskFrozenPhysical -Force
        if ($taskSourceItem.Attributes -band [IO.FileAttributes]::ReparsePoint) {
            $taskSourceTarget = $taskSourceItem.ResolveLinkTarget($true)
            if (-not $taskSourceTarget) { throw 'Could not resolve the read-only SourceRoot alias.' }
            $taskFrozenPhysical = $taskSourceTarget.FullName
        }
    }
}
$taskFrozenPhysical = $taskFrozenPhysical.TrimEnd([IO.Path]::DirectorySeparatorChar)
function Assert-LosslessWritePath([string]$Path) {
    if (-not [IO.Path]::IsPathRooted($Path)) { $Path = Join-Path $taskRoot $Path }
    $taskResolved = [IO.Path]::GetFullPath($Path)
    if (-not $taskResolved.StartsWith($taskRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Lossless writes must remain inside this checkout.' }
    if ($taskResolved.Equals($taskFrozenPhysical, [StringComparison]::OrdinalIgnoreCase) -or $taskResolved.StartsWith($taskFrozenPhysical + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Lossless writes must not enter the read-only reference checkout.' }
    $taskAncestor = $taskResolved
    while ($taskAncestor -and ($taskAncestor.Equals($taskRoot, [StringComparison]::OrdinalIgnoreCase) -or $taskAncestor.StartsWith($taskRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase))) {
        if (Test-Path -LiteralPath $taskAncestor) {
            $taskItem = Get-Item -LiteralPath $taskAncestor -Force
            if ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Lossless writes must not redirect through a reparse path.' }
            if ($taskAncestor -ne $taskResolved -and -not $taskItem.PSIsContainer) { throw 'Lossless write path has a file as an ancestor.' }
        }
        if ($taskAncestor.Equals($taskRoot, [StringComparison]::OrdinalIgnoreCase)) { break }
        $taskAncestor = Split-Path $taskAncestor -Parent
    }
    return $taskResolved
}
function Resolve-LosslessOutput([string]$Path) {
    if (-not $Path) { $Path = Join-Path $taskRoot ('output/reference-lossless/capture-' + [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss-fff')) }
    $taskResolved = Assert-LosslessWritePath $Path
    $null = Assert-LosslessWritePath ($taskResolved + '.run.log')
    if ((Test-Path -LiteralPath $taskResolved) -or (Test-Path -LiteralPath ($taskResolved + '.run.log'))) { throw 'Run requires a new output directory and log path.' }
    return $taskResolved
}
if ($Run) { $OutputDirectory = Resolve-LosslessOutput $OutputDirectory }
& (Join-Path $PSScriptRoot 'reference-verify.ps1') -SourceRoot $SourceRoot
$taskReference = [IO.Path]::GetFullPath((Join-Path $taskRoot 'output/reference-native-0.2.128'))
if (-not $taskReference.StartsWith($taskRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Reference export escapes the workspace.' }
foreach ($taskDirectory in @((Join-Path $taskRoot 'output'), $taskReference)) {
    if ((Get-Item -LiteralPath $taskDirectory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Reference export must not redirect through a reparse point.' }
}
$taskCommit = 'ab189159f10cf8e63c0502be75a59bbc487e0eaf'
if ((Get-Content -LiteralPath (Join-Path $taskReference '.source-commit') -Raw).Trim() -ne $taskCommit) { throw 'Build the frozen reference export first.' }
$taskStage = Join-Path $taskRoot 'output/reference-lossless/transport-source'
$taskBefore = Join-Path $taskStage 'before'
$taskAfter = Join-Path $taskStage 'after'
$taskSnapshotFiles = @('Cargo.toml', 'Cargo.lock', 'crates/ui/Cargo.toml', 'crates/voice/Cargo.toml')
foreach ($taskWrite in @(
    $taskBefore, $taskAfter, (Join-Path $taskStage 'build.log'),
    (Join-Path $taskReference 'crates/ui/examples/compact-picker-windows-lossless.rs'),
    (Join-Path $taskRoot 'reference/fixtures/native-capture-lossless-dependency.patch'),
    (Join-Path $taskRoot 'reference/native/lossless-capture-adapter.json'),
    (Join-Path $taskRoot 'output/reference-native/target'),
    (Join-Path $taskRoot 'output/reference-native/target/debug/examples'),
    (Join-Path $taskRoot 'output/reference-native/target/debug/build'),
    (Join-Path $taskRoot 'output/reference-native/target/debug/deps'),
    (Join-Path $taskRoot 'output/reference-native/target/debug/.fingerprint'),
    (Join-Path $taskRoot 'output/reference-native/target/debug/incremental')
)) { $null = Assert-LosslessWritePath $taskWrite }
foreach ($taskFile in $taskSnapshotFiles) {
    foreach ($taskBase in @($taskBefore, $taskAfter, $taskReference)) { $null = Assert-LosslessWritePath (Join-Path $taskBase $taskFile) }
}
New-Item -ItemType Directory -Path $taskBefore, $taskAfter -Force | Out-Null
foreach ($taskFile in $taskSnapshotFiles) {
    $taskSnapshot = Join-Path $taskBefore $taskFile
    if (-not (Test-Path -LiteralPath $taskSnapshot)) {
        New-Item -ItemType Directory -Path (Split-Path $taskSnapshot) -Force | Out-Null
        [IO.File]::Copy((Join-Path $taskReference $taskFile), $taskSnapshot, $false)
    }
}
$taskOriginal = Join-Path $SourceRoot 'crates/ui/examples/compact-picker-fixture.rs'
$taskAdapter = Join-Path $taskReference 'crates/ui/examples/compact-picker-windows-lossless.rs'
$taskTemplate = Join-Path $taskRoot 'reference/fixtures/native-capture-lossless.rs'
$taskText = [IO.File]::ReadAllText($taskOriginal).Replace("`r`n", "`n")
$taskStart = $taskText.IndexOf('fn capture(')
$taskEnd = $taskText.IndexOf('fn press(', $taskStart)
if ($taskStart -lt 0 -or $taskEnd -le $taskStart) { throw 'Frozen capture function changed.' }
$taskText = $taskText.Substring(0, $taskStart) + [IO.File]::ReadAllText($taskTemplate) + $taskText.Substring($taskEnd)
$taskText = [regex]::Replace($taskText, '(?ms)^(\s+)capture\((.*?)\)\?;', '$1capture($2).await?;')
$taskMarker = 'let mut prefs = settings::UiSettings::default();'
if (-not $taskText.Contains($taskMarker)) { throw 'Frozen fixture settings changed.' }
$taskText = $taskText.Replace($taskMarker, $taskMarker + @'

            prefs.reduce_motion = motion::ReduceMotion::On;
            if std::env::var("ZERON_PARITY_SURFACE").as_deref() == Ok("opaque") {
                prefs.surface = zeron_theme::SurfacePreference::Opaque;
            }
'@)
$taskText = $taskText.Replace('composer::init(cx, prefs.composer_send_behavior);', 'motion::set_reduced_motion(cx, true); composer::init(cx, prefs.composer_send_behavior);')
$taskLoopEnd = @'

                    window.update(cx, |view, window, cx| {
                        appearance::set_mode(appearance::AppearanceMode::Dark, cx);
'@
if (-not $taskText.Contains($taskLoopEnd)) { throw 'Frozen scene/keyboard boundary changed.' }
$taskLayoutExit = @'

                    if std::env::var("ZERON_LOSSLESS_LAYOUT_ONLY").as_deref() != Ok("0") {
                        std::fs::write(output.join("result.json"), serde_json::to_vec_pretty(&serde_json::json!({
                            "status": "layout-captured", "referenceCommit": "ab189159f10cf8e63c0502be75a59bbc487e0eaf",
                            "expectedScenes": 8, "keyboardAcceptance": "not run; frozen full keyboard assertions skipped",
                            "sceneNavigation": "frozen matrix Enter/Escape and model-list state assertion retained",
                            "productionRendererModified": false, "productionComponentsModified": false,
                            "capture": "complete WGC frame saved directly as PNG; no resize or crop"
                        }))?)?;
                        std::fs::write(output.join("result.txt"), "CAPTURED: eight frozen panel/list layout scenes. Full keyboard assertions were not run; no keyboard or pixel-parity acceptance.\n")?;
                        return Ok(());
                    }
'@
$taskText = $taskText.Replace($taskLoopEnd, $taskLayoutExit + $taskLoopEnd)
$taskKeyboardResult = @'
                        "PASS: compact picker panel/list layout at dark/light and narrow/wide sizes; keyboard effort, fast tier, and favorite identity behavior.\n",
                    )?;
'@
if (-not $taskText.Contains($taskKeyboardResult)) { throw 'Frozen keyboard result boundary changed.' }
$taskText = $taskText.Replace($taskKeyboardResult, $taskKeyboardResult + @'

                    std::fs::write(output.join("result.json"), serde_json::to_vec_pretty(&serde_json::json!({
                        "status": "frozen-assertions-completed", "expectedScenes": 14,
                        "keyboardAcceptance": "only the unchanged frozen fixture assertions completed; independent acceptance pending",
                        "productionRendererModified": false, "productionComponentsModified": false,
                        "capture": "complete WGC frame saved directly as PNG; no resize or crop"
                    }))?)?;
'@)
$taskText = $taskText.Replace('std::fs::create_dir_all(&output)?;', @'
    std::fs::create_dir_all(&output)?;
    anyhow::ensure!(std::fs::read_dir(&output)?.next().is_none(), "Lossless captures require a new empty output directory");
'@)
$taskText = $taskText.Replace('eprintln!("compact picker fixture failed: {error:#}");', @'
                    eprintln!("compact picker fixture failed: {error:#}");
                    let _ = std::fs::write(output.join("failure.txt"), format!("PENDING/FAILED: {error:#}\nNo full keyboard or pixel-parity acceptance.\n"));
'@)
[IO.File]::WriteAllText($taskAdapter, $taskText, [Text.UTF8Encoding]::new($false))
# Only a Windows development dependency is added. No package version changes.
$taskManifestPath = Join-Path $taskReference 'crates/ui/Cargo.toml'
$taskBaseManifest = [IO.File]::ReadAllText((Join-Path $taskBefore 'crates/ui/Cargo.toml'))
$taskManifestEol = if ($taskBaseManifest.Contains("`r`n")) { "`r`n" } else { "`n" }
$taskDependency = $taskManifestEol + "[target.'cfg(windows)'.dev-dependencies]" + $taskManifestEol + 'windows-capture = "=1.5.0"' + $taskManifestEol
$taskManifest = $taskBaseManifest.TrimEnd() + $taskManifestEol + $taskDependency
$taskExistingManifest = [IO.File]::ReadAllText($taskManifestPath)
if ($taskExistingManifest.Replace("`r`n", "`n") -ne $taskBaseManifest.Replace("`r`n", "`n") -and $taskExistingManifest.Replace("`r`n", "`n") -ne $taskManifest.Replace("`r`n", "`n")) { throw 'Exported UI manifest has unrecognized changes; preserving it.' }
[IO.File]::WriteAllText($taskManifestPath, $taskManifest, [Text.UTF8Encoding]::new($false))
$taskLockPath = Join-Path $taskReference 'Cargo.lock'
$taskBaseLockRaw = [IO.File]::ReadAllText((Join-Path $taskBefore 'Cargo.lock'))
$taskBaseLock = $taskBaseLockRaw.Replace("`r`n", "`n")
if (-not $taskBaseLock.Contains("name = `"windows-capture`"`nversion = `"1.5.0`"")) { throw 'The frozen lock does not already contain windows-capture 1.5.0.' }
$taskPackageStart = $taskBaseLock.IndexOf("[[package]]`nname = `"zeron-ui`"")
$taskPackageEnd = $taskBaseLock.IndexOf('[[package]]', $taskPackageStart + 12)
if ($taskPackageStart -lt 0 -or $taskPackageEnd -le $taskPackageStart) { throw 'Frozen zeron-ui lock entry changed.' }
$taskPackage = $taskBaseLock.Substring($taskPackageStart, $taskPackageEnd - $taskPackageStart)
$taskLockMarker = ' "windows-core 0.61.2",'
if (-not $taskPackage.Contains($taskLockMarker)) { throw 'Frozen Windows dependency order changed.' }
$taskUpdatedPackage = $taskPackage.Replace($taskLockMarker, " `"windows-capture`",`n" + $taskLockMarker)
$taskLock = $taskBaseLock.Substring(0, $taskPackageStart) + $taskUpdatedPackage + $taskBaseLock.Substring($taskPackageEnd)
$taskExistingLock = [IO.File]::ReadAllText($taskLockPath).Replace("`r`n", "`n")
if ($taskExistingLock -ne $taskBaseLock -and $taskExistingLock -ne $taskLock) { throw 'Exported lock has unrecognized changes; preserving it.' }
# Preserve every original byte outside the zeron-ui dependency insertion,
# including a fresh git archive's CRLF or mixed-ending dynamic-voice stage.
$taskRawPackage = [regex]::Match($taskBaseLockRaw, '(?ms)^\[\[package\]\]\r?\nname = "zeron-ui"\r?\n.*?(?=^\[\[package\]\])')
if (-not $taskRawPackage.Success -or $taskRawPackage.Value.Replace("`r`n", "`n") -ne $taskPackage) { throw 'Could not preserve the original lock byte layout.' }
$taskPackageEol = if ($taskRawPackage.Value.Contains("`r`n")) { "`r`n" } else { "`n" }
$taskUpdatedRawPackage = $taskRawPackage.Value.Replace($taskLockMarker, ' "windows-capture",' + $taskPackageEol + $taskLockMarker)
$taskLockRaw = $taskBaseLockRaw.Substring(0, $taskRawPackage.Index) + $taskUpdatedRawPackage + $taskBaseLockRaw.Substring($taskRawPackage.Index + $taskRawPackage.Length)
if ($taskLockRaw.Replace("`r`n", "`n") -ne $taskLock) { throw 'Lossless dependency insertion changed more than its declared lock entry.' }
[IO.File]::WriteAllText($taskLockPath, $taskLockRaw, [Text.UTF8Encoding]::new($false))
foreach ($taskFile in $taskSnapshotFiles) {
    $taskSnapshot = Join-Path $taskAfter $taskFile
    New-Item -ItemType Directory -Path (Split-Path $taskSnapshot) -Force | Out-Null
    [IO.File]::Copy((Join-Path $taskReference $taskFile), $taskSnapshot, $true)
}
$taskDependencyPatch = Join-Path $taskRoot 'reference/fixtures/native-capture-lossless-dependency.patch'
& git -C $taskRoot diff --no-index --binary --output=$taskDependencyPatch -- output/reference-lossless/transport-source/before output/reference-lossless/transport-source/after
if ($LASTEXITCODE -gt 1) { throw 'Could not record the isolated manifest/lock delta.' }
$taskPatchText = [IO.File]::ReadAllText($taskDependencyPatch).Replace('a/output/reference-lossless/transport-source/before/', 'a/').Replace('b/output/reference-lossless/transport-source/after/', 'b/')
[IO.File]::WriteAllText($taskDependencyPatch, $taskPatchText, [Text.UTF8Encoding]::new($false))
function Get-LosslessHash([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
$taskChanges = foreach ($taskFile in $taskSnapshotFiles) {
    @{ path = $taskFile; beforeSHA256 = (Get-LosslessHash (Join-Path $taskBefore $taskFile)); afterSHA256 = (Get-LosslessHash (Join-Path $taskAfter $taskFile)) }
}
$taskEvidence = [ordered]@{
    schemaVersion = 1; referenceCommit = $taskCommit; sourceRoot = [IO.Path]::GetFullPath($SourceRoot)
    originalExample = 'crates/ui/examples/compact-picker-fixture.rs'; originalSHA256 = (Get-LosslessHash $taskOriginal)
    template = 'reference/fixtures/native-capture-lossless.rs'; templateSHA256 = (Get-LosslessHash $taskTemplate)
    adapter = 'output/reference-native-0.2.128/crates/ui/examples/compact-picker-windows-lossless.rs'; adapterSHA256 = (Get-LosslessHash $taskAdapter)
    dependency = 'existing locked windows-capture =1.5.0; Windows dev-dependency only'; sourceSnapshots = 'output/reference-lossless/transport-source'; changes = @($taskChanges)
    dependencyPatch = 'reference/fixtures/native-capture-lossless-dependency.patch'; dependencyPatchSHA256 = (Get-LosslessHash $taskDependencyPatch)
    sourceSnapshotScope = 'before is the prior dynamic-voice/JPEG build stage; after adds only this fixture dev-dependency'
    inheritedDynamicVoiceProvenance = 'reference/validation/reference-dynamic-voice-runtime.json; its prior-stage lock hash is preserved'
    productionRendererModified = $false; productionComponentsModified = $false; frozenSourceCheckoutModified = $false
    existingJPEGAdapterModified = $false; captureTarget = 'exact fixture HWND and current process ID'; format = 'original complete BGRA8 WGC frame to PNG'
    transformations = @(); privacyOverrides = @(); captureBorder = 'system Default'; surface = $Surface; reducedMotion = $true
    keyboardAcceptance = 'not established; layout-only by default; IncludeKeyboardAssertions retains frozen assertions without corrections'
    captureBoundary = 'complete HWND WGC frame including native/nonclient extent; not directly equivalent to client-only CapturePage output'
    buildCommand = 'cargo build --offline --locked -j 2 --manifest-path output/reference-native-0.2.128/Cargo.toml -p zeron-ui --example compact-picker-windows-lossless --features compact-picker-fixture'
    buildStatus = 'pending'; runtimeStatus = 'not-run'
    rustcVersion = (& rustc --version); cargoVersion = (& cargo --version)
}
$taskEvidencePath = Join-Path $taskRoot 'reference/native/lossless-capture-adapter.json'
function Save-LosslessEvidence { [IO.File]::WriteAllText($taskEvidencePath, (($taskEvidence | ConvertTo-Json -Depth 8).Replace("`r`n", "`n") + "`n"), [Text.UTF8Encoding]::new($false)) }
Save-LosslessEvidence
$taskPreviousTarget = $env:CARGO_TARGET_DIR
$taskPreviousSurface = $env:ZERON_PARITY_SURFACE
$taskPreviousLayout = $env:ZERON_LOSSLESS_LAYOUT_ONLY
try {
    $env:CARGO_TARGET_DIR = Join-Path $taskRoot 'output/reference-native/target'
    & cargo build --offline --locked -j 2 --manifest-path (Join-Path $taskReference 'Cargo.toml') -p zeron-ui --example compact-picker-windows-lossless --features compact-picker-fixture 2>&1 | Tee-Object -FilePath (Join-Path $taskStage 'build.log')
    if ($LASTEXITCODE -ne 0) { throw 'Lossless fixture build failed.' }
    $taskBinary = Join-Path $env:CARGO_TARGET_DIR 'debug/examples/compact-picker-windows-lossless.exe'
    $taskEvidence.buildStatus = 'built'; $taskEvidence.binarySHA256 = Get-LosslessHash $taskBinary
    Save-LosslessEvidence
    if ($Run) {
        # Check again after the build in case the filesystem changed meanwhile.
        $OutputDirectory = Resolve-LosslessOutput $OutputDirectory
        $env:ZERON_PARITY_SURFACE = $Surface
        $env:ZERON_LOSSLESS_LAYOUT_ONLY = if ($IncludeKeyboardAssertions) { '0' } else { '1' }
        $taskEvidence.runtimeLog = $OutputDirectory + '.run.log'
        & $taskBinary $OutputDirectory 2>&1 | Tee-Object -FilePath $taskEvidence.runtimeLog
        $taskProcessExit = $LASTEXITCODE
        $taskEvidence.runtimeOutputDirectory = $OutputDirectory; $taskEvidence.runtimeExitCode = $taskProcessExit
        $taskResult = Join-Path $OutputDirectory 'result.json'
        if ($taskProcessExit -ne 0 -or -not (Test-Path -LiteralPath $taskResult)) {
            $taskEvidence.runtimeStatus = 'pending-or-failed'; Save-LosslessEvidence
            throw 'Lossless capture did not complete its declared layout-only matrix. Inspect the new output; do not change privacy settings.'
        }
        $taskCaptures = @(Get-ChildItem -LiteralPath $OutputDirectory -Filter '*.capture.json' | ForEach-Object { Get-Content -LiteralPath $_.FullName -Raw | ConvertFrom-Json })
        $taskExpectedScenes = if ($IncludeKeyboardAssertions) { 14 } else { 8 }
        if ($taskCaptures.Count -ne $taskExpectedScenes -or @($taskCaptures | Where-Object status -ne 'captured').Count -ne 0) { throw 'Lossless matrix did not produce its declared number of original PNG captures.' }
        foreach ($taskCapture in $taskCaptures) {
            if ((Get-LosslessHash (Join-Path $OutputDirectory $taskCapture.name)) -ne $taskCapture.pixels.pngSHA256) { throw 'Captured PNG hash changed.' }
        }
        $taskEvidence.runtimeStatus = if ($IncludeKeyboardAssertions) { 'frozen-assertions-completed; independent keyboard and visual parity pending' } else { 'eight-layout-scenes-captured; keyboard and visual parity pending' }; Save-LosslessEvidence
        Write-Output $OutputDirectory
    }
    Write-Output $taskBinary
} catch {
    if ($taskEvidence.buildStatus -eq 'pending') { $taskEvidence.buildStatus = 'failed-or-pending' }
    if ($Run) { $taskEvidence.runtimeStatus = 'pending-or-failed' }
    $taskEvidence.error = $_.Exception.Message
    Save-LosslessEvidence
    throw
} finally {
    $env:CARGO_TARGET_DIR = $taskPreviousTarget
    $env:ZERON_PARITY_SURFACE = $taskPreviousSurface
    $env:ZERON_LOSSLESS_LAYOUT_ONLY = $taskPreviousLayout
}
