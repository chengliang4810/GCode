[CmdletBinding()]
param([string]$OutputDirectory = 'output/native-wgc-bridge')
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (-not $IsWindows) { throw 'The fixture WGC bridge builds only on Windows.' }
if ([Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture -ne [Runtime.InteropServices.Architecture]::X64) { throw 'This fixture bridge build is currently verified only for Windows amd64.' }
function Resolve-BridgeWrite([string]$Path) {
    if (-not [IO.Path]::IsPathRooted($Path)) { $Path = Join-Path $taskRoot $Path }
    $taskPath = [IO.Path]::GetFullPath($Path)
    if (-not $taskPath.StartsWith($taskRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Bridge output must remain in this checkout.' }
    $taskFrozen = Join-Path $taskRoot 'output/reference-sources'
    if ($taskPath.Equals($taskFrozen, [StringComparison]::OrdinalIgnoreCase) -or $taskPath.StartsWith($taskFrozen + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Bridge output must not write into reference sources.' }
    $taskAncestor = $taskPath
    while ($taskAncestor -and ($taskAncestor.Equals($taskRoot, [StringComparison]::OrdinalIgnoreCase) -or $taskAncestor.StartsWith($taskRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase))) {
        $taskItem = $null
        try { $taskItem = Get-Item -LiteralPath $taskAncestor -Force -ErrorAction Stop }
        catch [System.Management.Automation.ItemNotFoundException] { }
        if ($taskItem) {
            if ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Bridge writes must not follow reparse paths.' }
            if ($taskAncestor -ne $taskPath -and -not $taskItem.PSIsContainer) { throw 'Bridge output ancestor is a file.' }
        }
        if ($taskAncestor.Equals($taskRoot, [StringComparison]::OrdinalIgnoreCase)) { break }
        $taskAncestor = Split-Path $taskAncestor -Parent
    }
    return $taskPath
}
$taskOutput = Resolve-BridgeWrite $OutputDirectory
$taskTarget = Resolve-BridgeWrite (Join-Path $taskOutput 'target')
$taskDLL = Resolve-BridgeWrite (Join-Path $taskOutput 'zeron_fixture_wgc.dll')
$taskLog = Resolve-BridgeWrite (Join-Path $taskOutput 'build.log')
$taskProvenance = Resolve-BridgeWrite (Join-Path $taskRoot 'reference/validation/native-wgc-bridge.json')
foreach ($taskSubdirectory in @('debug/build', 'debug/deps', 'debug/.fingerprint', 'debug/incremental', 'debug/zeron_fixture_wgc.dll')) { $null = Resolve-BridgeWrite (Join-Path $taskTarget $taskSubdirectory) }
$taskProject = Join-Path $taskRoot 'reference/fixtures/native-wgc-bridge'
$taskManifest = Join-Path $taskProject 'Cargo.toml'
$taskLock = Join-Path $taskProject 'Cargo.lock'
if (-not (Test-Path -LiteralPath $taskLock)) { throw 'The isolated bridge lock must be present; do not resolve dependencies during an evidence build.' }
function Get-BridgeHash([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
$taskSources = @('Cargo.toml', 'Cargo.lock', 'src/lib.rs', 'bridge.h') | ForEach-Object { @{ path = 'reference/fixtures/native-wgc-bridge/' + $_; sha256 = Get-BridgeHash (Join-Path $taskProject $_) } }
$taskEvidence = [ordered]@{
    schemaVersion = 1; bridgeABIVersion = 1; status = 'building'; platform = 'windows/amd64'
    scriptSHA256 = Get-BridgeHash $PSCommandPath; sources = $taskSources
    dependency = 'windows-capture =1.5.0; independent Cargo.lock; no production dependencies'
    buildCommand = 'cargo build --offline --locked -j 2 --manifest-path reference/fixtures/native-wgc-bridge/Cargo.toml'
    testCommand = 'cargo test --offline --locked -j 2 --manifest-path reference/fixtures/native-wgc-bridge/Cargo.toml'
    rustcVersion = (& rustc --version); cargoVersion = (& cargo --version)
    captureTarget = 'exact HWND owner must equal GetCurrentProcessId; no enumeration'
    cursor = 'WithoutCursor'; border = 'system Default'; transformations = @(); privacyOverrides = @()
    runtimeStatus = 'not-run by build script'; visualAcceptance = 'pending'; keyboardAcceptance = 'pending'
    lifetime = 'DLL retained until fixture process exit; explicit WGC stop and join'
    timeout = 'maximum 15 seconds for first frame only; upstream initialization and shutdown are not hard-bounded'
}
function Save-BridgeEvidence { [IO.File]::WriteAllText($taskProvenance, (($taskEvidence | ConvertTo-Json -Depth 8).Replace("`r`n", "`n") + "`n"), [Text.UTF8Encoding]::new($false)) }
New-Item -ItemType Directory -Path $taskOutput -Force | Out-Null
Save-BridgeEvidence
$taskPreviousTarget = $env:CARGO_TARGET_DIR
try {
    $env:CARGO_TARGET_DIR = $taskTarget
    & cargo test --offline --locked -j 2 --manifest-path $taskManifest 2>&1 | Tee-Object -FilePath $taskLog
    if ($LASTEXITCODE -ne 0) { throw 'Fixture WGC bridge contract tests failed.' }
    & cargo build --offline --locked -j 2 --manifest-path $taskManifest 2>&1 | Tee-Object -FilePath $taskLog -Append
    if ($LASTEXITCODE -ne 0) { throw 'Fixture WGC bridge build failed.' }
    $taskBuiltDLL = Join-Path $taskTarget 'debug/zeron_fixture_wgc.dll'
    [IO.File]::Copy($taskBuiltDLL, $taskDLL, $true)
    $taskEvidence.status = 'built-contract-tests-passed'
    $taskEvidence.dll = [IO.Path]::GetRelativePath($taskRoot, $taskDLL).Replace('\', '/')
    $taskEvidence.dllSHA256 = Get-BridgeHash $taskDLL
    $taskEvidence.buildLog = [IO.Path]::GetRelativePath($taskRoot, $taskLog).Replace('\', '/')
    $taskEvidence.buildLogSHA256 = Get-BridgeHash $taskLog
    Save-BridgeEvidence
    Write-Output $taskDLL
} catch {
    $taskEvidence.status = 'failed-or-pending'; $taskEvidence.error = $_.Exception.Message
    Save-BridgeEvidence
    throw
} finally {
    $env:CARGO_TARGET_DIR = $taskPreviousTarget
}
