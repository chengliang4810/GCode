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
function Assert-Footer([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Full-FooterPath([string]$Path) {
    if (-not [IO.Path]::IsPathRooted($Path)) { $Path = Join-Path $taskRoot $Path }
    [IO.Path]::TrimEndingDirectorySeparator([IO.Path]::GetFullPath($Path))
}
function Physical-FooterInput([string]$Path) {
    $taskFull = Full-FooterPath $Path
    $taskPhysical = [IO.Path]::GetPathRoot($taskFull)
    foreach ($taskPart in $taskFull.Substring($taskPhysical.Length).Split([IO.Path]::DirectorySeparatorChar, [StringSplitOptions]::RemoveEmptyEntries)) {
        $taskPhysical = Join-Path $taskPhysical $taskPart
        $taskItem = Get-Item -LiteralPath $taskPhysical -Force
        if ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint) {
            $taskTarget = $taskItem.ResolveLinkTarget($true)
            Assert-Footer ($null -ne $taskTarget) 'Unresolved input alias.'
            $taskPhysical = $taskTarget.FullName
            $taskItem = Get-Item -LiteralPath $taskPhysical -Force
        }
        Assert-Footer $taskItem.PSIsContainer 'Comparison inputs must be directories.'
    }
    [IO.Path]::TrimEndingDirectorySeparator($taskPhysical)
}
function Read-FooterJson([string]$Path) { Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json }
function Footer-Hash([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Footer-ArrayEqual($Left, $Right) { (@($Left) -join ',') -ceq (@($Right) -join ',') }
function Footer-Png([string]$Path, $Metadata) {
    $taskBytes = [IO.File]::ReadAllBytes($Path)
    Assert-Footer ($taskBytes.Length -ge 24 -and ($taskBytes[0..7] -join ',') -eq '137,80,78,71,13,10,26,10') "Invalid original PNG: $Path"
    $taskSize = @()
    foreach ($taskAt in @(16, 20)) { $taskSize += [long]$taskBytes[$taskAt]*16777216 + [long]$taskBytes[$taskAt+1]*65536 + [long]$taskBytes[$taskAt+2]*256 + $taskBytes[$taskAt+3] }
    $taskHash = Footer-Hash $Path
    Assert-Footer ((Footer-ArrayEqual $taskSize $Metadata.pixels.pngSizePx) -and (Footer-ArrayEqual $taskSize $Metadata.pixels.frameSizePx)) "PNG/frame dimensions disagree: $Path"
    Assert-Footer ($taskHash -ceq $Metadata.pixels.pngSHA256 -and $taskBytes.Length -eq $Metadata.pixels.pngBytes) "PNG bytes/hash disagree: $Path"
    [ordered]@{file=$Path;sha256=$taskHash;bytes=$taskBytes.Length;size=$taskSize;metadataSHA256=(Footer-Hash ($Path+'.capture.json'))}
}

$ReferenceDirectory = Physical-FooterInput $ReferenceDirectory
$ImplementationDirectory = Physical-FooterInput $ImplementationDirectory
$OutputDirectory = Full-FooterPath $OutputDirectory
Assert-Footer (-not $ReferenceDirectory.Equals($ImplementationDirectory, [StringComparison]::OrdinalIgnoreCase)) 'Inputs must be independent directories.'
Assert-Footer ($OutputDirectory.StartsWith((Join-Path $taskRoot 'output/footer-comparison-'), [StringComparison]::OrdinalIgnoreCase)) 'Use a new output/footer-comparison-* directory in this checkout.'
Assert-Footer (-not (Test-Path -LiteralPath $OutputDirectory)) 'Comparison output already exists.'
foreach ($taskInput in @($ReferenceDirectory, $ImplementationDirectory)) {
    Assert-Footer (-not $OutputDirectory.Equals($taskInput, [StringComparison]::OrdinalIgnoreCase) -and -not $OutputDirectory.StartsWith($taskInput+[IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) 'Comparison output cannot enter either input tree.'
}
$taskAncestor = $OutputDirectory
while ($taskAncestor -and $taskAncestor.StartsWith($taskRoot, [StringComparison]::OrdinalIgnoreCase)) {
    $taskItem = Get-Item -LiteralPath $taskAncestor -Force -ErrorAction SilentlyContinue
    Assert-Footer (-not $taskItem -or -not ($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) 'Comparison output cannot redirect through a reparse point.'
    if ($taskAncestor -eq $taskRoot) { break }
    $taskAncestor = [IO.Path]::GetDirectoryName($taskAncestor)
}
$taskManifestPath = Join-Path $ImplementationDirectory 'evidence.json'
$taskManifest = Read-FooterJson $taskManifestPath
Assert-Footer ($taskManifest.referenceCommit -ceq $taskCommit -and $taskManifest.captureMode -ceq 'wgc' -and @($taskManifest.captures).Count -eq 28) 'Expected 28 implementation WGC scenes at the frozen reference.'
$taskInputs = @()
$taskScale = $null
foreach ($taskAppearance in @('dark', 'light')) {
    foreach ($taskBase in @('known', 'unknown', 'capacity', 'zero', 'over', 'warning', 'danger')) {
        $taskCards = @('')
        if ($taskBase -ne 'unknown') { $taskCards += '-context' }
        if ($taskBase -eq 'known') { $taskCards += '-accounts' }
        foreach ($taskCard in $taskCards) {
            $taskScene = $taskBase+$taskCard
            $taskName = "footer-$taskScene-$taskAppearance"
            $taskRefFile = Join-Path $ReferenceDirectory ($taskName+'.png')
            $taskActualFile = Join-Path $ImplementationDirectory ($taskName+'.png')
            $taskRef = Read-FooterJson ($taskRefFile+'.capture.json')
            $taskActual = Read-FooterJson ($taskActualFile+'.capture.json')
            if ($null -eq $taskScale) { $taskScale = $taskRef.scale }
            Assert-Footer ($taskScale -in @(1,2) -and $taskRef.scale -eq $taskScale -and $taskActual.scale -eq $taskScale) "DPI/scale mismatch: $taskName"
            $taskEntries = @($taskManifest.captures | Where-Object name -CEQ $taskName)
            Assert-Footer ($taskEntries.Count -eq 1) "Missing or duplicate scene $taskName"
            $taskEntry = $taskEntries[0]
            foreach ($taskMetadata in @($taskRef, $taskActual)) {
                Assert-Footer ($taskMetadata.schemaVersion -eq 1 -and $taskMetadata.status -ceq 'captured' -and $taskMetadata.transport -ceq 'in-process windows-capture 1.5.0 WGC Frame::save_as_image(Png)' -and $taskMetadata.cursor -ceq 'WithoutCursor' -and $taskMetadata.captureBorder -ceq 'system Default; no privacy override' -and $taskMetadata.pixels.pixelFormat -ceq 'BGRA8' -and $taskMetadata.pixels.encoder -ceq 'Windows BitmapEncoder::PngEncoderId; no JPEG intermediate') "Capture transport/status mismatch: $taskName"
                Assert-Footer ($taskMetadata.referenceCommit -ceq $taskCommit -and $taskMetadata.cropped -ceq $false -and $taskMetadata.resized -ceq $false -and @($taskMetadata.imageTransformations).Count -eq 0) "Altered or wrong-source image: $taskName"
                Assert-Footer ((Footer-ArrayEqual $taskMetadata.clientSizePx @((840*$taskScale),(560*$taskScale))) -and (Footer-ArrayEqual $taskMetadata.clientRectPx @(0,0,(840*$taskScale),(560*$taskScale)))) "Client/DPI mismatch: $taskName"
                $taskWindowExtent = @(($taskMetadata.windowRectScreenPx[2]-$taskMetadata.windowRectScreenPx[0]), ($taskMetadata.windowRectScreenPx[3]-$taskMetadata.windowRectScreenPx[1]))
                Assert-Footer (Footer-ArrayEqual $taskWindowExtent $taskMetadata.windowSizePx) "Window extent metadata mismatch: $taskName"
                # Independently observed 1x and 2x Windows default frames.
                # Do not derive 2x nonclient pixels by doubling the 1x frame,
                # or accept a larger 2x PNG relabeled as a smaller 1x client.
                $taskFrameProfile = if ($taskScale -eq 1) { @{frame=@(2,32);window=@(16,39);inset=@(8,31)} } else { @{frame=@(4,60);window=@(26,71);inset=@(13,58)} }
                $taskMetadataOffset = @(($taskMetadata.clientOriginScreenPx[0]-$taskMetadata.windowRectScreenPx[0]), ($taskMetadata.clientOriginScreenPx[1]-$taskMetadata.windowRectScreenPx[1]))
                Assert-Footer ((Footer-ArrayEqual $taskMetadata.pixels.frameSizePx @((840*$taskScale+$taskFrameProfile.frame[0]),(560*$taskScale+$taskFrameProfile.frame[1]))) -and (Footer-ArrayEqual $taskWindowExtent @((840*$taskScale+$taskFrameProfile.window[0]),(560*$taskScale+$taskFrameProfile.window[1]))) -and (Footer-ArrayEqual $taskMetadataOffset $taskFrameProfile.inset)) "Unverified native frame/DPI profile: $taskName"
                if ($null -ne $taskMetadata.dpi) { Assert-Footer ($taskMetadata.dpi -eq (96*$taskScale)) "Reported DPI mismatch: $taskName" }
            }
            Assert-Footer ($taskActual.dpi -eq (96*$taskScale) -and (Footer-ArrayEqual $taskRef.windowSizePx $taskActual.windowSizePx)) "Native frame mismatch: $taskName"
            $taskOffsets = foreach ($taskMetadata in @($taskRef, $taskActual)) { ,@(($taskMetadata.clientOriginScreenPx[0]-$taskMetadata.windowRectScreenPx[0]), ($taskMetadata.clientOriginScreenPx[1]-$taskMetadata.windowRectScreenPx[1])) }
            Assert-Footer (Footer-ArrayEqual $taskOffsets[0] $taskOffsets[1]) "Client origin mismatch: $taskName"
            $taskState = $taskEntry.state
            Assert-Footer ($taskState.appearance -ceq $taskAppearance -and $taskState.scene -ceq $taskScene -and $taskState.geometryStable -ceq $true -and -not $taskState.layoutError -and $taskState.logicalWidth -eq 840 -and $taskState.logicalHeight -eq 560) "Implementation state mismatch: $taskName"
            Assert-Footer ($taskRef.appearance -ceq $taskAppearance -and $taskRef.surface -ceq 'opaque' -and $taskRef.reducedMotion -ceq $true -and $taskRef.fixtureState.scene -ceq $taskBase -and $taskRef.fixtureState.harness -ceq 'codex' -and $taskRef.fixtureState.targetId -ceq 'local') "Reference state mismatch: $taskName"
            Assert-Footer ($taskRef.fixtureState.engineBootstrapped -ceq $false -and $taskRef.fixtureState.activeAccountId -ceq 'a' -and [Math]::Abs($taskRef.fixtureState.accountFraction-.9) -lt .000001 -and (Footer-ArrayEqual $taskRef.fixtureState.lastSyntheticPointerLogical @(10,10))) "Reference component state/pointer mismatch: $taskName"
            $taskScaffold = $taskRef.fixtureState.scaffold
            Assert-Footer ((Footer-ArrayEqual $taskScaffold.clientSize @(840,560)) -and $taskScaffold.rootPadding -eq 24 -and $taskScaffold.footerHeight -eq 24 -and $taskScaffold.footerAlign -ceq 'end' -and $taskScaffold.font -ceq 'Geist' -and $taskScaffold.fontSize -eq 13 -and $taskScaffold.letterSpacing -eq 0) "Reference scaffold mismatch: $taskName"
            Assert-Footer ($taskState.accountsOpen -ceq ($taskCard -eq '-accounts') -and $taskState.contextOpen -ceq ($taskCard -eq '-context')) "Implementation popup mismatch: $taskName"
            if ($taskCard) {
                Assert-Footer ($taskRef.fixtureState.interaction.requestedCard -ceq $taskCard.Substring(1) -and $taskRef.fixtureState.interaction.privatePopupFieldsReadOrChanged -ceq $false -and $taskRef.fixtureState.interaction.path -ceq 'public Window::dispatch_event MouseDown/MouseUp; production on_click') "Reference event mismatch: $taskName"
            } else {
                Assert-Footer ($null -eq $taskRef.fixtureState.interaction) "Baseline retained a card interaction: $taskName"
            }
            Assert-Footer ((Footer-ArrayEqual @($taskState.row.x,$taskState.row.y,$taskState.row.width,$taskState.row.height) @(24,512,792,24)) -and (Footer-ArrayEqual $taskRef.fixtureState.geometryLogical.footer @(24,512,792,24))) "Footer scaffold mismatch: $taskName"
            $taskTokens = @{known=42000;capacity=$null;zero=0;over=240000;warning=150000;danger=180000;unknown=$null}[$taskBase]
            $taskWindow = if ($taskBase -eq 'unknown') { $null } else { 200000 }
            Assert-Footer ($taskState.contextUsage.tokens -eq $taskTokens -and $taskRef.fixtureState.context.tokens -eq $taskTokens -and $taskState.contextUsage.window -eq $taskWindow -and $taskRef.fixtureState.context.window -eq $taskWindow) "Context DTO mismatch: $taskName"
            Assert-Footer ($taskRef.fixtureState.hasContextWindow -ceq ($taskBase -ne 'unknown')) "Context availability mismatch: $taskName"
            if ($null -eq $taskTokens -or $null -eq $taskWindow) {
                Assert-Footer ($null -eq $taskRef.fixtureState.contextFraction) "Unreported context fraction mismatch: $taskName"
            } else {
                Assert-Footer ($null -ne $taskRef.fixtureState.contextFraction -and [Math]::Abs($taskRef.fixtureState.contextFraction - $taskTokens/$taskWindow) -lt 0.000000001) "Context fraction mismatch: $taskName"
            }
            Assert-Footer (@($taskState.accounts).Count -eq 2 -and @($taskRef.fixtureState.accounts).Count -eq 2) "Account count mismatch: $taskName"
            for ($taskIndex=0; $taskIndex -lt 2; $taskIndex++) {
                $taskAccount = $taskState.accounts[$taskIndex]; $taskRefAccount = $taskRef.fixtureState.accounts[$taskIndex]
                $taskUsage = @($taskAccount.usage | Where-Object { $null -ne $_ })
                $taskReferenceUsage = @($taskRefAccount.usageWindows | Where-Object { $null -ne $_ })
                Assert-Footer ($taskAccount.id -ceq $taskRefAccount.id -and $taskAccount.providerId -ceq 'codex' -and $taskRefAccount.harness -ceq 'codex' -and $taskAccount.targetId -ceq 'local' -and [string]$taskAccount.email -ceq [string]$taskRefAccount.email -and [string]$taskAccount.displayName -ceq [string]$taskRefAccount.displayName -and [string]$taskAccount.plan -ceq [string]$taskRefAccount.planLabel -and $taskAccount.inUse -ceq $taskRefAccount.active -and $taskAccount.switchable -ceq $taskRefAccount.switchable -and $taskUsage.Count -eq $taskReferenceUsage.Count) "Account DTO mismatch: $taskName"
                Assert-Footer ([string]$taskAccount.loginProvider -ceq [string]$taskRefAccount.provider -and [string]$taskAccount.organization -ceq [string]$taskRefAccount.organization -and [string]$taskAccount.usageError -ceq [string]$taskRefAccount.usageError) "Account group/error mismatch: $taskName"
                for ($taskUsageIndex=0; $taskUsageIndex -lt $taskUsage.Count; $taskUsageIndex++) {
                    Assert-Footer ($taskAccount.usage[$taskUsageIndex].label -ceq $taskRefAccount.usageWindows[$taskUsageIndex].label -and [Math]::Abs($taskAccount.usage[$taskUsageIndex].usedPercent/100-$taskRefAccount.usageWindows[$taskUsageIndex].usedFraction) -lt 0.000001 -and -not $taskRefAccount.usageWindows[$taskUsageIndex].resetsAt -and [DateTime]$taskAccount.usage[$taskUsageIndex].resetsAt -eq [DateTime]::MinValue) "Usage DTO mismatch: $taskName"
                }
            }
            $taskProofs = foreach ($taskPair in @(@($taskRefFile,$taskRef), @($taskActualFile,$taskActual))) { Footer-Png $taskPair[0] $taskPair[1] }
            Assert-Footer (Footer-ArrayEqual $taskProofs[0].size $taskProofs[1].size) "Full WGC dimensions differ: $taskName"
            Assert-Footer ($taskEntry.sha256 -ceq $taskProofs[1].sha256 -and $taskEntry.file -ceq ($taskName+'.png') -and (Footer-ArrayEqual @($taskEntry.pixelWidth,$taskEntry.pixelHeight) $taskProofs[1].size)) "Manifest/disk mismatch: $taskName"
            $taskInputs += [ordered]@{name=$taskName;reference=$taskProofs[0];implementation=$taskProofs[1];clientOriginInWindowPx=$taskOffsets[0]}
        }
    }
}
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
Copy-Item -LiteralPath $PSCommandPath -Destination (Join-Path $OutputDirectory 'compare-footer-fixtures.ps1')
$taskTool = Join-Path $OutputDirectory 'parity.exe'
Push-Location -LiteralPath $taskRoot
try {
    & go build -o $taskTool ./cmd/parity
    if ($LASTEXITCODE -ne 0) { throw 'Parity build failed.' }
    $taskScenes = foreach ($taskInput in $taskInputs) {
        $taskSceneDirectory = Join-Path $OutputDirectory $taskInput.name
        & $taskTool --reference $taskInput.reference.file --actual $taskInput.implementation.file --out $taskSceneDirectory --threshold $Threshold > (Join-Path $OutputDirectory ($taskInput.name+'.log'))
        $taskExit = $LASTEXITCODE
        Assert-Footer ($taskExit -in @(0,1)) "Comparison command failed: $($taskInput.name)"
        $taskMetrics = Read-FooterJson (Join-Path $taskSceneDirectory 'metrics.json')
        [ordered]@{name=$taskInput.name;reference=$taskInput.reference;implementation=$taskInput.implementation;metrics=$taskMetrics;thresholdMet=$taskMetrics.ssim -ge $Threshold;commandExitCode=$taskExit}
    }
} finally { Pop-Location }
$taskAll = @($taskScenes | Where-Object { -not $_.thresholdMet }).Count -eq 0
[ordered]@{schemaVersion=1;referenceCommit=$taskCommit;scale=$taskScale;status=$(if($taskAll){'pixel-threshold-met; acceptance-pending'}else{'pixel-differences-remain'});threshold=$Threshold;allThresholdsMet=$taskAll;imageTransformations=@();comparison='Complete original WGC PNG pairs; local 8x8 luminance SSIM, no registration, crop, resize or mask.';nativeFrameScope='Separately observed Windows default frame profiles at actual 1x or 2x; other native frame profiles require new evidence.';stateScope='28 component fixtures only. Scene DTOs and synthetic source clicks are not OS input, motion, accessibility, full-footer or platform acceptance.';scriptSHA256=(Footer-Hash $PSCommandPath);parityBinarySHA256=(Footer-Hash $taskTool);implementationManifestSHA256=(Footer-Hash $taskManifestPath);bridgeSHA256=$taskManifest.captureBridgeSHA256;scenes=@($taskScenes)} | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath (Join-Path $OutputDirectory 'comparison.json') -Encoding utf8
$taskScenes | ForEach-Object { '{0}: SSIM {1:F9}' -f $_.name,$_.metrics.ssim }
if (-not $taskAll) { exit 1 }
