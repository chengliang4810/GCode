[CmdletBinding()]
param([string]$ZeronRoot='D:/zeron',[string]$MygoExamplesRoot='D:/projects/mygo')
$ErrorActionPreference='Stop'
$taskSpecs=@(
    @{name='Zeron';path=$ZeronRoot;url='https://github.com/zeronsh/zeron.git';revision='ab189159f10cf8e63c0502be75a59bbc487e0eaf'},
    @{name='MyGo examples';path=$MygoExamplesRoot;url='https://github.com/egoist/mygo.git';revision='43199d0b0fb95fd0b71e5a225d2807c04ebcc609'}
)
foreach($taskSpec in $taskSpecs) {
    $taskDestination=[IO.Path]::GetFullPath($taskSpec.path)
    if(Test-Path -LiteralPath $taskDestination) {
        if(-not (Test-Path -LiteralPath (Join-Path $taskDestination '.git'))) {throw "Existing directory is not a reference checkout: $taskDestination"}
        $taskHead=(& git -C $taskDestination rev-parse HEAD).Trim()
        if($LASTEXITCODE -ne 0 -or $taskHead -ne $taskSpec.revision) {throw "Existing reference has a different revision; preserve it and choose a new destination: $taskDestination"}
        $taskDirty=@(& git -C $taskDestination status --porcelain)
        if($LASTEXITCODE -ne 0 -or $taskDirty.Count -gt 0) {throw "Existing reference has local changes; preserve it and choose a clean destination: $taskDestination"}
        Write-Output ("Verified {0}: {1}" -f $taskSpec.name,$taskHead)
        continue
    }
    $taskParent=Split-Path -Parent $taskDestination
    New-Item -ItemType Directory -Path $taskParent -Force | Out-Null
    & git clone --no-checkout -- $taskSpec.url $taskDestination
    if($LASTEXITCODE -ne 0) {throw "Clone failed: $($taskSpec.name)"}
    & git -C $taskDestination checkout --detach $taskSpec.revision
    if($LASTEXITCODE -ne 0) {throw "Reference checkout failed: $($taskSpec.name)"}
}
