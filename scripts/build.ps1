[CmdletBinding()]
param(
    [ValidateSet('windows/amd64', 'darwin/amd64', 'darwin/arm64', 'darwin/universal')]
    [string[]]$Targets = @('windows/amd64'),
    [string]$OutDir = 'build',
    [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$taskOutput = [IO.Path]::GetFullPath((Join-Path $taskRoot $OutDir))
if (-not $taskOutput.StartsWith($taskRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Release output must be inside this checkout: $taskOutput"
}
$taskPreviousLocation = (Get-Location).Path
$taskPreviousCgo = $env:CGO_ENABLED
$taskPreviousNoColor = $env:NO_COLOR
$taskPreviousMygoFlags = $env:MYGO_LDFLAGS
$taskPreviousGoWork = $env:GOWORK
$taskPreviousUpdateKey = $env:MYGO_UPDATER_PRIVATE_KEY
try {
    Set-Location -LiteralPath $taskRoot
    $env:CGO_ENABLED = '0'
    $env:NO_COLOR = '1'
    $env:GOWORK = 'off'
    if (-not $env:MYGO_UPDATER_PRIVATE_KEY) {
        $taskUpdateKeyFile = $env:RCODE_UPDATE_PRIVATE_KEY_FILE
        if (-not $taskUpdateKeyFile) {
            $taskUpdateKeyFile = Join-Path ([Environment]::GetFolderPath('UserProfile')) '.rcode/update-keys/mygo-update.key'
        }
        if (-not (Test-Path -LiteralPath $taskUpdateKeyFile -PathType Leaf)) {
            throw 'Release signing key missing. Set MYGO_UPDATER_PRIVATE_KEY or RCODE_UPDATE_PRIVATE_KEY_FILE; see docs/updates.md.'
        }
        $env:MYGO_UPDATER_PRIVATE_KEY = [IO.File]::ReadAllText($taskUpdateKeyFile).Trim()
    }
    & go run ./scripts/check-updates.go -check-key
    if ($LASTEXITCODE -ne 0) { throw 'Update signing key verification failed' }
    $taskGoVersion = (& go env GOVERSION).Trim()
    if ($LASTEXITCODE -ne 0 -or $taskGoVersion -ne 'go1.27.3') { throw "Expected go1.27.3; found $taskGoVersion" }
    $taskMygoVersion = (& go list -m -f '{{.Version}}' github.com/egoist/mygo).Trim()
    if ($LASTEXITCODE -ne 0 -or $taskMygoVersion -ne 'v0.8.3') { throw "Expected MyGo v0.8.3; found $taskMygoVersion" }
    $taskFrameworkStamp = (& go run ./scripts/check-framework.go -ldflags).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Pinned MyGo source/patch verification failed' }
    $env:MYGO_LDFLAGS = $taskFrameworkStamp
    & go run ./scripts/check-distribution.go
    if ($LASTEXITCODE -ne 0) { throw 'Independent distribution identity verification failed' }
    if (-not $SkipTests) {
        & go test ./...
        if ($LASTEXITCODE -ne 0) { throw 'Tests failed' }
        & go vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
        & go tool mygo vet .
        if ($LASTEXITCODE -ne 0) { throw 'MyGo vet failed' }
    }
    $taskTargetList = $Targets -join ','
    & go tool mygo build -platform $taskTargetList -o $taskOutput -skip-notarize
    if ($LASTEXITCODE -ne 0) { throw 'MyGo release build failed' }
    & go run ./scripts/verify-package.go -root $taskOutput -targets $taskTargetList
    if ($LASTEXITCODE -ne 0) { throw 'Release package verification failed' }
    & go run ./scripts/check-updates.go -root $taskOutput -targets $taskTargetList
    if ($LASTEXITCODE -ne 0) { throw 'Signed update verification failed' }
} finally {
    $env:CGO_ENABLED = $taskPreviousCgo
    $env:NO_COLOR = $taskPreviousNoColor
    $env:MYGO_LDFLAGS = $taskPreviousMygoFlags
    $env:GOWORK = $taskPreviousGoWork
    $env:MYGO_UPDATER_PRIVATE_KEY = $taskPreviousUpdateKey
    Set-Location -LiteralPath $taskPreviousLocation
}
