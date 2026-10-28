param([string]$SourceRoot = 'D:\zeron')
$ErrorActionPreference = 'Stop'
$taskWorkspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskBaseline = Get-Content -LiteralPath (Join-Path $taskWorkspace 'reference\baseline.json') -Raw | ConvertFrom-Json
$taskCommit = (& git -C $SourceRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $taskCommit -ne $taskBaseline.sourceCommit) {
    throw 'Reference commit differs from the approved baseline.'
}
$taskSearchSource = 'crates/ui/src/settings/search.rs'
$taskIndexSource = Get-Content -LiteralPath (Join-Path $SourceRoot $taskSearchSource) -Raw
$taskIndexPattern = '\((?<id>\w+),\s*(?<section>\w+),\s*(?:None|Some\("(?<group>[^"]+)"\)),\s*"(?<title>[^"]+)",\s*"(?<terms>[^"]+)"\)'
$taskIndexRows = foreach ($taskMatch in [regex]::Matches($taskIndexSource, $taskIndexPattern)) {
    $taskSection = $taskMatch.Groups['section'].Value
    [ordered]@{
        id = $taskMatch.Groups['id'].Value
        section = $taskSection
        displaySection = $(if ($taskSection -eq 'Harnesses') { 'Providers' } else { $taskSection })
        group = $taskMatch.Groups['group'].Value
        title = $taskMatch.Groups['title'].Value
        terms = $taskMatch.Groups['terms'].Value
        scope = $(if ($taskSection -eq 'Voice') { 'excluded' } elseif ($taskSection -eq 'Appshots') { 'macOS-only' } else { 'included' })
        acceptance = 'pending-reference-capture'
    }
}
if ($taskIndexRows.Count -ne 58) { throw 'Frozen source settings index no longer has the expected 58 entries.' }
[ordered]@{
    schemaVersion = 1
    sourceCommit = $taskCommit
    source = "$($taskSearchSource):settings_index"
    note = 'Extracted source index, not a UI completion claim. Feature shortcuts and provider-expanded rows require their own cases.'
    rows = @($taskIndexRows)
} | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $taskWorkspace 'reference\settings-index.json')
Write-Output "Extracted $($taskIndexRows.Count) source settings rows, including five excluded voice rows and five macOS-only rows."
