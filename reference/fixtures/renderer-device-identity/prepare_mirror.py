"""New isolated source mirrors and exact append-only device bridge guards.

Copies old source as data; never mutates original export/global Git cache.
"""
from pathlib import Path
import argparse
import hashlib
import json
import shutil
import subprocess

ROOT=Path(__file__).resolve().parents[3]
TEMPLATE=Path(__file__).resolve().parent
ORIGINAL=ROOT/'output/reference-native-0.2.128'
FROZEN=ROOT/'output/reference-sources/zeron'
GPUI=Path('C:/Users/chengliang/.cargo/git/checkouts/zui-05467652636d6f6d/b564a34')
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def checked(path):
    path=path.resolve()
    assert path.is_relative_to(ROOT/'output') and path.name.startswith('renderer-device-r17-')
    for ancestor in [path,*path.parents]:
        if ancestor.exists():assert not ancestor.is_symlink() and not ancestor.is_junction()
        if ancestor==ROOT:break
    return path
def inventory(root):
    rows=[]
    for path in sorted(root.rglob('*')):
        assert not path.is_symlink() and not path.is_junction(),f'reparse input: {path}'
        if path.is_file():rows.append({'path':path.relative_to(root).as_posix(),'bytes':path.stat().st_size,'sha256':sha(path)})
    return rows
def save(path,value):path.write_text(json.dumps(value,indent=2)+'\n',encoding='utf-8')
def replace_once(text,before,after):
    assert text.count(before)==1,f'expected unique adapter insertion anchor: {before[:80]}'
    return text.replace(before,after,1)

parser=argparse.ArgumentParser()
parser.add_argument('--out-dir',required=True)
args=parser.parse_args()
out=checked(Path(args.out_dir))
assert not out.exists(),'Preserve old attempts'
out.mkdir(parents=True)
# Cached upstream fixtures contain Go sample files. Keep the complete copied
# Cargo bytes while making this isolated output a boundary for parent go ./....
(out/'go.mod').write_bytes(b'module diagnostic.invalid/renderer-device-r17-isolated-cache\n\ngo 1.27.3\n')
evidence=out/'evidence';evidence.mkdir()
save(evidence/'go-wildcard-boundary.json',{'path':str(out/'go.mod'),'sha256':sha(out/'go.mod'),'purpose':'exclude complete copied Cargo inputs from parent Go wildcards without modifying their bytes','goCommandExecuted':False})
mirror=out/'zeron';gpui=out/'gpui_windows'
commit=subprocess.check_output(['git','--no-optional-locks','-C',str(FROZEN),'rev-parse','HEAD'],text=True).strip()
assert commit=='ab189159f10cf8e63c0502be75a59bbc487e0eaf'
production=['crates/ui/src/shell.rs','crates/ui/src/shell/tabs.rs','crates/ui/src/shell/side_chats.rs','crates/ui/src/surface_chrome.rs','crates/ui/src/state.rs','crates/ui/src/theme.rs','crates/ui/src/typography.rs','crates/ui/src/browser/mod.rs','crates/proto/src/entities.rs','crates/ui/src/files/preview.rs','crates/ui/src/files/mod.rs','crates/ui/src/files/document.rs','crates/ui/src/files/editor.rs','crates/ui/src/files/client.rs','crates/ui/src/file_icons.rs']
guards={'crates/ui/src/shell.rs':b'\n#[cfg(feature = "compact-picker-fixture")]\npub mod tabstrip_fixture;\n',
        'crates/ui/src/files/preview.rs':b'\n#[cfg(feature = "compact-picker-fixture")]\nmod tabstrip_fixture;\n'}
production_rows=[]
for name in production:
    expected=(FROZEN/name).read_bytes()+guards.get(name,b'')
    assert (ORIGINAL/name).read_bytes()==expected,f'original mutable export production body differs: {name}'
    production_rows.append({'path':name,'frozenSHA256':sha(FROZEN/name),'exportSHA256':sha(ORIGINAL/name),'allowedDelta':guards.get(name,b'').decode()})
before=inventory(ORIGINAL)
gpui_before=inventory(GPUI/'crates/gpui_windows')
save(evidence/'zeron-export-before.json',before)
save(evidence/'gpui-windows-before.json',gpui_before)
save(evidence/'production-guards.json',production_rows)
shutil.copytree(ORIGINAL,mirror)
shutil.copytree(GPUI/'crates/gpui_windows',gpui)
assert inventory(mirror)==before,'mirror copy differs from actual export bytes'
assert inventory(gpui)==gpui_before,'GPUI mirror copy differs from cache bytes'
for name in ['Cargo.toml','Cargo.lock','crates/ui/Cargo.toml']:
    shutil.copyfile(mirror/name,evidence/(name.replace('/','__')+'.before'))
gpui_guards={
 'src/directx_renderer.rs':b'\n#[cfg(feature = "renderer-device-identity")]\npub(crate) mod renderer_device_identity;\n',
 'src/gpui_windows.rs':b'\n#[cfg(feature = "renderer-device-identity")]\npub use directx_renderer::renderer_device_identity::{query_renderer_device_identity, RendererDeviceIdentity};\n',
}
for name,suffix in gpui_guards.items():
    source=(gpui/name).read_bytes()
    (gpui/name).write_bytes(source+suffix)
    shutil.copyfile(GPUI/'crates/gpui_windows'/name,evidence/(Path(name).name+'.before'))
child=gpui/'src/directx_renderer/renderer_device_identity.rs'
child.parent.mkdir(exist_ok=True)
shutil.copyfile(TEMPLATE/'gpui-device-bridge.rs',child)
example=mirror/'crates/ui/examples/renderer-device-native.rs'
original_example=(mirror/'crates/ui/examples/tabstrip-native.rs').read_bytes()
text=original_example.decode('utf-8')
text=replace_once(text,'include_str!("tabstrip-source-manifest.json")','include_str!("renderer-device-source-manifest.json")')
text=replace_once(text,'include!("tabstrip-style-native.rs");','include!("tabstrip-style-native.rs");\ninclude!("renderer-device-native-metadata.rs");')
text=replace_once(text,'let (hwnd, mut metadata) = lossless_transport::describe(window)?;','let (hwnd, mut metadata) = lossless_transport::describe(window)?;\n        metadata["renderer"] = "frozen production GPUI rendering methods with append-only actual-device diagnostic; new instrumented GUI binary".into();\n        metadata["diagnosticBinary"] = true.into();\n        metadata["historicalR16BinaryDeviceBound"] = false.into();')
text=replace_once(text,'metadata["nativeActivityBefore"] = native_activity(hwnd, window.is_window_active());','metadata["nativeActivityBefore"] = native_activity(hwnd, window.is_window_active());\n        metadata["rendererDeviceBefore"] = actual_renderer_device(window);')
text=replace_once(text,'if !activity_is_own(&metadata["nativeActivityBefore"]) ||','if metadata["rendererDeviceBefore"]["gatePassed"] != true || !activity_is_own(&metadata["nativeActivityBefore"]) ||')
text=replace_once(text,'let activity_after_ok = activity_is_own(&metadata["nativeActivityAfter"]);','metadata["rendererDeviceAfter"] = window.update(cx,|_,window,_|actual_renderer_device(window))?;\n    let device_after_ok = actual_renderer_identity_stable(&metadata["rendererDeviceBefore"],&metadata["rendererDeviceAfter"]);\n    let activity_after_ok = activity_is_own(&metadata["nativeActivityAfter"]);')
text=replace_once(text,'if !activity_after_ok ||','if !device_after_ok || !activity_after_ok ||')
text=replace_once(text,'"failed-native-activity-or-style-input-before"','"failed-renderer-device-or-native-activity-or-style-input-before"')
text=replace_once(text,'"failed-native-activity-before"','"failed-renderer-device-or-native-activity-before"')
text=replace_once(text,'"raw native activity before WGC is not own foreground/owner-active; scene {name}"','"current renderer device query/crosscheck or native activity/style gate failed before WGC; scene {name}"')
text=replace_once(text,'"failed-native-activity-or-style-input-after"','"failed-renderer-device-or-native-activity-or-style-input-after"')
text=replace_once(text,'"failed-native-activity-after"','"failed-renderer-device-or-native-activity-after"')
text=replace_once(text,'"own foreground/owner-active gate failed after WGC; preserve raw PNG as failed evidence"','"observed renderer device identity/crosscheck or native activity/style gate failed after WGC; preserve raw PNG as failed evidence"')
text=replace_once(text,'"raw native activity after WGC is not own foreground/owner-active; scene {name}"','"observed renderer device identity/crosscheck or native activity/style gate failed after WGC; scene {name}"')
text=replace_once(text,'"productionShellModuleDeclarationAppended": true,','"productionShellModuleDeclarationAppended": true,\n                    "diagnosticBinary":true,"oldR16BinaryIdentityBound":false,"deviceQueryPolicy":"current renderer device before/after every capture; actual adapter LUID/FL, no substitute device",')
example.write_bytes(text.encode('utf-8'))
shutil.copyfile(TEMPLATE/'native-metadata.rs',mirror/'crates/ui/examples/renderer-device-native-metadata.rs')
save(evidence/'mirror-prepared.json',{'schemaVersion':1,'referenceCommit':commit,'originalExport':str(ORIGINAL),'gpuiOriginal':str(GPUI),'mirror':str(mirror),'gpuiWindowsMirror':str(gpui),
 'productionBodiesUnchanged':True,'productionGuardCount':len(production_rows),'gpuiAppendedGuards':{name:value.decode() for name,value in gpui_guards.items()},
 'gpuiBridgeSHA256':sha(child),'originalTabstripExampleSHA256':hashlib.sha256(original_example).hexdigest(),'newDiagnosticExampleSHA256':sha(example),
 'scope':'new instrumented GUI binary reuses existing original Shell/tabstrip_fixture production path; not oldR16 binary identity or fullvisual parity',
 'oldExportsAndTemplatesModified':False,'globalCacheModified':False,'buildStatus':'not-started','nativeRun':False})
assert inventory(ORIGINAL)==before,'original export changed during copy; preserve evidence and stop'
assert inventory(GPUI/'crates/gpui_windows')==gpui_before,'global GPUI crate changed during copy; preserve evidence and stop'
print(json.dumps({'mirror':str(mirror),'gpui':str(gpui),'evidence':str(evidence),'originalFiles':len(before),'gpuiFiles':len(gpui_before)}))
