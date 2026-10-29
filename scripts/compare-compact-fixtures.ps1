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
function Resolve-TaskPath([string]$Path) {
    if (-not [IO.Path]::IsPathRooted($Path)) { $Path = Join-Path $taskRoot $Path }
    return [IO.Path]::TrimEndingDirectorySeparator([IO.Path]::GetFullPath($Path))
}
function Assert-That([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}
function Resolve-InputDirectory([string]$Path) {
    $taskFullPath = Resolve-TaskPath $Path
    $taskPhysical = [IO.Path]::GetPathRoot($taskFullPath)
    foreach ($taskPart in $taskFullPath.Substring($taskPhysical.Length).Split([IO.Path]::DirectorySeparatorChar, [StringSplitOptions]::RemoveEmptyEntries)) {
        $taskPhysical = Join-Path $taskPhysical $taskPart
        $taskItem = Get-Item -LiteralPath $taskPhysical -Force
        if ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint) {
            $taskTarget = $taskItem.ResolveLinkTarget($true)
            Assert-That ($null -ne $taskTarget) 'Could not resolve a comparison input alias.'
            $taskPhysical = $taskTarget.FullName
            $taskItem = Get-Item -LiteralPath $taskPhysical -Force
        }
        Assert-That $taskItem.PSIsContainer 'Comparison input must be a directory.'
    }
    return [IO.Path]::TrimEndingDirectorySeparator($taskPhysical)
}
function Read-Json([string]$Path) { return Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json }
function Get-SHA([string]$Path) { return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Same-Pair($Left, $Right) { return @($Left).Count -eq 2 -and @($Right).Count -eq 2 -and ($Left -join ',') -eq ($Right -join ',') }
function Check-LayoutRect($Rect, [string]$Name) {
    foreach ($taskField in @('x', 'y', 'width', 'height')) {
        Assert-That ($null -ne $Rect.$taskField -and [double]::IsFinite([double]$Rect.$taskField)) "Incomplete layout geometry: $Name"
    }
    Assert-That ($Rect.width -gt 0 -and $Rect.height -gt 0) "Empty layout geometry: $Name"
}
function Check-NativeGeometry($Metadata, [int]$LogicalWidth, [int]$Scale, [string]$Name) {
    $taskClient = @(($LogicalWidth * $Scale), (560 * $Scale))
    Assert-That ((Same-Pair $Metadata.clientSizePx $taskClient) -and @($Metadata.clientRectPx).Count -eq 4 -and ($Metadata.clientRectPx -join ',') -ceq "0,0,$($taskClient[0]),$($taskClient[1])") "Client/DPI dimensions differ: $Name"
    Assert-That (@($Metadata.windowRectScreenPx).Count -eq 4 -and @($Metadata.windowSizePx).Count -eq 2 -and @($Metadata.clientOriginScreenPx).Count -eq 2) "Incomplete native geometry: $Name"
    $taskExtent = @(($Metadata.windowRectScreenPx[2] - $Metadata.windowRectScreenPx[0]), ($Metadata.windowRectScreenPx[3] - $Metadata.windowRectScreenPx[1]))
    Assert-That ((Same-Pair $taskExtent $Metadata.windowSizePx) -and $taskExtent[0] -ge $taskClient[0] -and $taskExtent[1] -ge $taskClient[1]) "Window extent metadata differs: $Name"
    $taskOffset = @(($Metadata.clientOriginScreenPx[0] - $Metadata.windowRectScreenPx[0]), ($Metadata.clientOriginScreenPx[1] - $Metadata.windowRectScreenPx[1]))
    Assert-That ($taskOffset[0] -ge 0 -and $taskOffset[1] -ge 0 -and $taskOffset[0] + $taskClient[0] -le $taskExtent[0] -and $taskOffset[1] + $taskClient[1] -le $taskExtent[1]) "Client origin lies outside its window: $Name"
    # WGC excludes some invisible native resize borders. Validate each extent,
    # then compare both originals; do not equate WGC with GetWindowRect or scale
    # the old nonclient borders to manufacture a 2x expected frame.
    Assert-That ($Metadata.pixels.frameSizePx[0] -ge $taskClient[0] -and $Metadata.pixels.frameSizePx[1] -ge $taskClient[1] -and $Metadata.pixels.frameSizePx[0] -le $taskExtent[0] -and $Metadata.pixels.frameSizePx[1] -le $taskExtent[1]) "WGC/client/window extent relationship differs: $Name"
    # These are two separately observed Windows default-frame profiles, not a
    # scaling formula for nonclient pixels. Matching only the two inputs would
    # otherwise allow a 2x PNG to be relabeled as a smaller 1x client in a large
    # window. A different OS/frame profile requires new native evidence.
    $taskProfile = if ($Scale -eq 1) { @{frame=@(2,32);window=@(16,39);inset=@(8,31)} } else { @{frame=@(4,60);window=@(26,71);inset=@(13,58)} }
    Assert-That ((Same-Pair $Metadata.pixels.frameSizePx @(($taskClient[0]+$taskProfile.frame[0]),($taskClient[1]+$taskProfile.frame[1]))) -and (Same-Pair $taskExtent @(($taskClient[0]+$taskProfile.window[0]),($taskClient[1]+$taskProfile.window[1]))) -and (Same-Pair $taskOffset $taskProfile.inset)) "Unverified native frame/DPI profile: $Name"
    if ($null -ne $Metadata.dpi) { Assert-That ($Metadata.dpi -eq (96 * $Scale)) "Reported DPI differs: $Name" }
    return ,$taskOffset
}
function Check-Png([string]$Path, $Metadata) {
    $taskBytes = [IO.File]::ReadAllBytes($Path)
    Assert-That ($taskBytes.Length -ge 24) "Truncated PNG: $Path"
    Assert-That ([Convert]::ToHexString($taskBytes[0..7]) -eq '89504E470D0A1A0A') "Non-PNG input: $Path"
    $taskSize = @(
        [BitConverter]::ToUInt32([byte[]]@($taskBytes[19], $taskBytes[18], $taskBytes[17], $taskBytes[16]), 0),
        [BitConverter]::ToUInt32([byte[]]@($taskBytes[23], $taskBytes[22], $taskBytes[21], $taskBytes[20]), 0)
    )
    $taskHash = Get-SHA $Path
    Assert-That ($Metadata.schemaVersion -eq 1 -and $Metadata.status -ceq 'captured') "Uncaptured input: $Path"
    Assert-That ($Metadata.referenceCommit -ceq $taskCommit) "Reference commit mismatch: $Path"
    Assert-That ($Metadata.cropped -ceq $false -and $Metadata.resized -ceq $false -and @($Metadata.imageTransformations).Count -eq 0) "Transformed input: $Path"
    Assert-That ($Metadata.pixels.pixelFormat -ceq 'BGRA8' -and $Metadata.cursor -ceq 'WithoutCursor' -and $Metadata.captureBorder -ceq 'system Default; no privacy override' -and $Metadata.pixels.encoder -ceq 'Windows BitmapEncoder::PngEncoderId; no JPEG intermediate') "Transport mismatch: $Path"
    Assert-That ($Metadata.transport -eq 'in-process windows-capture 1.5.0 WGC Frame::save_as_image(Png)') "Not the full WGC transport: $Path"
    Assert-That ($Metadata.pixels.pngSHA256 -ceq $taskHash -and $Metadata.pixels.pngBytes -eq $taskBytes.Length) "PNG hash or byte count mismatch: $Path"
    Assert-That ((Same-Pair $taskSize $Metadata.pixels.pngSizePx) -and (Same-Pair $taskSize $Metadata.pixels.frameSizePx)) "PNG dimensions mismatch: $Path"
    return [ordered]@{ file = $Path; sha256 = $taskHash; bytes = $taskBytes.Length; size = $taskSize; metadataSHA256 = Get-SHA ($Path + '.capture.json') }
}
$ReferenceDirectory = Resolve-InputDirectory $ReferenceDirectory
$ImplementationDirectory = Resolve-InputDirectory $ImplementationDirectory
$OutputDirectory = Resolve-TaskPath $OutputDirectory
Assert-That ($ReferenceDirectory -ne $ImplementationDirectory) 'Reference and implementation directories must differ.'
Assert-That ($OutputDirectory.StartsWith($taskRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) 'Comparison output must stay inside this checkout.'
Assert-That (-not $OutputDirectory.StartsWith((Join-Path $taskRoot 'output/reference-sources') + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) 'Comparison output must not enter the read-only reference checkouts.'
foreach ($taskInput in @($ReferenceDirectory, $ImplementationDirectory)) {
    Assert-That (-not ($OutputDirectory.Equals($taskInput, [StringComparison]::OrdinalIgnoreCase) -or $OutputDirectory.StartsWith($taskInput + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase))) 'Output must not alter an input directory.'
}
$taskAncestor = $OutputDirectory
while ($taskAncestor) {
    $taskItem = Get-Item -LiteralPath $taskAncestor -Force -ErrorAction SilentlyContinue
    if ($taskItem) {
        Assert-That (-not ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) 'Comparison output must not traverse a reparse point.'
        Assert-That ($taskAncestor -ne $OutputDirectory) 'Comparison requires a new output directory.'
        Assert-That $taskItem.PSIsContainer 'Comparison output has a file ancestor.'
    }
    if ($taskAncestor.Equals($taskRoot, [StringComparison]::OrdinalIgnoreCase)) { break }
    $taskAncestor = Split-Path $taskAncestor -Parent
}
$taskManifestPath = Join-Path $ImplementationDirectory 'evidence.json'
$taskManifest = Read-Json $taskManifestPath
Assert-That ($taskManifest.captureMode -eq 'wgc' -and $taskManifest.referenceCommit -ceq $taskCommit -and @($taskManifest.captures).Count -eq 8) 'Expected an eight-scene native WGC implementation manifest.'
$taskInputs = @()
$taskScale = $null
foreach ($taskAppearance in @('dark', 'light')) {
    foreach ($taskWidth in @('wide', 'narrow')) {
        foreach ($taskPage in @('panel', 'model-list')) {
            $taskName = "compact-$taskPage-$taskAppearance-$taskWidth"
            $taskEntry = @($taskManifest.captures | Where-Object name -CEQ $taskName)
            Assert-That ($taskEntry.Count -eq 1) "Missing or duplicate scene: $taskName"
            $taskEntry = $taskEntry[0]
            Assert-That ($taskEntry.file -ceq "$taskName.png" -and $taskEntry.captureMetadataFile -ceq "$taskName.png.capture.json") "Unexpected scene path: $taskName"
            $taskRefPath = Join-Path $ReferenceDirectory "$taskName.png"
            $taskActualPath = Join-Path $ImplementationDirectory $taskEntry.file
            $taskRef = Read-Json ($taskRefPath + '.capture.json')
            $taskActual = Read-Json ($taskActualPath + '.capture.json')
            if ($null -eq $taskScale) { $taskScale = $taskRef.scale }
            Assert-That ($taskScale -in @(1, 2) -and $taskRef.scale -eq $taskScale -and $taskActual.scale -eq $taskScale -and $taskActual.dpi -eq (96 * $taskScale)) "DPI/scale differs: $taskName"
            Assert-That ($taskRef.name -ceq "$taskName.png" -and $taskActual.name -ceq "$taskName.png") "Capture scene name differs: $taskName"
            $taskRefProof = Check-Png $taskRefPath $taskRef
            $taskActualProof = Check-Png $taskActualPath $taskActual
            Assert-That (Same-Pair $taskRefProof.size $taskActualProof.size) "Full frame dimensions differ: $taskName"
            $taskLogicalWidth = if ($taskWidth -eq 'wide') { 840 } else { 360 }
            $taskRefOffset = Check-NativeGeometry $taskRef $taskLogicalWidth $taskScale $taskName
            $taskActualOffset = Check-NativeGeometry $taskActual $taskLogicalWidth $taskScale $taskName
            Assert-That ((Same-Pair $taskRef.gpuiViewportLogical @($taskLogicalWidth, 560)) -and @($taskRef.gpuiBoundsLogical).Count -eq 4 -and $taskRef.gpuiBoundsLogical[2] -eq $taskLogicalWidth -and $taskRef.gpuiBoundsLogical[3] -eq 560) "Reference logical viewport differs: $taskName"
            Assert-That ($taskRef.gpuiBoundsLogical[0] * $taskScale -eq $taskRef.clientOriginScreenPx[0] -and $taskRef.gpuiBoundsLogical[1] * $taskScale -eq $taskRef.clientOriginScreenPx[1]) "Reference logical/client origin differs: $taskName"
            Assert-That ((Same-Pair $taskRef.windowSizePx $taskActual.windowSizePx)) "Native window extents differ: $taskName"
            Assert-That (Same-Pair $taskRefOffset $taskActualOffset) "Client placement within the native frame differs: $taskName"
            $taskList = $taskPage -eq 'model-list'
            $taskState = $taskEntry.state
            $taskExpectedPage = if ($taskList) { 'list' } else { 'panel' }
            Assert-That ($taskRef.appearance -ceq $taskAppearance -and $taskState.appearance -ceq $taskAppearance -and $taskState.page -ceq $taskExpectedPage) "Appearance/page differs: $taskName"
            Assert-That ($taskRef.surface -eq 'opaque' -and $taskRef.reducedMotion -ceq $true -and $taskState.surface -eq 'opaque' -and $taskState.motionPreference -eq 'reduced') "Surface/motion differs: $taskName"
            Assert-That ($taskRef.fixtureState.catalog -eq 'pickers.rs::fixture_compact_catalog' -and $taskRef.fixtureState.modelList -ceq $taskList -and $taskRef.fixtureState.reasoning -eq 'high' -and $taskRef.fixtureState.fast -ceq $false) "Reference typed state differs: $taskName"
            Assert-That ($taskState.logicalWidth -eq $taskLogicalWidth -and $taskState.logicalHeight -eq 560 -and $taskState.providerId -eq 'codex' -and $taskState.modelId -eq 'gpt-5.4' -and $taskState.effectiveReasoning -eq 'High' -and $taskState.fast -ceq $false -and $taskState.contextWindow -eq 'standard' -and $taskState.popoverOpen -ceq $true -and $taskState.geometryStable -ceq $true -and -not $taskState.layoutError) "Implementation typed state differs: $taskName"
            # Legacy R6 1x captures predate this explicit binding evidence. At
            # 2x it is mandatory: an unattached fixture otherwise selects the
            # 1x-only option text path while claiming a physical 2x capture.
            if ($taskScale -eq 2 -or $null -ne $taskState.nativeWindowAttached -or $null -ne $taskState.oneXOptionText) {
                Assert-That ($taskState.nativeWindowAttached -is [bool] -and $taskState.oneXOptionText -is [bool] -and $taskState.nativeWindowAttached -ceq $true -and $taskState.oneXOptionText -ceq ($taskScale -eq 1)) "Native window/text adaptation differs: $taskName"
            }
            foreach ($taskPart in @('header', 'row', 'anchor', 'popup')) { Check-LayoutRect $taskState.geometry.$taskPart "$taskName/$taskPart" }
            $taskGeometry = $taskState.geometry
            Assert-That ((@($taskGeometry.header.x, $taskGeometry.header.y, $taskGeometry.header.width, $taskGeometry.header.height) -join ',') -ceq "24,446,$($taskLogicalWidth-48),26" -and (@($taskGeometry.row.x, $taskGeometry.row.y, $taskGeometry.row.width, $taskGeometry.row.height) -join ',') -ceq "24,488,$($taskLogicalWidth-48),48") "Implementation scaffold geometry differs: $taskName"
            Assert-That ($taskGeometry.anchor.x -ge $taskGeometry.row.x -and $taskGeometry.anchor.y -ge $taskGeometry.row.y -and $taskGeometry.anchor.x + $taskGeometry.anchor.width -le $taskGeometry.row.x + $taskGeometry.row.width -and $taskGeometry.anchor.y + $taskGeometry.anchor.height -le $taskGeometry.row.y + $taskGeometry.row.height) "Anchor is outside its fixture row: $taskName"
            Assert-That ($taskGeometry.popup.x -ge 0 -and $taskGeometry.popup.y -ge 0 -and $taskGeometry.popup.x + $taskGeometry.popup.width -le $taskLogicalWidth -and $taskGeometry.popup.y + $taskGeometry.popup.height -le 560) "Popup is outside its fixture client: $taskName"
            Assert-That ($taskEntry.sha256 -ceq $taskActualProof.sha256 -and (Same-Pair @($taskEntry.pixelWidth, $taskEntry.pixelHeight) $taskActualProof.size)) "Implementation manifest differs from disk: $taskName"
            $taskInputs += [ordered]@{ name = $taskName; reference = $taskRefProof; implementation = $taskActualProof; clientOriginInWindowPx = $taskRefOffset; clientSizePx = $taskRef.clientSizePx; windowSizePx = $taskRef.windowSizePx }
        }
    }
}
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
Copy-Item -LiteralPath $PSCommandPath -Destination (Join-Path $OutputDirectory 'compare-compact-fixtures.ps1')
$taskTool = Join-Path $OutputDirectory ('parity' + $(if ($IsWindows) { '.exe' } else { '' }))
Push-Location $taskRoot
try {
    & go build -o $taskTool ./cmd/parity
    if ($LASTEXITCODE -ne 0) { throw 'Parity tool build failed.' }
    $taskScenes = foreach ($taskInput in $taskInputs) {
        $taskSceneDirectory = Join-Path $OutputDirectory $taskInput.name
        & $taskTool --reference $taskInput.reference.file --actual $taskInput.implementation.file --out $taskSceneDirectory --threshold $Threshold > (Join-Path $OutputDirectory ($taskInput.name + '.log'))
        $taskExit = $LASTEXITCODE
        if ($taskExit -notin @(0, 1)) { throw "Parity command failed: $($taskInput.name), exit $taskExit" }
        $taskMetrics = Read-Json (Join-Path $taskSceneDirectory 'metrics.json')
        [ordered]@{ name = $taskInput.name; reference = $taskInput.reference; implementation = $taskInput.implementation; clientOriginInWindowPx = $taskInput.clientOriginInWindowPx; clientSizePx = $taskInput.clientSizePx; windowSizePx = $taskInput.windowSizePx; metrics = $taskMetrics; thresholdMet = $taskMetrics.ssim -ge $Threshold; commandExitCode = $taskExit }
    }
} finally { Pop-Location }
$taskAllPassed = @($taskScenes | Where-Object { -not $_.thresholdMet }).Count -eq 0
$taskReport = [ordered]@{
    schemaVersion = 1; referenceCommit = $taskCommit; comparedAtUTC = [DateTime]::UtcNow.ToString('o'); scale = $taskScale; dpi = 96 * $taskScale
    status = if ($taskAllPassed) { 'pixel-threshold-met; acceptance-pending' } else { 'pixel-differences-remain' }
    threshold = $Threshold; allThresholdsMet = $taskAllPassed; imageTransformations = @()
    comparison = 'Complete original WGC PNG pairs; local 8x8 luminance SSIM, no registration, crop, resize or mask.'
    nativeFrameScope = 'Separately observed Windows default frame profiles at actual 1x or 2x; other native frame profiles require new evidence.'
    stateScope = 'Reference runtime exports appearance/page/effort/Fast; its selected model and context defaults also depend on the frozen fixture catalog. Keyboard, motion, accessibility and other scenes are not accepted by these scores.'
    implementationManifestSHA256 = Get-SHA $taskManifestPath; bridgeSHA256 = $taskManifest.captureBridgeSHA256
    scriptSHA256 = Get-SHA $PSCommandPath; parityBinarySHA256 = Get-SHA $taskTool; scenes = @($taskScenes)
    comparisonSources = @('cmd/parity/main.go', 'internal/visual/compare.go') | ForEach-Object { [ordered]@{ path = $_; sha256 = Get-SHA (Join-Path $taskRoot $_) } }
}
$taskReport | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath (Join-Path $OutputDirectory 'comparison.json') -Encoding utf8
$taskScenes | ForEach-Object { '{0}: SSIM {1:F9}, {2} changed pixels' -f $_.name, $_.metrics.ssim, $_.metrics.changedPixels }
if (-not $taskAllPassed) { exit 1 }
