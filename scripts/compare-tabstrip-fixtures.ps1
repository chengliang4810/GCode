[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$ReferenceDirectory,
    [Parameter(Mandatory)][string]$ImplementationDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [ValidateRange(0, 1)][double]$Threshold = 0.995
)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskCommit = 'ab189159f10cf8e63c0502be75a59bbc487e0eaf'
$script:taskTabstripJsonHashes = @{}
function Assert-Tabstrip([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Full-TabstripPath([string]$Path) {
    if (-not [IO.Path]::IsPathRooted($Path)) { $Path = Join-Path $taskRoot $Path }
    [IO.Path]::TrimEndingDirectorySeparator([IO.Path]::GetFullPath($Path))
}
function Physical-TabstripInput([string]$Path) {
    $taskFull = Full-TabstripPath $Path
    Assert-Tabstrip (-not $taskFull.Equals([IO.Path]::GetPathRoot($taskFull),[StringComparison]::OrdinalIgnoreCase)) 'Capture input cannot be a drive or share root.'
    $taskPhysical = [IO.Path]::GetPathRoot($taskFull)
    foreach ($taskPart in $taskFull.Substring($taskPhysical.Length).Split([IO.Path]::DirectorySeparatorChar, [StringSplitOptions]::RemoveEmptyEntries)) {
        $taskPhysical = Join-Path $taskPhysical $taskPart
        $taskItem = Get-Item -LiteralPath $taskPhysical -Force
        if ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint) {
            $taskTarget = $taskItem.ResolveLinkTarget($true)
            Assert-Tabstrip ($null -ne $taskTarget) 'Unresolved input alias.'
            $taskPhysical = $taskTarget.FullName
            $taskItem = Get-Item -LiteralPath $taskPhysical -Force
        }
        Assert-Tabstrip $taskItem.PSIsContainer 'Comparison inputs must be directories.'
    }
    $taskPhysical = [IO.Path]::TrimEndingDirectorySeparator($taskPhysical)
    Assert-Tabstrip (-not $taskPhysical.Equals([IO.Path]::GetPathRoot($taskPhysical),[StringComparison]::OrdinalIgnoreCase)) 'Capture input alias cannot resolve to a drive or share root.'
    $taskPhysical
}
function Read-TabstripJson([string]$Path) {
    $taskFile = Get-Item -LiteralPath $Path -Force
    Assert-Tabstrip (-not $taskFile.PSIsContainer -and -not ($taskFile.Attributes -band [IO.FileAttributes]::ReparsePoint)) "Input must be an original regular file: $Path"
    $taskBytes = [IO.File]::ReadAllBytes($Path)
    $script:taskTabstripJsonHashes[$Path] = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($taskBytes)).ToLowerInvariant()
    [Text.Encoding]::UTF8.GetString($taskBytes) | ConvertFrom-Json
}
function Tabstrip-Hash([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Snapshot-TabstripFile([string]$Source, [string]$Destination, [string]$ExpectedSHA256) {
    [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($Destination))
    [IO.File]::Copy($Source,$Destination,$false)
    Assert-Tabstrip ((Tabstrip-Hash $Destination) -ceq $ExpectedSHA256) "Input changed before its verified byte snapshot: $Source"
}
function Tabstrip-Equal($Left, $Right) { (@($Left) -join ',') -ceq (@($Right) -join ',') }
function Tabstrip-Number($Value) { $null -ne $Value -and $Value -is [ValueType] -and $Value -isnot [bool] -and [double]::IsFinite([double]$Value) }
function Tabstrip-Bool($Value, [bool]$Expected) { $Value -is [bool] -and $Value -eq $Expected }
function Tabstrip-Near($Value, [double]$Expected, [double]$Tolerance = 0.05) { (Tabstrip-Number $Value) -and [Math]::Abs([double]$Value-$Expected) -lt $Tolerance }
function Assert-TabstripBox($Box, [string]$Name, [bool]$AllowEmpty = $false) {
    foreach ($taskKey in @('x','y','width','height')) { Assert-Tabstrip (Tabstrip-Number $Box.$taskKey) "Missing/nonfinite measured $Name.$taskKey" }
    Assert-Tabstrip ($(if ($AllowEmpty) { $Box.width -ge 0 -and $Box.height -ge 0 } else { $Box.width -gt 0 -and $Box.height -gt 0 })) "Invalid measured box: $Name"
}
function Tabstrip-Png([string]$Path, $Metadata) {
    $taskItem = Get-Item -LiteralPath $Path -Force
    Assert-Tabstrip (-not $taskItem.PSIsContainer -and -not ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) "PNG is not an original regular file: $Path"
    $taskBytes = [IO.File]::ReadAllBytes($Path)
    Assert-Tabstrip ($taskBytes.Length -ge 24 -and ($taskBytes[0..7] -join ',') -eq '137,80,78,71,13,10,26,10') "Invalid original PNG: $Path"
    $taskSize = @()
    foreach ($taskAt in @(16,20)) { $taskSize += [long]$taskBytes[$taskAt]*16777216 + [long]$taskBytes[$taskAt+1]*65536 + [long]$taskBytes[$taskAt+2]*256 + $taskBytes[$taskAt+3] }
    $taskHash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($taskBytes)).ToLowerInvariant()
    Assert-Tabstrip ((Tabstrip-Equal $taskSize $Metadata.pixels.pngSizePx) -and (Tabstrip-Equal $taskSize $Metadata.pixels.frameSizePx)) "PNG/frame dimensions disagree: $Path"
    Assert-Tabstrip ($taskHash -ceq $Metadata.pixels.pngSHA256 -and $taskBytes.Length -eq $Metadata.pixels.pngBytes) "PNG bytes/hash disagree: $Path"
    [ordered]@{file=$Path;sha256=$taskHash;bytes=$taskBytes.Length;size=$taskSize;metadataSHA256=$script:taskTabstripJsonHashes[$Path+'.capture.json']}
}
function Assert-TabstripState($State, [string]$Appearance, [int]$Width, [int]$Count, [double]$Fraction, [string]$Name) {
    Assert-Tabstrip ($State.scene.appearance -ceq $Appearance -and $State.scene.hostWidth -eq $Width -and $State.scene.count -eq $Count -and (Tabstrip-Near $State.scene.scrollFraction $Fraction 0.000001)) "Scene mismatch: $Name"
    Assert-Tabstrip ($State.logicalWidth -eq 840 -and $State.logicalHeight -eq 560 -and $State.parentSessionId -ceq 'parent' -and (Tabstrip-Bool $State.expanded $false)) "Scaffold/parent mismatch: $Name"
    Assert-Tabstrip ((Tabstrip-Bool $State.geometryStable $true) -and (Tabstrip-Number $State.stableFrames) -and $State.stableFrames -ge 2 -and (Tabstrip-Bool $State.scrollApplied $true) -and -not $State.layoutError) "Unsettled layout: $Name"
    Assert-Tabstrip ((Tabstrip-Number $State.maxX) -and $State.maxX -ge 0 -and (Tabstrip-Number $State.positiveX) -and $State.positiveX -ge -0.05 -and (Tabstrip-Near $State.positiveX ($Fraction*$State.maxX))) "Requested scroll fraction not applied: $Name"
    Assert-Tabstrip ($(if ($Count -eq 0) { $null -eq $State.activeIndex } else { $null -ne $State.activeIndex -and $State.activeIndex -eq 0 })) "Active index mismatch: $Name"
    $taskGeometry = $State.geometry
    foreach ($taskKey in @('host','expand')) { Assert-TabstripBox $taskGeometry.$taskKey "$Name.$taskKey" }
    # The original production empty strip has a measured zero-sized scroller.
    Assert-TabstripBox $taskGeometry.scrollViewport "$Name.scrollViewport" ($Count -eq 0)
    Assert-Tabstrip ((Tabstrip-Near $taskGeometry.host.x 40) -and (Tabstrip-Near $taskGeometry.host.y 40) -and (Tabstrip-Near $taskGeometry.host.width $Width) -and (Tabstrip-Near $taskGeometry.host.height 38)) "Actual testhost envelope mismatch: $Name"
    Assert-Tabstrip (@($taskGeometry.chips).Count -eq $Count) "Measured chip count mismatch: $Name"
    Assert-Tabstrip ($(if ($Count -eq 0) { $null -eq $taskGeometry.plus } else { $null -ne $taskGeometry.plus })) "Plus availability mismatch: $Name"
    if ($Count -gt 0) { Assert-TabstripBox $taskGeometry.plus "$Name.plus" }
    for ($taskIndex=0; $taskIndex -lt $Count; $taskIndex++) {
        $taskChip = $taskGeometry.chips[$taskIndex]
        Assert-Tabstrip ($taskChip.sessionId -ceq ('side-{0:D2}' -f ($taskIndex+1)) -and $taskChip.title -ceq ('Tab{0:D2}' -f ($taskIndex+1)) -and $taskChip.kind -ceq 'session' -and $taskChip.icon -ceq 'chat-round-line' -and (Tabstrip-Bool $taskChip.idle $true)) "Chip data/kind mismatch: $Name chip$taskIndex"
        Assert-TabstripBox $taskChip.bounds "$Name.chip$taskIndex"
    }
}

$taskScriptSHA256 = Tabstrip-Hash $PSCommandPath
$ReferenceDirectory = Physical-TabstripInput $ReferenceDirectory
$ImplementationDirectory = Physical-TabstripInput $ImplementationDirectory
$OutputDirectory = Full-TabstripPath $OutputDirectory
foreach ($taskPair in @(@($ReferenceDirectory,$ImplementationDirectory),@($ImplementationDirectory,$ReferenceDirectory))) {
    Assert-Tabstrip (-not $taskPair[0].Equals($taskPair[1], [StringComparison]::OrdinalIgnoreCase) -and -not $taskPair[0].StartsWith($taskPair[1]+[IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) 'Inputs must be independent, nonoverlapping physical directories.'
}
Assert-Tabstrip ($OutputDirectory.StartsWith((Join-Path $taskRoot 'output/tabstrip-comparison-'), [StringComparison]::OrdinalIgnoreCase)) 'Use a new output/tabstrip-comparison-* directory in this checkout.'
Assert-Tabstrip (-not (Test-Path -LiteralPath $OutputDirectory)) 'Comparison output already exists.'
foreach ($taskInput in @($ReferenceDirectory,$ImplementationDirectory)) {
    Assert-Tabstrip (-not $OutputDirectory.Equals($taskInput, [StringComparison]::OrdinalIgnoreCase) -and -not $OutputDirectory.StartsWith($taskInput+[IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) 'Comparison output cannot enter either input tree.'
}
$taskAncestor = $OutputDirectory
while ($taskAncestor) {
    $taskItem = Get-Item -LiteralPath $taskAncestor -Force -ErrorAction SilentlyContinue
    Assert-Tabstrip (-not $taskItem -or ($taskItem.PSIsContainer -and -not ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint))) 'Comparison output cannot redirect through an alias or file.'
    $taskAncestor = [IO.Path]::GetDirectoryName($taskAncestor)
}
$taskManifestPath = Join-Path $ImplementationDirectory 'evidence.json'
$taskManifest = Read-TabstripJson $taskManifestPath
$taskReferenceManifestPath = Join-Path $ReferenceDirectory 'result.json'
$taskReferenceManifest = Read-TabstripJson $taskReferenceManifestPath
Assert-Tabstrip ($taskReferenceManifest.status -ceq '36-production-tabstrip-scenes-captured' -and $taskReferenceManifest.expectedScenes -eq 36 -and $taskReferenceManifest.referenceCommit -ceq $taskCommit -and $taskReferenceManifest.binarySHA256 -cmatch '^[0-9a-f]{64}$') 'Reference matrix is incomplete or has no binary identity.'
Assert-Tabstrip ($taskManifest.referenceCommit -ceq $taskCommit -and $taskManifest.captureMode -ceq 'wgc' -and @($taskManifest.captures).Count -eq 36) 'Expected36 implementation WGC scenes at the frozen reference.'
Assert-Tabstrip ($taskManifest.binarySHA256 -cmatch '^[0-9a-f]{64}$' -and $taskManifest.captureBridgeSHA256 -cmatch '^[0-9a-f]{64}$') 'Missing fixture/bridge binary identity.'
Assert-Tabstrip ($taskManifest.framework.UpstreamVersion -ceq 'v0.6.4' -and $taskManifest.framework.Revision -ceq 'rcode-native-ui.9' -and $taskManifest.framework.SourceSHA256 -ceq 'b2c4fc2817725ad69b915e53e764b4af08971ed327d933004c773f6fbf44be43' -and $taskManifest.sourceStamp -and $taskManifest.sourceStamp -cne 'unstamped') 'Missing/wrong verified framework or fixture source stamp.'
$taskInputs = @()
$taskScale = $null
$taskProcesses = @($null,$null)
$taskWindows = @($null,$null)
foreach ($taskAppearance in @('dark','light')) {
    foreach ($taskWidth in @(400,220)) {
        foreach ($taskCount in @(0,2,6)) {
            foreach ($taskPosition in @(@('start',0),@('middle',0.5),@('end',1))) {
                $taskFraction = [double]$taskPosition[1]
                $taskName = "tabstrip-$taskAppearance-w$taskWidth-n$taskCount-scroll$($taskPosition[0])"
                $taskRefFile = Join-Path $ReferenceDirectory ($taskName+'.png')
                $taskActualFile = Join-Path $ImplementationDirectory ($taskName+'.png')
                $taskRef = Read-TabstripJson ($taskRefFile+'.capture.json')
                $taskActual = Read-TabstripJson ($taskActualFile+'.capture.json')
                $taskEntries = @($taskManifest.captures | Where-Object name -CEQ $taskName)
                Assert-Tabstrip ($taskEntries.Count -eq 1) "Missing/duplicate scene: $taskName"
                $taskEntry = $taskEntries[0]
                if ($null -eq $taskScale) { $taskScale = $taskRef.scale }
                Assert-Tabstrip ($taskScale -in @(1,2) -and $taskRef.scale -eq $taskScale -and $taskActual.scale -eq $taskScale) "DPI/scale mismatch: $taskName"
                $taskFrameProfile = if ($taskScale -eq 1) { @{frame=@(2,32);window=@(16,39);inset=@(8,31)} } else { @{frame=@(4,60);window=@(26,71);inset=@(13,58)} }
                $taskSide = 0
                foreach ($taskMetadata in @($taskRef,$taskActual)) {
                    Assert-Tabstrip ($taskMetadata.schemaVersion -eq 1 -and $taskMetadata.status -ceq 'captured' -and $taskMetadata.transport -ceq 'in-process windows-capture 1.5.0 WGC Frame::save_as_image(Png)' -and $taskMetadata.cursor -ceq 'WithoutCursor' -and $taskMetadata.captureBorder -ceq 'system Default; no privacy override' -and $taskMetadata.pixels.pixelFormat -ceq 'BGRA8' -and $taskMetadata.pixels.encoder -ceq 'Windows BitmapEncoder::PngEncoderId; no JPEG intermediate') "Capture transport/status mismatch: $taskName"
                    Assert-Tabstrip ($taskMetadata.referenceCommit -ceq $taskCommit -and (Tabstrip-Bool $taskMetadata.cropped $false) -and (Tabstrip-Bool $taskMetadata.resized $false) -and @($taskMetadata.imageTransformations).Count -eq 0) "Altered/wrong-source image: $taskName"
                    Assert-Tabstrip ((Tabstrip-Number $taskMetadata.processId) -and $taskMetadata.processId -gt 0 -and $taskMetadata.hwnd -match '^0x[0-9a-fA-F]+$') "Missing actual process/window identity: $taskName"
                    if ($null -eq $taskProcesses[$taskSide]) { $taskProcesses[$taskSide] = $taskMetadata.processId; $taskWindows[$taskSide] = $taskMetadata.hwnd }
                    Assert-Tabstrip ($taskMetadata.processId -eq $taskProcesses[$taskSide] -and $taskMetadata.hwnd -ceq $taskWindows[$taskSide]) "Matrix changed capture process/window: $taskName"
                    Assert-Tabstrip ((Tabstrip-Equal $taskMetadata.clientSizePx @((840*$taskScale),(560*$taskScale))) -and (Tabstrip-Equal $taskMetadata.clientRectPx @(0,0,(840*$taskScale),(560*$taskScale)))) "Actual client/DPI mismatch: $taskName"
                    $taskExtent = @(($taskMetadata.windowRectScreenPx[2]-$taskMetadata.windowRectScreenPx[0]),($taskMetadata.windowRectScreenPx[3]-$taskMetadata.windowRectScreenPx[1]))
                    $taskInset = @(($taskMetadata.clientOriginScreenPx[0]-$taskMetadata.windowRectScreenPx[0]),($taskMetadata.clientOriginScreenPx[1]-$taskMetadata.windowRectScreenPx[1]))
                    Assert-Tabstrip ((Tabstrip-Equal $taskExtent $taskMetadata.windowSizePx) -and (Tabstrip-Equal $taskExtent @((840*$taskScale+$taskFrameProfile.window[0]),(560*$taskScale+$taskFrameProfile.window[1]))) -and (Tabstrip-Equal $taskMetadata.pixels.frameSizePx @((840*$taskScale+$taskFrameProfile.frame[0]),(560*$taskScale+$taskFrameProfile.frame[1]))) -and (Tabstrip-Equal $taskInset $taskFrameProfile.inset)) "Unverified native frame/DPI profile: $taskName"
                    if ($null -ne $taskMetadata.dpi) { Assert-Tabstrip ($taskMetadata.dpi -eq 96*$taskScale) "Reported DPI mismatch: $taskName" }
                    Assert-TabstripState $taskMetadata.fixtureState $taskAppearance $taskWidth $taskCount $taskFraction "$taskName side$taskSide"
                    $taskSide++
                }
                Assert-Tabstrip ($taskProcesses[0] -ne $taskProcesses[1] -and $taskActual.dpi -eq 96*$taskScale) "Independent actual capture process/DPI mismatch: $taskName"
                Assert-Tabstrip ($taskRef.appearance -ceq $taskAppearance -and $taskRef.surface -ceq 'opaque' -and (Tabstrip-Bool $taskRef.reducedMotion $true) -and (Tabstrip-Bool $taskRef.fixtureState.engineBootstrapped $false) -and (Tabstrip-Bool $taskRef.fixtureState.externalServicesUsed $false) -and (Tabstrip-Bool $taskRef.fixtureState.productionRenderingMethodsChanged $false)) "Reference production/safety state mismatch: $taskName"
                # GPUI's cached is_window_active is not Win32 foreground proof.
                foreach ($taskActivity in @($taskRef.nativeActivityBefore,$taskRef.nativeActivityAfter)) {
                    foreach ($taskKey in @('processId','foregroundProcessId','ownerProcessId','ownerThreadId','queryThreadId','foregroundThreadId')) {
                        Assert-Tabstrip ((Tabstrip-Number $taskActivity.$taskKey) -and $taskActivity.$taskKey -gt 0) "Reference activity numeric type/identity mismatch: $taskName $taskKey"
                    }
                    foreach ($taskKey in @('hwnd','foregroundHWND','callingThreadActiveHWND','ownerThreadActiveHWND')) {
                        Assert-Tabstrip ($taskActivity.$taskKey -is [string] -and $taskActivity.$taskKey -ceq $taskRef.hwnd) "Reference activity HWND type/identity mismatch: $taskName $taskKey"
                    }
                    Assert-Tabstrip ($taskActivity.processId -eq $taskRef.processId -and $taskActivity.hwnd -ceq $taskRef.hwnd -and $taskActivity.foregroundHWND -ceq $taskRef.hwnd -and $taskActivity.foregroundProcessId -eq $taskRef.processId -and (Tabstrip-Bool $taskActivity.nativeWindowActive $true)) "Reference actual foreground/window identity mismatch: $taskName"
                    Assert-Tabstrip ($taskActivity.ownerProcessId -eq $taskRef.processId -and $taskActivity.queryThreadId -eq $taskActivity.ownerThreadId -and $taskActivity.foregroundThreadId -eq $taskActivity.ownerThreadId -and (Tabstrip-Bool $taskActivity.queriedOnOwnerUiThread $true) -and (Tabstrip-Bool $taskActivity.ownerThreadActiveIsOwn $true) -and $null -eq $taskActivity.ownerThreadInfoError) "Reference owner UI-thread/native activity mismatch: $taskName"
                }
                Assert-Tabstrip ((Tabstrip-Equal $taskRef.fixtureState.lastSyntheticPointerLogical @(10,10)) -and (Tabstrip-Bool $taskRef.fixtureState.osPointerMoved $false) -and (Tabstrip-Bool $taskRef.fixtureState.nativeWindowActive $true)) "Reference pointer/window state mismatch: $taskName"
                Assert-Tabstrip ($taskRef.binarySHA256 -ceq $taskReferenceManifest.binarySHA256 -and ($taskRef.sourceManifest | ConvertTo-Json -Depth 40 -Compress) -ceq ($taskReferenceManifest.sourceManifest | ConvertTo-Json -Depth 40 -Compress) -and (Tabstrip-Equal $taskRef.contract.windowOriginDIP @(40,40))) "Reference binary/source/host identity mismatch: $taskName"
                $taskActualState = $taskActual.fixtureState
                Assert-Tabstrip ((Tabstrip-Bool $taskActualState.nativeWindowAttached $true) -and (Tabstrip-Bool $taskActualState.pointer.moveSeen $true) -and (Tabstrip-Near $taskActualState.pointer.lastMoveX 10 0.000001) -and (Tabstrip-Near $taskActualState.pointer.lastMoveY 10 0.000001) -and (Tabstrip-Bool $taskActualState.pointer.anyControlHovered $false) -and $taskActualState.pointer.source -ceq 'native input observations; CLI requests synthetic child-HWND messages; not OS cursor position') "Implementation pointer/window state mismatch: $taskName"
                $taskNative = $taskActual.fixtureNative
                $taskNativeAfter = $taskActual.fixtureNativeAfter
                foreach ($taskNativeSample in @($taskNative,$taskNativeAfter)) {
                    foreach ($taskKey in @('pid','hwnd','windowDpi','actualScale','pointerTargetHWND','foregroundHWND')) {
                        Assert-Tabstrip ((Tabstrip-Number $taskNativeSample.$taskKey) -and $taskNativeSample.$taskKey -gt 0) "Implementation native numeric type mismatch: $taskName $taskKey"
                    }
                    Assert-Tabstrip ($taskNativeSample.pid -eq $taskActual.processId -and $taskNativeSample.hwnd -eq [Convert]::ToUInt64($taskActual.hwnd.Substring(2),16) -and $taskNativeSample.windowDpi -eq $taskActual.dpi -and $taskNativeSample.actualScale -eq $taskScale -and (Tabstrip-Equal $taskNativeSample.requestedWindowOriginDIP @(40,40))) "Implementation fixture/transport native identity mismatch: $taskName"
                    Assert-Tabstrip ($taskNativeSample.pointerTargetHWND -gt 0 -and $taskNativeSample.pointerTargetHWND -ne $taskNativeSample.hwnd -and $taskNativeSample.foregroundHWND -eq $taskNativeSample.hwnd -and (Tabstrip-Bool $taskNativeSample.nativeWindowActive $true) -and (Tabstrip-Bool $taskNativeSample.pointerMessagesSent $true) -and $taskActualState.pointer.moveCount -ge 2) "Implementation input target/window activity mismatch: $taskName"
                }
                Assert-Tabstrip (($taskEntry.native | ConvertTo-Json -Depth 12 -Compress) -ceq ($taskNative | ConvertTo-Json -Depth 12 -Compress)) "Manifest/sidecar native identity mismatch: $taskName"
                Assert-Tabstrip (($taskEntry.nativeAfter | ConvertTo-Json -Depth 12 -Compress) -ceq ($taskNativeAfter | ConvertTo-Json -Depth 12 -Compress)) "Manifest/sidecar post-capture native identity mismatch: $taskName"
                $taskOriginalTransportPath = $taskActualFile+'.capture.json.transport-original.json'
                $taskOriginalTransport = Read-TabstripJson $taskOriginalTransportPath
                Assert-Tabstrip (@($taskOriginalTransport.PSObject.Properties).Count+3 -eq @($taskActual.PSObject.Properties).Count) "Transport field set changed: $taskName"
                foreach ($taskProperty in $taskOriginalTransport.PSObject.Properties) {
                    Assert-Tabstrip (($taskProperty.Value | ConvertTo-Json -Depth 40 -Compress) -ceq ($taskActual.($taskProperty.Name) | ConvertTo-Json -Depth 40 -Compress)) "Original transport field changed: $taskName $($taskProperty.Name)"
                }
                Assert-TabstripState $taskEntry.state $taskAppearance $taskWidth $taskCount $taskFraction "$taskName manifest"
                Assert-Tabstrip (($taskEntry.state | ConvertTo-Json -Depth 30 -Compress) -ceq ($taskActual.fixtureState | ConvertTo-Json -Depth 30 -Compress)) "Manifest/sidecar state mismatch: $taskName"
                Assert-Tabstrip ($taskEntry.captureMetadataFile -ceq ($taskName+'.png.capture.json') -and $taskEntry.captureMetadataSHA256 -ceq $script:taskTabstripJsonHashes[$taskActualFile+'.capture.json']) "Manifest/sidecar identity mismatch: $taskName"
                $taskProofs = foreach ($taskPair in @(@($taskRefFile,$taskRef),@($taskActualFile,$taskActual))) { Tabstrip-Png $taskPair[0] $taskPair[1] }
                Assert-Tabstrip ((Tabstrip-Equal $taskProofs[0].size $taskProofs[1].size) -and $taskEntry.sha256 -ceq $taskProofs[1].sha256 -and $taskEntry.file -ceq ($taskName+'.png') -and (Tabstrip-Equal @($taskEntry.pixelWidth,$taskEntry.pixelHeight) $taskProofs[1].size)) "Manifest/PNG identity mismatch: $taskName"
                $taskInputs += [ordered]@{name=$taskName;reference=$taskProofs[0];implementation=$taskProofs[1];referenceState=$taskRef.fixtureState;implementationState=$taskEntry.state;clientOriginInWindowPx=$taskFrameProfile.inset;referenceClientOriginScreenPx=$taskRef.clientOriginScreenPx;implementationClientOriginScreenPx=$taskActual.clientOriginScreenPx;originalTransportSHA256=$script:taskTabstripJsonHashes[$taskOriginalTransportPath]}
            }
        }
    }
}
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
Snapshot-TabstripFile $PSCommandPath (Join-Path $OutputDirectory 'compare-tabstrip-fixtures.ps1') $taskScriptSHA256
# Score only fresh, SHA-verified full-byte snapshots. A later build or another
# process cannot change which original pixels the recorded metrics consume.
Snapshot-TabstripFile $taskManifestPath (Join-Path $OutputDirectory 'inputs/implementation/evidence.json') $script:taskTabstripJsonHashes[$taskManifestPath]
Snapshot-TabstripFile $taskReferenceManifestPath (Join-Path $OutputDirectory 'inputs/reference/result.json') $script:taskTabstripJsonHashes[$taskReferenceManifestPath]
foreach ($taskInput in $taskInputs) {
    foreach ($taskSide in @('reference','implementation')) {
        $taskProof = $taskInput[$taskSide]
        $taskSnapshot = Join-Path $OutputDirectory ('inputs/'+$taskSide+'/'+$taskInput.name+'.png')
        Snapshot-TabstripFile $taskProof.file $taskSnapshot $taskProof.sha256
        Snapshot-TabstripFile ($taskProof.file+'.capture.json') ($taskSnapshot+'.capture.json') $taskProof.metadataSHA256
        $taskProof.snapshotFile = $taskSnapshot
    }
    Snapshot-TabstripFile ($taskInput.implementation.file+'.capture.json.transport-original.json') ($taskInput.implementation.snapshotFile+'.capture.json.transport-original.json') $taskInput.originalTransportSHA256
}
$taskTool = Join-Path $OutputDirectory 'parity.exe'
Push-Location -LiteralPath $taskRoot
try {
    & go build -o $taskTool ./cmd/parity
    if ($LASTEXITCODE -ne 0) { throw 'Parity build failed.' }
    $taskScenes = foreach ($taskInput in $taskInputs) {
        $taskSceneDirectory = Join-Path $OutputDirectory $taskInput.name
        & $taskTool --reference $taskInput.reference.snapshotFile --actual $taskInput.implementation.snapshotFile --out $taskSceneDirectory --threshold $Threshold > (Join-Path $OutputDirectory ($taskInput.name+'.log'))
        $taskExit = $LASTEXITCODE
        Assert-Tabstrip ($taskExit -in @(0,1)) "Comparison command failed: $($taskInput.name)"
        $taskMetrics = Read-TabstripJson (Join-Path $taskSceneDirectory 'metrics.json')
        foreach ($taskProof in @($taskInput.reference,$taskInput.implementation)) { Assert-Tabstrip ((Tabstrip-Hash $taskProof.snapshotFile) -ceq $taskProof.sha256) "Comparison snapshot changed: $($taskProof.snapshotFile)" }
        [ordered]@{name=$taskInput.name;reference=$taskInput.reference;implementation=$taskInput.implementation;referenceState=$taskInput.referenceState;implementationState=$taskInput.implementationState;referenceClientOriginScreenPx=$taskInput.referenceClientOriginScreenPx;implementationClientOriginScreenPx=$taskInput.implementationClientOriginScreenPx;originalTransportSHA256=$taskInput.originalTransportSHA256;metrics=$taskMetrics;thresholdMet=$taskMetrics.ssim -ge $Threshold;commandExitCode=$taskExit}
    }
} finally { Pop-Location }
foreach ($taskInput in $taskInputs) {
    foreach ($taskProof in @($taskInput.reference,$taskInput.implementation)) {
        Assert-Tabstrip ((Tabstrip-Hash $taskProof.snapshotFile) -ceq $taskProof.sha256 -and (Tabstrip-Hash ($taskProof.snapshotFile+'.capture.json')) -ceq $taskProof.metadataSHA256) "Scored evidence snapshot changed: $($taskProof.snapshotFile)"
    }
    Assert-Tabstrip ((Tabstrip-Hash ($taskInput.implementation.snapshotFile+'.capture.json.transport-original.json')) -ceq $taskInput.originalTransportSHA256) "Original transport snapshot changed: $($taskInput.name)"
}
$taskAll = @($taskScenes | Where-Object { -not $_.thresholdMet }).Count -eq 0
[ordered]@{schemaVersion=1;referenceCommit=$taskCommit;scale=$taskScale;status=$(if($taskAll){'pixel-threshold-met; acceptance-pending'}else{'pixel-differences-remain'});threshold=$Threshold;allThresholdsMet=$taskAll;imageTransformations=@();comparison='Complete original WGC PNG pairs; local8x8 luminance SSIM, no registration, crop, resize or mask.';nativeActivityPolicy='Actual caller-owned foreground HWND/owner PID required before/after capture on both sides; GPUI cached active remains distinct.';nativeFrameScope='Previously independently observed Windows default frame profiles at actual1x/2x; this run records only its actual scale.';stateScope='36 controlled production-component fixtures. Requested data, active tab, host and relative scroll match. Actual chip/viewport/max-scroll/font/color differences remain product evidence, not normalized away. Synthetic setup does not establish OS input, full titlebar, accessibility or other-platform acceptance.';scoringCaveat='Full-window blank scaffold dominates SSIM; meeting0.995 is triage only and never visual-parity acceptance.';scriptSHA256=$taskScriptSHA256;parityBinarySHA256=(Tabstrip-Hash $taskTool);implementationManifestSHA256=$script:taskTabstripJsonHashes[$taskManifestPath];referenceManifestSHA256=$script:taskTabstripJsonHashes[$taskReferenceManifestPath];inputSnapshotPolicy='SHA-verified full original byte copies before parity build; each scored snapshot rechecked after comparison. Originals may change later without changing scored pixels.';windowPositionPolicy='Both request40/40DIP; actual client screen origins remain measured and recorded because GPUI/MyGo window-bounds semantics differ. Opaque complete frames are compared without image registration.';bridgeSHA256=$taskManifest.captureBridgeSHA256;scenes=@($taskScenes)} | ConvertTo-Json -Depth 35 | Set-Content -LiteralPath (Join-Path $OutputDirectory 'comparison.json') -Encoding utf8
$taskScenes | ForEach-Object { '{0}: SSIM {1:F9}' -f $_.name,$_.metrics.ssim }
if (-not $taskAll) { exit 1 }
