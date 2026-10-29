// Style-only native queue/cursor diagnostics. No OS cursor/activation writes.
mod style_input {
    use super::*;
    use std::sync::atomic::{AtomicI32, AtomicU64, AtomicUsize, Ordering};
    use windows::Win32::{Foundation::{LPARAM, LRESULT, POINT, WPARAM}, Graphics::Gdi::{ClientToScreen, ScreenToClient}, UI::WindowsAndMessaging::{
        CallNextHookEx, GetCursorPos, HHOOK, MSG, PM_REMOVE, PostMessageW,
        SetWindowsHookExW, UnhookWindowsHookEx, WH_GETMESSAGE, WM_MOUSEMOVE,
    }};

    static OWN: AtomicUsize = AtomicUsize::new(0);
    static RECEIVED: AtomicU64 = AtomicU64::new(0);
    static MATCHED: AtomicU64 = AtomicU64::new(0);
    static X: AtomicI32 = AtomicI32::new(0);
    static Y: AtomicI32 = AtomicI32::new(0);
    static EXPECTED_X: AtomicI32 = AtomicI32::new(0);
    static EXPECTED_Y: AtomicI32 = AtomicI32::new(0);

    // WH_GETMESSAGE observes queue retrieval, not dispatch completion. The
    // independent Window.mouse_position sample below checks GPUI's effect.
    unsafe extern "system" fn observe(code: i32, wparam: WPARAM, lparam: LPARAM) -> LRESULT {
        if code == 0 && wparam.0 == PM_REMOVE.0 as usize && lparam.0 != 0 {
            let message = unsafe { &*(lparam.0 as *const MSG) };
            if message.hwnd.0 as usize == OWN.load(Ordering::Relaxed) && message.message == WM_MOUSEMOVE {
                let x = message.lParam.0 as u16 as i16 as i32;
                let y = (message.lParam.0 >> 16) as u16 as i16 as i32;
                X.store(x,Ordering::Relaxed); Y.store(y,Ordering::Relaxed);
                RECEIVED.fetch_add(1,Ordering::Relaxed);
                if x == EXPECTED_X.load(Ordering::Relaxed) && y == EXPECTED_Y.load(Ordering::Relaxed) { MATCHED.fetch_add(1,Ordering::Relaxed); }
            }
        }
        unsafe { CallNextHookEx(None,code,wparam,lparam) }
    }

    pub struct QueueHook(HHOOK);
    impl QueueHook {
        pub fn install(hwnd: usize) -> anyhow::Result<Self> {
            anyhow::ensure!(OWN.load(Ordering::Relaxed)==0,"style hook already installed");
            OWN.store(hwnd,Ordering::Relaxed);
            let thread = unsafe { windows_sys::Win32::System::Threading::GetCurrentThreadId() };
            match unsafe { SetWindowsHookExW(WH_GETMESSAGE,Some(observe),None,thread) } {
                Ok(hook) => Ok(Self(hook)), Err(error) => { OWN.store(0,Ordering::Relaxed); Err(error.into()) }
            }
        }
    }
    impl Drop for QueueHook { fn drop(&mut self) { let _ = unsafe { UnhookWindowsHookEx(self.0) }; OWN.store(0,Ordering::Relaxed); } }

    #[derive(Clone, Copy)]
    pub struct Expectation { pub hwnd: usize, pub target: [f32;2], pub scale:f32, pub require_cursor:bool, pub matched_before:u64 }

    fn raw_cursor(hwnd:usize,scale:f32) -> anyhow::Result<serde_json::Value> {
        let mut screen = POINT::default();
        unsafe { GetCursorPos(&mut screen)?; }
        let mut client = screen;
        anyhow::ensure!(unsafe { ScreenToClient(HWND(hwnd as *mut _),&mut client) }.as_bool(),"ScreenToClient failed");
        Ok(serde_json::json!({"screenPx":[screen.x,screen.y],"clientPx":[client.x,client.y],"clientDIP":[client.x as f32/scale,client.y as f32/scale],"fixtureMovedOsCursor":false}))
    }

    fn near(point:[f32;2],target:[f32;2]) -> bool { (point[0]-target[0]).abs()<=2.0 && (point[1]-target[1]).abs()<=2.0 }

    pub fn sample(expect:Expectation,window:&gpui::Window) -> anyhow::Result<serde_json::Value> {
        let cursor = raw_cursor(expect.hwnd,expect.scale)?;
        let cursor_dip = [cursor["clientDIP"][0].as_f64().unwrap() as f32,cursor["clientDIP"][1].as_f64().unwrap() as f32];
        let pointer = window.mouse_position();
        let logical = [f32::from(pointer.x),f32::from(pointer.y)];
        let receipt = MATCHED.load(Ordering::Relaxed)>expect.matched_before;
        let keyboard = window.last_input_was_keyboard();
        Ok(serde_json::json!({
            "targetClientDIP":expect.target,"actualGpuiMousePositionClientDIP":logical,
            "gpuiPositionAtTarget":near(logical,expect.target),"gpuiLastInputWasKeyboard":keyboard,
            "rawQueue":{"hook":"WH_GETMESSAGE/HC_ACTION/PM_REMOVE; current owner UI thread; observer calls CallNextHookEx unchanged",
                "receiptCount":RECEIVED.load(Ordering::Relaxed),"matchingTargetReceiptCount":MATCHED.load(Ordering::Relaxed),
                "matchingTargetReceiptAfterPost":receipt,"lastClientPx":[X.load(Ordering::Relaxed),Y.load(Ordering::Relaxed)]},
            "nativeCursor":cursor,"requireNativeCursor":expect.require_cursor,"nativeCursorAtTarget":near(cursor_dip,expect.target),
            "actualHovered":null,"actualHoveredLimitation":"production per-chip computed hover is not public; native receipt is not a hover assertion",
            "inputGatePassed":receipt && near(logical,expect.target) && !keyboard && (!expect.require_cursor || near(cursor_dip,expect.target)),
        }))
    }

    pub fn passed(sample:&serde_json::Value)->bool { sample["inputGatePassed"]==true }

    // Time begins at a committed UI-thread sample, not at an inferred OS
    // motion time. Polling cannot exclude an away-and-back between samples.
    pub async fn dwell(window:gpui::WindowHandle<TabstripFixture>,cx:&mut AsyncApp,expect:Expectation,duration:Duration,geometry:serde_json::Value,output:&Path)->anyhow::Result<serde_json::Value> {
        let mut observations=Vec::new();
        let mut start=None;
        let mut started_at=0u128;
        loop {
            let observation=window.update(cx,|view,window,cx| {
                let input=sample(expect,window)?;
                let native=native_activity(expect.hwnd,window.is_window_active());
                let actual_geometry=view.stability_sample(cx);
                let now=std::time::Instant::now();
                let wall=std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH)?.as_millis();
                Ok::<_,anyhow::Error>((now,wall,input,native,actual_geometry,view.capture_name()))
            })??;
            let (now,wall,input,native,actual_geometry,name)=observation;
            let ready=passed(&input) && activity_is_own(&native) && actual_geometry==geometry;
            let first=*start.get_or_insert(now);
            if observations.is_empty() { started_at=wall; }
            let elapsed=now.duration_since(first);
            observations.push(serde_json::json!({"elapsedNanoseconds":elapsed.as_nanos() as u64,"sampledAtUnixMilliseconds":wall,"input":input,"native":native,"geometry":actual_geometry,"ready":ready}));
            let evidence=serde_json::json!({
                "requestedNanoseconds":duration.as_nanos() as u64,"elapsedNanoseconds":elapsed.as_nanos() as u64,
                "startedAtUnixMilliseconds":started_at,"endedAtUnixMilliseconds":wall,"observationCount":observations.len(),
                "timingSource":"monotonic Instant sampled on owner UI thread after committed input/native/geometry gate; not exact OS motion timestamp",
                "pollIntervalMilliseconds":16,"betweenPollTransitionsExcluded":false,"proves350MillisecondTooltipDelay":false,
                "observations":observations,"gatePassed":ready && elapsed>=duration,
            });
            if !ready {
                std::fs::write(output.join(format!("{name}.hover-dwell-failed.json")),serde_json::to_vec_pretty(&evidence)?)?;
                anyhow::bail!("hover dwell observed input/native/geometry gate loss for {name}");
            }
            if elapsed>=duration { return Ok(evidence); }
            cx.background_executor().timer(Duration::from_millis(16)).await;
        }
    }

    pub async fn prepare(window:gpui::WindowHandle<TabstripFixture>,cx:&mut AsyncApp,output:&Path,expected_scale:f32,require_cursor:bool)->anyhow::Result<(Expectation,serde_json::Value)> {
        let (hwnd,target,name,screen) = window.update(cx,|view,window,cx| {
            let (hwnd,_) = lossless_transport::describe(window)?;
            anyhow::ensure!((window.scale_factor()-expected_scale).abs()<0.001,"style actual DPI mismatch");
            let target=view.pointer_target(window,cx)?;
            let mut screen=POINT{x:(target[0]*expected_scale).round() as i32,y:(target[1]*expected_scale).round() as i32};
            anyhow::ensure!(unsafe{ClientToScreen(HWND(hwnd as *mut _),&mut screen)}.as_bool(),"ClientToScreen failed");
            Ok::<_,anyhow::Error>((hwnd,target,view.capture_name(),[screen.x,screen.y]))
        })??;
        let mut expect=Expectation{hwnd,target,scale:expected_scale,require_cursor,matched_before:MATCHED.load(Ordering::Relaxed)};
        std::fs::write(output.join(format!("{name}.style-ready.json")),serde_json::to_vec_pretty(&serde_json::json!({
            "status":if require_cursor{"waiting-for-root-owned-os-pointer"}else{"native-message-only-no-os-cursor-requirement"},
            "processId":std::process::id(),"hwnd":format!("0x{hwnd:x}"),"name":name,"targetClientDIP":target,"targetScreenPx":screen,
            "timeoutMilliseconds":30000,"fixtureMovesOsCursor":false,"action":"root may move actual OS pointer to target with approved input; fixture does not click or activate",
        }))?)?;
        let start=std::time::Instant::now();
        let mut cursor_observations=Vec::new();
        if require_cursor {
            loop {
                let sample=window.update(cx,|_,window,_|sample(expect,window))??;
                let ready=sample["nativeCursorAtTarget"]==true;
                cursor_observations.push(serde_json::json!({"elapsedMilliseconds":start.elapsed().as_millis(),"cursor":sample["nativeCursor"]}));
                if ready { break; }
                if start.elapsed()>=Duration::from_secs(30) {
                    std::fs::write(output.join(format!("{name}.style-input-failed.json")),serde_json::to_vec_pretty(&serde_json::json!({"status":"native-cursor-timeout","observations":cursor_observations}))?)?;
                    anyhow::bail!("root-owned native cursor wait timed out for {name}");
                }
                cx.background_executor().timer(Duration::from_millis(100)).await;
            }
        }
        let x=(target[0]*expected_scale).round() as i32;
        let y=(target[1]*expected_scale).round() as i32;
        anyhow::ensure!(i16::try_from(x).is_ok() && i16::try_from(y).is_ok(),"native mouse target exceeds signed client coordinate range");
        EXPECTED_X.store(x,Ordering::Relaxed);EXPECTED_Y.store(y,Ordering::Relaxed);
        // Take the matching receipt baseline after the optional root cursor
        // wait, immediately before posting our own queued message.
        expect.matched_before=MATCHED.load(Ordering::Relaxed);
        unsafe { PostMessageW(Some(HWND(hwnd as *mut _)),WM_MOUSEMOVE,WPARAM(0),LPARAM(((y as u16 as u32)<<16 | x as u16 as u32) as isize))?; }
        let post_start=std::time::Instant::now();
        let mut dispatch=Vec::new();
        loop {
            cx.background_executor().timer(Duration::from_millis(50)).await;
            let sample=window.update(cx,|_,window,_|sample(expect,window))??;
            let ready=passed(&sample);
            dispatch.push(sample);
            if ready { break; }
            if post_start.elapsed()>=Duration::from_secs(3) {
                std::fs::write(output.join(format!("{name}.style-input-failed.json")),serde_json::to_vec_pretty(&serde_json::json!({"status":"native-queue-or-dispatch-gate-failed","observations":dispatch}))?)?;
                anyhow::bail!("native queue receipt/GPUI mouse gate failed for {name}");
            }
        }
        Ok((expect,serde_json::json!({"transport":"PostMessageW WM_MOUSEMOVE; no OS cursor move or activation","cursorWaitObservations":cursor_observations,"dispatchObservations":dispatch})))
    }
}
