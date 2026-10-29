"""Validate raw readbacks and report measured differences without fitting colors."""
import hashlib
import json
from pathlib import Path
import struct
import sys
import tomllib

def load(path):
    return json.loads(path.read_text(encoding="utf-8-sig"))

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def f32(value):
    return struct.unpack("<f", struct.pack("<f", value))[0]

def bits(value):
    return f"0x{struct.unpack('<I', struct.pack('<f', value))[0]:08x}"

run = Path(sys.argv[1]).resolve()
build = Path(sys.argv[2]).resolve()
root = Path(__file__).resolve().parents[3]
data = load(run / "readbacks.json")
provenance = load(build / "provenance.json")
exit_status = load(build / "run-attempt01.json")
assert exit_status["exitCode"] == 0
assert provenance["buildExitCode"] == 0
assert sha(Path(provenance["binaryPath"])) == provenance["binarySHA256"]
assert data["status"] == "hardware-readbacks-complete" and data["noWindow"] and data["noSwapchain"]
assert len(data["cases"]) == 54
profiles = ["common-control", "gpui-debug-counterpart", "mygo-effect-counterpart"]
modes = ["straight", "premul", "dual-source"]
cases = {}
for case in data["cases"]:
    key = (case["endpoint"], case["format"], case["compilerProfile"], case["mode"], case.get("unblendedShaderOutput", False))
    assert key not in cases
    cases[key] = case
    desc = case["blendState"]["actualDesc"]
    assert desc["query"] == "ID3D11BlendState::GetDesc" and len(desc["RenderTarget"]) == 8
    assert not desc["AlphaToCoverageEnable"] and not desc["IndependentBlendEnable"]
    rt = desc["RenderTarget"][0]
    for actual, recorded in [("SrcBlend","srcRGB"),("DestBlend","dstRGB"),("BlendOp","opRGB"),("SrcBlendAlpha","srcAlpha"),("DestBlendAlpha","dstAlpha"),("BlendOpAlpha","opAlpha"),("RenderTargetWriteMask","writeMask"),("BlendEnable","enabled")]:
        assert rt[actual] == case["blendState"][recorded]
    for output in [case.get("before"), case.get("after"), case.get("target0"), case.get("target1")]:
        if output is None:
            continue
        assert output["allPixelsIdentical"] and output["pixelCount"] == 64
        raw = bytes.fromhex(output["allPackedBytesHex"])
        assert hashlib.sha256(raw).hexdigest() == output["packedRowsSHA256"]
        if "firstPixelRGBA32Float" in output:
            assert [f"0x{x:08x}" for x in struct.unpack("<4I", raw[:16])] == output["firstPixelRGBA32Float"]["bitsHex"]
    if case.get("unblendedShaderOutput"):
        wash = case["washUniform"]["values"]
        expected = wash if case["mode"] == "straight" else [f32(value*wash[3]) for value in wash[:3]] + [wash[3]]
        assert case["target0"]["firstPixelRGBA32Float"]["bitsHex"] == [bits(value) for value in expected]
        if case["mode"] == "dual-source":
            assert case["target1"]["firstPixelRGBA32Float"]["bitsHex"] == [bits(wash[3])]*4
    elif case["format"] == "BGRA8_UNORM":
        assert case["before"]["firstPixelRGBA8"] == ([243,243,245,255] if case["endpoint"] == "light-byte-shell" else [24,24,27,255])

def sample(endpoint, fmt, profile, mode):
    value = cases[(endpoint, fmt, profile, mode, False)]["after"]
    return value.get("firstPixelRGBA8", value.get("firstPixelRGBA32Float", {}).get("bitsHex"))

comparisons = []
for endpoint in ["light-byte-shell", "dark-controlled"]:
    for fmt in ["BGRA8_UNORM", "RGBA32_FLOAT"]:
        for profile in profiles:
            values = {mode: sample(endpoint, fmt, profile, mode) for mode in modes}
            comparisons.append({"endpoint": endpoint, "format": fmt, "compilerProfile":profile, "after":values,
                "straightEqualsPremul":values["straight"]==values["premul"],"premulEqualsDualSource":values["premul"]==values["dual-source"]})
compiler_invariant = all(sample(endpoint, fmt, profile, mode)==sample(endpoint, fmt, profiles[0], mode)
    for endpoint in ["light-byte-shell","dark-controlled"] for fmt in ["BGRA8_UNORM","RGBA32_FLOAT"] for profile in profiles for mode in modes)

pixel_file = root / "output/tabstrip-r15-light-residual-preflight/existing-pixel-evidence.json"
pixels = load(pixel_file)
pixel_matches = []
for record in pixels["records"]:
    assert sha(Path(record["originalPNG"])) == record["sha256"]
    mode = "straight" if record["side"] == "reference" else "dual-source"
    measured = sample("light-byte-shell", "BGRA8_UNORM", "common-control", mode)
    rgba = "#"+"".join(f"{value:02x}" for value in measured)
    flat = [value for value in record["samples"] if value["name"].startswith("selected-")]
    pixel_matches.append({"side":record["side"],"originalPNG":record["originalPNG"],"sha256":record["sha256"],
        "probeMode":mode,"measuredProbeRGBA":rgba,"existingInteriorSamples":flat,"allExistingInteriorSamplesMatch":all(value["rgba"]==rgba for value in flat)})

cim = load(run / "video-controller-cim.json")
adapter = data["adapter"]
identity = f"VEN_{adapter['vendorId']:04X}&DEV_{adapter['deviceId']:04X}&SUBSYS_{adapter['subsystemId']:08X}&REV_{adapter['revision']:02X}"
drivers = [entry for entry in cim if identity in entry["PNPDeviceID"].upper() and entry["Name"]==adapter["description"]]
assert len(drivers) == 1

source_paths = [
    Path("C:/Users/chengliang/.cargo/git/checkouts/zui-05467652636d6f6d/b564a34/crates/gpui_windows/src/directx_renderer.rs"),
    Path("C:/Users/chengliang/.cargo/git/checkouts/zui-05467652636d6f6d/b564a34/crates/gpui_windows/src/shaders.hlsl"),
    Path("C:/Users/chengliang/.cargo/git/checkouts/zui-05467652636d6f6d/b564a34/crates/gpui_windows/src/directx_devices.rs"),
    root/"output/reference-sources/zeron/crates/ui/src/theme.rs",
    root/"output/reference-sources/zeron/crates/theme/src/builtins.rs",
    root/"third_party/mygo/internal/gpu/d3d11/d3d11.go",
    root/"third_party/mygo/internal/gpu/d3d11/effect.hlsl",
    root/"third_party/mygo/internal/gpu/d3d11/shader.go",
    root/"internal/workbench/tabs_style.go",
]
for source in provenance["sources"]:
    assert sha(build / "source" / source["path"]) == source["sha256"]
lock = tomllib.loads((build / "source/Cargo.lock").read_text(encoding="utf-8"))
dependency_graph = [{key:value for key,value in package.items() if key in ("name","version","source","checksum","dependencies")} for package in lock["package"]]
(run/"exact-locked-dependency-graph.json").write_text(json.dumps({"method":"parsed exact successfully --offline --locked built Cargo.lock; no registry/version resolution","cargoLockSHA256":sha(build/"source/Cargo.lock"),"packages":dependency_graph},indent=2)+"\n",encoding="utf-8",newline="\n")
source_hasher = hashlib.sha256()
for source in sorted(provenance["sources"],key=lambda entry:entry["path"]):
    source_hasher.update(source["path"].encode()+b"\0")
    source_hasher.update((build/"source"/source["path"]).read_bytes()+b"\0")

result = {
    "schemaVersion":1,"stage":"R16","status":"same-hardware-no-window-readback-verified",
    "binaryPath":provenance["binaryPath"],"binarySHA256":provenance["binarySHA256"],
    "readbacksSHA256":sha(run/"readbacks.json"),"runExitCode":0,"buildExitCode":0,
    "compiledSourceTreeSHA256":source_hasher.hexdigest(),"compiledSourceTreeHashMethod":"sorted relative path UTF8 + NUL + exact snapshot bytes + NUL for each provenance source file",
    "dependencyGraphSHA256":sha(run/"exact-locked-dependency-graph.json"),"dependencyGraph":dependency_graph,
    "adapter":adapter,"driverSupplement":{"method":"read-only Win32_VideoController exact unique PCI vendor/device/subsystem/revision plus name match; not a direct LUID driver query","identity":identity,"record":drivers[0],"directDXGIQueryErrorRetained":adapter["driver"]},
    "compiler":data["shaderCompiler"],"compilerFileVersion":load(run/"compiler-file-version.json"),
    "validation":{"caseCount":54,"fullCoveragePixelsPerReadback":64,"packedByteHashesAndUniformityPass":True,
        "unblendedFloatOutputsMatchUniformArithmetic":True,"clearActualBytesVerified":True,"compilerProfilesInvariantForThisMatrix":compiler_invariant,"fullBlendGetDescMatchesRecordedState":True},
    "comparisons":comparisons,"existingPixelEvidence":{"path":str(pixel_file),"sha256":sha(pixel_file),"records":pixel_matches},
    "conclusion":{
        "measured":"On this same hardware device, BGRA8_UNORM straight SRC_ALPHA/INV_SRC_ALPHA yields light RGBA8 222/222/223/255; both premultiplied ONE/INV_SRC_ALPHA and ONE/INV_SRC1_COLOR yield 222/222/224/255 from identical verified 243/243/245/255 destination bytes.",
        "isolatedCause":"The controlled light difference occurs in the fixed-point RT output-merger/format path under straight versus premultiplied decomposition. It is not specific to dual-source factors, compiler profile, HSL roundtrip, compositor or WGC in this probe.",
        "floatBound":"Light RGBA32_FLOAT blended results are bit-identical across all three modes. Dark RGBA32_FLOAT straight blue is one ULP above premultiplied blue, although dark BGRA8 bytes agree. Do not generalize float equality to all endpoints.",
        "sourceOutputBound":"Measured unblended light straight .1 is 0x3dcccccd; actual uniform .1*.1 yields premul 0x3c23d70b, not literal .01 0x3c23d70a. Original formulas/states are matched for a flat full-coverage pixel; production shader binaries and complete frame are not reproduced.",
        "notEstablished":["the internal hardware precision/rounding order or which conversion/multiply term rounds first","that both GUI captures used this exact LUID and feature level","other GPUs/drivers/formats, all alpha/color endpoints, antialiasing/clip/opacity edges","complete visual parity or correctness of a production renderer change"],
        "nextAction":"If matching the source is required, review an explicit opt-in straight-alpha blend/output path for this flat wash; do not alter theme tokens or fit a per-channel offset. Preserve premultiplied renderer defaults and separately test edges/clip/opacity/dual-source text before adopting any API."
    },
    "sourceEvidence":[{"path":str(path),"sha256":sha(path)} for path in source_paths],
    "probeSources":provenance["sources"],"analysisSourceSHA256":sha(Path(__file__)),
    "failedAttempt":{"directory":str(root/"output/native-blend-readback-r16-build-attempt01"),"error":"wrong BOOL import path; preserved failed source/build log/provenance, no binary run"},
    "noProductionOrTabstripFixtureChanges":True,"noWindowOrOSInput":True,"noGoBuildOrPackage":True,
}
(run/"result.json").write_text(json.dumps(result,indent=2)+"\n",encoding="utf-8",newline="\n")
print(json.dumps({"result":str(run/"result.json"),"sha256":sha(run/"result.json"),"caseCount":54,"compilerInvariant":compiler_invariant,"allOldInteriorSamplesMatch":all(row["allExistingInteriorSamplesMatch"] for row in pixel_matches)},indent=2))
