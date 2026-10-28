[CmdletBinding()]
param([string]$OutDir='output/captures/matrix',[switch]$AllThemes,[switch]$BoundarySizes)
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$taskOutput=[IO.Path]::GetFullPath((Join-Path $taskRoot $OutDir))
if (-not $taskOutput.StartsWith($taskRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Capture output must stay in this checkout.' }
Push-Location -LiteralPath $taskRoot
try {
    New-Item -ItemType Directory -Path $taskOutput -Force | Out-Null
    $taskExecutable=Join-Path $taskOutput 'capture-runner.exe'
    & go build -o $taskExecutable ./cmd/zeron
    if ($LASTEXITCODE -ne 0) { throw 'Capture build failed.' }
    $taskCases=@()
    foreach ($taskRoute in @('chat','chat/chat-0','history','changes','files','pull-requests','settings/general','settings/appearance','settings/notifications','settings/shortcuts','settings/providers','settings/devices','settings/cloud','settings/billing','settings/files','settings/archived sessions')) {
        foreach ($taskAppearance in @('light','dark')) { $taskCases+=@{scenario='default';route=$taskRoute;appearance=$taskAppearance;width=1320;height=880;scale=1;theme=''} }
    }
    foreach ($taskScenario in @('showcase','transcript','live','questions','permissions','empty','offline','error','stress')) { $taskCases+=@{scenario=$taskScenario;route='chat/chat-0';appearance='dark';width=1320;height=880;scale=1;theme=''} }
    if ($AllThemes) {
        $taskRegistry=Get-Content -LiteralPath internal/design/themes.json -Raw | ConvertFrom-Json
        foreach ($taskFamily in $taskRegistry.families) { foreach ($taskTheme in $taskFamily.variants) { $taskCases+=@{scenario='showcase';route='chat/chat-0';appearance=$taskTheme.appearance;width=1320;height=880;scale=1;theme=$taskTheme.id} } }
    }
    if ($BoundarySizes) {
        foreach ($taskSize in @(@(900,600),@(1600,1000))) { foreach ($taskScale in @(1,1.25,1.5,2)) { $taskCases+=@{scenario='showcase';route='chat/chat-0';appearance='dark';width=$taskSize[0];height=$taskSize[1];scale=$taskScale;theme=''} } }
    }
    $taskEvidence=@(); $taskIndex=0
    foreach ($taskCase in $taskCases) {
        $taskIndex++; $taskName=('{0:D3}-{1}-{2}-{3}{4}.png' -f $taskIndex,$taskCase.scenario,$taskCase.route.Replace('/','-').Replace(' ','-'),$taskCase.appearance,$taskCase.theme)
        $taskPath=Join-Path $taskOutput $taskName
        & $taskExecutable --scenario $taskCase.scenario --route $taskCase.route --appearance $taskCase.appearance --theme $taskCase.theme --width $taskCase.width --height $taskCase.height --scale $taskCase.scale --headless --capture $taskPath
        if ($LASTEXITCODE -ne 0) { throw "Capture failed: $taskName" }
        $taskCase.file=$taskName; $taskCase.sha256=(Get-FileHash -LiteralPath $taskPath -Algorithm SHA256).Hash.ToLowerInvariant(); $taskEvidence+=$taskCase
    }
    @{schemaVersion=1;sourceCommit='ab189159f10cf8e63c0502be75a59bbc487e0eaf';runnerSha256=(Get-FileHash -LiteralPath $taskExecutable -Algorithm SHA256).Hash.ToLowerInvariant();renderer='MyGo Tester';status='implementation-captures-only';nativeCompositorVerified=$false;referenceComparison='pending';captures=$taskEvidence} | ConvertTo-Json -Depth 7 | Set-Content -LiteralPath (Join-Path $taskOutput 'evidence.json') -Encoding utf8
} finally { Pop-Location }
