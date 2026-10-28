param([string]$SourceRoot = 'D:\zeron', [switch]$SkipGenerated)
$ErrorActionPreference = 'Stop'
$taskWorkspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskBaseline = Get-Content -LiteralPath (Join-Path $taskWorkspace 'reference\baseline.json') -Raw | ConvertFrom-Json
$taskCommit = (& git -C $SourceRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $taskCommit -ne $taskBaseline.sourceCommit) { throw 'Reference commit differs from the approved baseline.' }
foreach ($taskSource in $taskBaseline.sourceFiles) {
    $taskHash = (Get-FileHash -LiteralPath (Join-Path $SourceRoot $taskSource.path) -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($taskHash -ne $taskSource.sha256) { throw "Reference source changed: $($taskSource.path)" }
}
$taskManifest = Get-Content -LiteralPath (Join-Path $taskWorkspace 'internal\assets\manifest.json') -Raw | ConvertFrom-Json
foreach ($taskAsset in $taskManifest.files) {
    $taskOriginal = (Get-FileHash -LiteralPath (Join-Path $SourceRoot $taskAsset.source) -Algorithm SHA256).Hash.ToLowerInvariant()
    $taskCopy = (Get-FileHash -LiteralPath (Join-Path $taskWorkspace ('internal\assets\data\' + $taskAsset.path)) -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($taskOriginal -ne $taskAsset.sha256 -or $taskCopy -ne $taskAsset.sha256) { throw "Imported artwork changed: $($taskAsset.path)" }
}
foreach ($taskEvidence in $taskBaseline.repositoryEvidence) {
    $taskHash = (Get-FileHash -LiteralPath (Join-Path $taskWorkspace $taskEvidence.copy) -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($taskHash -ne $taskEvidence.sha256) { throw "Historical evidence changed: $($taskEvidence.copy)" }
}
$taskDiagramManifestPath = Join-Path $taskWorkspace 'internal\assets\data\artifacts\diagram-manifest.json'
if (-not $SkipGenerated -and (Test-Path -LiteralPath $taskDiagramManifestPath)) {
    $taskDiagramManifest = Get-Content -LiteralPath $taskDiagramManifestPath -Raw | ConvertFrom-Json
    if ($taskDiagramManifest.sourceCommit -ne $taskCommit -or $taskDiagramManifest.diagrams.Count -ne 30) { throw 'Generated diagram source differs from the approved baseline.' }
    foreach ($taskDiagram in $taskDiagramManifest.diagrams) {
        $taskRawHash = (Get-FileHash -LiteralPath (Join-Path $taskWorkspace $taskDiagram.rawSVG) -Algorithm SHA256).Hash.ToLowerInvariant()
        $taskOutlinedHash = (Get-FileHash -LiteralPath (Join-Path $taskWorkspace ('internal/assets/data/' + $taskDiagram.asset)) -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($taskRawHash -ne $taskDiagram.rawSha256 -or $taskOutlinedHash -ne $taskDiagram.sha256) { throw "Generated diagram changed: $($taskDiagram.id)" }
    }
}
Write-Output "Verified reference $taskCommit, $($taskManifest.files.Count) assets, and $($taskBaseline.repositoryEvidence.Count) historical images."
Write-Output 'This verifies provenance, not native UI parity.'
