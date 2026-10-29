// Diagnostic metadata only; actual production Shell/rendering stays unchanged.
fn actual_renderer_device(window:&gpui::Window)->serde_json::Value{
    match gpui_windows::query_renderer_device_identity(window){
        Ok(value)=>serde_json::json!({
            "status":"queried-current-renderer-device","gatePassed":value.gate_passed,
            "processId":value.process_id,"hwnd":format!("0x{:x}",value.hwnd),"ownerThreadId":value.owner_thread_id,"queryThreadId":value.query_thread_id,
            "deviceInstanceToken":format!("0x{:x}",value.device_token),"deviceTokenScope":"canonical IUnknown address valid only within this process/lifetime; not a global identifier",
            "featureLevelHex":format!("0x{:x}",value.feature_level),"creationFlagsHex":format!("0x{:x}",value.creation_flags),
            "adapter":{"description":value.adapter_description,"luid":{"low":value.adapter_luid_low,"high":value.adapter_luid_high,"hex":format!("0x{:08x}{:08x}",value.adapter_luid_high as u32,value.adapter_luid_low)},
                "vendorId":value.vendor_id,"deviceId":value.device_id,"subsystemId":value.subsystem_id,"revision":value.revision,"flags":value.adapter_flags,"software":value.software},
            "driver":{"method":"actual renderer device -> IDXGIDevice::GetAdapter -> CheckInterfaceSupport(IDXGIDevice::IID)","version":value.driver_version,"packedHex":value.driver_version_packed.map(|number|format!("0x{number:016x}")),"error":value.driver_error},
            "crossChecks":{"storedSelectedAdapterMatchesActualDevice":value.stored_adapter_matches_actual,"contextDeviceMatches":value.context_device_matches,"swapchainDeviceMatches":value.swapchain_device_matches,
                "renderTargetDeviceMatches":value.render_target_device_matches,"overlayDeviceMatches":value.overlay_device_matches,"overlayAttached":value.overlay_attached},
            "actualRenderTarget":{"format":value.render_target_format,"sampleCount":value.render_target_sample_count,"sampleQuality":value.render_target_sample_quality},
            "actualSwapchain":{"format":value.swapchain_format,"sampleCount":value.swapchain_sample_count,"alphaMode":value.swapchain_alpha_mode},
            "publicGpuSpecs":window.gpu_specs(),
            "sampledAtUnixMilliseconds":std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).ok().map(|time|time.as_millis()),
            "source":"current owned gpui::Window raw handle -> its WindowsWindowInner.renderer -> existing devices.device; never creates/enumerates a substitute device",
            "evidenceScope":"new diagnostic GUI binary using original production TabStrip; cannot retroactively bind old R16 binaries",
        }),
        Err(error)=>serde_json::json!({"status":"failed-or-pending-current-renderer-query","gatePassed":false,"error":format!("{error:#}"),"noSubstituteDeviceCreated":true}),
    }
}

fn actual_renderer_identity_stable(before:&serde_json::Value,after:&serde_json::Value)->bool{
    before["gatePassed"]==true&&after["gatePassed"]==true
        &&["processId","hwnd","ownerThreadId","deviceInstanceToken","featureLevelHex","creationFlagsHex","adapter","actualRenderTarget","actualSwapchain"]
            .iter().all(|key|before[*key]==after[*key])
}
