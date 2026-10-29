[CmdletBinding()]
param(
    [string]$ReferenceDirectory = 'output/reference-lossless/capture-native-state-20261028-01',
    [string]$ImplementationDirectory = 'output/captures/model-picker-fixture-wgc-20261028-r6',
    [string]$Scale2ReferenceDirectory,
    [string]$Scale2ImplementationDirectory
)
# Only copies are mutated. Valid inputs stop at an intercepted go build; this
# suite does not build/run parity, start native UI, or modify original evidence.
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$taskComparator = Join-Path $taskRoot 'scripts/compare-compact-fixtures.ps1'
if ([bool]$Scale2ReferenceDirectory -ne [bool]$Scale2ImplementationDirectory) { throw 'Provide both actual 2x input directories, or neither.' }
foreach ($taskVariable in @('ReferenceDirectory', 'ImplementationDirectory', 'Scale2ReferenceDirectory', 'Scale2ImplementationDirectory')) {
    $taskValue = Get-Variable -Name $taskVariable -ValueOnly
    if ($taskValue -and -not [IO.Path]::IsPathRooted($taskValue)) { Set-Variable -Name $taskVariable -Value (Join-Path $taskRoot $taskValue) }
}
$taskStamp = [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss-fff')
$taskStage = Join-Path $taskRoot ('output/compact-comparison-tests-' + $taskStamp)
$taskResults = [Collections.Generic.List[object]]::new()
$taskComparatorBefore = (Get-FileHash -LiteralPath $taskComparator -Algorithm SHA256).Hash.ToLowerInvariant()
$taskAncestor = $taskStage
while ($taskAncestor) {
    $taskItem = Get-Item -LiteralPath $taskAncestor -Force -ErrorAction SilentlyContinue
    if ($taskItem -and (($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -or -not $taskItem.PSIsContainer)) { throw 'Test output must not traverse an alias or file.' }
    if ($taskAncestor.Equals($taskRoot, [StringComparison]::OrdinalIgnoreCase)) { break }
    $taskAncestor = [IO.Path]::GetDirectoryName($taskAncestor)
}
New-Item -ItemType Directory -Path $taskStage | Out-Null
[IO.File]::Copy($taskComparator, (Join-Path $taskStage 'comparator.before.ps1'), $false)

function Read-TestJson([string]$Path) { Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json }
function Write-TestJson([string]$Path, $Value) {
    $taskFull = [IO.Path]::GetFullPath($Path)
    if (-not $taskFull.StartsWith($taskStage + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test mutations must stay inside the new owned test directory.' }
    [IO.File]::WriteAllText($taskFull, ($Value | ConvertTo-Json -Depth 30), [Text.UTF8Encoding]::new($false))
}
function Edit-Metadata([string]$Directory, [string]$Scene, [scriptblock]$Edit) {
    $taskPath = Join-Path $Directory ($Scene + '.png.capture.json')
    $taskJson = Read-TestJson $taskPath
    & $Edit $taskJson
    Write-TestJson $taskPath $taskJson
}
function Edit-State([string]$Directory, [string]$Scene, [scriptblock]$Edit) {
    $taskPath = Join-Path $Directory 'evidence.json'
    $taskJson = Read-TestJson $taskPath
    $taskEntry = @($taskJson.captures | Where-Object name -CEQ $Scene)
    if ($taskEntry.Count -ne 1) { throw 'Missing test scene.' }
    & $Edit $taskEntry[0].state
    Write-TestJson $taskPath $taskJson
}
function Copy-Input([string]$From, [string]$To) {
    New-Item -ItemType Directory -Path $To | Out-Null
    foreach ($taskFile in Get-ChildItem -LiteralPath $From -File) { [IO.File]::Copy($taskFile.FullName, (Join-Path $To $taskFile.Name), $false) }
}
function File-Inventory([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) { return @() }
    @(Get-ChildItem -LiteralPath $Path -Recurse -File | ForEach-Object { $_.FullName + ':' + (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash })
}

$taskPreviousGoFunction = Get-Item -LiteralPath Function:\go -ErrorAction SilentlyContinue
$taskPreviousGoScriptBlock = if ($taskPreviousGoFunction) { $taskPreviousGoFunction.ScriptBlock }
$taskPreviousGoMarker = Get-Variable -Name CompactComparisonTestParityReached -Scope Global -ErrorAction SilentlyContinue
$taskPreviousGoMarkerValue = if ($taskPreviousGoMarker) { $taskPreviousGoMarker.Value }
try {
# A local stub shadows even a caller-local wrapper. A global stub would not;
# it could let that wrapper execute a real build from the child comparator.
function local:go {
    $global:CompactComparisonTestParityReached = $true
    throw 'COMPACT_TEST_PARITY_STAGE_INTERCEPTED: no build/parity process was run.'
}
$taskCases = @(
    @{name='actual-1x-preflight-trailing-input-separators';control=$true;mutate={param($c) $c.reference += [IO.Path]::DirectorySeparatorChar; $c.implementation += [IO.Path]::DirectorySeparatorChar}},
    @{name='mismatch-dpi';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.dpi += 1}}},
    @{name='mismatch-scale';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.scale = 2}}},
    @{name='unsupported-matched-scale';mutate={param($c) Edit-Metadata $c.reference 'compact-panel-dark-wide' {param($m) $m.scale = 1.25}; Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.scale = 1.25; $m.dpi = 120}}},
    @{name='mixed-scale-last-scene';mutate={param($c) Edit-Metadata $c.reference 'compact-model-list-light-narrow' {param($m) $m.scale = 2}; Edit-Metadata $c.implementation 'compact-model-list-light-narrow' {param($m) $m.scale = 2; $m.dpi = 192}}},
    @{name='relabel-whole-1x-matrix-as-2x';mutate={param($c) foreach($directory in @($c.reference,$c.implementation)){foreach($file in Get-ChildItem -LiteralPath $directory -Filter '*.png.capture.json'){ $m=Read-TestJson $file.FullName; $m.scale=2; if($null -ne $m.dpi){$m.dpi=192}; Write-TestJson $file.FullName $m }}}},
    @{name='relabel-2x-client-with-1x-original-png';mutate={param($c) foreach($directory in @($c.reference,$c.implementation)){Edit-Metadata $directory 'compact-panel-dark-wide' {param($m) $m.scale=2; if($null -ne $m.dpi){$m.dpi=192}; $m.clientSizePx=@(1680,1120);$m.clientRectPx=@(0,0,1680,1120);$m.windowSizePx=@(1706,1191);$m.windowRectScreenPx[2]=$m.windowRectScreenPx[0]+1706;$m.windowRectScreenPx[3]=$m.windowRectScreenPx[1]+1191}}}},
    @{name='mismatch-frame';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.pixels.frameSizePx[0] += 1}}},
    @{name='mismatch-window-rect-extent';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.windowRectScreenPx[2] += 1}}},
    @{name='mismatch-valid-native-window-extent';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.windowRectScreenPx[2] += 1; $m.windowSizePx[0] += 1}}},
    @{name='matched-unverified-native-window-profile';mutate={param($c) foreach($directory in @($c.reference,$c.implementation)){Edit-Metadata $directory 'compact-panel-dark-wide' {param($m) $m.windowRectScreenPx[2] += 1; $m.windowSizePx[0] += 1}}}},
    @{name='matched-unverified-client-inset-profile';mutate={param($c) foreach($directory in @($c.reference,$c.implementation)){Edit-Metadata $directory 'compact-panel-dark-wide' {param($m) $m.windowRectScreenPx[0] -= 1; $m.windowRectScreenPx[2] -= 1}}}},
    @{name='mismatch-client-size';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.clientSizePx[0] += 1}}},
    @{name='mismatch-client-rect';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.clientRectPx[0] = 1}}},
    @{name='mismatch-client-origin-inset';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.clientOriginScreenPx[1] += 1}}},
    @{name='client-origin-outside-window';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.clientOriginScreenPx[0] += 1000}}},
    @{name='missing-window-rect';mutate={param($c) Edit-Metadata $c.reference 'compact-panel-dark-wide' {param($m) $m.windowRectScreenPx = @()}}},
    @{name='wrong-reference-viewport';mutate={param($c) Edit-Metadata $c.reference 'compact-panel-dark-wide' {param($m) $m.gpuiViewportLogical[0] = 1680}}},
    @{name='wrong-reference-bounds-size';mutate={param($c) Edit-Metadata $c.reference 'compact-panel-dark-wide' {param($m) $m.gpuiBoundsLogical[2] += 1}}},
    @{name='wrong-reference-bounds-origin';mutate={param($c) Edit-Metadata $c.reference 'compact-panel-dark-wide' {param($m) $m.gpuiBoundsLogical[0] += 1}}},
    @{name='wrong-logical-dto';mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.logicalWidth = 1680}}},
    @{name='missing-layout-geometry';mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.geometry = $null}}},
    @{name='empty-popup-geometry';mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.geometry.popup.width = 0}}},
    @{name='wrong-scaffold-row';mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.geometry.row.y += 1}}},
    @{name='anchor-outside-row';mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.geometry.anchor.x = -1}}},
    @{name='popup-outside-client';mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.geometry.popup.y = -1}}},
    @{name='wrong-selected-model';mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.modelId = 'other'}}},
    @{name='wrong-reference-page';mutate={param($c) Edit-Metadata $c.reference 'compact-panel-dark-wide' {param($m) $m.fixtureState.modelList = $true}}},
    @{name='wrong-scene-name';mutate={param($c) Edit-Metadata $c.reference 'compact-panel-dark-wide' {param($m) $m.name = 'other.png'}}},
    @{name='pending-capture';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.status = 'pending'}}},
    @{name='wrong-transport';mutate={param($c) Edit-Metadata $c.reference 'compact-panel-dark-wide' {param($m) $m.transport = 'client screenshot'}}},
    @{name='wrong-capture-border';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.captureBorder = 'privacy override'}}},
    @{name='wrong-encoder';mutate={param($c) Edit-Metadata $c.reference 'compact-panel-dark-wide' {param($m) $m.pixels.encoder = 'JPEG transcoded to PNG'}}},
    @{name='cropped-flag';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.cropped = $true}}},
    @{name='transformed-image';mutate={param($c) Edit-Metadata $c.implementation 'compact-panel-dark-wide' {param($m) $m.imageTransformations = @('resize')}}},
    @{name='png-byte-corruption';mutate={param($c) $path=Join-Path $c.implementation 'compact-panel-dark-wide.png';$bytes=[IO.File]::ReadAllBytes($path);$bytes[$bytes.Length-1]=$bytes[$bytes.Length-1]-bxor 1;[IO.File]::WriteAllBytes($path,$bytes)}},
    @{name='wrong-original-png-hash';mutate={param($c) Edit-Metadata $c.reference 'compact-panel-dark-wide' {param($m) $m.pixels.pngSHA256 = ('0'*64)}}},
    @{name='duplicate-manifest-scene';mutate={param($c) $path=Join-Path $c.implementation 'evidence.json';$m=Read-TestJson $path;$m.captures[1].name=$m.captures[0].name;Write-TestJson $path $m}},
    @{name='manifest-path-traversal';mutate={param($c) $path=Join-Path $c.implementation 'evidence.json';$m=Read-TestJson $path;$m.captures[0].file='../outside.png';Write-TestJson $path $m}},
    @{name='independent-inputs-resolve-same-alias';mutate={param($c) $alias=Join-Path $c.root 'same-alias';New-Item -ItemType Junction -Path $alias -Target $c.reference|Out-Null;$c.implementation=$alias}},
    @{name='output-ancestor-junction';mutate={param($c) $target=Join-Path $c.root 'redirect-target';New-Item -ItemType Directory -Path $target|Out-Null;$alias=Join-Path $c.root 'redirect';New-Item -ItemType Junction -Path $alias -Target $target|Out-Null;$c.output=Join-Path $alias 'comparison'}},
    @{name='output-already-exists';mutate={param($c) New-Item -ItemType Directory -Path $c.output|Out-Null;[IO.File]::WriteAllText((Join-Path $c.output 'preserve.txt'),'owned existing evidence')}},
    @{name='output-already-exists-trailing-separator';mutate={param($c) New-Item -ItemType Directory -Path $c.output|Out-Null;[IO.File]::WriteAllText((Join-Path $c.output 'preserve.txt'),'owned existing evidence');$c.output += [IO.Path]::DirectorySeparatorChar}},
    @{name='output-inside-input-trailing-separator';mutate={param($c) $c.output=Join-Path $c.reference 'comparison';$c.reference += [IO.Path]::DirectorySeparatorChar}},
    @{name='output-inside-physical-input-alias';mutate={param($c) $alias=Join-Path $c.root 'ref-alias';New-Item -ItemType Junction -Path $alias -Target $c.reference|Out-Null;$c.output=Join-Path $c.reference 'comparison';$c.reference=$alias}},
    @{name='output-file-ancestor';mutate={param($c) $file=Join-Path $c.root 'file-ancestor';[IO.File]::WriteAllText($file,'owned file');$c.output=Join-Path $file 'comparison'}},
    @{name='output-outside-checkout';mutate={param($c) $c.output=[IO.Path]::Combine([IO.Path]::GetPathRoot($taskRoot),'compact-preflight-must-not-create-'+$taskStamp)}},
    @{name='output-readonly-reference-tree';mutate={param($c) $c.output=Join-Path $taskRoot ('output/reference-sources/compact-preflight-must-not-create-'+$taskStamp)}}
)
if ($Scale2ReferenceDirectory) {
    $taskCases += @(
        @{name='actual-2x-preflight';control=$true;referenceSource=$Scale2ReferenceDirectory;implementationSource=$Scale2ImplementationDirectory;mutate={param($c)}},
        @{name='actual-2x-missing-native-binding-evidence';referenceSource=$Scale2ReferenceDirectory;implementationSource=$Scale2ImplementationDirectory;mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.PSObject.Properties.Remove('nativeWindowAttached'); $s.PSObject.Properties.Remove('oneXOptionText')}}},
        @{name='actual-2x-unattached-native-window';referenceSource=$Scale2ReferenceDirectory;implementationSource=$Scale2ImplementationDirectory;mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.nativeWindowAttached=$false}}},
        @{name='actual-2x-wrong-one-x-text-adaptation';referenceSource=$Scale2ReferenceDirectory;implementationSource=$Scale2ImplementationDirectory;mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.oneXOptionText=$true}}},
        @{name='actual-2x-numeric-binding-values';referenceSource=$Scale2ReferenceDirectory;implementationSource=$Scale2ImplementationDirectory;mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.nativeWindowAttached=1; $s.oneXOptionText=0}}},
        @{name='actual-2x-string-binding-values';referenceSource=$Scale2ReferenceDirectory;implementationSource=$Scale2ImplementationDirectory;mutate={param($c) Edit-State $c.implementation 'compact-panel-dark-wide' {param($s) $s.nativeWindowAttached='True'; $s.oneXOptionText='False'}}}
    )
    $taskCases += @{name='actual-2x-relabel-whole-matrix-as-1x';referenceSource=$Scale2ReferenceDirectory;implementationSource=$Scale2ImplementationDirectory;mutate={param($c) foreach($directory in @($c.reference,$c.implementation)){foreach($file in Get-ChildItem -LiteralPath $directory -Filter '*.png.capture.json'){$m=Read-TestJson $file.FullName;$m.scale=1;if($null -ne $m.dpi){$m.dpi=96};$logicalWidth=if($m.name -like '*-wide.png'){840}else{360};$m.clientSizePx=@($logicalWidth,560);$m.clientRectPx=@(0,0,$logicalWidth,560);if($m.gpuiBoundsLogical){$m.gpuiBoundsLogical[0]=$m.clientOriginScreenPx[0];$m.gpuiBoundsLogical[1]=$m.clientOriginScreenPx[1]};Write-TestJson $file.FullName $m}};$p=Join-Path $c.implementation 'evidence.json';$e=Read-TestJson $p;foreach($capture in $e.captures){$capture.state.oneXOptionText=$true};Write-TestJson $p $e}}
}
foreach ($taskTest in $taskCases) {
    $taskCaseRoot = Join-Path $taskStage $taskTest.name
    New-Item -ItemType Directory -Path $taskCaseRoot | Out-Null
    $taskCase = @{root=$taskCaseRoot;reference=(Join-Path $taskCaseRoot 'reference');implementation=(Join-Path $taskCaseRoot 'implementation');output=(Join-Path $taskCaseRoot 'comparison')}
    $taskReferenceSource = if ($taskTest.referenceSource) { $taskTest.referenceSource } else { $ReferenceDirectory }
    $taskImplementationSource = if ($taskTest.implementationSource) { $taskTest.implementationSource } else { $ImplementationDirectory }
    Copy-Input $taskReferenceSource $taskCase.reference
    Copy-Input $taskImplementationSource $taskCase.implementation
    & $taskTest.mutate $taskCase
    $taskExistedBefore = Test-Path -LiteralPath $taskCase.output
    $taskInventoryBefore = @(File-Inventory $taskCase.output)
    $global:CompactComparisonTestParityReached = $false
    $taskError = $null
    try { & $taskComparator -ReferenceDirectory $taskCase.reference -ImplementationDirectory $taskCase.implementation -OutputDirectory $taskCase.output *> (Join-Path $taskCaseRoot 'command.log') }
    catch { $taskError = $_.Exception.Message }
    $taskExistsAfter = Test-Path -LiteralPath $taskCase.output
    $taskInventoryAfter = @(File-Inventory $taskCase.output)
    $taskPassed = if ($taskTest.control) { $global:CompactComparisonTestParityReached -and $taskExistsAfter -and $taskError -like 'COMPACT_TEST_PARITY_STAGE_INTERCEPTED*' }
        else { $null -ne $taskError -and -not $global:CompactComparisonTestParityReached -and $taskExistsAfter -eq $taskExistedBefore -and (($taskInventoryBefore -join "`n") -ceq ($taskInventoryAfter -join "`n")) }
    $taskResults.Add([ordered]@{name=$taskTest.name;control=($taskTest.control -eq $true);passed=$taskPassed;preflightRejected=($null -ne $taskError -and -not $global:CompactComparisonTestParityReached);comparisonOutputExistedBefore=$taskExistedBefore;comparisonOutputExistsAfter=$taskExistsAfter;parityStageReached=$global:CompactComparisonTestParityReached;parityProcessExecuted=$false;message=$taskError})
    Write-Output ('{0}: {1}' -f $taskTest.name, $(if($taskPassed){'PASS'}else{'FAIL'}))
}
$taskComparatorAfter = (Get-FileHash -LiteralPath $taskComparator -Algorithm SHA256).Hash.ToLowerInvariant()
$taskFailures = @($taskResults | Where-Object { -not $_.passed })
$taskEvidence = [ordered]@{schemaVersion=1;status=$(if($taskFailures.Count -eq 0){'all-guards-passed'}else{'guard-failures-found'});comparisonScript=$taskComparator;comparatorBeforeSHA256=$taskComparatorBefore;comparatorAfterSHA256=$taskComparatorAfter;comparatorChangedDuringSuite=($taskComparatorBefore -cne $taskComparatorAfter);testScriptSHA256=(Get-FileHash -LiteralPath $PSCommandPath -Algorithm SHA256).Hash.ToLowerInvariant();referenceSource=$ReferenceDirectory;implementationSource=$ImplementationDirectory;actualScale2ReferenceSource=$Scale2ReferenceDirectory;actualScale2ImplementationSource=$Scale2ImplementationDirectory;actualScale2ControlRun=[bool]$Scale2ReferenceDirectory;sourceInputsEdited=$false;nativeUIStarted=$false;parityProcessExecuted=$false;positiveControl='Actual whole eight-scene matrix reaches intercepted go build; no build or parity process is run.';negativeRequirement='Reject invalid input before creating comparison output or reaching parity; existing output inventory stays unchanged.';caseCount=$taskResults.Count;failedCases=@($taskFailures|ForEach-Object name);cases=@($taskResults)}
[IO.File]::WriteAllText((Join-Path $taskStage 'result.json'), ($taskEvidence|ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
Write-Output $taskStage
if ($taskFailures.Count -ne 0 -or $taskComparatorBefore -cne $taskComparatorAfter) { exit 1 }
} finally {
    if ($taskPreviousGoFunction) { Set-Item -LiteralPath Function:\go -Value $taskPreviousGoScriptBlock }
    else { Remove-Item -LiteralPath Function:\go -ErrorAction SilentlyContinue }
    if ($taskPreviousGoMarker) { Set-Variable -Name CompactComparisonTestParityReached -Value $taskPreviousGoMarkerValue -Scope Global }
    else { Remove-Variable -Name CompactComparisonTestParityReached -Scope Global -ErrorAction SilentlyContinue }
}
