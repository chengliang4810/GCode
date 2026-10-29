"""Read-only verification, plus a new immutable evidence seal file."""
from pathlib import Path
import hashlib
import json
import sys

root=Path(__file__).resolve().parents[3]
run=Path(sys.argv[1]).resolve()
build=Path(sys.argv[2]).resolve()
prior=root/"output/native-blend-readback-r16-run-attempt01"
def load(path):return json.loads(path.read_text(encoding="utf-8-sig"))
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
assert not (run/"seal.json").exists(),"preserve an existing seal"
result=load(run/"result.json")
data=load(run/"readbacks.json")
old=load(prior/"readbacks.json")
provenance=load(build/"provenance.json")
for item in provenance["sources"]:
    assert sha(build/"source"/item["path"])==item["sha256"]
    assert sha(Path(provenance["sourceDirectory"])/item["path"])==item["sha256"]
assert sha(Path(result["binaryPath"]))==result["binarySHA256"]
assert sha(run/"readbacks.json")==result["readbacksSHA256"]
assert load(run/"dependency-tree-command.json")["exitCodes"]==[0,0]
assert load(run/"dependency-tree-command.json")["cargoLockUnchanged"]
assert old["adapter"]["adapterLuid"]==data["adapter"]["adapterLuid"]
for before,after in zip(old["cases"],data["cases"],strict=True):
    copy=json.loads(json.dumps(after))
    copy["blendState"].pop("actualDesc")
    assert copy==before
    desc=after["blendState"]["actualDesc"]
    assert len(desc["RenderTarget"])==8 and all(rt==desc["RenderTarget"][0] for rt in desc["RenderTarget"])
old_shaders={item["entry"]+item.get("profile",""):item for item in old["shaderCompiler"]["shaders"]}
shader_comparison=[]
for item in data["shaderCompiler"]["shaders"]:
    path=run/((item["profile"]+"-" if "profile" in item else "")+item["entry"]+".dxbc")
    assert sha(path)==item["shaderSHA256"] and path.stat().st_size==item["shaderBytes"]
    key=item["entry"]+item.get("profile","")
    shader_comparison.append({"entry":item["entry"],"profile":item.get("profile"),"beforeSHA256":old_shaders[key]["shaderSHA256"],"afterSHA256":item["shaderSHA256"],"sameBytesAcrossBuilds":old_shaders[key]["shaderSHA256"]==item["shaderSHA256"]})
artifacts=[]
for name in ["readbacks.json","result.json","adapter.json","video-controller-cim.json","compiler-file-version.json","active-build-dependency-tree.txt","active-build-features-tree.txt","exact-locked-dependency-graph.json","dependency-tree-command.json","audit.md","independent-review.json"]:
    path=run/name
    artifacts.append({"path":str(path),"bytes":path.stat().st_size,"sha256":sha(path)})
for name in ["provenance.json","build.log","run-attempt01.json","run-attempt01.log"]:
    path=build/name
    artifacts.append({"path":str(path),"bytes":path.stat().st_size,"sha256":sha(path)})
profiles={}
for item in data["shaderCompiler"]["shaders"]:
    if "profile" in item:
        profiles[item["profile"]]={"target":item["target"],"flags":item["flags"]}
states={case["mode"]:case["blendState"]["actualDesc"] for case in data["cases"] if case["endpoint"]=="light-byte-shell" and case["format"]=="BGRA8_UNORM" and case["compilerProfile"]=="common-control"}
seal={"schemaVersion":1,"stage":"R16","status":"frozen-independent-no-window-diagnostic",
    "resultSHA256":sha(run/"result.json"),"binarySHA256":result["binarySHA256"],"compiledSourceTreeSHA256":result["compiledSourceTreeSHA256"],
    "sourceTreeHashMethod":result["compiledSourceTreeHashMethod"],"sources":provenance["sources"],
    "adapter":data["adapter"],"driverSupplement":result["driverSupplement"],"compilerProfiles":profiles,
    "actualCompleteBlendDescriptors":states,"actualDescriptorScope":"ID3D11BlendState::GetDesc readback on this device; all8 native returned entries are retained, not fabricated; only1 RTV bound during blending",
    "repeatRun":{"priorDirectory":str(prior),"priorReadbacksSHA256":sha(prior/"readbacks.json"),"sameAdapterLUID":True,"all54ReadbackCasesIdenticalExceptNewGetDesc":True,"shaderBlobComparison":shader_comparison,"debugBytecodeNote":"Some debug DXBC hashes differ across builds; raw blobs are retained, while measured numeric outputs agree. Do not claim bytecode identity from equal results."},
    "limits":result["conclusion"]["notEstablished"],"sameMachineVsApplicationAdapterBoundary":"One hardware device/context per probe run and same adapter LUID across these2 runs are measured. Neither existing GUI app is bound to this measured LUID/feature level by its old metadata.",
    "formatInferenceBoundary":"Light float32 RT outputs agree bitwise and BGRA8 outputs diverge under identical inputs and device/state except output decomposition; this supports a fixed-point output-merger/format cause for the tested light endpoint, not a universal internal rounding formula. Dark float1ULP difference remains separately recorded.",
    "artifactHashes":artifacts,"analysisSource":{"path":str(Path(__file__).with_name("analyze.py")),"sha256":sha(Path(__file__).with_name("analyze.py"))},"sealSourceSHA256":sha(Path(__file__)),
    "scope":{"newIsolatedFixtureOnly":True,"changedProductionOrFramework":False,"changedTabstripFixtures":False,"changedFrozenClonesOrMutableOriginalExport":False,"newNetworkOrDependencyResolution":False,"isolatedCargoHome":provenance["isolatedCargoHome"],"desktopWindowOrInput":False,"goBuildOrPackage":False}}
(run/"seal.json").write_text(json.dumps(seal,indent=2)+"\n",encoding="utf-8",newline="\n")
print(json.dumps({"seal":str(run/"seal.json"),"sha256":sha(run/"seal.json"),"binarySHA256":result["binarySHA256"],"sourceTreeSHA256":result["compiledSourceTreeSHA256"]},indent=2))
