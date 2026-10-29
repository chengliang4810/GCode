[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$ReferenceDirectory,
    [Parameter(Mandatory)][string]$ImplementationDirectory
)
# Exercise the actual comparator with independent byte copies. Never edit an
# original capture/manifest, run a renderer, invoke parity, or delete evidence.
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$taskComparator = Join-Path $taskRoot 'scripts/compare-tabstrip-fixtures.ps1'
foreach ($taskVariable in @('ReferenceDirectory', 'ImplementationDirectory')) {
    $taskValue = Get-Variable -Name $taskVariable -ValueOnly
    if (-not [IO.Path]::IsPathRooted($taskValue)) { Set-Variable -Name $taskVariable -Value (Join-Path $taskRoot $taskValue) }
}
$taskStamp = [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss-fff')
$taskStage = Join-Path $taskRoot ('output/tabstrip-comparison-tests-' + $taskStamp)
$taskAncestor = $taskStage
while ($taskAncestor) {
    $taskItem = Get-Item -LiteralPath $taskAncestor -Force -ErrorAction SilentlyContinue
    if ($taskItem -and (($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -or -not $taskItem.PSIsContainer)) { throw 'Test output must not traverse an alias or file.' }
    $taskAncestor = [IO.Path]::GetDirectoryName($taskAncestor)
}
New-Item -ItemType Directory -Path $taskStage | Out-Null
$taskResults = [Collections.Generic.List[object]]::new()
$taskComparatorBefore = (Get-FileHash -LiteralPath $taskComparator -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::Copy($taskComparator, (Join-Path $taskStage 'comparator.before.ps1'), $false)

function Read-ComparisonTestJson([string]$Path) { Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json }
function Write-ComparisonTestJson([string]$Path, $Value) {
    $taskFull = [IO.Path]::GetFullPath($Path)
    if (-not $taskFull.StartsWith($taskStage + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test mutations must remain in the new owned output tree.' }
    [IO.File]::WriteAllText($taskFull, ($Value | ConvertTo-Json -Depth 30), [Text.UTF8Encoding]::new($false))
}
function Add-ComparisonTestProperty($Object, [string]$Name, $Value) { $Object | Add-Member -NotePropertyName $Name -NotePropertyValue $Value -Force }
function Edit-ComparisonMetadata([string]$Directory, [string]$Scene, [scriptblock]$Edit) {
    $taskPath = Join-Path $Directory ($Scene + '.png.capture.json')
    $taskJson = Read-ComparisonTestJson $taskPath
    & $Edit $taskJson
    Write-ComparisonTestJson $taskPath $taskJson
}
function Edit-ComparisonState([string]$Directory, [string]$Scene, [scriptblock]$Edit) {
    $taskPath = Join-Path $Directory 'evidence.json'
    $taskJson = Read-ComparisonTestJson $taskPath
    $taskEntry = @($taskJson.captures | Where-Object name -CEQ $Scene)
    if ($taskEntry.Count -ne 1) { throw 'Test fixture scene missing.' }
    & $Edit $taskEntry[0].state
    Write-ComparisonTestJson $taskPath $taskJson
}
function Copy-ComparisonInput([string]$From, [string]$To) {
    New-Item -ItemType Directory -Path $To | Out-Null
    foreach ($taskFile in Get-ChildItem -LiteralPath $From -File) { [IO.File]::Copy($taskFile.FullName, (Join-Path $To $taskFile.Name), $false) }
}
function Comparison-FileInventory([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) { return @() }
    @(Get-ChildItem -LiteralPath $Path -Recurse -File | ForEach-Object { $_.FullName + ':' + (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash })
}

# The product comparator creates output before building parity. Intercept its
# first possible parity-stage command; no Go build or parity process can run.
$taskPreviousGoFunction = Get-Item -LiteralPath Function:\go -ErrorAction SilentlyContinue
$taskPreviousGoScriptBlock = if ($taskPreviousGoFunction) { $taskPreviousGoFunction.ScriptBlock }
$taskPreviousGoMarker = Get-Variable -Name TabstripComparisonTestParityReached -Scope Global -ErrorAction SilentlyContinue
$taskPreviousGoMarkerValue = if ($taskPreviousGoMarker) { $taskPreviousGoMarker.Value }
try {
function local:go {
    $global:TabstripComparisonTestParityReached = $true
    if ($taskCase.testOriginalDrift) {
        $path = Join-Path $taskCase.reference ($taskScene+'.png')
        $snapshot = Join-Path $taskCase.output ('inputs/reference/'+$taskScene+'.png')
        $before = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash
        $bytes = [IO.File]::ReadAllBytes($path)
        $bytes[$bytes.Length-1] = $bytes[$bytes.Length-1] -bxor 1
        [IO.File]::WriteAllBytes($path,$bytes)
        $taskCase.snapshotIsolated = (Get-FileHash -LiteralPath $snapshot -Algorithm SHA256).Hash -ceq $before -and (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -cne $before
    }
    throw 'TABSTRIP_TEST_PARITY_STAGE_INTERCEPTED: no build/parity process was run.'
}

$taskScene = 'tabstrip-dark-w400-n6-scrollstart'
function Edit-SynchronizedState([string]$Directory, [scriptblock]$Edit) {
    $path = Join-Path $Directory ($taskScene+'.png.capture.json')
    $m = Read-ComparisonTestJson $path
    & $Edit $m.fixtureState
    Write-ComparisonTestJson $path $m
    $manifestPath = Join-Path $Directory 'evidence.json'
    $manifest = Read-ComparisonTestJson $manifestPath
    $entry = @($manifest.captures | Where-Object name -CEQ $taskScene)[0]
    $entry.state = $m.fixtureState
    $entry.captureMetadataSHA256 = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    Write-ComparisonTestJson $manifestPath $manifest
}
$taskCases = @(
    @{name='valid-preflight-trailing-separators';control=$true;mutate={param($c) $c.reference += [IO.Path]::DirectorySeparatorChar;$c.implementation += [IO.Path]::DirectorySeparatorChar}},
    @{name='actual-component-geometry-differences-remain-comparable';control=$true;mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.geometry.chips[0].bounds.width += 1; $s.maxX += 1}}},
    @{name='original-drift-after-snapshot-cannot-change-scored-file';control=$true;mutate={param($c) $c.testOriginalDrift=$true}},
    @{name='previous-framework-revision';mutate={param($c) $p=Join-Path $c.implementation 'evidence.json';$m=Read-ComparisonTestJson $p;$m.framework.Revision='zeron-native-ui.6';Write-ComparisonTestJson $p $m}},
    @{name='previous-framework-tree';mutate={param($c) $p=Join-Path $c.implementation 'evidence.json';$m=Read-ComparisonTestJson $p;$m.framework.SourceSHA256='cefb5a5c8694f538ac4caeecb080d2fa96adbaf15c6c12e234e1a4bd06cce914';Write-ComparisonTestJson $p $m}},
    @{name='reference-pointer-on-component';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.fixtureState.lastSyntheticPointerLogical=@(60,55)}}},
    @{name='implementation-pointer-unseen';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.pointer.moveSeen=$false}}},
    @{name='implementation-pointer-on-component';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.pointer.lastMoveX=60}}},
    @{name='implementation-control-hovered';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.pointer.anyControlHovered=$true}}},
    @{name='implementation-native-unattached';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.nativeWindowAttached=$false}}},
    @{name='fixture-transport-pid-mismatch';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.fixtureNative.pid += 1}}},
    @{name='fixture-transport-hwnd-mismatch';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.fixtureNative.hwnd += 1}}},
    @{name='fixture-transport-dpi-mismatch';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.fixtureNative.windowDpi += 1}}},
    @{name='implementation-after-foreground-mismatch';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.fixtureNativeAfter.foregroundHWND += 1}}},
    @{name='implementation-after-dpi-mismatch';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.fixtureNativeAfter.windowDpi += 1}}},
    @{name='implementation-after-manifest-mismatch';mutate={param($c) $p=Join-Path $c.implementation 'evidence.json';$m=Read-ComparisonTestJson $p;@($m.captures|Where-Object name -CEQ $taskScene)[0].nativeAfter.hwnd += 1;Write-ComparisonTestJson $p $m}},
    @{name='reference-window-inactive';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.fixtureState.nativeWindowActive=$false}}},
    @{name='reference-raw-foreground-wrong-before';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityBefore.foregroundHWND='0x1'}}},
    @{name='reference-raw-foreground-owner-wrong';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityBefore.foregroundProcessId += 1}}},
    @{name='reference-raw-foreground-inactive-after';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityAfter.nativeWindowActive=$false}}},
    @{name='reference-activity-boolean-number';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityBefore.nativeWindowActive=1}}},
    @{name='reference-activity-boolean-string';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityAfter.nativeWindowActive='True'}}},
    @{name='reference-activity-boolean-null';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityBefore.nativeWindowActive=$null}}},
    @{name='reference-activity-boolean-missing';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityBefore.PSObject.Properties.Remove('nativeWindowActive')}}},
    @{name='reference-owner-ui-thread-false';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityBefore.queriedOnOwnerUiThread=$false}}},
    @{name='reference-owner-ui-thread-string';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityBefore.queriedOnOwnerUiThread='True'}}},
    @{name='reference-owner-active-false';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityAfter.ownerThreadActiveIsOwn=$false}}},
    @{name='reference-owner-thread-id-mismatch';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityAfter.queryThreadId += 1}}},
    @{name='reference-owner-process-id-string';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityBefore.ownerProcessId=[string]$m.nativeActivityBefore.ownerProcessId}}},
    @{name='reference-owner-hwnd-numeric';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.nativeActivityBefore.ownerThreadActiveHWND=[Convert]::ToUInt64($m.hwnd.Substring(2),16)}}},
    @{name='implementation-after-activity-number';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.fixtureNativeAfter.nativeWindowActive=1}}},
    @{name='implementation-after-messages-string';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.fixtureNativeAfter.pointerMessagesSent='True'}}},
    @{name='wrong-dpi';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.dpi += 1}}},
    @{name='wrong-scale';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.scale = 1}}},
    @{name='frame-size-mismatch';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.pixels.frameSizePx[0] += 1}}},
    @{name='matching-unverified-frame';mutate={param($c) foreach($directory in @($c.reference,$c.implementation)){Edit-ComparisonMetadata $directory $taskScene {param($m) $m.windowRectScreenPx[2] += 1;$m.windowSizePx[0] += 1}}}},
    @{name='matching-unverified-client-inset';mutate={param($c) foreach($directory in @($c.reference,$c.implementation)){Edit-ComparisonMetadata $directory $taskScene {param($m) $m.clientOriginScreenPx[1] += 1}}}},
    @{name='relabel-2x-matrix-as-1x';mutate={param($c) foreach($directory in @($c.reference,$c.implementation)){foreach($file in Get-ChildItem -LiteralPath $directory -Filter '*.png.capture.json'){$m=Read-ComparisonTestJson $file.FullName;$m.scale=1;if($null -ne $m.dpi){$m.dpi=96};$m.clientSizePx=@(840,560);$m.clientRectPx=@(0,0,840,560);Write-ComparisonTestJson $file.FullName $m}}}},
    @{name='wrong-source';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.referenceCommit=('0'*40)}}},
    @{name='capture-pending';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.status='pending'}}},
    @{name='client-crop-instead-of-wgc';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.transport='client snapshot'}}},
    @{name='cropped-image';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.cropped=$true}}},
    @{name='transformed-image';mutate={param($c) Edit-ComparisonMetadata $c.implementation $taskScene {param($m) $m.imageTransformations=@('resize')}}},
    @{name='original-png-byte-corruption';mutate={param($c) $p=Join-Path $c.implementation ($taskScene+'.png');$b=[IO.File]::ReadAllBytes($p);$b[$b.Length-1]=$b[$b.Length-1] -bxor 1;[IO.File]::WriteAllBytes($p,$b)}},
    @{name='reference-hash-corruption';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.pixels.pngSHA256=('0'*64)}}},
    @{name='manifest-sidecar-hash-corruption';mutate={param($c) $p=Join-Path $c.implementation 'evidence.json';$m=Read-ComparisonTestJson $p;@($m.captures|Where-Object name -CEQ $taskScene)[0].captureMetadataSHA256=('0'*64);Write-ComparisonTestJson $p $m}},
    @{name='different-host';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.geometry.host.width -= 1}}},
    @{name='different-scene';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.scene.hostWidth=220}}},
    @{name='different-active';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.activeIndex=1}}},
    @{name='different-parent';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.parentSessionId='other'}}},
    @{name='different-tab-id';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.geometry.chips[0].sessionId='other'}}},
    @{name='different-title';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.geometry.chips[0].title='Wrong'}}},
    @{name='different-icon-kind';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.geometry.chips[0].icon='subagent'}}},
    @{name='busy-tab';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.geometry.chips[0].idle=$false}}},
    @{name='missing-measurement';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.geometry.chips[0].bounds=$null}}},
    @{name='unsettled-layout';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.geometryStable=$false}}},
    @{name='one-stable-frame';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.stableFrames=1}}},
    @{name='wrong-scroll-fraction';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.positiveX=3}}},
    @{name='unapplied-scroll';mutate={param($c) Edit-SynchronizedState $c.implementation {param($s) $s.scrollApplied=$false}}},
    @{name='reference-engine-attached';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.fixtureState.engineBootstrapped=$true}}},
    @{name='matrix-process-changed';mutate={param($c) Edit-ComparisonMetadata $c.reference $taskScene {param($m) $m.processId += 1}}},
    @{name='same-input-alias';mutate={param($c) $alias=Join-Path $c.root 'same-alias';New-Item -ItemType Junction -Path $alias -Target $c.reference|Out-Null;$c.implementation=$alias}},
    @{name='drive-root-input';mutate={param($c) $c.reference=[IO.Path]::GetPathRoot($taskRoot)}},
    @{name='alias-to-drive-root-input';mutate={param($c) $alias=Join-Path $c.root 'root-alias';New-Item -ItemType Junction -Path $alias -Target ([IO.Path]::GetPathRoot($taskRoot))|Out-Null;$c.reference=$alias}},
    @{name='checkout-parent-is-junction';mutate={param($c) $alias=Join-Path $c.root 'checkout-parent-alias';New-Item -ItemType Junction -Path $alias -Target ([IO.Path]::GetDirectoryName($taskRoot))|Out-Null;$aliasRoot=Join-Path $alias ([IO.Path]::GetFileName($taskRoot));$c.comparator=Join-Path $aliasRoot 'scripts/compare-tabstrip-fixtures.ps1';$c.output=Join-Path $aliasRoot ('output/tabstrip-comparison-ancestor-rejected-'+$taskStamp)}},
    @{name='output-ancestor-junction';mutate={param($c) $target=Join-Path $c.root 'redirect-destination';New-Item -ItemType Directory -Path $target|Out-Null;$alias=Join-Path $c.root 'redirect';New-Item -ItemType Junction -Path $alias -Target $target|Out-Null;$c.output=Join-Path $alias 'comparison'}},
    @{name='output-existing';mutate={param($c) New-Item -ItemType Directory -Path $c.output|Out-Null;[IO.File]::WriteAllText((Join-Path $c.output 'preserve.txt'),'existing evidence')}},
    @{name='output-existing-trailing-separator';mutate={param($c) New-Item -ItemType Directory -Path $c.output|Out-Null;[IO.File]::WriteAllText((Join-Path $c.output 'preserve.txt'),'existing evidence');$c.output += [IO.Path]::DirectorySeparatorChar}},
    @{name='output-inside-input';mutate={param($c) $c.output=Join-Path $c.reference 'comparison';$c.reference += [IO.Path]::DirectorySeparatorChar}},
    @{name='output-inside-physical-alias';mutate={param($c) $alias=Join-Path $c.root 'reference-alias';New-Item -ItemType Junction -Path $alias -Target $c.reference|Out-Null;$c.output=Join-Path $c.reference 'comparison';$c.reference=$alias}},
    @{name='wrong-output-prefix';mutate={param($c) $c.output=Join-Path $taskRoot ('output/tabstrip-wrong-prefix-'+$taskStamp)}}
)
foreach ($taskTest in $taskCases) {
    $taskCaseRoot = Join-Path $taskStage $taskTest.name
    New-Item -ItemType Directory -Path $taskCaseRoot | Out-Null
    $taskCase = @{root=$taskCaseRoot;reference=(Join-Path $taskCaseRoot 'reference');implementation=(Join-Path $taskCaseRoot 'implementation');output=(Join-Path $taskCaseRoot 'comparison')}
    $taskReferenceSource = if ($taskTest.referenceSource) { $taskTest.referenceSource } else { $ReferenceDirectory }
    $taskImplementationSource = if ($taskTest.implementationSource) { $taskTest.implementationSource } else { $ImplementationDirectory }
    Copy-ComparisonInput $taskReferenceSource $taskCase.reference
    Copy-ComparisonInput $taskImplementationSource $taskCase.implementation
    & $taskTest.mutate $taskCase
    $taskExistedBefore = Test-Path -LiteralPath $taskCase.output
    $taskInventoryBefore = @(Comparison-FileInventory $taskCase.output)
    $global:TabstripComparisonTestParityReached = $false
    $taskError = $null
    $taskCaseComparator = if ($taskCase.comparator) { $taskCase.comparator } else { $taskComparator }
    $taskCaseComparatorSHA = (Get-FileHash -LiteralPath $taskCaseComparator -Algorithm SHA256).Hash.ToLowerInvariant()
    try {
        & $taskCaseComparator -ReferenceDirectory $taskCase.reference -ImplementationDirectory $taskCase.implementation -OutputDirectory $taskCase.output *> (Join-Path $taskCaseRoot 'command.log')
    } catch { $taskError = $_.Exception.Message }
    $taskExistsAfter = Test-Path -LiteralPath $taskCase.output
    $taskInventoryAfter = @(Comparison-FileInventory $taskCase.output)
    $taskPassed = $false
    if ($taskTest.control) {
        $taskPassed = $global:TabstripComparisonTestParityReached -and $taskExistsAfter -and $taskError -like 'TABSTRIP_TEST_PARITY_STAGE_INTERCEPTED*'
        if ($taskCase.testOriginalDrift) { $taskPassed = $taskPassed -and $taskCase.snapshotIsolated }
    } else {
        $taskPassed = $null -ne $taskError -and -not $global:TabstripComparisonTestParityReached -and $taskExistsAfter -eq $taskExistedBefore -and (($taskInventoryBefore -join "`n") -ceq ($taskInventoryAfter -join "`n"))
    }
    $taskResult = [ordered]@{name=$taskTest.name;control=($taskTest.control -eq $true);passed=$taskPassed;originalDriftAfterSnapshot=($taskCase.testOriginalDrift -eq $true);snapshotIsolated=$taskCase.snapshotIsolated;preflightRejected=($null -ne $taskError -and -not $global:TabstripComparisonTestParityReached);comparisonOutputExistedBefore=$taskExistedBefore;comparisonOutputExistsAfter=$taskExistsAfter;parityStageReached=$global:TabstripComparisonTestParityReached;parityProcessExecuted=$false;message=$taskError;comparatorSHA256=$taskCaseComparatorSHA}
    $taskResults.Add($taskResult)
    Write-Output ('{0}: {1}' -f $taskTest.name, $(if ($taskPassed) {'PASS'} else {'FAIL'}))
}
$taskComparatorAfter = (Get-FileHash -LiteralPath $taskComparator -Algorithm SHA256).Hash.ToLowerInvariant()
$taskFailures = @($taskResults | Where-Object { -not $_.passed })
$taskEvidence = [ordered]@{schemaVersion=1;status=$(if ($taskFailures.Count -eq 0) {'all-guards-passed'} else {'guard-failures-found'});comparisonScript=$taskComparator;comparatorBeforeSHA256=$taskComparatorBefore;comparatorAfterSHA256=$taskComparatorAfter;comparatorChangedDuringSuite=($taskComparatorBefore -cne $taskComparatorAfter);testScriptSHA256=(Get-FileHash -LiteralPath $PSCommandPath -Algorithm SHA256).Hash.ToLowerInvariant();referenceSource=$ReferenceDirectory;implementationSource=$ImplementationDirectory;sourceInputsEdited=$false;nativeUIStarted=$false;parityProcessExecuted=$false;positiveControl='Valid whole-scene preflight with trailing input separators reaches intercepted go build; no parity process is run.';negativeRequirement='Every invalid input must throw before creating comparison output or reaching parity; existing evidence must keep its inventory.';caseCount=$taskResults.Count;failedCases=@($taskFailures | ForEach-Object name);cases=@($taskResults)}
[IO.File]::WriteAllText((Join-Path $taskStage 'result.json'), ($taskEvidence | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
Write-Output $taskStage
if ($taskFailures.Count -ne 0 -or $taskComparatorBefore -cne $taskComparatorAfter) { exit 1 }
} finally {
    if ($taskPreviousGoFunction) { Set-Item -LiteralPath Function:\go -Value $taskPreviousGoScriptBlock }
    else { Remove-Item -LiteralPath Function:\go -ErrorAction SilentlyContinue }
    if ($taskPreviousGoMarker) { Set-Variable -Name TabstripComparisonTestParityReached -Value $taskPreviousGoMarkerValue -Scope Global }
    else { Remove-Variable -Name TabstripComparisonTestParityReached -Scope Global -ErrorAction SilentlyContinue }
}
