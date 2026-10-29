"""Build only the isolated R17 GUI diagnostic using exact cached dependencies.

No binary is launched. Old exports, frozen source and the global Cargo cache
are input data only. A locked/offline failure is retained, never relaxed.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
import tomllib

ROOT = Path(__file__).resolve().parents[3]
TEMPLATES = Path(__file__).resolve().parent
CARGO = Path.home() / '.cargo'
REGISTRY = 'index.crates.io-1949cf8c6b5b557f'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def save(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False) + '\n', encoding='utf-8')


def inventory(root):
    rows = []
    for path in sorted(root.rglob('*')):
        if path.is_symlink() or path.is_junction():
            raise RuntimeError(f'reparse source is not accepted: {path}')
        if path.is_file():
            rows.append({'path': path.relative_to(root).as_posix(), 'bytes': path.stat().st_size, 'sha256': sha(path)})
    return rows


def copy_file(source, dest):
    dest.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, dest)
    assert sha(source) == sha(dest)


def seed_cache(destination, lock, evidence):
    assert not destination.exists(), 'private Cargo home must be fresh'
    destination.mkdir()
    copied, missing = [], []
    for package in lock['package']:
        if not package.get('source', '').startswith('registry+'):
            continue
        name, version = package['name'], package['version']
        archive = Path('registry/cache') / REGISTRY / f'{name}-{version}.crate'
        source = CARGO / archive
        if not source.is_file():
            missing.append({'name': name, 'version': version, 'reason': 'uncached locked archive; may be inactive on this target'})
            continue
        assert sha(source) == package['checksum'], f'cached archive checksum mismatch: {name}-{version}'
        copy_file(source, destination / archive)
        copied.append({'path': archive.as_posix(), 'sha256': sha(source), 'bytes': source.stat().st_size})
    # Copy cached sparse entries, including inactive targets, without invoking a
    # registry updater. Cargo can validate the original complete lock offline.
    for relative in [Path('registry/index') / REGISTRY, Path('git/db'), Path('git/checkouts')]:
        source = CARGO / relative
        assert source.is_dir(), f'missing existing cache subtree: {source}'
        rows = inventory(source)
        for row in rows:
            # Git alternates could send writes or object lookup outside the
            # isolated home. This cache has no alternates; reject if introduced.
            assert not row['path'].endswith('/objects/info/alternates'), 'Git alternates require an explicit local rewrite plan'
        shutil.copytree(source, destination / relative)
        assert inventory(destination / relative) == rows
        save(evidence / (relative.as_posix().replace('/', '__') + '-input.json'), rows)
    (destination / 'config.toml').write_text('[net]\noffline = true\n[cache]\nauto-clean-frequency = "never"\n', encoding='utf-8')
    save(evidence / 'cached-registry-archives.json', copied)
    save(evidence / 'uncached-inactive-or-blocking-lock-packages.json', missing)
    return {'registryArchivesCopied': len(copied), 'uncachedLockPackages': len(missing),
            'policy': 'Only byte-verified existing archives/index/Git caches copied; no registry source modification, network or dependency-version selection permitted'}


def run_logged(command, cwd, env, stdout_path, stderr_path):
    with stdout_path.open('w', encoding='utf-8') as out, stderr_path.open('w', encoding='utf-8') as err:
        process = subprocess.run(command, cwd=cwd, env=env, stdout=out, stderr=err, text=True)
    return process.returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--attempt-root', required=True)
    parser.add_argument('--build-name', default='build-attempt01')
    args = parser.parse_args()
    attempt = Path(args.attempt_root).resolve(strict=True)
    assert attempt.parent == ROOT / 'output' and attempt.name.startswith('renderer-device-r17-')
    assert Path(args.build_name).name == args.build_name and args.build_name.startswith('build-attempt')
    mirror, gpui = attempt / 'zeron', attempt / 'gpui_windows'
    build = attempt / args.build_name
    assert not build.exists(), 'preserve previous build evidence'
    build.mkdir()
    result = {'schemaVersion': 1, 'stage': 'R17', 'status': 'preparing', 'nativeRun': False,
              'startedAtUTC': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'binaryScope': 'new diagnostic GUI using actual original Shell TabStrip path; does not bind historical R16 binaries',
              'oldExportModified': False, 'productionMethodBodiesModified': False, 'globalCargoCacheWriteByBuild': False}
    try:
        boundary = attempt / 'go.mod'
        assert boundary.read_bytes() == b'module diagnostic.invalid/renderer-device-r17-isolated-cache\n\ngo 1.27.3\n'
        result['goWildcardBoundary'] = {'path': str(boundary), 'sha256': sha(boundary), 'goCommandExecutedHere': False}
        processes = subprocess.check_output(['tasklist', '/FI', 'IMAGENAME eq cargo.exe', '/FO', 'CSV', '/NH'], text=True)
        assert 'cargo.exe' not in processes.lower(), 'another Cargo builder is running'
        example = mirror / 'crates/ui/examples/renderer-device-native.rs'
        old_example = example.read_bytes()
        old_include = b'include_str!("tabstrip-source-manifest.json")'
        new_include = b'include_str!("renderer-device-source-manifest.json")'
        if old_include in old_example:
            assert old_example.count(old_include) == 1
            (build / 'renderer-device-native.rs.before-manifest-rename').write_bytes(old_example)
            example.write_bytes(old_example.replace(old_include, new_include))
        else:
            assert old_example.count(new_include) == 1
        assert (mirror / 'Cargo.lock').read_bytes() == (attempt / 'evidence/manifests/after/zeron__Cargo.lock').read_bytes()
        source_manifest = {'schemaVersion': 1, 'stage': 'R17', 'referenceCommit': 'ab189159f10cf8e63c0502be75a59bbc487e0eaf',
            'diagnosticBinary': True, 'historicalR16BinaryDeviceBound': False,
            'productionGuards': json.loads((attempt / 'evidence/production-guards.json').read_text()),
            'manifestLockPreparation': json.loads((attempt / 'evidence/manifests/result.json').read_text()),
            'diagnosticExampleSHA256': sha(example), 'diagnosticMetadataSHA256': sha(mirror / 'crates/ui/examples/renderer-device-native-metadata.rs'),
            'gpuiBridgeSHA256': sha(gpui / 'src/directx_renderer/renderer_device_identity.rs'),
            'measurement': 'current live typed gpui::Window -> owned HWND/UI thread -> actual renderer devices.device; context/swapchain/RTV COM identity crosschecks',
            'scope': 'append-only guarded diagnostics, exact original production rendering bodies; new instrumented binary, actual identity still pending execution'}
        save(mirror / 'crates/ui/examples/renderer-device-source-manifest.json', source_manifest)
        for root, name in [(mirror, 'zeron'), (gpui, 'gpui_windows'), (TEMPLATES, 'templates')]:
            rows = inventory(root)
            save(build / f'{name}-source-hashes.json', rows)
            shutil.copytree(root, build / 'source' / name)
            assert inventory(build / 'source' / name) == rows
        result['sourceManifestSHA256'] = sha(mirror / 'crates/ui/examples/renderer-device-source-manifest.json')
        local_cargo = attempt / 'cargo-home'
        if not local_cargo.exists():
            result['cachePreparation'] = seed_cache(local_cargo, tomllib.loads((mirror / 'Cargo.lock').read_text()), build)
        else:
            assert (local_cargo / 'config.toml').read_text() == '[net]\noffline = true\n[cache]\nauto-clean-frequency = "never"\n'
            result['cachePreparation'] = {'policy': 'reusing this new R17 private cache only; earlier seed inventory retained'}
        result['privateCargoHome'] = str(local_cargo)
        env = os.environ.copy()
        env.update(CARGO_HOME=str(local_cargo), CARGO_TARGET_DIR=str(ROOT / 'output/reference-native/target'), CARGO_NET_OFFLINE='true')
        result['buildEnvironment'] = {name: env.get(name) for name in ['CARGO_HOME', 'CARGO_TARGET_DIR', 'CARGO_NET_OFFLINE',
            'RUSTFLAGS', 'CARGO_ENCODED_RUSTFLAGS', 'RUSTUP_TOOLCHAIN', 'RUSTC_WRAPPER', 'RUSTC_WORKSPACE_WRAPPER', 'CARGO_BUILD_RUSTFLAGS']}
        command = ['cargo', 'build', '--offline', '--locked', '-j', '2', '--manifest-path', str(mirror / 'Cargo.toml'), '-p', 'zeron-ui', '--example', 'renderer-device-native', '--features', 'compact-picker-fixture']
        result['command'] = command
        result['cargoVersion'] = subprocess.check_output(['cargo', '--version'], text=True).strip()
        result['rustcVersion'] = subprocess.check_output(['rustc', '-Vv'], text=True).strip()
        result['status'] = 'building'
        save(build / 'provenance.json', result)
        print('R17 private cache ready; cargo --offline --locked -j 2 beginning', flush=True)
        start = time.monotonic()
        code = run_logged(command, mirror, env, build / 'build.stdout.log', build / 'build.stderr.log')
        result['buildSeconds'] = time.monotonic() - start
        result['buildExitCode'] = code
        result['buildFinishedAtUTC'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
        assert (mirror / 'Cargo.lock').read_bytes() == (build / 'source/zeron/Cargo.lock').read_bytes(), 'Cargo changed the prepared lock'
        for root, name in [(mirror, 'zeron'), (gpui, 'gpui_windows')]:
            assert inventory(root) == json.loads((build / f'{name}-source-hashes.json').read_text()), f'{name} source drift during build'
        assert code == 0, f'Cargo offline/locked failed with exit {code}; no resolver fallback permitted'
        source_binary = ROOT / 'output/reference-native/target/debug/examples/renderer-device-native.exe'
        binary = build / 'renderer-device-native.exe'
        copy_file(source_binary, binary)
        result.update(status='built-native-pending', binaryPath=str(binary), binarySHA256=sha(binary), binaryBytes=binary.stat().st_size)
        for name, cmd in [('cargo-tree-target', ['cargo','tree','--offline','--locked','--target','x86_64-pc-windows-msvc','--manifest-path',str(mirror/'Cargo.toml'),'-p','zeron-ui','--features','compact-picker-fixture']),
                          ('cargo-tree-features', ['cargo','tree','--offline','--locked','--target','x86_64-pc-windows-msvc','--manifest-path',str(mirror/'Cargo.toml'),'-p','zeron-ui','--features','compact-picker-fixture','-e','features'])]:
            result[name+'ExitCode'] = run_logged(cmd, mirror, env, build/(name+'.txt'), build/(name+'.stderr.log'))
            assert result[name+'ExitCode'] == 0
        save(build / 'provenance.json', result)
        print(json.dumps(result, indent=2))
        return 0
    except Exception as error:
        result.update(status='failed-preserve-evidence', error=f'{type(error).__name__}: {error}')
        save(build / 'provenance.json', result)
        print(result['error'], file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
