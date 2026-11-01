[CmdletBinding()]
param(
    [ValidateSet('default','transcript','live','questions','permissions','showcase')]
    [string]$Scene='default',
    [ValidateSet('light','dark','system')]
    [string]$Appearance='light',
    [string]$ReleaseDir='build/ui-audit/windows-amd64'
)
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$taskRelease=[IO.Path]::GetFullPath((Join-Path $taskRoot $ReleaseDir))
if (-not $taskRelease.StartsWith($taskRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Review package must be inside this checkout.' }
$taskExecutable=Join-Path $taskRelease 'GCode.exe'
if (-not (Test-Path -LiteralPath $taskExecutable)) { throw 'Build the current review package with scripts/build.ps1 -OutDir build/ui-audit first.' }
# An interactive review window is intentionally visible. Scenario data does not
# reuse the ordinary persisted profile or the installed original application.
$taskProcess=Start-Process -FilePath $taskExecutable -WorkingDirectory $taskRelease -ArgumentList @('--scenario',$Scene,'--route','chat/chat-0','--appearance',$Appearance) -PassThru
[PSCustomObject]@{processId=$taskProcess.Id;executable=$taskExecutable;sha256=(Get-FileHash -LiteralPath $taskExecutable -Algorithm SHA256).Hash.ToLowerInvariant();scenario=$Scene}
