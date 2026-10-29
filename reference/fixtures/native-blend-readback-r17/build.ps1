[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$BuildDirectory)
$ErrorActionPreference='Stop'
$taskRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
if(-not [IO.Path]::IsPathRooted($BuildDirectory)){$BuildDirectory=Join-Path $taskRoot $BuildDirectory}
$BuildDirectory=[IO.Path]::GetFullPath($BuildDirectory)
if(-not $BuildDirectory.StartsWith((Join-Path $taskRoot 'output/native-blend-readback-r17-'),[StringComparison]::OrdinalIgnoreCase)){throw 'Use a new checkout output/native-blend-readback-r17-* directory'}
if(Test-Path -LiteralPath $BuildDirectory){throw 'Preserve old attempts; directory exists'}
foreach($taskCheck in @($PSScriptRoot,[IO.Path]::GetDirectoryName($BuildDirectory))){
 $taskAncestor=[IO.Path]::GetFullPath($taskCheck)
 while($taskAncestor -and $taskAncestor.StartsWith($taskRoot,[StringComparison]::OrdinalIgnoreCase)){
  $taskItem=Get-Item -LiteralPath $taskAncestor -Force -ErrorAction SilentlyContinue
  if($taskItem -and ($taskItem.Attributes-band[IO.FileAttributes]::ReparsePoint)){throw 'Probe paths must not contain a reparse point'}
  $taskAncestor=[IO.Path]::GetDirectoryName($taskAncestor)
 }
}
New-Item -ItemType Directory -Path $BuildDirectory|Out-Null
function Get-ProbeHash([string]$Path){(Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()}
$taskSources=@('Cargo.toml','Cargo.lock','lock-origin.json','probe.hlsl','src/main.rs','build.ps1','baseline-source-origin.json','README.md')
$taskSourceEvidence=foreach($taskPath in $taskSources){
 $taskDestination=Join-Path $BuildDirectory ('source/'+$taskPath)
 [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($taskDestination))|Out-Null
 [IO.File]::Copy((Join-Path $PSScriptRoot $taskPath),$taskDestination,$false)
 [ordered]@{path=$taskPath;sha256=Get-ProbeHash (Join-Path $PSScriptRoot $taskPath)}
}
$taskOrigin=Get-Content -LiteralPath (Join-Path $PSScriptRoot 'lock-origin.json') -Raw|ConvertFrom-Json
$taskGlobalCargo=Join-Path $env:USERPROFILE '.cargo'
$taskLocalCargo=Join-Path $BuildDirectory 'cargo-home'
$taskRegistry='index.crates.io-1949cf8c6b5b557f'
$taskIndex='index.crates.io-1949cf8c6b5b557f'
$taskCacheCopies=foreach($taskPackage in $taskOrigin.packages){
 $taskName=$taskPackage.name;$taskVersion=$taskPackage.version
 $taskIndexPath=if($taskName.Length-eq1){'1/'+$taskName}elseif($taskName.Length-eq2){'2/'+$taskName}elseif($taskName.Length-eq3){'3/'+$taskName.Substring(0,1)+'/'+$taskName}else{$taskName.Substring(0,2)+'/'+$taskName.Substring(2,2)+'/'+$taskName}
 foreach($taskRelative in @("registry/src/$taskRegistry/$taskName-$taskVersion","registry/cache/$taskRegistry/$taskName-$taskVersion.crate","registry/index/$taskIndex/.cache/$taskIndexPath")){
  $taskFrom=Join-Path $taskGlobalCargo $taskRelative;$taskTo=Join-Path $taskLocalCargo $taskRelative
  if(-not(Test-Path -LiteralPath $taskFrom)){throw "Required exact cached dependency unavailable: $taskRelative"}
  [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($taskTo))|Out-Null
  Copy-Item -LiteralPath $taskFrom -Destination $taskTo -Recurse
 }
 $taskCrateHash=Get-ProbeHash (Join-Path $taskGlobalCargo "registry/cache/$taskRegistry/$taskName-$taskVersion.crate")
 if($taskCrateHash-ne$taskPackage.checksum){throw 'Cached archive differs from exact existing registry lock checksum'}
 [ordered]@{name=$taskName;version=$taskVersion;crateSHA256=$taskCrateHash;existingLockChecksum=$taskPackage.checksum}
}
$taskIndexConfig=Join-Path $taskLocalCargo "registry/index/$taskIndex/config.json"
[IO.File]::Copy((Join-Path $taskGlobalCargo "registry/index/$taskIndex/config.json"),$taskIndexConfig,$false)
[IO.File]::WriteAllText((Join-Path $taskLocalCargo 'config.toml'),"[net]`noffline = true`n[cache]`nauto-clean-frequency = 'never'`n",[Text.UTF8Encoding]::new($false))
$taskEvidence=[ordered]@{
 schemaVersion=1;stage='R17';status='building';sourceDirectory=$PSScriptRoot;sources=@($taskSourceEvidence)
 dependencyIdentity='Exact cached locked registry identities/checksums extracted; no new version selection, registry request or download'
 isolatedCargoHome=$taskLocalCargo;cachedCopies=@($taskCacheCopies);globalCargoCacheWritesByBuild=$false
 buildCommand='cargo build --offline --locked -j 2 --manifest-path reference/fixtures/native-blend-readback-r17/Cargo.toml'
 cargoVersion=(& cargo --version|Out-String).Trim();rustcVersion=(& rustc -Vv|Out-String).Trim();nativeRun='not-run-by-build-helper'
}
function Save-ProbeEvidence{[IO.File]::WriteAllText((Join-Path $BuildDirectory 'provenance.json'),(($taskEvidence|ConvertTo-Json -Depth 16)+"`n"),[Text.UTF8Encoding]::new($false))}
Save-ProbeEvidence
$taskOldCargo=$env:CARGO_HOME;$taskOldTarget=$env:CARGO_TARGET_DIR;$taskOldOffline=$env:CARGO_NET_OFFLINE
try{
 $env:CARGO_HOME=$taskLocalCargo;$env:CARGO_TARGET_DIR=Join-Path $BuildDirectory 'target';$env:CARGO_NET_OFFLINE='true'
 & cargo build --offline --locked -j 2 --manifest-path (Join-Path $PSScriptRoot 'Cargo.toml') 2>&1|Tee-Object -FilePath (Join-Path $BuildDirectory 'build.log')
 $taskEvidence.buildExitCode=$LASTEXITCODE
 if($LASTEXITCODE-ne0){throw 'No-window probe build failed; preserve diagnostics'}
 foreach($taskSource in $taskSourceEvidence){if((Get-ProbeHash (Join-Path $PSScriptRoot $taskSource.path))-ne$taskSource.sha256){throw 'Probe source changed during build'}}
 $taskBinary=Join-Path $BuildDirectory 'native-blend-readback.exe'
 [IO.File]::Copy((Join-Path $env:CARGO_TARGET_DIR 'debug/native-blend-readback.exe'),$taskBinary,$false)
 $taskEvidence.binaryPath=$taskBinary;$taskEvidence.binarySHA256=Get-ProbeHash $taskBinary;$taskEvidence.status='built';Save-ProbeEvidence
 Write-Output $taskBinary
}catch{$taskEvidence.status='failed';$taskEvidence.error=$_.Exception.Message;Save-ProbeEvidence;throw}finally{$env:CARGO_HOME=$taskOldCargo;$env:CARGO_TARGET_DIR=$taskOldTarget;$env:CARGO_NET_OFFLINE=$taskOldOffline}
