[CmdletBinding()]
param([string]$ReleaseDir='build/windows-amd64')
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$taskRelease=[IO.Path]::GetFullPath((Join-Path $taskRoot $ReleaseDir))
$taskOutput=Join-Path $taskRoot ('output/install-smoke-'+(Get-Date -Format 'yyyyMMdd-HHmmss'))
$taskInstall=Join-Path $taskOutput 'application'
foreach ($taskPath in @($taskRelease,$taskOutput,$taskInstall)) {
    if (-not $taskPath.StartsWith($taskRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Smoke paths must stay in this checkout.' }
}
$taskRegistry='HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\dev.gcode.zeronui'
$taskShortcut=Join-Path ([Environment]::GetFolderPath('Programs')) 'Zeron Native UI.lnk'
$taskOriginalShortcut=Join-Path ([Environment]::GetFolderPath('Programs')) 'Zeron.lnk'
if ((Test-Path -LiteralPath $taskRegistry) -or (Test-Path -LiteralPath $taskShortcut)) { throw 'An existing replica installation must be preserved; smoke test skipped.' }
$taskOriginalHash=if (Test-Path -LiteralPath $taskOriginalShortcut) { (Get-FileHash -LiteralPath $taskOriginalShortcut).Hash } else { '' }
$taskSetup=Get-ChildItem -LiteralPath $taskRelease -Filter '*Setup*.exe' -File | Select-Object -First 1
if (-not $taskSetup) { throw 'Installer not found.' }
New-Item -ItemType Directory -Path $taskOutput -Force | Out-Null
$taskResult=@{schemaVersion=1;status='in-progress';installerSha256=(Get-FileHash -LiteralPath $taskSetup.FullName -Algorithm SHA256).Hash.ToLowerInvariant();nativeVisualAcceptance=$false;captures=@()}
try {
    $taskProcess=Start-Process -FilePath $taskSetup.FullName -ArgumentList @('/S',('/D='+$taskInstall)) -WindowStyle Hidden -PassThru -Wait
    if ($taskProcess.ExitCode -ne 0) { throw "Installer exited $($taskProcess.ExitCode)" }
    $taskRegistration=Get-ItemProperty -LiteralPath $taskRegistry
    if ($taskRegistration.InstallLocation -ne $taskInstall -or $taskRegistration.DisplayName -ne 'Zeron Native UI') { throw 'Installation identity/location differs from the independent configuration.' }
    if (-not (Test-Path -LiteralPath $taskShortcut)) { throw 'Independent Start-menu shortcut is missing.' }
    $taskExecutable=Join-Path $taskInstall 'Zeron.exe'
    foreach ($taskCase in @(@{route='chat/chat-0';appearance='light';name='chat'},@{route='terminal';appearance='dark';name='terminal'},@{route='browser';appearance='light';name='browser'})) {
        $taskCapture=Join-Path $taskOutput ($taskCase.name+'.png')
        $taskArguments=@('--scenario','default','--route',$taskCase.route,'--appearance',$taskCase.appearance,'--data-dir',(Join-Path $taskOutput 'profile'),'--capture',$taskCapture,'--capture-delay','8s','--exit-after-capture')
        $taskProcess=Start-Process -FilePath $taskExecutable -ArgumentList $taskArguments -WindowStyle Hidden -PassThru -Wait
        if ($taskProcess.ExitCode -ne 0 -or -not (Test-Path -LiteralPath $taskCapture)) { throw "Installed application capture failed: $($taskCase.name)" }
        $taskResult.captures+=@{route=$taskCase.route;file=$taskCase.name+'.png';sha256=(Get-FileHash -LiteralPath $taskCapture -Algorithm SHA256).Hash.ToLowerInvariant();kind='application-self-capture'}
    }
    $taskResult.status='passed-install-start-capture'
} catch { $taskResult.status='failed'; $taskResult.error=$_.Exception.Message; throw }
finally {
    $taskUninstaller=Join-Path $taskInstall 'Uninstall.exe'
    if (Test-Path -LiteralPath $taskUninstaller) {
        $taskProcess=Start-Process -FilePath $taskUninstaller -ArgumentList '/S' -WindowStyle Hidden -PassThru -Wait
        $taskResult.uninstallExitCode=$taskProcess.ExitCode
    }
    for ($taskAttempt=0; $taskAttempt -lt 50 -and ((Test-Path -LiteralPath $taskRegistry) -or (Test-Path -LiteralPath $taskShortcut)); $taskAttempt++) { Start-Sleep -Milliseconds 100 }
    $taskResult.registryRemoved=-not (Test-Path -LiteralPath $taskRegistry)
    $taskResult.shortcutRemoved=-not (Test-Path -LiteralPath $taskShortcut)
    $taskCurrentOriginalHash=if (Test-Path -LiteralPath $taskOriginalShortcut) { (Get-FileHash -LiteralPath $taskOriginalShortcut).Hash } else { '' }
    $taskResult.originalShortcutUnchanged=$taskOriginalHash -eq $taskCurrentOriginalHash
    if (-not $taskResult.registryRemoved -or -not $taskResult.shortcutRemoved -or -not $taskResult.originalShortcutUnchanged) { $taskResult.status='failed-cleanup' }
    $taskResult | ConvertTo-Json -Depth 7 | Set-Content -LiteralPath (Join-Path $taskOutput 'result.json') -Encoding utf8
}
if (-not $taskResult.registryRemoved -or -not $taskResult.shortcutRemoved -or -not $taskResult.originalShortcutUnchanged) { throw 'Installation cleanup/coexistence check failed; inspect result.json.' }
Write-Output (Join-Path $taskOutput 'result.json')
