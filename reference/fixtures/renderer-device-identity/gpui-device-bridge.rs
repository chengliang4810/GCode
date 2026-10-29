//! Diagnostic-only child of the frozen DirectXRenderer implementation.
//! This module never creates a device, changes render state or retains handles.
use super::DirectXRenderer;
use anyhow::{Context as _, Result, ensure};
use raw_window_handle::{HasWindowHandle, RawWindowHandle};
use windows::{core::{IUnknown, Interface}, Win32::{
    Foundation::HWND,
    Graphics::{Direct3D11::{ID3D11Device,D3D11_TEXTURE2D_DESC},Dxgi::{IDXGIAdapter1,IDXGIDevice,DXGI_ADAPTER_FLAG_SOFTWARE}},
    System::Threading::GetCurrentThreadId,
    UI::WindowsAndMessaging::{GetWindowThreadProcessId,IsWindow},
}};

#[derive(Clone,Debug)]
pub struct RendererDeviceIdentity {
    pub process_id:u32,
    pub hwnd:usize,
    pub owner_thread_id:u32,
    pub query_thread_id:u32,
    pub device_token:usize,
    pub feature_level:u32,
    pub creation_flags:u32,
    pub adapter_description:String,
    pub adapter_luid_low:u32,
    pub adapter_luid_high:i32,
    pub vendor_id:u32,
    pub device_id:u32,
    pub subsystem_id:u32,
    pub revision:u32,
    pub adapter_flags:u32,
    pub software:bool,
    pub driver_version:Option<[u16;4]>,
    pub driver_version_packed:Option<u64>,
    pub driver_error:Option<String>,
    pub stored_adapter_matches_actual:bool,
    pub context_device_matches:bool,
    pub swapchain_device_matches:bool,
    pub render_target_device_matches:bool,
    pub overlay_device_matches:Option<bool>,
    pub overlay_attached:bool,
    pub render_target_format:u32,
    pub render_target_sample_count:u32,
    pub render_target_sample_quality:u32,
    pub swapchain_format:u32,
    pub swapchain_sample_count:u32,
    pub swapchain_alpha_mode:u32,
    pub gate_passed:bool,
}

fn same_device(device:&ID3D11Device,other:&ID3D11Device)->Result<bool>{
    let own:IUnknown=device.cast()?;
    let other:IUnknown=other.cast()?;
    Ok(own.as_raw()==other.as_raw())
}

impl DirectXRenderer {
    fn diagnostic_current_device(&self,hwnd:HWND,owner:u32,query:u32)->Result<RendererDeviceIdentity>{
        ensure!(self.hwnd==hwnd,"actual renderer HWND differs from current GPUI window");
        let devices=self.devices.as_ref().context("actual renderer devices are not ready")?;
        let device=&devices.device;
        let dxgi:IDXGIDevice=device.cast().context("actual renderer device has no IDXGIDevice")?;
        let adapter=unsafe{dxgi.GetAdapter()?};
        let actual=unsafe{adapter.GetDesc()?};
        let adapter1:IDXGIAdapter1=adapter.cast()?;
        let flags=unsafe{adapter1.GetDesc1()?}.Flags;
        let stored=unsafe{devices.adapter.GetDesc1()?};
        let stored_matches=stored.AdapterLuid.LowPart==actual.AdapterLuid.LowPart && stored.AdapterLuid.HighPart==actual.AdapterLuid.HighPart;
        let feature=unsafe{device.GetFeatureLevel()}.0 as u32;
        let creation_flags=unsafe{device.GetCreationFlags()};
        let canonical:IUnknown=device.cast()?;
        let driver=unsafe{adapter.CheckInterfaceSupport(&IDXGIDevice::IID)};
        let (driver_version,driver_packed,driver_error)=match driver{
            Ok(number)=>{let number=number as u64;(Some([(number>>48)as u16,(number>>32)as u16,(number>>16)as u16,number as u16]),Some(number),None)},
            Err(error)=>(None,None,Some(format!("{error:#}"))),
        };
        let context_device=unsafe{devices.device_context.GetDevice()?};
        let resources=self.resources.as_ref().context("actual window render resources not ready")?;
        let swapchain_device:ID3D11Device=unsafe{resources.swap_chain.GetDevice()?};
        let rtv=resources.render_target_view.as_ref().context("actual window RTV not ready")?;
        let target_device=unsafe{rtv.GetDevice()?};
        let texture=resources.render_target.as_ref().context("actual window render target not ready")?;
        let mut target_desc=D3D11_TEXTURE2D_DESC::default();unsafe{texture.GetDesc(&mut target_desc);}
        let swap_desc=unsafe{resources.swap_chain.GetDesc1()?};
        let context_matches=same_device(device,&context_device)?;
        let swap_matches=same_device(device,&swapchain_device)?;
        let target_matches=same_device(device,&target_device)?;
        let overlay_matches=self.overlay.as_ref().map(|overlay|{
            let other:ID3D11Device=unsafe{overlay.resources.swap_chain.GetDevice()?};
            same_device(device,&other)
        }).transpose()?;
        let gate=stored_matches&&context_matches&&swap_matches&&target_matches&&overlay_matches.unwrap_or(true);
        Ok(RendererDeviceIdentity{
            process_id:std::process::id(),hwnd:hwnd.0 as usize,owner_thread_id:owner,query_thread_id:query,
            device_token:canonical.as_raw() as usize,feature_level:feature,creation_flags,
            adapter_description:String::from_utf16_lossy(&actual.Description).trim_matches(char::from(0)).to_string(),
            adapter_luid_low:actual.AdapterLuid.LowPart,adapter_luid_high:actual.AdapterLuid.HighPart,
            vendor_id:actual.VendorId,device_id:actual.DeviceId,subsystem_id:actual.SubSysId,revision:actual.Revision,adapter_flags:flags,
            software:flags&DXGI_ADAPTER_FLAG_SOFTWARE.0 as u32!=0,
            driver_version,driver_version_packed:driver_packed,driver_error,
            stored_adapter_matches_actual:stored_matches,context_device_matches:context_matches,swapchain_device_matches:swap_matches,
            render_target_device_matches:target_matches,overlay_device_matches:overlay_matches,overlay_attached:self.overlay.as_ref().is_some_and(|overlay|overlay.attached),
            render_target_format:target_desc.Format.0 as u32,render_target_sample_count:target_desc.SampleDesc.Count,render_target_sample_quality:target_desc.SampleDesc.Quality,
            swapchain_format:swap_desc.Format.0 as u32,swapchain_sample_count:swap_desc.SampleDesc.Count,swapchain_alpha_mode:swap_desc.AlphaMode.0 as u32,gate_passed:gate,
        })
    }
}

/// Query only the current live GPUI window. An arbitrary integer HWND would
/// not establish that GWLP_USERDATA contains GPUI's private Weak owner type.
pub fn query_renderer_device_identity(window:&gpui::Window)->Result<RendererDeviceIdentity>{
    let handle=HasWindowHandle::window_handle(window).context("current GPUI window has no native handle")?;
    let RawWindowHandle::Win32(raw)=handle.as_raw() else{anyhow::bail!("diagnostic is Windows-only");};
    let hwnd=HWND(raw.hwnd.get() as *mut std::ffi::c_void);
    ensure!(unsafe{IsWindow(Some(hwnd))}.as_bool(),"current GPUI window is not live");
    let mut process=0;
    let owner=unsafe{GetWindowThreadProcessId(hwnd,Some(&mut process))};
    let query=unsafe{GetCurrentThreadId()};
    ensure!(owner!=0&&owner==query&&process==std::process::id(),"actual GPUI window query must run on owner UI thread/process");
    let inner=crate::window::window_from_hwnd(hwnd).context("current GPUI window has no live GPUI owner")?;
    let renderer=inner.state.renderer.try_borrow().context("current GPUI renderer is borrowed; retry outside drawing")?;
    renderer.diagnostic_current_device(hwnd,owner,query)
}
