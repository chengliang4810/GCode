//! Native production footer fixture. Lossless transport is copied unchanged
//! from reference/fixtures/native-capture-lossless.rs, not a replacement UI.
use gpui::{AppContext, AsyncApp, Bounds, MouseButton, MouseDownEvent, MouseUpEvent, PlatformInput, WindowBounds, WindowOptions, point, px, size};
use std::{path::Path, path::PathBuf, time::Duration};
use zeron_ui::{*, footer_usage_fixture::FooterFixture};

include!("footer-wgc-transport.rs");

async fn pause(cx: &mut AsyncApp) {
    cx.background_executor().timer(Duration::from_millis(260)).await;
}

async fn capture(window: gpui::WindowHandle<FooterFixture>, cx: &mut AsyncApp, output: &Path, name: &str) -> anyhow::Result<()> {
    window.update(cx, |_, window, _| window.refresh())?;
    cx.background_executor().timer(Duration::from_millis(120)).await;
    let (hwnd, mut metadata) = window.update(cx, |view, window, cx| {
        let (hwnd, mut metadata) = lossless_transport::describe(window)?;
        metadata["appearance"] = format!("{:?}", appearance::mode(cx)).to_lowercase().into();
        metadata["surface"] = serde_json::to_value(settings::current(cx).surface)?;
        metadata["fixtureState"] = view.snapshot(window, cx);
        metadata["fixtureInstrumentation"] = serde_json::json!({
            "adapter": "new exported crate-internal feature-guarded footer_usage_fixture module; production UI sources untouched",
            "geometry": "nonpainting absolute canvas outside production AccountUsage", "visualParityAcceptance": "pending",
        });
        Ok::<_, anyhow::Error>((hwnd, metadata))
    })??;
    let path = output.join(name);
    let pixels = cx.background_executor().spawn(async move { lossless_transport::capture(hwnd, path) }).await;
    metadata["name"] = name.into();
    match pixels {
        Ok(pixels) => { metadata["status"] = "captured".into(); metadata["pixels"] = pixels; }
        Err(error) => {
            metadata["status"] = "pending".into(); metadata["error"] = format!("{error:#}").into();
            std::fs::write(output.join(format!("{name}.capture.json")), serde_json::to_vec_pretty(&metadata)?)?;
            return Err(error);
        }
    }
    std::fs::write(output.join(format!("{name}.capture.json")), serde_json::to_vec_pretty(&metadata)?)?;
    Ok(())
}

fn click(window: gpui::WindowHandle<FooterFixture>, cx: &mut AsyncApp, card: &str) -> anyhow::Result<()> {
    window.update(cx, |view, window, cx| {
        let at = view.click_position(card, cx)?;
        let position = point(px(at[0]), px(at[1]));
        window.simulate_mouse_move(position, cx);
        window.dispatch_event(PlatformInput::MouseDown(MouseDownEvent { button: MouseButton::Left, position, modifiers: Default::default(), click_count: 1, first_mouse: false }), cx);
        window.dispatch_event(PlatformInput::MouseUp(MouseUpEvent { button: MouseButton::Left, position, modifiers: Default::default(), click_count: 1 }), cx);
        // Keep open-state wash, but eliminate pointer-hover from every capture.
        window.simulate_mouse_move(point(px(10.0), px(10.0)), cx);
        view.record_pointer([10.0, 10.0]);
        Ok::<_, anyhow::Error>(())
    })??;
    Ok(())
}

fn main() -> anyhow::Result<()> {
    let runtime = tokio::runtime::Runtime::new()?;
    let _runtime_guard = runtime.enter();
    let output = PathBuf::from(std::env::args().nth(1).expect("new output directory"));
    anyhow::ensure!(!output.exists(), "footer capture requires a fresh output directory");
    std::fs::create_dir_all(&output)?;
    let temp = tempfile::tempdir()?;
    let data = temp.path().to_path_buf();
    gpui_platform::application().with_assets(icons::Assets).run(move |cx| {
        gpui_tokio::init(cx);
        gpui_base::init(cx);
        let mut prefs = settings::UiSettings::default();
        prefs.reduce_motion = motion::ReduceMotion::On;
        prefs.surface = zeron_theme::SurfacePreference::Opaque;
        settings::init(prefs.clone(), data.clone(), cx);
        let fonts = typography::register_fonts(cx);
        typography::init(prefs.ui_font_family.clone(), prefs.ui_font_size, prefs.terminal_font_family.clone(), prefs.terminal_font_size, prefs.code_font_family.clone(), prefs.code_font_size, fonts, cx);
        theme_library::init(data, cx);
        appearance::init(appearance::AppearanceMode::Dark, prefs.theme_selection, prefs.accent, prefs.surface, cx);
        motion::set_reduced_motion(cx, true);
        composer::init(cx, prefs.composer_send_behavior);
        let window = cx.open_window(WindowOptions {
            window_bounds: Some(WindowBounds::Windowed(Bounds::new(point(px(40.0), px(40.0)), size(px(840.0), px(560.0))))),
            ..Default::default()
        }, |window, cx| {
            window.set_window_title("");
            cx.new(FooterFixture::new)
        }).expect("native footer fixture window");
        cx.activate(true);
        cx.spawn(async move |cx| {
            let result: anyhow::Result<()> = async {
                pause(cx).await;
                for (mode, label) in [(appearance::AppearanceMode::Dark, "dark"), (appearance::AppearanceMode::Light, "light")] {
                    for scene in ["known", "capacity", "zero", "over", "warning", "danger", "unknown"] {
                        window.update(cx, |view, window, cx| {
                            appearance::set_mode(mode, cx); view.set_scene(scene, cx);
                            window.simulate_mouse_move(point(px(10.0), px(10.0)), cx); view.record_pointer([10.0, 10.0]);
                        })?;
                        pause(cx).await;
                        capture(window, cx, &output, &format!("footer-{scene}-{label}.png")).await?;
                        if scene != "unknown" {
                            click(window, cx, "context")?;
                            pause(cx).await;
                            capture(window, cx, &output, &format!("footer-{scene}-context-{label}.png")).await?;
                        }
                        if scene == "known" {
                            window.update(cx, |view, window, cx| {
                                view.set_scene(scene, cx);
                                window.simulate_mouse_move(point(px(10.0), px(10.0)), cx); view.record_pointer([10.0, 10.0]);
                            })?;
                            pause(cx).await;
                            click(window, cx, "accounts")?;
                            pause(cx).await;
                            capture(window, cx, &output, &format!("footer-{scene}-accounts-{label}.png")).await?;
                        }
                    }
                }
                std::fs::write(output.join("result.json"), serde_json::to_vec_pretty(&serde_json::json!({
                    "status": "footer-production-components-captured", "expectedScenes": 28,
                    "referenceCommit": "ab189159f10cf8e63c0502be75a59bbc487e0eaf", "productionComponentsModified": false, "productionRendererModified": false,
                    "nativeInteraction": "public GPUI mouse move/down/up dispatch to the actual production triggers", "engineBootstrapped": false,
                    "acceptance": "instrumented reference evidence only; pixel, timing and platform acceptance pending"
                }))?)?;
                Ok(())
            }.await;
            if let Err(error) = result {
                eprintln!("footer fixture pending/failed: {error:#}");
                let _ = std::fs::write(output.join("failure.txt"), format!("PENDING/FAILED: {error:#}\nNo parity acceptance.\n"));
            }
            cx.update(|cx| cx.quit());
        }).detach();
    });
    Ok(())
}
