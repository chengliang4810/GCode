"""Verify isolated R17 build bytes and unchanged read-only inputs. Never runs GUI."""
import argparse
import hashlib
import json
from pathlib import Path
import struct
import sys

ROOT = Path(__file__).resolve().parents[3]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify_rows(root, rows):
    for row in rows:
        path = root / row['path']
        assert path.is_file() and path.stat().st_size == row['bytes'] and sha(path) == row['sha256'], f'byte drift: {path}'
    actual = {p.relative_to(root).as_posix() for p in root.rglob('*') if p.is_file()}
    assert actual == {row['path'] for row in rows}, f'file set drift: {root}'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--build-dir', required=True)
    args = parser.parse_args()
    build = Path(args.build_dir).resolve(strict=True)
    attempt = build.parent
    assert attempt.parent == ROOT / 'output' and attempt.name.startswith('renderer-device-r17-')
    dest = build / 'seal.json'
    assert not dest.exists(), 'retain existing seal'
    result = {'schemaVersion': 1, 'stage': 'R17', 'nativeRun': False,
              'verifierPath': str(Path(__file__).resolve()), 'verifierSHA256': sha(Path(__file__)),
              'cacheScope': 'Copied global git/db, git/checkouts, sparse-index and selected archive bytes checked before/after; not a trace of all global Cargo metadata writes. Private checkout origins retain read references to global cached Git db.'}
    try:
        provenance = json.loads((build / 'provenance.json').read_text())
        assert provenance['buildExitCode'] == 0 and provenance['status'] == 'built-native-pending'
        assert sha(Path(provenance['binaryPath'])) == provenance['binarySHA256']
        image = Path(provenance['binaryPath']).read_bytes()
        assert image[:2] == b'MZ'
        pe = struct.unpack_from('<I', image, 0x3c)[0]
        assert image[pe:pe+4] == b'PE\0\0'
        machine = struct.unpack_from('<H', image, pe+4)[0]
        assert machine == 0x8664, f'expected Windows amd64 diagnostic, got PE machine {machine:#x}'
        for name in ['zeron', 'gpui_windows']:
            rows = json.loads((build / (name + '-source-hashes.json')).read_text())
            verify_rows(attempt / name, rows)
            verify_rows(build / 'source' / name, rows)
        # Verify the actual historical helper snapshot, not today's helper
        # directory: later seal/review tools are deliberately independent.
        verify_rows(build / 'source/templates', json.loads((build / 'templates-source-hashes.json').read_text()))
        frozen = ROOT / 'output/reference-sources/zeron'
        guards = json.loads((attempt / 'evidence/production-guards.json').read_text())
        assert len(guards) == 15
        for row in guards:
            original = (frozen / row['path']).read_bytes()
            assert hashlib.sha256(original).hexdigest() == row['frozenSHA256']
            actual = (build / 'source/zeron' / row['path']).read_bytes()
            assert actual == original + row['allowedDelta'].encode('utf-8'), f'production prefix/guard differs: {row["path"]}'
            assert hashlib.sha256(actual).hexdigest() == row['exportSHA256']
        gpui_original = Path('C:/Users/chengliang/.cargo/git/checkouts/zui-05467652636d6f6d/b564a34/crates/gpui_windows')
        gpui_guards = json.loads((attempt / 'evidence/mirror-prepared.json').read_text())['gpuiAppendedGuards']
        assert set(gpui_guards) == {'src/directx_renderer.rs', 'src/gpui_windows.rs'}
        for name, guard in gpui_guards.items():
            assert (build / 'source/gpui_windows' / name).read_bytes() == (gpui_original / name).read_bytes() + guard.encode('utf-8')
        assert (build / 'source/templates/gpui-device-bridge.rs').read_bytes() == (build / 'source/gpui_windows/src/directx_renderer/renderer_device_identity.rs').read_bytes()
        assert (build / 'source/templates/native-metadata.rs').read_bytes() == (build / 'source/zeron/crates/ui/examples/renderer-device-native-metadata.rs').read_bytes()
        verify_rows(ROOT / 'output/reference-native-0.2.128', json.loads((attempt / 'evidence/zeron-export-before.json').read_text()))
        verify_rows(Path('C:/Users/chengliang/.cargo/git/checkouts/zui-05467652636d6f6d/b564a34/crates/gpui_windows'), json.loads((attempt / 'evidence/gpui-windows-before.json').read_text()))
        seed = attempt / 'build-attempt01'
        global_cache = Path.home() / '.cargo'
        for name in ['git/db', 'git/checkouts', 'registry/index/index.crates.io-1949cf8c6b5b557f']:
            verify_rows(global_cache / name, json.loads((seed / (name.replace('/', '__') + '-input.json')).read_text()))
        for row in json.loads((seed / 'cached-registry-archives.json').read_text()):
            assert sha(global_cache / row['path']) == row['sha256']
        evidence_paths = ['provenance.json', 'zeron-source-hashes.json', 'gpui_windows-source-hashes.json', 'templates-source-hashes.json',
                          'build.stdout.log', 'build.stderr.log', 'cargo-tree-target.txt', 'cargo-tree-features.txt']
        result.update(status='build-bytes-and-read-only-inputs-verified-native-pending',
                      binaryPath=provenance['binaryPath'], binarySHA256=provenance['binarySHA256'],
                      peMachineHex=f'0x{machine:04x}',
                      unchangedOldExport=True, unchangedGlobalGpuiSource=True, unchangedGlobalCargoCopiedInputs=True,
                      unchangedCompiledSourceAndFullByteBackups=True,
                      originalProductionMethodBodiesChanged=False,
                      productionBodyPrefixAndExactGuardRecheckCount=len(guards), gpuiExactAppendGuardRecheckCount=len(gpui_guards),
                      evidence=[{'path': str(build / path), 'sha256': sha(build / path)} for path in evidence_paths],
                      evidenceBoundary=['New instrumented diagnostic GUI only; cannot retroactively identify old R16 EXEs.',
                                        'No native run or actual renderer LUID/FL measured by this build seal.',
                                        'Before/after canonical IUnknown address snapshots do not prove uninterrupted lifetime or rule out same-address reuse.',
                                        'Overlay match checks swapchain.GetDevice only; it does not independently identify overlay RTV/texture.'])
        code = 0
    except Exception as error:
        result.update(status='failed-preserve-evidence', error=f'{type(error).__name__}: {error}')
        code = 1
    dest.write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(result, indent=2))
    return code


if __name__ == '__main__':
    sys.exit(main())
