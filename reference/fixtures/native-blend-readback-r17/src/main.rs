//! No window, swapchain, compositor, screenshot, WARP fallback or OS input.
use anyhow::{Context, Result, ensure};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use std::{ffi::CString, path::{Path, PathBuf}, time::Instant};
use windows::{core::{BOOL, IUnknown, Interface, PCSTR, w}, Win32::{Foundation::HMODULE, System::LibraryLoader::{GetModuleFileNameW,GetModuleHandleW}, Graphics::{
    Direct3D::{D3D_DRIVER_TYPE_HARDWARE, D3D_FEATURE_LEVEL, D3D_FEATURE_LEVEL_11_0, D3D_FEATURE_LEVEL_11_1, D3D_PRIMITIVE_TOPOLOGY_TRIANGLELIST, ID3DBlob,
        Fxc::{D3DCompile, D3DCOMPILE_ENABLE_STRICTNESS, D3DCOMPILE_IEEE_STRICTNESS, D3DCOMPILE_OPTIMIZATION_LEVEL3,D3DCOMPILE_DEBUG,D3DCOMPILE_SKIP_OPTIMIZATION}},
    Direct3D11::*, Dxgi::{IDXGIDevice, Common::*},
}}};

const HLSL:&str=include_str!("../probe.hlsl");
const COMPILER_FLAGS:u32=D3DCOMPILE_ENABLE_STRICTNESS|D3DCOMPILE_IEEE_STRICTNESS|D3DCOMPILE_OPTIMIZATION_LEVEL3;
const WIDTH:u32=8;
const HEIGHT:u32=8;

fn float_values(values:&[f32])->Value {
    json!({"values":values,"bitsHex":values.iter().map(|v|format!("0x{:08x}",v.to_bits())).collect::<Vec<_>>()})
}
fn bytes(blob:&ID3DBlob)->&[u8] {unsafe{std::slice::from_raw_parts(blob.GetBufferPointer() as *const u8,blob.GetBufferSize())}}
fn compile(entry:&str,target:&str,flags:u32)->Result<(ID3DBlob,Value)> {
    let entry_c=CString::new(entry)?; let target_c=CString::new(target)?;
    let mut code=None; let mut errors=None;
    let result=unsafe{D3DCompile(HLSL.as_ptr().cast(),HLSL.len(),PCSTR::null(),None,None,
        PCSTR(entry_c.as_ptr().cast()),PCSTR(target_c.as_ptr().cast()),flags,0,&mut code,Some(&mut errors))};
    let diagnostics=errors.as_ref().map(|blob|String::from_utf8_lossy(bytes(blob)).to_string());
    result.with_context(||format!("D3DCompile {entry}/{target}: {diagnostics:?}"))?;
    let code=code.context("compiler returned no shader blob")?;
    let metadata=json!({"entry":entry,"target":target,"flags":flags,"shaderBytes":bytes(&code).len(),"shaderSHA256":format!("{:x}",Sha256::digest(bytes(&code))),"diagnostics":diagnostics});
    Ok((code,metadata))
}

struct Target { texture:ID3D11Texture2D, staging:ID3D11Texture2D, view:ID3D11RenderTargetView, format:DXGI_FORMAT }
fn create_target(device:&ID3D11Device,format:DXGI_FORMAT)->Result<Target>{
    let mut desc=D3D11_TEXTURE2D_DESC{Width:WIDTH,Height:HEIGHT,MipLevels:1,ArraySize:1,Format:format,
        SampleDesc:DXGI_SAMPLE_DESC{Count:1,Quality:0},Usage:D3D11_USAGE_DEFAULT,BindFlags:D3D11_BIND_RENDER_TARGET.0 as u32,..Default::default()};
    let mut texture=None; unsafe{device.CreateTexture2D(&desc,None,Some(&mut texture))?;}
    let texture=texture.context("no render texture")?;
    let mut view=None; unsafe{device.CreateRenderTargetView(&texture,None,Some(&mut view))?;}
    desc.Usage=D3D11_USAGE_STAGING; desc.BindFlags=0; desc.CPUAccessFlags=D3D11_CPU_ACCESS_READ.0 as u32;
    let mut staging=None; unsafe{device.CreateTexture2D(&desc,None,Some(&mut staging))?;}
    Ok(Target{texture,staging:staging.context("no staging texture")?,view:view.context("no RTV")?,format})
}

fn readback(context:&ID3D11DeviceContext,target:&Target)->Result<Value>{
    unsafe{context.CopyResource(&target.staging,&target.texture);}
    let mut mapped=D3D11_MAPPED_SUBRESOURCE::default();
    unsafe{context.Map(&target.staging,0,D3D11_MAP_READ,0,Some(&mut mapped))?;}
    let pixel_bytes=if target.format==DXGI_FORMAT_B8G8R8A8_UNORM{4}else{16};
    let row_bytes=WIDTH as usize*pixel_bytes;
    let mut raw=Vec::with_capacity(row_bytes*HEIGHT as usize);
    for row in 0..HEIGHT as usize {
        raw.extend_from_slice(unsafe{std::slice::from_raw_parts((mapped.pData as *const u8).add(row*mapped.RowPitch as usize),row_bytes)});
    }
    unsafe{context.Unmap(&target.staging,0);}
    let samples=raw.chunks_exact(pixel_bytes).collect::<Vec<_>>();
    let uniform=samples.iter().all(|sample|*sample==samples[0]);
    let mut result=json!({"rowPitch":mapped.RowPitch,"pixelCount":WIDTH*HEIGHT,"allPixelsIdentical":uniform,
        "packedRowsSHA256":format!("{:x}",Sha256::digest(&raw)),"allPackedBytesHex":raw.iter().map(|b|format!("{b:02x}")).collect::<String>()});
    if target.format==DXGI_FORMAT_B8G8R8A8_UNORM{
        let bgra=&raw[..4]; result["firstPixelBGRA8"]=json!(bgra); result["firstPixelRGBA8"]=json!([bgra[2],bgra[1],bgra[0],bgra[3]]);
    }else{
        let rgba:Vec<f32>=raw[..16].chunks_exact(4).map(|word|f32::from_le_bytes(word.try_into().unwrap())).collect();
        result["firstPixelRGBA32Float"]=float_values(&rgba);
    }
    ensure!(uniform,"full-coverage diagnostic target unexpectedly nonuniform");
    Ok(result)
}

fn blend(device:&ID3D11Device,mode:&str,enabled:bool)->Result<(ID3D11BlendState,Value)>{
    let (mut src,mut dst)=match mode{
        "straight"=>(D3D11_BLEND_SRC_ALPHA,D3D11_BLEND_INV_SRC_ALPHA),
        "premul"=>(D3D11_BLEND_ONE,D3D11_BLEND_INV_SRC_ALPHA),
        "dual-source"=>(D3D11_BLEND_ONE,D3D11_BLEND_INV_SRC1_COLOR),
        _=>anyhow::bail!("invalid blend mode"),
    };
    let mut dst_alpha=if mode=="dual-source"{D3D11_BLEND_INV_SRC1_ALPHA}else{D3D11_BLEND_INV_SRC_ALPHA};
    if !enabled { src=D3D11_BLEND_ONE;dst=D3D11_BLEND_ZERO;dst_alpha=D3D11_BLEND_ZERO; }
    let rt=D3D11_RENDER_TARGET_BLEND_DESC{BlendEnable:BOOL::from(enabled),SrcBlend:src,DestBlend:dst,BlendOp:D3D11_BLEND_OP_ADD,
        SrcBlendAlpha:D3D11_BLEND_ONE,DestBlendAlpha:dst_alpha,BlendOpAlpha:D3D11_BLEND_OP_ADD,RenderTargetWriteMask:D3D11_COLOR_WRITE_ENABLE_ALL.0 as u8};
    let mut desc=D3D11_BLEND_DESC::default(); desc.RenderTarget[0]=rt;
    let mut state=None; unsafe{device.CreateBlendState(&desc,Some(&mut state))?;}
    let state=state.context("no blend state")?;
    let mut actual=D3D11_BLEND_DESC::default();unsafe{state.GetDesc(&mut actual);}
    let actual_desc=json!({"query":"ID3D11BlendState::GetDesc","AlphaToCoverageEnable":actual.AlphaToCoverageEnable.as_bool(),"IndependentBlendEnable":actual.IndependentBlendEnable.as_bool(),
        "RenderTarget":actual.RenderTarget.iter().map(|rt|json!({"BlendEnable":rt.BlendEnable.as_bool(),"SrcBlend":rt.SrcBlend.0,"DestBlend":rt.DestBlend.0,"BlendOp":rt.BlendOp.0,
            "SrcBlendAlpha":rt.SrcBlendAlpha.0,"DestBlendAlpha":rt.DestBlendAlpha.0,"BlendOpAlpha":rt.BlendOpAlpha.0,"RenderTargetWriteMask":rt.RenderTargetWriteMask})).collect::<Vec<_>>()});
    Ok((state,json!({"enabled":enabled,"alphaToCoverage":false,"independentBlend":false,"actualDesc":actual_desc,
        "srcRGB":src.0,"dstRGB":dst.0,"opRGB":D3D11_BLEND_OP_ADD.0,"srcAlpha":D3D11_BLEND_ONE.0,"dstAlpha":dst_alpha.0,"opAlpha":D3D11_BLEND_OP_ADD.0,
        "writeMask":15,"blendFactor":[0,0,0,0],"sampleMask":"0xffffffff"})))
}

fn draw(context:&ID3D11DeviceContext,targets:&[&Target],shader:&ID3D11PixelShader,blend:&ID3D11BlendState){
    let views:Vec<_>=targets.iter().map(|target|Some(target.view.clone())).collect();
    unsafe{
        context.OMSetRenderTargets(Some(&views),None);
        context.OMSetBlendState(blend,Some(&[0.0;4]),u32::MAX);
        context.PSSetShader(shader,None); context.Draw(3,0);
        context.OMSetRenderTargets(None,None);
    }
}

#[derive(Clone, Copy)]
enum FeatureRequest { Baseline, Exact11One, Exact11Zero }

impl FeatureRequest {
    fn parse(value:&str)->Result<Self>{
        match value {
            "baseline"=>Ok(Self::Baseline), "11_1"=>Ok(Self::Exact11One), "11_0"=>Ok(Self::Exact11Zero),
            _=>anyhow::bail!("requested-feature-level must be baseline, 11_1 or 11_0"),
        }
    }
    fn label(self)->&'static str{
        match self {Self::Baseline=>"baseline",Self::Exact11One=>"11_1",Self::Exact11Zero=>"11_0"}
    }
    fn levels(self)->&'static [D3D_FEATURE_LEVEL]{
        match self {
            // Preserve the original R16 ordered request and fallback by default.
            Self::Baseline=>&[D3D_FEATURE_LEVEL_11_1,D3D_FEATURE_LEVEL_11_0],
            Self::Exact11One=>&[D3D_FEATURE_LEVEL_11_1],
            Self::Exact11Zero=>&[D3D_FEATURE_LEVEL_11_0],
        }
    }
    fn exact(self)->Option<D3D_FEATURE_LEVEL>{
        match self {Self::Baseline=>None,Self::Exact11One=>Some(D3D_FEATURE_LEVEL_11_1),Self::Exact11Zero=>Some(D3D_FEATURE_LEVEL_11_0)}
    }
}

fn probe(output:&Path,request:FeatureRequest)->Result<Value>{
    let mut device=None;let mut context=None;let mut feature=D3D_FEATURE_LEVEL::default();
    unsafe{D3D11CreateDevice(None,D3D_DRIVER_TYPE_HARDWARE,HMODULE::default(),D3D11_CREATE_DEVICE_BGRA_SUPPORT,
        Some(request.levels()),D3D11_SDK_VERSION,Some(&mut device),Some(&mut feature),Some(&mut context))?;}
    let device=device.context("no hardware D3D11 device")?;let context=context.context("no immediate context")?;
    // Canonical IUnknown equality binds the immediate context to this new
    // standalone device; pointer values are process/lifetime snapshots only.
    let device_identity:IUnknown=device.cast()?;
    let context_device=unsafe{context.GetDevice()?};
    let context_identity:IUnknown=context_device.cast()?;
    ensure!(device_identity.as_raw()==context_identity.as_raw(),"context GetDevice canonical IUnknown differs from this probe device");
    let device_ownership=json!({"processId":std::process::id(),"deviceInstanceToken":format!("{:p}",device_identity.as_raw()),
        "contextDeviceInstanceToken":format!("{:p}",context_identity.as_raw()),"contextDeviceMatches":true,
        "scope":"new standalone probe device canonical IUnknown snapshot; not a GUI device or continuity proof"});
    let actual_feature=unsafe{device.GetFeatureLevel()};
    let actual_flags=unsafe{device.GetCreationFlags()};
    ensure!(actual_feature.0==feature.0,"actual GetFeatureLevel differs from D3D11CreateDevice out parameter");
    if let Some(expected)=request.exact(){ensure!(actual_feature.0==expected.0,"chosen feature level differs from strict request");}
    let request_info=json!({"mode":request.label(),"orderedFeatureLevelHexes":request.levels().iter().map(|level|format!("0x{:x}",level.0)).collect::<Vec<_>>(),
        "strictChosenFeatureLevelHex":request.exact().map(|level|format!("0x{:x}",level.0)),"createDeviceOutParameterHex":format!("0x{:x}",feature.0),
        "actualGetFeatureLevelHex":format!("0x{:x}",actual_feature.0),"actualGetCreationFlagsHex":format!("0x{actual_flags:x}"),
        "requestedCreationFlagsHex":format!("0x{:x}",D3D11_CREATE_DEVICE_BGRA_SUPPORT.0),"singleDeviceContextForAllCases":true});
    let dxgi:IDXGIDevice=device.cast()?;let adapter=unsafe{dxgi.GetAdapter()?};let desc=unsafe{adapter.GetDesc()?};
    let name=String::from_utf16_lossy(&desc.Description[..desc.Description.iter().position(|c|*c==0).unwrap_or(desc.Description.len())]);
    let driver=unsafe{adapter.CheckInterfaceSupport(&IDXGIDevice::IID)};
    let driver_json=match driver{Ok(value)=>{let v=value as u64;json!({"packedHex":format!("0x{v:016x}"),"version":[(v>>48)&0xffff,(v>>32)&0xffff,(v>>16)&0xffff,v&0xffff]})},Err(error)=>json!({"error":format!("{error:#}")})};
    let adapter_info=json!({"description":name,"vendorId":desc.VendorId,"deviceId":desc.DeviceId,"subsystemId":desc.SubSysId,"revision":desc.Revision,
        "adapterLuid":{"high":desc.AdapterLuid.HighPart,"low":desc.AdapterLuid.LowPart,"hex":format!("0x{:08x}{:08x}",desc.AdapterLuid.HighPart as u32,desc.AdapterLuid.LowPart)},
        "deviceOwnership":device_ownership,"driver":driver_json,"driverQuery":"actual device -> IDXGIDevice.GetAdapter -> CheckInterfaceSupport(IDXGIDevice::IID)","deviceRequest":request_info,"featureLevelHex":format!("0x{:x}",actual_feature.0),"actualCreationFlags":actual_flags,"deviceFlags":D3D11_CREATE_DEVICE_BGRA_SUPPORT.0,"deviceSelection":"default hardware; no WARP fallback; all cases share this exact device/context"});
    std::fs::write(output.join("adapter.json"),serde_json::to_vec_pretty(&adapter_info)?)?;
    let (vs_blob,vs_meta)=compile("vs","vs_5_0",COMPILER_FLAGS)?;
    let mut vs=None;unsafe{device.CreateVertexShader(bytes(&vs_blob),None,Some(&mut vs))?;}
    let mut shader_meta=vec![vs_meta];let mut profiles=Vec::new();
    for(profile,target,flags)in [
        ("common-control","ps_5_0",COMPILER_FLAGS),
        ("gpui-debug-counterpart","ps_4_1",D3DCOMPILE_DEBUG|D3DCOMPILE_SKIP_OPTIMIZATION),
        ("mygo-effect-counterpart","ps_4_0",D3DCOMPILE_IEEE_STRICTNESS|D3DCOMPILE_OPTIMIZATION_LEVEL3),
    ]{
        let mut shaders=Vec::new();
        for entry in ["straight_ps","premul_ps","dual_ps"]{
            let(blob,mut metadata)=compile(entry,target,flags)?;metadata["profile"]=profile.into();
            std::fs::write(output.join(format!("{profile}-{entry}.dxbc")),bytes(&blob))?;
            let mut shader=None;unsafe{device.CreatePixelShader(bytes(&blob),None,Some(&mut shader))?;}
            shaders.push(shader.context("no pixel shader")?);shader_meta.push(metadata);
        }
        profiles.push((profile,shaders));
    }
    std::fs::write(output.join("vs.dxbc"),bytes(&vs_blob))?;
    let mut raster=None;unsafe{device.CreateRasterizerState(&D3D11_RASTERIZER_DESC{FillMode:D3D11_FILL_SOLID,CullMode:D3D11_CULL_NONE,DepthClipEnable:BOOL(1),..Default::default()},Some(&mut raster))?;}
    let mut depth=None;unsafe{device.CreateDepthStencilState(&D3D11_DEPTH_STENCIL_DESC{DepthEnable:BOOL(0),DepthWriteMask:D3D11_DEPTH_WRITE_MASK_ZERO,DepthFunc:D3D11_COMPARISON_ALWAYS,..Default::default()},Some(&mut depth))?;}
    let mut uniform=None;unsafe{device.CreateBuffer(&D3D11_BUFFER_DESC{ByteWidth:16,Usage:D3D11_USAGE_DEFAULT,BindFlags:D3D11_BIND_CONSTANT_BUFFER.0 as u32,..Default::default()},None,Some(&mut uniform))?;}
    let uniform=uniform.context("no uniform buffer")?;
    unsafe{
        context.VSSetShader(vs.as_ref(),None);context.IASetPrimitiveTopology(D3D_PRIMITIVE_TOPOLOGY_TRIANGLELIST);
        context.RSSetState(raster.as_ref());context.RSSetViewports(Some(&[D3D11_VIEWPORT{Width:WIDTH as f32,Height:HEIGHT as f32,MinDepth:0.0,MaxDepth:1.0,..Default::default()}]));
        context.OMSetDepthStencilState(depth.as_ref(),0);context.PSSetConstantBuffers(0,Some(&[Some(uniform.clone())]));
    }
    let formats=[("BGRA8_UNORM",DXGI_FORMAT_B8G8R8A8_UNORM),("RGBA32_FLOAT",DXGI_FORMAT_R32G32B32A32_FLOAT)];
    let mut format_info=Vec::new();let mut results=Vec::new();
    for(format_name,format)in formats{
        let support=unsafe{device.CheckFormatSupport(format)?};
        let blendable=support&D3D11_FORMAT_SUPPORT_BLENDABLE.0 as u32!=0;
        format_info.push(json!({"name":format_name,"value":format.0,"supportBits":format!("0x{support:08x}"),"blendable":blendable}));
        if !blendable{continue;}
        let target=create_target(&device,format)?;
        for(clear_name,clear,wash)in [
            ("light-byte-shell",[243.0f32/255.0,243.0/255.0,245.0/255.0,1.0],[0.10f32,0.10,0.10,0.10]),
            ("dark-controlled",[24.0f32/255.0,24.0/255.0,27.0/255.0,1.0],[0.92f32,0.92,0.92,0.10]),
        ]{
            unsafe{context.UpdateSubresource(&uniform,0,None,wash.as_ptr().cast(),0,0);}
            for(profile,shaders)in &profiles{
            for(index,mode)in ["straight","premul","dual-source"].iter().enumerate(){
                let(state,state_meta)=blend(&device,mode,true)?;
                unsafe{context.ClearRenderTargetView(&target.view,&clear);}
                let before=readback(&context,&target)?;
                draw(&context,&[&target],&shaders[index],&state);
                let after=readback(&context,&target)?;
                results.push(json!({"endpoint":clear_name,"format":format_name,"compilerProfile":profile,"mode":mode,"clearInput":float_values(&clear),"washUniform":float_values(&wash),"blendState":state_meta,"before":before,"after":after}));
            }
            if format==DXGI_FORMAT_R32G32B32A32_FLOAT{
                for(index,mode)in ["straight","premul","dual-source"].iter().enumerate(){
                    let(state,state_meta)=blend(&device,mode,false)?;
                    unsafe{context.ClearRenderTargetView(&target.view,&[0.0;4]);}
                    // MRT with blending disabled exposes the actual dual PS
                    // second output; it is not a replacement alpha shader.
                    let second=create_target(&device,format)?;
                    unsafe{context.ClearRenderTargetView(&second.view,&[0.0;4]);}
                    if *mode=="dual-source"{draw(&context,&[&target,&second],&shaders[index],&state);}else{draw(&context,&[&target],&shaders[index],&state);}
                    results.push(json!({"endpoint":clear_name,"format":format_name,"compilerProfile":profile,"mode":mode,"unblendedShaderOutput":true,"washUniform":float_values(&wash),"blendState":state_meta,
                        "target0":readback(&context,&target)?,"target1":if *mode=="dual-source"{Some(readback(&context,&second)?)}else{None}}));
                }
            }
            }
        }
    }
    unsafe{context.ClearState();context.Flush();}
    let compiler_module=unsafe{GetModuleHandleW(w!("d3dcompiler_47.dll"))?};
    let mut module_path=vec![0u16;32768];let path_length=unsafe{GetModuleFileNameW(Some(compiler_module),&mut module_path)} as usize;
    ensure!(path_length>0 && path_length<module_path.len(),"compiler DLL actual path unavailable/truncated");
    let compiler_path=PathBuf::from(String::from_utf16(&module_path[..path_length])?);
    Ok(json!({"schemaVersion":1,"status":"hardware-readbacks-complete","noWindow":true,"noSwapchain":true,"noPresentationOrCapture":true,
        "adapter":adapter_info,"shaderCompiler":{"api":"D3DCompile","dll":"d3dcompiler_47.dll","actualDLLPath":compiler_path,"actualDLLSHA256":format!("{:x}",Sha256::digest(std::fs::read(&compiler_path)?)),"sourceSHA256":format!("{:x}",Sha256::digest(HLSL.as_bytes())),"shaders":shader_meta},
        "texture":{"width":WIDTH,"height":HEIGHT,"mipLevels":1,"arraySize":1,"sampleCount":1,"sampleQuality":0,"readback":"CopyResource to CPU READ staging; Map(D3D11_MAP_READ) synchronizes; all 64 pixels recorded and uniform-checked"},
        "formats":format_info,"cases":results,"scope":"new no-window hardware device/context isolated output-merger control; actual adapter and strict requested/chosen feature level are measured here, not queried from or attached to either GUI renderer; does not retroactively bind any GUI or prove screenshot parity"}))
}

fn main()->Result<()>{
    let mut args=std::env::args().skip(1);let mut output=None;let mut request=FeatureRequest::Baseline;let mut request_seen=false;
    while let Some(arg)=args.next(){
        match arg.as_str(){
            "--out-dir"=>{ensure!(output.is_none(),"duplicate --out-dir");output=Some(PathBuf::from(args.next().context("missing output directory")?));},
            "--requested-feature-level"=>{ensure!(!request_seen,"duplicate --requested-feature-level");request=FeatureRequest::parse(&args.next().context("missing requested feature level")?)?;request_seen=true;},
            _=>anyhow::bail!("usage: native-blend-readback --out-dir NEW_OUTPUT_DIRECTORY [--requested-feature-level baseline|11_1|11_0]"),
        }
    }
    let output=output.context("missing --out-dir")?;
    ensure!(!output.exists(),"preserve old evidence; output directory must be new");
    std::fs::create_dir_all(&output)?;let start=Instant::now();
    match probe(&output,request){
        Ok(mut result)=>{result["elapsedMilliseconds"]=json!(start.elapsed().as_millis());std::fs::write(output.join("readbacks.json"),serde_json::to_vec_pretty(&result)?)?;println!("{}",output.display());Ok(())},
        Err(error)=>{let _=std::fs::write(output.join("failure.json"),serde_json::to_vec_pretty(&json!({"status":"failed","error":format!("{error:#}"),"elapsedMilliseconds":start.elapsed().as_millis(),"noWindow":true,"requestedFeatureLevelMode":request.label()}))?);Err(error)}
    }
}
