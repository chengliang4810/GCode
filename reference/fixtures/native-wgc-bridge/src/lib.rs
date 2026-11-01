//! Test-only, in-process WGC transport. No production renderer or UI is linked.
#![cfg(windows)]

use anyhow::{Context as _, ensure};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use std::{
    ffi::c_void,
    os::windows::fs::MetadataExt,
    panic::{AssertUnwindSafe, catch_unwind},
    path::{Path, PathBuf},
    sync::{Mutex, mpsc},
    time::{Duration, SystemTime, UNIX_EPOCH},
};
use windows::Win32::{
    Foundation::{HWND, POINT, RECT},
    Graphics::Gdi::ClientToScreen,
    System::Threading::GetCurrentProcessId,
    UI::{
        HiDpi::GetDpiForWindow,
        WindowsAndMessaging::{GetClientRect, GetWindowRect, GetWindowThreadProcessId, IsWindow},
    },
};
use windows_capture::{
    capture::{CaptureControl, Context, GraphicsCaptureApiHandler},
    frame::{Frame, ImageFormat},
    graphics_capture_api::InternalCaptureControl,
    settings::{
        ColorFormat, CursorCaptureSettings, DirtyRegionSettings, DrawBorderSettings,
        MinimumUpdateIntervalSettings, SecondaryWindowSettings, Settings,
    },
    window::Window as CaptureWindow,
};

const MAX_PATH_BYTES: usize = 32768;
const MAX_ERROR_BYTES: usize = 65536;
const MAX_FRAME_WAIT_MS: u32 = 15000;
static CAPTURE: Mutex<()> = Mutex::new(());

fn require_new_entry(path: &Path) -> anyhow::Result<()> {
    // Check the directory entry itself: Path::exists follows symlinks and
    // would treat an existing dangling link as a new filename.
    match std::fs::symlink_metadata(path) {
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Ok(_) => anyhow::bail!("refusing to overwrite an existing capture or metadata entry"),
        Err(error) => Err(error).context("could not inspect the capture output entry"),
    }
}

fn require_plain_parents(path: &Path) -> anyhow::Result<()> {
    for parent in path.ancestors().skip(1) {
        let entry = std::fs::symlink_metadata(parent)
            .context("could not inspect the capture output ancestor")?;
        ensure!(entry.is_dir(), "capture output ancestor is not a directory");
        ensure!(
            entry.file_attributes() & 0x400 == 0,
            "capture output must not follow a reparse ancestor"
        );
    }
    Ok(())
}

fn own_window(raw: usize) -> anyhow::Result<HWND> {
    ensure!(raw != 0, "a fixture HWND is required");
    let hwnd = HWND(raw as *mut c_void);
    let mut owner = 0;
    unsafe {
        ensure!(IsWindow(Some(hwnd)).as_bool(), "fixture HWND is invalid");
        GetWindowThreadProcessId(hwnd, Some(&mut owner));
        ensure!(
            owner != 0 && owner == GetCurrentProcessId(),
            "refusing to capture a window outside the calling fixture process"
        );
    }
    Ok(hwnd)
}

fn describe(raw: usize) -> anyhow::Result<Value> {
    let hwnd = own_window(raw)?;
    let mut frame = RECT::default();
    let mut client = RECT::default();
    let mut origin = POINT::default();
    unsafe {
        GetWindowRect(hwnd, &mut frame)?;
        GetClientRect(hwnd, &mut client)?;
        ensure!(
            ClientToScreen(hwnd, &mut origin).as_bool(),
            "ClientToScreen failed"
        );
    }
    ensure!(
        client.right > client.left && client.bottom > client.top,
        "fixture client has no pixels"
    );
    let dpi = unsafe { GetDpiForWindow(hwnd) };
    ensure!(dpi != 0, "fixture window DPI is unavailable");
    Ok(json!({
        "schemaVersion": 1, "bridgeABIVersion": 1,
        "referenceCommit": "ab189159f10cf8e63c0502be75a59bbc487e0eaf",
        "renderer": "unmodified caller renderer; test-only transport",
        "transport": "in-process windows-capture 1.5.0 WGC Frame::save_as_image(Png)",
        "processId": unsafe { GetCurrentProcessId() }, "hwnd": format!("0x{raw:x}"),
        "windowRectScreenPx": [frame.left, frame.top, frame.right, frame.bottom],
        "windowSizePx": [frame.right - frame.left, frame.bottom - frame.top],
        "clientRectPx": [client.left, client.top, client.right, client.bottom],
        "clientOriginScreenPx": [origin.x, origin.y],
        "clientSizePx": [client.right - client.left, client.bottom - client.top],
        "dpi": dpi, "scale": f64::from(dpi) / 96.0,
        "captureTarget": "exact caller-owned HWND; no window or display enumeration",
        "cursor": "WithoutCursor", "captureBorder": "system Default; no privacy override",
        "secondaryWindows": "system Default", "imageTransformations": [],
        "cropped": false, "resized": false,
        "captureBoundary": "complete original HWND WGC frame including native/nonclient extent; no file crop, resize, or mask",
        "keyboardAcceptance": "not established by capture transport",
        "visualAcceptance": "pending independent comparison"
    }))
}

struct CaptureFlags {
    hwnd: usize,
    output: PathBuf,
    result: mpsc::Sender<Result<Value, String>>,
}

struct Handler {
    flags: CaptureFlags,
    completed: bool,
}

struct Session(Option<CaptureControl<Handler, <Handler as GraphicsCaptureApiHandler>::Error>>);

impl Session {
    fn stop(mut self) -> anyhow::Result<()> {
        self.0
            .take()
            .context("WGC session was already stopped")?
            .stop()
            .map_err(|error| anyhow::anyhow!("WGC shutdown failed: {error}"))
    }
}

impl Drop for Session {
    fn drop(&mut self) {
        // CaptureControl has no stopping Drop. Also clean up if our code
        // unwinds between startup and the explicit stop/join below.
        if let Some(control) = self.0.take() {
            let _ = catch_unwind(AssertUnwindSafe(|| control.stop()));
        }
    }
}

fn panic_message(panic: Box<dyn std::any::Any + Send>) -> String {
    let detail = if let Some(value) = panic.downcast_ref::<&str>() {
        *value
    } else if let Some(value) = panic.downcast_ref::<String>() {
        value.as_str()
    } else {
        "non-string panic"
    };
    format!("capture panic contained: {detail}")
}

impl GraphicsCaptureApiHandler for Handler {
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
        // Contain our callback panics before returning through WinRT's ABI.
        let pixels = catch_unwind(AssertUnwindSafe(|| -> anyhow::Result<Value> {
            own_window(self.flags.hwnd)?;
            let (width, height) = (frame.width(), frame.height());
            ensure!(width > 0 && height > 0 && width <= 16384 && height <= 16384, "unexpected WGC dimensions");
            require_new_entry(&self.flags.output)?;
            frame.save_as_image(&self.flags.output, ImageFormat::Png)?;
            let png = std::fs::read(&self.flags.output)?;
            let (png_width, png_height) = png_dimensions(&png)?;
            ensure!((png_width, png_height) == (width, height), "PNG dimensions differ from the complete WGC frame");
            Ok(json!({
                "pixelFormat": "BGRA8", "frameSizePx": [width, height],
                "pngSizePx": [png_width, png_height], "pngBytes": png.len(),
                "pngSHA256": format!("{:x}", Sha256::digest(&png)),
                "capturedAtUnixMilliseconds": SystemTime::now().duration_since(UNIX_EPOCH)?.as_millis(),
                "encoder": "Windows BitmapEncoder::PngEncoderId; no JPEG intermediate"
            }))
        }))
        .map_err(panic_message)
        .and_then(|result| result.map_err(|error| format!("{error:#}")));
        let _ = self.flags.result.send(pixels);
        control.stop();
        Ok(())
    }

    fn on_closed(&mut self) -> Result<(), Self::Error> {
        let _ = self
            .flags
            .result
            .send(Err("fixture window closed before capture completed".into()));
        Ok(())
    }
}

fn png_dimensions(png: &[u8]) -> anyhow::Result<(u32, u32)> {
    ensure!(
        png.len() >= 33 && png.starts_with(b"\x89PNG\r\n\x1a\n") && &png[12..16] == b"IHDR",
        "invalid PNG signature or IHDR"
    );
    let width = u32::from_be_bytes(png[16..20].try_into()?);
    let height = u32::from_be_bytes(png[20..24].try_into()?);
    ensure!(width > 0 && height > 0, "PNG has no pixels");
    Ok((width, height))
}

fn capture_pixels(hwnd: usize, output: PathBuf, timeout_ms: u32) -> anyhow::Result<Value> {
    let handle = own_window(hwnd)?;
    let (sender, receiver) = mpsc::channel();
    let settings = Settings::new(
        CaptureWindow::from_raw_hwnd(handle.0),
        CursorCaptureSettings::WithoutCursor,
        DrawBorderSettings::Default,
        SecondaryWindowSettings::Default,
        MinimumUpdateIntervalSettings::Default,
        DirtyRegionSettings::Default,
        ColorFormat::Bgra8,
        CaptureFlags {
            hwnd,
            output,
            result: sender,
        },
    );
    // This library owns the MTA/dispatcher thread, all WinRT objects and PNG
    // encoding. No COM object or frame is handed to the caller's Go thread.
    let control = Handler::start_free_threaded(settings).map_err(|error| {
        anyhow::anyhow!(
            "WGC unavailable; capture remains pending without changing privacy settings: {error}"
        )
    })?;
    let session = Session(Some(control));
    let pixels = receiver.recv_timeout(Duration::from_millis(u64::from(timeout_ms)));
    // Always stop/join before returning, even on timeout or callback failure.
    let stopped = session.stop();
    let pixels = pixels
        .context("no original WGC frame; capture remains pending")?
        .map_err(anyhow::Error::msg)?;
    stopped?;
    Ok(pixels)
}

fn capture_to_file(hwnd: usize, path: &Path, timeout_ms: u32) -> anyhow::Result<()> {
    let _capture = CAPTURE
        .lock()
        .map_err(|_| anyhow::anyhow!("capture lock is poisoned"))?;
    ensure!(path.is_absolute(), "PNG output path must be absolute");
    ensure!(
        path.extension()
            .and_then(|extension| extension.to_str())
            .is_some_and(|extension| extension.eq_ignore_ascii_case("png")),
        "capture output must end in .png"
    );
    require_plain_parents(path)?;
    let metadata_path = PathBuf::from(format!("{}.capture.json", path.display()));
    require_new_entry(path)?;
    require_new_entry(&metadata_path)?;
    let mut metadata = describe(hwnd)?;
    metadata["name"] = path
        .file_name()
        .and_then(|name| name.to_str())
        .context("output filename is not UTF-8")?
        .into();
    metadata["firstFrameWaitTimeoutMilliseconds"] = timeout_ms.into();
    match capture_pixels(hwnd, path.to_path_buf(), timeout_ms) {
        Ok(pixels) => {
            metadata["status"] = "captured".into();
            metadata["pixels"] = pixels;
            std::fs::write(metadata_path, serde_json::to_vec_pretty(&metadata)?)?;
            Ok(())
        }
        Err(error) => {
            metadata["status"] = "pending".into();
            metadata["error"] = format!("{error:#}").into();
            std::fs::write(metadata_path, serde_json::to_vec_pretty(&metadata)?)?;
            Err(error)
        }
    }
}

/// ABI v1. All pointers refer to caller-owned buffers valid until return.
/// Input is length-delimited UTF-8 without NUL; error output is bounded UTF-8
/// with a trailing NUL. The function is synchronous and must run off the UI
/// thread. Returns 0 on success, 1 on error, 2 on contained panic, 3 on invalid
/// buffer sizes. Never unload this DLL before the fixture process exits.
///
/// # Safety
/// `path` must reference `path_len` readable bytes; `error` must reference
/// `error_capacity` writable bytes. Neither pointer is retained after return.
#[unsafe(no_mangle)]
pub unsafe extern "system" fn rcode_fixture_wgc_capture(
    hwnd: usize,
    path: *const u8,
    path_len: usize,
    timeout_ms: u32,
    error: *mut u8,
    error_capacity: usize,
) -> u32 {
    if path.is_null()
        || path_len == 0
        || path_len > MAX_PATH_BYTES
        || error.is_null()
        || error_capacity == 0
        || error_capacity > MAX_ERROR_BYTES
    {
        return 3;
    }
    unsafe {
        *error = 0;
    }
    let result = catch_unwind(AssertUnwindSafe(|| -> anyhow::Result<()> {
        ensure!(
            timeout_ms > 0 && timeout_ms <= MAX_FRAME_WAIT_MS,
            "first-frame wait must be 1..15000 milliseconds"
        );
        let bytes = unsafe { std::slice::from_raw_parts(path, path_len) };
        let path = std::str::from_utf8(bytes).context("output path must be valid UTF-8")?;
        ensure!(!path.contains('\0'), "output path contains NUL");
        let path = PathBuf::from(path);
        capture_to_file(hwnd, &path, timeout_ms)
    }));
    let (status, message) = match result {
        Ok(Ok(())) => return 0,
        Ok(Err(error)) => (1, format!("{error:#}")),
        Err(panic) => (2, panic_message(panic)),
    };
    let mut length = message.len().min(error_capacity - 1);
    while !message.is_char_boundary(length) {
        length -= 1;
    }
    unsafe {
        std::ptr::copy_nonoverlapping(message.as_ptr(), error, length);
        *error.add(length) = 0;
    }
    status
}

#[unsafe(no_mangle)]
pub extern "system" fn rcode_fixture_wgc_abi_version() -> u32 {
    1
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn fresh_entry_check_rejects_existing_files_and_directories() {
        let project = Path::new(env!("CARGO_MANIFEST_DIR"));
        assert!(require_new_entry(&project.join("Cargo.toml")).is_err());
        assert!(require_new_entry(project).is_err());
        assert!(require_plain_parents(&project.join("new-test.png")).is_ok());
        assert!(
            require_new_entry(
                &project.join(format!("absent-contract-test-{}.png", std::process::id()))
            )
            .is_ok()
        );
    }

    #[test]
    fn invalid_handle_does_not_start_capture_or_write_files() {
        let path = b"D:\\not-created-by-wgc-test.png";
        let mut error = [0u8; 256];
        let code = unsafe {
            rcode_fixture_wgc_capture(
                0,
                path.as_ptr(),
                path.len(),
                1,
                error.as_mut_ptr(),
                error.len(),
            )
        };
        assert_eq!(code, 1);
        assert!(std::str::from_utf8(&error).unwrap().contains("HWND"));
    }

    #[test]
    fn ffi_rejects_oversized_and_invalid_utf8_buffers() {
        let mut error = [0u8; 4];
        assert_eq!(
            unsafe {
                rcode_fixture_wgc_capture(
                    0,
                    std::ptr::null(),
                    1,
                    1,
                    error.as_mut_ptr(),
                    error.len(),
                )
            },
            3
        );
        assert_eq!(
            unsafe {
                rcode_fixture_wgc_capture(
                    0,
                    b"x".as_ptr(),
                    MAX_PATH_BYTES + 1,
                    1,
                    error.as_mut_ptr(),
                    error.len(),
                )
            },
            3
        );
        let path = [255u8];
        assert_eq!(
            unsafe {
                rcode_fixture_wgc_capture(0, path.as_ptr(), 1, 1, error.as_mut_ptr(), error.len())
            },
            1
        );
        assert_eq!(error[3], 0);
        assert!(std::str::from_utf8(&error).is_ok());
    }
}
