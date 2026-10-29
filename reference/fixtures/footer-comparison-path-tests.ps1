[CmdletBinding()]
param(
    [string]$ReferenceDirectory = 'output/footer-reference-capture-20261028-r3-2x',
    [string]$ImplementationDirectory = 'output/footer-implementation-r3'
)
# Exercise the actual comparator with independent byte copies. Never edit an
# original capture/manifest, run a renderer, invoke parity, or delete evidence.
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$taskComparator = Join-Path $taskRoot 'scripts/compare-footer-fixtures.ps1'
foreach ($taskVariable in @('ReferenceDirectory', 'ImplementationDirectory')) {
    $taskValue = Get-Variable -Name $taskVariable -ValueOnly
    if (-not [IO.Path]::IsPathRooted($taskValue)) { Set-Variable -Name $taskVariable -Value (Join-Path $taskRoot $taskValue) }
}
$taskStamp = [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss-fff')
$taskStage = Join-Path $taskRoot ('output/footer-comparison-tests-' + $taskStamp)
$taskAncestor = $taskStage
while ($taskAncestor) {
    $taskItem = Get-Item -LiteralPath $taskAncestor -Force -ErrorAction SilentlyContinue
    if ($taskItem -and (($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -or -not $taskItem.PSIsContainer)) { throw 'Test output must not traverse an alias or file.' }
    if ($taskAncestor.Equals($taskRoot, [StringComparison]::OrdinalIgnoreCase)) { break }
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
$taskPreviousGoMarker = Get-Variable -Name FooterComparisonTestParityReached -Scope Global -ErrorAction SilentlyContinue
$taskPreviousGoMarkerValue = if ($taskPreviousGoMarker) { $taskPreviousGoMarker.Value }
try {
function local:go {
    $global:FooterComparisonTestParityReached = $true
    throw 'FOOTER_TEST_PARITY_STAGE_INTERCEPTED: no build/parity process was run.'
}

$taskCases = @(
    @{name='valid-preflight-with-trailing-input-slashes'; control=$true; mutate={param($case) $case.reference += [IO.Path]::DirectorySeparatorChar; $case.implementation += [IO.Path]::DirectorySeparatorChar}},
    @{name='valid-1x-preflight'; control=$true; referenceSource=(Join-Path $taskRoot 'output/footer-reference-capture-20261028-r2');implementationSource=(Join-Path $taskRoot 'output/footer-implementation-r2');mutate={param($case)}},
    @{name='mismatch-dpi'; mutate={param($case) Edit-ComparisonMetadata $case.implementation 'footer-known-dark' {param($m) $m.dpi += 1}}},
    @{name='mismatch-scale'; mutate={param($case) Edit-ComparisonMetadata $case.implementation 'footer-known-dark' {param($m) $m.scale = $(if ($m.scale -eq 1) {2} else {1})}}},
    @{name='mismatch-frame'; mutate={param($case) Edit-ComparisonMetadata $case.implementation 'footer-known-dark' {param($m) $m.pixels.frameSizePx[0] += 1}}},
    @{name='mismatch-window-rect-extent'; mutate={param($case) Edit-ComparisonMetadata $case.implementation 'footer-known-dark' {param($m) $m.windowRectScreenPx[2] += 1}}},
    @{name='matched-unverified-window-profile'; mutate={param($case) foreach($directory in @($case.reference,$case.implementation)){Edit-ComparisonMetadata $directory 'footer-known-dark' {param($m) $m.windowRectScreenPx[2] += 1; $m.windowSizePx[0] += 1}}}},
    @{name='matched-unverified-client-inset-profile'; mutate={param($case) foreach($directory in @($case.reference,$case.implementation)){Edit-ComparisonMetadata $directory 'footer-known-dark' {param($m) $m.windowRectScreenPx[0] -= 1; $m.windowRectScreenPx[2] -= 1}}}},
    @{name='relabel-actual-2x-matrix-as-1x'; referenceSource=(Join-Path $taskRoot 'output/footer-reference-capture-20261028-r3-2x');implementationSource=(Join-Path $taskRoot 'output/footer-implementation-r3');mutate={param($case) foreach($directory in @($case.reference,$case.implementation)){foreach($file in Get-ChildItem -LiteralPath $directory -Filter '*.png.capture.json'){$m=Read-ComparisonTestJson $file.FullName;$m.scale=1;if($null -ne $m.dpi){$m.dpi=96};$m.clientSizePx=@(840,560);$m.clientRectPx=@(0,0,840,560);Write-ComparisonTestJson $file.FullName $m}}}},
    @{name='mismatch-client-size'; mutate={param($case) Edit-ComparisonMetadata $case.implementation 'footer-known-dark' {param($m) $m.clientSizePx[0] += 1}}},
    @{name='mismatch-client-origin'; mutate={param($case) Edit-ComparisonMetadata $case.implementation 'footer-known-dark' {param($m) $m.clientOriginScreenPx[1] += 1}}},
    @{name='mismatch-tokens'; mutate={param($case) Edit-ComparisonState $case.implementation 'footer-known-dark' {param($s) $s.contextUsage.tokens = 42001}}},
    @{name='mismatch-window'; mutate={param($case) Edit-ComparisonState $case.implementation 'footer-known-dark' {param($s) $s.contextUsage.window = 199999}}},
    @{name='unknown-became-real-zero'; mutate={param($case) Edit-ComparisonState $case.implementation 'footer-unknown-dark' {param($s) Add-ComparisonTestProperty $s.contextUsage 'tokens' 0}}},
    @{name='mismatch-implementation-account-group'; mutate={param($case) Edit-ComparisonState $case.implementation 'footer-known-dark' {param($s) Add-ComparisonTestProperty $s.accounts[0] 'loginProvider' 'other'}}},
    @{name='mismatch-reference-account-group'; mutate={param($case) Edit-ComparisonMetadata $case.reference 'footer-known-dark' {param($m) Add-ComparisonTestProperty $m.fixtureState.accounts[0] 'provider' 'other'}}},
    @{name='mismatch-account-target'; mutate={param($case) Edit-ComparisonState $case.implementation 'footer-known-dark' {param($s) $s.accounts[0].targetId = 'other'}}},
    @{name='mismatch-account-usage-fraction'; mutate={param($case) Edit-ComparisonState $case.implementation 'footer-known-dark' {param($s) $s.accounts[0].usage[0].usedPercent = 21}}},
    @{name='mismatch-reported-account-fraction'; mutate={param($case) Edit-ComparisonMetadata $case.reference 'footer-known-dark' {param($m) $m.fixtureState.accountFraction = .8}}},
    @{name='mismatch-reported-context-fraction'; mutate={param($case) Edit-ComparisonMetadata $case.reference 'footer-known-dark' {param($m) $m.fixtureState.contextFraction = .99}}},
    @{name='mismatch-inactive-usage-error'; mutate={param($case) Edit-ComparisonMetadata $case.reference 'footer-known-accounts-dark' {param($m) Add-ComparisonTestProperty $m.fixtureState.accounts[1] 'usageError' 'Signed out'}}},
    @{name='pending-capture-status'; mutate={param($case) Edit-ComparisonMetadata $case.implementation 'footer-known-dark' {param($m) $m.status = 'pending'}}},
    @{name='wrong-capture-transport'; mutate={param($case) Edit-ComparisonMetadata $case.reference 'footer-known-dark' {param($m) $m.transport = 'client screenshot'}}},
    @{name='cropped-flag'; mutate={param($case) Edit-ComparisonMetadata $case.implementation 'footer-known-dark' {param($m) $m.cropped = $true}}},
    @{name='image-transformation'; mutate={param($case) Edit-ComparisonMetadata $case.implementation 'footer-known-dark' {param($m) $m.imageTransformations = @('resize')}}},
    @{name='original-png-byte-corruption'; mutate={param($case) $path = Join-Path $case.implementation 'footer-known-dark.png'; $bytes = [IO.File]::ReadAllBytes($path); $bytes[$bytes.Length - 1] = $bytes[$bytes.Length - 1] -bxor 1; [IO.File]::WriteAllBytes($path,$bytes)}},
    @{name='reference-png-hash-mismatch'; mutate={param($case) Edit-ComparisonMetadata $case.reference 'footer-known-dark' {param($m) $m.pixels.pngSHA256 = ('0' * 64)}}},
    @{name='private-popup-adapter'; mutate={param($case) Edit-ComparisonMetadata $case.reference 'footer-known-context-dark' {param($m) $m.fixtureState.interaction.privatePopupFieldsReadOrChanged = $true; $m.fixtureState.interaction.path = 'adapter wrote popup'}}},
    @{name='baseline-retained-interaction'; mutate={param($case) Edit-ComparisonMetadata $case.reference 'footer-known-dark' {param($m) $m.fixtureState.interaction = [pscustomobject]@{requestedCard='context';privatePopupFieldsReadOrChanged=$false}}}},
    @{name='independent-inputs-resolve-same-alias'; mutate={param($case) $alias = Join-Path $case.root 'same-input-alias'; New-Item -ItemType Junction -Path $alias -Target $case.reference | Out-Null; $case.implementation = $alias}},
    @{name='output-ancestor-junction'; mutate={param($case) $destination = Join-Path $case.root 'redirect-destination'; New-Item -ItemType Directory -Path $destination | Out-Null; $alias = Join-Path $case.root 'redirect'; New-Item -ItemType Junction -Path $alias -Target $destination | Out-Null; $case.output = Join-Path $alias 'comparison'}},
    @{name='output-already-exists'; mutate={param($case) New-Item -ItemType Directory -Path $case.output | Out-Null; [IO.File]::WriteAllText((Join-Path $case.output 'preserve.txt'),'owned existing evidence')}},
    @{name='output-already-exists-trailing-slash'; mutate={param($case) New-Item -ItemType Directory -Path $case.output | Out-Null; [IO.File]::WriteAllText((Join-Path $case.output 'preserve.txt'),'owned existing evidence'); $case.output += [IO.Path]::DirectorySeparatorChar}},
    @{name='output-inside-input-trailing-slash'; mutate={param($case) $case.output = Join-Path $case.reference 'new-comparison'; $case.reference += [IO.Path]::DirectorySeparatorChar}},
    @{name='output-inside-physical-input-alias'; mutate={param($case) $alias = Join-Path $case.root 'reference-alias'; New-Item -ItemType Junction -Path $alias -Target $case.reference | Out-Null; $case.output = Join-Path $case.reference 'new-comparison'; $case.reference = $alias}},
    @{name='output-wrong-prefix'; mutate={param($case) $case.output = Join-Path $taskRoot ('output/footer-wrong-prefix-' + $taskStamp)}}
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
    $global:FooterComparisonTestParityReached = $false
    $taskError = $null
    $taskCaseComparatorSHA = (Get-FileHash -LiteralPath $taskComparator -Algorithm SHA256).Hash.ToLowerInvariant()
    try {
        & $taskComparator -ReferenceDirectory $taskCase.reference -ImplementationDirectory $taskCase.implementation -OutputDirectory $taskCase.output *> (Join-Path $taskCaseRoot 'command.log')
    } catch { $taskError = $_.Exception.Message }
    $taskExistsAfter = Test-Path -LiteralPath $taskCase.output
    $taskInventoryAfter = @(Comparison-FileInventory $taskCase.output)
    $taskPassed = $false
    if ($taskTest.control) {
        $taskPassed = $global:FooterComparisonTestParityReached -and $taskExistsAfter -and $taskError -like 'FOOTER_TEST_PARITY_STAGE_INTERCEPTED*'
    } else {
        $taskPassed = $null -ne $taskError -and -not $global:FooterComparisonTestParityReached -and $taskExistsAfter -eq $taskExistedBefore -and (($taskInventoryBefore -join "`n") -ceq ($taskInventoryAfter -join "`n"))
    }
    $taskResult = [ordered]@{name=$taskTest.name;control=($taskTest.control -eq $true);passed=$taskPassed;preflightRejected=($null -ne $taskError -and -not $global:FooterComparisonTestParityReached);comparisonOutputExistedBefore=$taskExistedBefore;comparisonOutputExistsAfter=$taskExistsAfter;parityStageReached=$global:FooterComparisonTestParityReached;parityProcessExecuted=$false;message=$taskError;comparatorSHA256=$taskCaseComparatorSHA}
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
    if ($taskPreviousGoMarker) { Set-Variable -Name FooterComparisonTestParityReached -Value $taskPreviousGoMarkerValue -Scope Global }
    else { Remove-Variable -Name FooterComparisonTestParityReached -Scope Global -ErrorAction SilentlyContinue }
}
