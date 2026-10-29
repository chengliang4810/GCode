//! Independent native reference fixture. UI methods are frozen production
//! Shell methods; only the host/data/measurement/transport adapter is new.
use gpui::{AppContext, AsyncApp, Bounds, WindowBounds, WindowOptions, point, px, size};
use sha2::{Digest, Sha256};
use std::{cell::RefCell, path::{Path, PathBuf}, rc::Rc, time::Duration};
use zeron_ui::{*, shell::tabstrip_fixture::{Scene, StyleCase, TabstripFixture}};
use windows::Win32::{
    Foundation::HWND,
    UI::{Input::KeyboardAndMouse::GetActiveWindow, WindowsAndMessaging::{
        GetForegroundWindow, GetGUIThreadInfo, GetWindowThreadProcessId, GUITHREADINFO,
    }},
};

include!("tabstrip-wgc-transport.rs");
include!("tabstrip-style-native.rs");

const CONTRACT: &str = include_str!("tabstrip-contract.json");
const SOURCE_MANIFEST: &str = include_str!("tabstrip-source-manifest.json");
const STYLE_CONTRACT: &str = include_str!("tabstrip-style-contract.json");
const ACTIVITY_POLICY: &str = "UI-thread raw GetForegroundWindow==fixture HWND and foreground PID==fixture PID before/after WGC; owner GUI thread active HWND also equals fixture; initial asynchronous wait<=10s; no SetForeground/OS input by fixture; GPUI cached flag is separately named and is not OS foreground evidence";
const STYLE_ACTIVITY_POLICY: &str = "UI-thread raw GetForegroundWindow==fixture HWND and foreground PID==fixture PID before/after WGC; owner GUI thread active HWND also equals fixture; initial asynchronous wait<=10s; fixture never activates, moves OS cursor, clicks or injects keyboard; style-only own HWND PostMessageW WM_MOUSEMOVE is explicitly recorded; optional real OS cursor is root-owned; GPUI cached flag is separate";

// Queries only. Never activate a window, attach input queues, send an
// activation message, or change nonclient painting to manufacture parity.
fn native_activity(hwnd_value: usize, gpui_cached_active: bool) -> serde_json::Value {
    let own = HWND(hwnd_value as *mut std::ffi::c_void);
    let mut owner_pid = 0;
    let owner_tid = unsafe { GetWindowThreadProcessId(own, Some(&mut owner_pid)) };
    let foreground = unsafe { GetForegroundWindow() };
    let mut foreground_pid = 0;
    let foreground_tid = unsafe { GetWindowThreadProcessId(foreground, Some(&mut foreground_pid)) };
    let calling_tid = unsafe { windows_sys::Win32::System::Threading::GetCurrentThreadId() };
    let calling_active = unsafe { GetActiveWindow() };
    let mut info = GUITHREADINFO { cbSize: std::mem::size_of::<GUITHREADINFO>() as u32, ..Default::default() };
    let info_status = unsafe { GetGUIThreadInfo(owner_tid, &mut info) };
    let process_id = std::process::id();
    serde_json::json!({
        "processId": process_id, "hwnd": format!("0x{hwnd_value:x}"),
        "foregroundHWND": format!("0x{:x}", foreground.0 as usize), "foregroundProcessId": foreground_pid,
        "nativeWindowActive": foreground == own && foreground_pid == process_id && owner_pid == process_id,
        "nativeWindowActiveSource": "UI-thread GetForegroundWindow equality and owner PID; not GPUI cache",
        "ownerProcessId": owner_pid, "ownerThreadId": owner_tid, "foregroundThreadId": foreground_tid,
        "queryThreadId": calling_tid, "queriedOnOwnerUiThread": calling_tid == owner_tid,
        "callingThreadActiveHWND": format!("0x{:x}", calling_active.0 as usize),
        "ownerThreadActiveHWND": info_status.as_ref().ok().map(|_| format!("0x{:x}", info.hwndActive.0 as usize)),
        "ownerThreadActiveIsOwn": info_status.is_ok() && info.hwndActive == own,
        "ownerThreadInfoError": info_status.err().map(|error| format!("{error:#}")),
        "gpuiCachedWindowActive": gpui_cached_active,
        "sampledAtUnixMilliseconds": std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).ok().map(|time| time.as_millis()),
    })
}

fn activity_is_own(sample: &serde_json::Value) -> bool {
    sample["nativeWindowActive"].as_bool() == Some(true)
        && sample["hwnd"] == sample["foregroundHWND"]
        && sample["processId"] == sample["foregroundProcessId"]
        && sample["ownerThreadActiveIsOwn"].as_bool() == Some(true)
        && sample["queriedOnOwnerUiThread"].as_bool() == Some(true)
}

async fn wait_for_foreground(window: gpui::WindowHandle<TabstripFixture>, cx: &mut AsyncApp, output: &Path, activity_policy:&str) -> anyhow::Result<serde_json::Value> {
    let start = std::time::Instant::now();
    let mut observations = Vec::new();
    loop {
        let sample = window.update(cx, |_, window, _| {
            let (hwnd, _) = lossless_transport::describe(window)?;
            Ok::<_, anyhow::Error>(native_activity(hwnd, window.is_window_active()))
        })??;
        if observations.is_empty() {
            std::fs::write(output.join("activation-wait.started.json"), serde_json::to_vec_pretty(&serde_json::json!({
                "status": "waiting-for-root-owned-activation", "timeoutMilliseconds":10000, "initialNativeActivity":sample,
                "policy":activity_policy, "fixtureCallsSetForeground":false,
            }))?)?;
            eprintln!("TabStrip waiting <=10s for root activation: PID {}, HWND {}", std::process::id(), sample["hwnd"]);
        }
        let ready = activity_is_own(&sample);
        observations.push(serde_json::json!({"elapsedMilliseconds":start.elapsed().as_millis(),"sample":sample}));
        if ready || start.elapsed() >= Duration::from_secs(10) {
            let result = serde_json::json!({
                "status":if ready {"own-foreground-and-owner-active"} else {"timeout"},
                "policy":activity_policy,"timeoutMilliseconds":10000,"elapsedMilliseconds":start.elapsed().as_millis(),"observations":observations,
            });
            std::fs::write(output.join("activation-wait.result.json"), serde_json::to_vec_pretty(&result)?)?;
            anyhow::ensure!(ready, "own foreground/owner-active wait timed out; root activation required, no fixture activation attempted");
            return Ok(result);
        }
        cx.background_executor().timer(Duration::from_millis(100)).await;
    }
}

fn arguments() -> anyhow::Result<(PathBuf, f32, StyleCase, bool, Duration)> {
    let mut args = std::env::args().skip(1);
    let mut output = None;
    let mut scale = None;
    let mut style_case = StyleCase::Baseline;
    let mut require_cursor = false;
    let mut hover_dwell_ms = 0u64;
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--out-dir" => output = Some(PathBuf::from(args.next().ok_or_else(|| anyhow::anyhow!("--out-dir requires a new directory"))?)),
            "--expected-scale" => scale = Some(args.next().ok_or_else(|| anyhow::anyhow!("--expected-scale requires 1 or 2"))?.parse::<f32>()?),
            "--style-case" => style_case = StyleCase::parse(&args.next().ok_or_else(|| anyhow::anyhow!("--style-case requires baseline|inactive-hover|dirty-file|file-hover|file-hover-edge"))?)?,
            "--require-native-cursor" => require_cursor = true,
            "--hover-dwell-ms" => hover_dwell_ms = args.next().ok_or_else(|| anyhow::anyhow!("--hover-dwell-ms requires an integer"))?.parse()?,
            _ => anyhow::bail!("unknown argument {arg}"),
        }
    }
    let scale = scale.ok_or_else(|| anyhow::anyhow!("--expected-scale is required; actual DPI cannot be assumed"))?;
    anyhow::ensure!(scale == 1.0 || scale == 2.0, "expected scale must be 1 or 2");
    let output = output.ok_or_else(|| anyhow::anyhow!("--out-dir is required"))?;
    anyhow::ensure!(!output.exists(), "capture output must not already exist");
    anyhow::ensure!(!require_cursor || style_case != StyleCase::Baseline,"native cursor gate is style-only; baseline36 input stays unchanged");
    anyhow::ensure!(hover_dwell_ms <= 10_000 && (hover_dwell_ms == 0 || style_case.is_hover()),"hover dwell must be at most10000ms and requires a hover style case");
    Ok((output, scale, style_case, require_cursor, Duration::from_millis(hover_dwell_ms)))
}

fn scenes(contract: &serde_json::Value) -> anyhow::Result<Vec<Scene>> {
    let mut result = Vec::new();
    for appearance in contract["appearances"].as_array().ok_or_else(|| anyhow::anyhow!("contract appearance array"))? {
        for width in contract["hostWidthsDIP"].as_array().ok_or_else(|| anyhow::anyhow!("contract width array"))? {
            for count in contract["counts"].as_array().ok_or_else(|| anyhow::anyhow!("contract count array"))? {
                for fraction in contract["scrollFractions"].as_array().ok_or_else(|| anyhow::anyhow!("contract scroll array"))? {
                    let scene: Scene = serde_json::from_value(serde_json::json!({"appearance": appearance, "hostWidth": width, "count": count, "scrollFraction": fraction}))?;
                    scene.validate()?;
                    result.push(scene);
                }
            }
        }
    }
    anyhow::ensure!(result.len() == 36 && contract["expectedScenes"] == 36, "contract scene count changed");
    Ok(result)
}

// Two equal samples must come from distinct completed prepaint/layout passes,
// observed by a nonpainting canvas. Wall-clock sleep alone never proves ready.
async fn settle(window: gpui::WindowHandle<TabstripFixture>, cx: &mut AsyncApp, phase: &str) -> anyhow::Result<serde_json::Value> {
    let initial_frame = window.update(cx, |view, _, _| view.layout_frame())?;
    let mut last_frame = initial_frame;
    let mut previous = None;
    let mut stable = 0;
    let mut observations = Vec::new();
    for attempt in 0..40 {
        window.update(cx, |_, window, _| window.refresh())?;
        cx.background_executor().timer(Duration::from_millis(70)).await;
        let (frame, sample) = window.update(cx, |view, _, cx| (view.layout_frame(), view.stability_sample(cx)))?;
        if frame <= last_frame { continue; }
        stable = if previous.as_ref() == Some(&sample) { stable + 1 } else { 1 };
        observations.push(serde_json::json!({"attempt": attempt, "layoutFrame": frame, "sameAsPrevious": previous.as_ref() == Some(&sample)}));
        previous = Some(sample);
        last_frame = frame;
        if stable >= 2 {
            return Ok(serde_json::json!({"phase": phase, "initialLayoutFrame": initial_frame, "finalLayoutFrame": frame, "consecutiveStableDistinctFrames": stable, "observations": observations, "sample": previous}));
        }
    }
    anyhow::bail!("{phase} did not reach two distinct stable layout frames: {observations:?}")
}

async fn capture(window: gpui::WindowHandle<TabstripFixture>, cx: &mut AsyncApp, output: &Path, expected_scale: f32, source: &serde_json::Value, binary_sha: &str, proofs: serde_json::Value, style_input:Option<style_input::Expectation>) -> anyhow::Result<()> {
    window.update(cx, |_, window, _| window.refresh())?;
    cx.background_executor().timer(Duration::from_millis(120)).await;
    let (hwnd, mut metadata, name) = window.update(cx, |view, window, cx| {
        let viewport = window.viewport_size();
        anyhow::ensure!((window.scale_factor() - expected_scale).abs() < 0.001, "actual scale {} differs from required {expected_scale}", window.scale_factor());
        anyhow::ensure!((f32::from(viewport.width) - 840.0).abs() < 0.05 && (f32::from(viewport.height) - 560.0).abs() < 0.05, "actual logical client viewport mismatch");
        anyhow::ensure!((f32::from(window.rem_size()) - 16.0).abs() < 0.05, "actual source UI rem must remain16");
        let (hwnd, mut metadata) = lossless_transport::describe(window)?;
        let expected_px = [840.0 * expected_scale, 560.0 * expected_scale];
        let client = metadata["clientSizePx"].as_array().ok_or_else(|| anyhow::anyhow!("missing actual client metadata"))?;
        anyhow::ensure!(client.len() == 2 && (client[0].as_f64().unwrap_or(-1.0) - f64::from(expected_px[0])).abs() < 0.5 && (client[1].as_f64().unwrap_or(-1.0) - f64::from(expected_px[1])).abs() < 0.5, "native client pixels differ from declared DIP/actual scale");
        anyhow::ensure!(view.stability_sample(cx) == proofs["scroll"]["sample"], "layout changed after stable-scroll proof");
        let mut state = view.snapshot(window, cx)?;
        state["geometryStable"] = true.into();
        state["stableFrames"] = proofs["scroll"]["consecutiveStableDistinctFrames"].clone();
        let host = state["geometryLogical"]["outerHost"].as_array().ok_or_else(|| anyhow::anyhow!("outer host has no observed layout"))?;
        let requested = state["scaffold"]["hostRequestedDIP"].as_array().unwrap();
        anyhow::ensure!(host.len() == 4 && host.iter().zip(requested).all(|(a,b)| (a.as_f64().unwrap_or(-1.0)-b.as_f64().unwrap_or(-2.0)).abs() < 0.05), "actual outer host differs from contract");
        let scene: Scene = serde_json::from_value(state["scene"].clone())?;
        metadata["fixtureState"] = state;
        metadata["fixtureState"]["gpuiCachedWindowActive"] = metadata["fixtureState"]["nativeWindowActive"].clone();
        metadata["fixtureState"]["nativeWindowActiveSource"] = "GPUI Window::is_window_active cached Cell; not raw Win32 foreground or nonclient paint state".into();
        metadata["nativeActivityBefore"] = native_activity(hwnd, window.is_window_active());
        metadata["rawNativeActivityPolicy"] = if style_input.is_some(){STYLE_ACTIVITY_POLICY}else{ACTIVITY_POLICY}.into();
        if let Some(expect) = style_input {
            metadata["styleInputBefore"] = style_input::sample(expect,window)?;
            metadata["styleContract"] = serde_json::from_str(STYLE_CONTRACT)?;
        }
        metadata["appearance"] = scene.appearance.clone().into();
        metadata["surface"] = serde_json::to_value(settings::current(cx).surface)?;
        metadata["expectedScale"] = expected_scale.into();
        metadata["contract"] = serde_json::from_str(CONTRACT)?;
        metadata["sourceManifest"] = source.clone();
        metadata["binarySHA256"] = binary_sha.into();
        metadata["settleProofs"] = proofs;
        metadata["fixtureInstrumentation"] = serde_json::json!({
            "adapter": "feature-guarded Shell child module plus independent example; production Shell file has one append-only module declaration",
            "productionMethodBodiesChanged": false, "geometry": "public actual ScrollHandle and nonpainting outer wrapper measurements; no Inspector", "visualParityAcceptance": "pending",
        });
        Ok::<_, anyhow::Error>((hwnd, metadata, view.capture_name()))
    })??;
    metadata["name"] = name.clone().into();
    if !activity_is_own(&metadata["nativeActivityBefore"]) || (style_input.is_some() && !style_input::passed(&metadata["styleInputBefore"])) {
        metadata["nativeActivityAfter"] = window.update(cx, |_, window, _| native_activity(hwnd, window.is_window_active()))?;
        metadata["status"] = if style_input.is_some(){"failed-native-activity-or-style-input-before"}else{"failed-native-activity-before"}.into();
        metadata["captureStarted"] = false.into();
        std::fs::write(output.join(format!("{name}.capture.json")), serde_json::to_vec_pretty(&metadata)?)?;
        anyhow::bail!("raw native activity before WGC is not own foreground/owner-active; scene {name}");
    }
    metadata["captureStarted"] = true.into();
    let path = output.join(&name);
    let pixels = cx.background_executor().spawn(async move { lossless_transport::capture(hwnd, path) }).await;
    metadata["nativeActivityAfter"] = window.update(cx, |_, window, _| native_activity(hwnd, window.is_window_active()))?;
    if let Some(expect) = style_input { metadata["styleInputAfter"] = window.update(cx,|_,window,_|style_input::sample(expect,window))??; }
    let activity_after_ok = activity_is_own(&metadata["nativeActivityAfter"]);
    match pixels {
        Ok(pixels) => { metadata["status"] = "captured".into(); metadata["pixels"] = pixels; }
        Err(error) => {
            metadata["status"] = "failed-or-pending".into(); metadata["error"] = format!("{error:#}").into();
            std::fs::write(output.join(format!("{name}.capture.json")), serde_json::to_vec_pretty(&metadata)?)?;
            return Err(error);
        }
    }
    if !activity_after_ok || (style_input.is_some() && !style_input::passed(&metadata["styleInputAfter"])) {
        metadata["status"] = if style_input.is_some(){"failed-native-activity-or-style-input-after"}else{"failed-native-activity-after"}.into();
        metadata["error"] = "own foreground/owner-active gate failed after WGC; preserve raw PNG as failed evidence".into();
        std::fs::write(output.join(format!("{name}.capture.json")), serde_json::to_vec_pretty(&metadata)?)?;
        anyhow::bail!("raw native activity after WGC is not own foreground/owner-active; scene {name}");
    }
    std::fs::write(output.join(format!("{name}.capture.json")), serde_json::to_vec_pretty(&metadata)?)?;
    Ok(())
}

fn main() -> anyhow::Result<()> {
    let (output, expected_scale, style_case, require_cursor, hover_dwell) = arguments()?;
    let contract: serde_json::Value = serde_json::from_str(CONTRACT)?;
    let source: serde_json::Value = serde_json::from_str(SOURCE_MANIFEST)?;
    let cases = if style_case == StyleCase::Baseline { scenes(&contract)? } else {
        vec![Scene{appearance:"dark".into(),host_width:400.0,count:2,scroll_fraction:0.0},Scene{appearance:"light".into(),host_width:400.0,count:2,scroll_fraction:0.0}]
    };
    let binary_sha = format!("{:x}", Sha256::digest(std::fs::read(std::env::current_exe()?)?));
    std::fs::create_dir_all(&output)?;
    let runtime = tokio::runtime::Runtime::new()?;
    let _runtime_guard = runtime.enter();
    let temp = tempfile::tempdir()?;
    let data = temp.path().to_path_buf();
    let outcome: Rc<RefCell<Option<Result<(), String>>>> = Rc::default();
    let app_outcome = outcome.clone();
    gpui_platform::application().with_assets(icons::Assets).run(move |cx| {
        gpui_tokio::init(cx);
        gpui_base::init(cx);
        app_menus::init(cx);
        let mut prefs = settings::UiSettings::default();
        prefs.reduce_motion = motion::ReduceMotion::On;
        prefs.surface = zeron_theme::SurfacePreference::Opaque;
        settings::init(prefs.clone(), data.clone(), cx);
        history::init(Default::default(), Default::default(), Default::default(), Default::default(), cx);
        let fonts = typography::register_fonts(cx);
        typography::init(prefs.ui_font_family.clone(), prefs.ui_font_size, prefs.terminal_font_family.clone(), prefs.terminal_font_size, prefs.code_font_family.clone(), prefs.code_font_size, fonts, cx);
        theme_library::init(data.clone(), cx);
        appearance::init(appearance::AppearanceMode::Dark, prefs.theme_selection, prefs.accent, prefs.surface, cx);
        motion::set_reduced_motion(cx, true);
        composer::init(cx, prefs.composer_send_behavior);
        let first = Scene { appearance: "dark".into(), host_width: 400.0, count: 0, scroll_fraction: 0.0 };
        let window = cx.open_window(WindowOptions {
            window_bounds: Some(WindowBounds::Windowed(Bounds::new(point(px(40.0), px(40.0)), size(px(840.0), px(560.0))))),
            ..Default::default()
        }, |window, cx| {
            window.set_window_title("");
            window.set_rem_size(px(16.0));
            cx.new(|cx| TabstripFixture::new(data, first, cx))
        }).expect("isolated production TabStrip fixture window");
        cx.activate(true);
        cx.spawn(async move |cx| {
            let result: anyhow::Result<()> = async {
                let activity_policy=if style_case==StyleCase::Baseline{ACTIVITY_POLICY}else{STYLE_ACTIVITY_POLICY};
                let activation_wait = wait_for_foreground(window, cx, &output,activity_policy).await?;
                let _style_hook = if style_case == StyleCase::Baseline {None} else {Some(window.update(cx,|_,window,_| {
                    let (hwnd,_) = lossless_transport::describe(window)?;
                    style_input::QueueHook::install(hwnd)
                })??)};
                for scene in cases {
                    window.update(cx, |view, window, cx| {
                        appearance::set_mode(if scene.appearance == "dark" { appearance::AppearanceMode::Dark } else { appearance::AppearanceMode::Light }, cx);
                        view.reset_scene(scene, cx)?;
                        view.set_style_case(style_case);
                        if style_case == StyleCase::Baseline { window.simulate_mouse_move(point(px(10.0), px(10.0)), cx); }
                        Ok::<_, anyhow::Error>(())
                    })??;
                    // Let constructor/global notifications finish before the
                    // engine=None parent's SideChat collection is populated.
                    let constructor = settle(window, cx, "constructor-before-seed").await?;
                    window.update(cx, |view, window, cx| if style_case.is_file() {view.seed_file_tabs(window,cx)} else {view.seed_tabs(cx)})??;
                    let mut file_seed = None;
                    if style_case.is_file() {
                        file_seed = Some(settle(window,cx,"clean-file-editors-subscriptions-installed").await?);
                        window.update(cx,|view,window,cx|view.edit_first_file(window,cx))??;
                    }
                    let selection = settle(window, cx, "production-seed-and-active-zero").await?;
                    window.update(cx, |view, _, cx| view.set_scroll_fraction(cx))??;
                    let mut scroll = settle(window, cx, "requested-scroll-fraction").await?;
                    let mut proofs=serde_json::json!({"constructor":constructor,"selection":selection,"scroll":scroll});
                    if let Some(file_seed)=file_seed {proofs["cleanFileSeed"]=file_seed;}
                    let input=if style_case==StyleCase::Baseline {None} else {
                        let (expect,evidence)=style_input::prepare(window,cx,&output,expected_scale,require_cursor).await?;
                        scroll=settle(window,cx,"native-style-input-dispatched").await?;
                        proofs["scroll"]=scroll;
                        proofs["styleInput"]=evidence;
                        if !hover_dwell.is_zero() {
                            let geometry=proofs["scroll"]["sample"].clone();
                            proofs["hoverDwell"]=style_input::dwell(window,cx,expect,hover_dwell,geometry,&output).await?;
                            // The tooltip may add paint/layout work. Require a
                            // fresh stable underlying production strip, never
                            // suppress or alter the actual tooltip renderer.
                            proofs["scroll"]=settle(window,cx,"after-measured-hover-dwell").await?;
                        }
                        Some(expect)
                    };
                    capture(window, cx, &output, expected_scale, &source, &binary_sha, proofs,input).await?;
                }
                let mut result=serde_json::json!({
                    "status": "36-production-tabstrip-scenes-captured", "expectedScenes": 36, "referenceCommit": "ab189159f10cf8e63c0502be75a59bbc487e0eaf",
                    "binarySHA256": binary_sha, "sourceManifest": source, "contract": contract, "engineBootstrapped": false,
                    "productionMethodBodiesChanged": false, "productionShellModuleDeclarationAppended": true,
                    "rawNativeActivityPolicy":activity_policy, "initialActivationWait":activation_wait,
                    "acceptance": "reference instrumentation only; strict independent pairing and visual/native interaction acceptance pending",
                });
                if style_case!=StyleCase::Baseline {
                    result["status"]="2-production-tabstrip-style-scenes-captured".into();result["expectedScenes"]=2.into();
                    result["styleCase"]=style_case.name().into();result["styleContract"]=serde_json::from_str(STYLE_CONTRACT)?;
                    result["requireNativeCursor"]=require_cursor.into();
                    if !hover_dwell.is_zero() { result["requestedHoverDwellNanoseconds"]=(hover_dwell.as_nanos() as u64).into(); }
                }
                std::fs::write(output.join("result.json"),serde_json::to_vec_pretty(&result)?)?;
                Ok(())
            }.await;
            if let Err(error) = &result {
                eprintln!("TabStrip fixture failed/pending: {error:#}");
                let _ = std::fs::write(output.join("failure.json"), serde_json::to_vec_pretty(&serde_json::json!({"status":"failed-or-pending","error":format!("{error:#}"),"processId":std::process::id(),"visualParityAcceptance":false})).unwrap());
            }
            *app_outcome.borrow_mut() = Some(result.map_err(|error| format!("{error:#}")));
            // Normal focus teardown, preserving actual process exit separately.
            let _ = window.update(cx, |_, window, _| { window.disable_focus(); window.refresh(); });
            cx.background_executor().timer(Duration::from_millis(140)).await;
            cx.update(|cx| cx.quit());
        }).detach();
    });
    match outcome.borrow_mut().take() {
        Some(Ok(())) => Ok(()),
        Some(Err(error)) => anyhow::bail!(error),
        None => anyhow::bail!("fixture exited before capture result; no acceptance"),
    }
}
