# Exercise only trusted builder path helpers, without invoking Cargo or touching
# the real reference checkout. Retain all fixtures under a fresh output/footer-*.
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$taskScript = Join-Path $PSScriptRoot 'footer-build.ps1'
$taskAst = [Management.Automation.Language.Parser]::ParseFile($taskScript, [ref]$null, [ref]$null)
foreach ($taskFunction in $taskAst.FindAll({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] }, $true)) {
    if ($taskFunction.Name -in @('Resolve-FooterSourcePhysical', 'Assert-FooterPath', 'Resolve-FooterOutput', 'Assert-FooterCargoCache')) {
        . ([scriptblock]::Create($taskFunction.Extent.Text))
    }
}
$taskStage = Join-Path $taskRoot ('output/footer-path-tests-' + [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss-fff'))
New-Item -ItemType Directory -Path $taskStage | Out-Null
$taskFrozenPhysical = Join-Path $taskStage 'frozen'
New-Item -ItemType Directory -Path $taskFrozenPhysical | Out-Null
$taskResults = [Collections.Generic.List[object]]::new()
function Assert-FooterRejects([string]$Name, [scriptblock]$Action) {
    $taskRejected = $false
    try { $null = & $Action } catch { $taskRejected = $true }
    if (-not $taskRejected) { throw "Boundary regression: $Name was accepted." }
    $taskResults.Add(@{ name = $Name; rejected = $true })
}
$taskNormal = Join-Path $taskStage 'new-capture'
if ((Resolve-FooterOutput $taskNormal 'unused') -ne $taskNormal) { throw 'Normal checkout-local path changed.' }
$taskResults.Add(@{ name = 'normal new checkout path'; accepted = $true })
Assert-FooterRejects 'outside checkout' { Resolve-FooterOutput (Join-Path ([IO.Path]::GetDirectoryName($taskRoot)) 'footer-outside-test') 'unused' }
Assert-FooterRejects 'existing evidence path' { Resolve-FooterOutput $taskStage 'unused' }
Assert-FooterRejects 'frozen source descendant' { Resolve-FooterOutput (Join-Path $taskFrozenPhysical 'new-capture') 'unused' }
$taskAlias = Join-Path $taskStage 'source-alias'
New-Item -ItemType Junction -Path $taskAlias -Target $taskFrozenPhysical | Out-Null
if ((Resolve-FooterSourcePhysical $taskAlias) -ne $taskFrozenPhysical) { throw 'SourceRoot alias was not physically resolved.' }
Assert-FooterRejects 'physical source reached through alias' { Resolve-FooterOutput (Join-Path (Resolve-FooterSourcePhysical $taskAlias) 'new-capture') 'unused' }
$taskDestination = Join-Path $taskStage 'destination'
New-Item -ItemType Directory -Path $taskDestination | Out-Null
$taskCargo = Join-Path $taskStage 'cargo'
New-Item -ItemType Directory -Path $taskCargo | Out-Null
Assert-FooterCargoCache $taskCargo
$taskResults.Add(@{ name = 'normal Cargo cache'; accepted = $true })
New-Item -ItemType Junction -Path (Join-Path $taskCargo 'debug') -Target $taskDestination | Out-Null
Assert-FooterRejects 'Cargo debug junction' { Assert-FooterCargoCache $taskCargo }
$taskDeepCargo = Join-Path $taskStage 'cargo-deep'
New-Item -ItemType Directory -Path (Join-Path $taskDeepCargo 'debug/deps/nested') -Force | Out-Null
New-Item -ItemType Junction -Path (Join-Path $taskDeepCargo 'debug/deps/nested/redirect') -Target $taskDestination | Out-Null
Assert-FooterRejects 'nested Cargo junction' { Assert-FooterCargoCache $taskDeepCargo }
[IO.File]::WriteAllText((Join-Path $taskStage 'result.json'), ($taskResults | ConvertTo-Json -Depth 5), [Text.UTF8Encoding]::new($false))
Write-Output $taskStage
