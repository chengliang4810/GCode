// Included only in the exported compact-picker-windows-lossless example.
// The production renderer and component sources are not changed.
mod lossless_transport {
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};
    use sha2::{Digest, Sha256};
    use std::{path::PathBuf, sync::mpsc, time::Duration};
    use windows::Win32::{
        Foundation::{HWND, POINT, RECT},
        Graphics::Gdi::ClientToScreen,
        UI::WindowsAndMessaging::{GetClientRect, GetWindowRect, GetWindowThreadProcessId},
    };
    use windows_capture::{
        capture::{Context, GraphicsCaptureApiHandler},
        frame::{Frame, ImageFormat},
        graphics_capture_api::InternalCaptureControl,
        settings::{
            ColorFormat, CursorCaptureSettings, DirtyRegionSettings, DrawBorderSettings,
            MinimumUpdateIntervalSettings, SecondaryWindowSettings, Settings,
        },
        window::Window as CaptureWindow,
    };

    pub fn describe(window: &gpui::Window) -> anyhow::Result<(usize, serde_json::Value)> {
        let raw = HasWindowHandle::window_handle(window)?.as_raw();
        let RawWindowHandle::Win32(raw) = raw else {
            anyhow::bail!("The lossless adapter supports only its own native Windows HWND");
        };
        let hwnd = HWND(raw.hwnd.get() as *mut std::ffi::c_void);
        let mut owner = 0;
        let mut frame = RECT::default();
        let mut client = RECT::default();
        let mut client_origin = POINT::default();
        unsafe {
            GetWindowThreadProcessId(hwnd, Some(&mut owner));
            GetWindowRect(hwnd, &mut frame)?;
            GetClientRect(hwnd, &mut client)?;
            // Geometry only: no GDI image capture or image transformation.
            anyhow::ensure!(
                ClientToScreen(hwnd, &mut client_origin).as_bool(),
                "ClientToScreen failed"
            );
        }
        anyhow::ensure!(
            owner == std::process::id(),
            "Refusing to capture a window outside this fixture process"
        );
        let bounds = window.bounds();
        let viewport = window.viewport_size();
        Ok((
            raw.hwnd.get() as usize,
            serde_json::json!({
                "schemaVersion": 1,
                "referenceCommit": "ab189159f10cf8e63c0502be75a59bbc487e0eaf",
                "renderer": "unmodified frozen GPUI Windows renderer",
                "transport": "in-process windows-capture 1.5.0 WGC Frame::save_as_image(Png)",
                "processId": owner,
                "hwnd": format!("0x{:x}", raw.hwnd.get()),
                "windowRectScreenPx": [frame.left, frame.top, frame.right, frame.bottom],
                "windowSizePx": [frame.right - frame.left, frame.bottom - frame.top],
                "clientRectPx": [client.left, client.top, client.right, client.bottom],
                "clientOriginScreenPx": [client_origin.x, client_origin.y],
                "clientSizePx": [client.right - client.left, client.bottom - client.top],
                "gpuiBoundsLogical": [f32::from(bounds.origin.x), f32::from(bounds.origin.y), f32::from(bounds.size.width), f32::from(bounds.size.height)],
                "gpuiViewportLogical": [f32::from(viewport.width), f32::from(viewport.height)],
                "scale": window.scale_factor(),
                "surface": std::env::var("ZERON_PARITY_SURFACE").unwrap_or_else(|_| "theme".into()),
                "reducedMotion": true,
                "captureTarget": "exact fixture HWND; no window enumeration",
                "cursor": "WithoutCursor",
                "captureBorder": "system Default; no privacy override",
                "secondaryWindows": "system Default",
                "imageTransformations": [],
                "cropped": false,
                "resized": false,
            "captureBoundary": "complete HWND WGC frame including native/nonclient extent; not a cropped client screenshot",
            "preCaptureFrameSettleMilliseconds": 120,
                "keyboardAcceptance": "not established by capture transport",
            }),
        ))
    }

    struct CaptureFlags {
        output: PathBuf,
        result: mpsc::Sender<Result<serde_json::Value, String>>,
    }

    struct CaptureHandler {
        flags: CaptureFlags,
        completed: bool,
    }

    impl GraphicsCaptureApiHandler for CaptureHandler {
        type Flags = CaptureFlags;
        type Error = Box<dyn std::error::Error + Send + Sync>;

        fn new(context: Context<Self::Flags>) -> Result<Self, Self::Error> {
            Ok(Self {
                flags: context.flags,
                completed: false,
            })
        }

        fn on_frame_arrived(
            &mut self,
            frame: &mut Frame,
            control: InternalCaptureControl,
        ) -> Result<(), Self::Error> {
            if self.completed {
                control.stop();
                return Ok(());
            }
            self.completed = true;
            let result = (|| -> anyhow::Result<serde_json::Value> {
                let (width, height) = (frame.width(), frame.height());
                anyhow::ensure!(width > 0 && height > 0 && width <= 16384 && height <= 16384, "Unexpected WGC frame dimensions");
                anyhow::ensure!(!self.flags.output.exists(), "Refusing to overwrite a prior capture");
                // Save the complete original WGC frame. Do not crop, resize,
                // re-encode a JPEG, or hide a mismatch in window/client bounds.
                frame.save_as_image(&self.flags.output, ImageFormat::Png)?;
                let bytes = std::fs::read(&self.flags.output)?;
                anyhow::ensure!(bytes.starts_with(b"\x89PNG\r\n\x1a\n"), "Capture is not a PNG");
                let dimensions = image::image_dimensions(&self.flags.output)?;
                anyhow::ensure!(dimensions == (width, height), "PNG dimensions differ from the unmodified WGC frame");
                Ok(serde_json::json!({
                    "pixelFormat": "BGRA8",
                    "frameSizePx": [width, height],
                    "pngSizePx": [dimensions.0, dimensions.1],
                    "pngBytes": bytes.len(),
                    "pngSHA256": format!("{:x}", Sha256::digest(&bytes)),
                    "capturedAtUnixMilliseconds": std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH)?.as_millis(),
                    "encoder": "Windows BitmapEncoder::PngEncoderId; no JPEG intermediate",
                }))
            })().map_err(|error| format!("{error:#}"));
            let _ = self.flags.result.send(result);
            control.stop();
            Ok(())
        }
    }

    pub fn capture(hwnd: usize, output: PathBuf) -> anyhow::Result<serde_json::Value> {
        let hwnd = HWND(hwnd as *mut std::ffi::c_void);
        let mut owner = 0;
        unsafe {
            GetWindowThreadProcessId(hwnd, Some(&mut owner));
        }
        anyhow::ensure!(
            owner == std::process::id(),
            "Fixture HWND owner changed before capture"
        );
        let (sender, receiver) = mpsc::channel();
        let settings = Settings::new(
            CaptureWindow::from_raw_hwnd(hwnd.0),
            CursorCaptureSettings::WithoutCursor,
            DrawBorderSettings::Default,
            SecondaryWindowSettings::Default,
            MinimumUpdateIntervalSettings::Default,
            DirtyRegionSettings::Default,
            ColorFormat::Bgra8,
            CaptureFlags {
                output,
                result: sender,
            },
        );
        // The library owns its MTA/dispatcher thread. No picker, access request,
        // border suppression or system-setting change is attempted here.
        let control = CaptureHandler::start_free_threaded(settings).map_err(|error| {
            anyhow::anyhow!(
                "WGC unavailable; leave capture pending without changing privacy settings: {error}"
            )
        })?;
        let result = receiver.recv_timeout(Duration::from_secs(15));
        let stopped = control
            .stop()
            .map_err(|error| anyhow::anyhow!("WGC shutdown failed: {error}"));
        let pixels = result
            .map_err(|error| {
                anyhow::anyhow!("No lossless fixture frame; capture remains pending: {error}")
            })?
            .map_err(anyhow::Error::msg)?;
        stopped?;
        Ok(pixels)
    }
}

async fn capture(
    window: gpui::AnyWindowHandle,
    cx: &mut AsyncApp,
    output: &Path,
    name: &str,
) -> anyhow::Result<()> {
    window.update(cx, |_, window, _| {
        window.refresh();
    })?;
    // Use the normal platform frame/presentation path. The optional GPUI
    // bench-only present_if_needed API is not enabled for this fixture.
    cx.background_executor()
        .timer(Duration::from_millis(120))
        .await;
    let fixture_window = window
        .downcast::<Fixture>()
        .ok_or_else(|| anyhow::anyhow!("Capture target is not the compact-picker fixture"))?;
    let (hwnd, mut metadata) = fixture_window.update(cx, |view, window, cx| {
        let state = view.pickers.read(cx).fixture_compact_state(cx);
        let (hwnd, mut metadata) = lossless_transport::describe(window)?;
        metadata["appearance"] = format!("{:?}", appearance::mode(cx)).to_lowercase().into();
        metadata["fixtureState"] = serde_json::json!({
            "catalog": "pickers.rs::fixture_compact_catalog",
            "modelList": state.model_list,
            "reasoning": state.reasoning.map(|level| format!("{level:?}").to_lowercase()),
            "explicitReasoning": state.explicit_reasoning.map(|level| format!("{level:?}").to_lowercase()),
            "fast": state.fast,
            "focusedModel": state.focused_model,
            "focusedModelFavorite": state.focused_model_favorite,
        });
        Ok::<_, anyhow::Error>((hwnd, metadata))
    })??;
    let png = output.join(name);
    let pixels = cx
        .background_executor()
        .spawn(async move { lossless_transport::capture(hwnd, png) })
        .await;
    metadata["name"] = name.into();
    match pixels {
        Ok(pixels) => {
            metadata["status"] = "captured".into();
            metadata["pixels"] = pixels;
            std::fs::write(
                output.join(format!("{name}.capture.json")),
                serde_json::to_vec_pretty(&metadata)?,
            )?;
            Ok(())
        }
        Err(error) => {
            metadata["status"] = "pending".into();
            metadata["error"] = format!("{error:#}").into();
            std::fs::write(
                output.join(format!("{name}.capture.json")),
                serde_json::to_vec_pretty(&metadata)?,
            )?;
            Err(error)
        }
    }
}
