[CmdletBinding()]
param([string]$SourceRoot='D:/zeron')
$ErrorActionPreference='Stop'
$taskRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
& (Join-Path $PSScriptRoot 'reference-verify.ps1') -SourceRoot $SourceRoot
$taskReference=Join-Path $taskRoot 'output/reference-native-0.2.128'
if (-not (Test-Path -LiteralPath (Join-Path $taskReference '.source-commit'))) { throw 'Build the frozen reference extraction first.' }
$taskOriginal=Join-Path $SourceRoot 'crates/ui/examples/compact-picker-fixture.rs'
$taskAdapter=Join-Path $taskReference 'crates/ui/examples/compact-picker-windows-parity.rs'
$taskText=[IO.File]::ReadAllText($taskOriginal)
$taskStart=$taskText.IndexOf('fn capture(')
$taskEnd=$taskText.IndexOf('fn press(',$taskStart)
if ($taskStart -lt 0 -or $taskEnd -le $taskStart) { throw 'Reference fixture capture function changed.' }
# Only the example's capture transport is adapted. Production views, theme,
# fonts, catalog fixture, state changes and keyboard assertions remain original.
$taskCapture=@'
async fn capture(
    window: gpui::AnyWindowHandle,
    cx: &mut AsyncApp,
    output: &Path,
    name: &str,
) -> anyhow::Result<()> {
    let metadata = window.update(cx, |_, window, cx| {
        window.draw(cx).clear();
        let bounds = window.bounds();
        let viewport = window.viewport_size();
        serde_json::json!({
            "schemaVersion": 1,
            "name": name,
            "referenceCommit": "ab189159f10cf8e63c0502be75a59bbc487e0eaf",
            "renderer": "original GPUI Windows native window",
            "scale": window.scale_factor(),
            "clientOrigin": [f32::from(bounds.origin.x), f32::from(bounds.origin.y)],
            "clientSize": [f32::from(viewport.width), f32::from(viewport.height)],
            "surface": std::env::var("ZERON_PARITY_SURFACE").unwrap_or_else(|_| "theme".into()),
            "reducedMotion": true,
        })
    })?;
    let ready = output.join(format!("{name}.ready.json"));
    let acknowledgment = output.join(format!("{name}.captured"));
    std::fs::write(ready, serde_json::to_vec_pretty(&metadata)?)?;
    for _ in 0..3600 {
        if acknowledgment.is_file() { return Ok(()); }
        cx.background_executor().timer(Duration::from_millis(50)).await;
    }
    anyhow::bail!("native screenshot was not acknowledged: {name}")
}

'@
$taskText=$taskText.Substring(0,$taskStart)+$taskCapture+$taskText.Substring($taskEnd)
$taskText=[regex]::Replace($taskText,'(?ms)^(\s+)capture\((.*?)\)\?;','$1capture($2).await?;')
$taskMarker='let mut prefs = settings::UiSettings::default();'
if (-not $taskText.Contains($taskMarker)) { throw 'Reference settings setup changed.' }
$taskText=$taskText.Replace($taskMarker,$taskMarker+@'

            prefs.reduce_motion = motion::ReduceMotion::On;
            if std::env::var("ZERON_PARITY_SURFACE").as_deref() == Ok("opaque") {
                prefs.surface = zeron_theme::SurfacePreference::Opaque;
            }
'@)
$taskText=$taskText.Replace('composer::init(cx, prefs.composer_send_behavior);','motion::set_reduced_motion(cx, true); composer::init(cx, prefs.composer_send_behavior);')
[IO.File]::WriteAllText($taskAdapter,$taskText,[Text.UTF8Encoding]::new($false))
$taskEvidence=@{schemaVersion=1;referenceCommit='ab189159f10cf8e63c0502be75a59bbc487e0eaf';originalExample='crates/ui/examples/compact-picker-fixture.rs';originalSha256=(Get-FileHash -LiteralPath $taskOriginal -Algorithm SHA256).Hash.ToLowerInvariant();adapter='output/reference-native-0.2.128/crates/ui/examples/compact-picker-windows-parity.rs';adapterSha256=(Get-FileHash -LiteralPath $taskAdapter -Algorithm SHA256).Hash.ToLowerInvariant();modifications=@('Replace unsupported Windows render_to_image export with async scene-ready/acknowledgment transport for Computer Use window capture','Optional opaque surface and reduced motion selected as test configuration');productionRendererModified=$false;productionComponentsModified=$false}
$taskEvidenceJson = ($taskEvidence | ConvertTo-Json -Depth 5).Replace("`r`n", "`n") + "`n"
[IO.File]::WriteAllText((Join-Path $taskRoot 'reference/native/capture-adapter.json'), $taskEvidenceJson, [Text.UTF8Encoding]::new($false))
$taskPreviousTarget=$env:CARGO_TARGET_DIR
try {
    $env:CARGO_TARGET_DIR=Join-Path $taskRoot 'output/reference-native/target'
    & cargo build --locked -j 2 --manifest-path (Join-Path $taskReference 'Cargo.toml') -p zeron-ui --example compact-picker-windows-parity --features compact-picker-fixture
    if ($LASTEXITCODE -ne 0) { throw 'Reference capture adapter build failed.' }
} finally { $env:CARGO_TARGET_DIR=$taskPreviousTarget }
Write-Output (Join-Path $taskRoot 'output/reference-native/target/debug/examples/compact-picker-windows-parity.exe')
