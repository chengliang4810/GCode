"""Validate actual R17 diagnostic sidecars after a parent-coordinated GUI run.

This is read-only except for a fresh result path. It never launches a process.
"""
import argparse
import hashlib
import json
from pathlib import Path


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_device(value, metadata):
    assert value['status'] == 'queried-current-renderer-device' and value['gatePassed'] is True
    assert value['processId'] == metadata['processId']
    assert value['hwnd'].lower() == metadata['hwnd'].lower()
    assert value['ownerThreadId'] > 0 and value['queryThreadId'] == value['ownerThreadId']
    assert int(value['deviceInstanceToken'], 16) != 0
    luid = value['adapter']['luid']
    assert luid['hex'].lower() == f"0x{luid['high'] & 0xffffffff:08x}{luid['low']:08x}"
    checks = value['crossChecks']
    for name in ['storedSelectedAdapterMatchesActualDevice', 'contextDeviceMatches', 'swapchainDeviceMatches', 'renderTargetDeviceMatches']:
        assert checks[name] is True, name
    assert checks['overlayDeviceMatches'] in (None, True)
    if checks['overlayAttached']:
        assert checks['overlayDeviceMatches'] is True
    assert value['actualRenderTarget']['sampleCount'] > 0 and value['actualSwapchain']['sampleCount'] > 0
    driver = value['driver']
    if driver['error'] is None:
        packed = int(driver['packedHex'], 16)
        assert driver['version'] == [(packed >> shift) & 0xffff for shift in (48, 32, 16, 0)]
    else:
        assert isinstance(driver['error'], str) and driver['version'] is None and driver['packedHex'] is None


parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--build-dir', required=True)
parser.add_argument('--run-dir', required=True)
parser.add_argument('--expected-scenes', required=True, type=int, choices=(2, 36))
parser.add_argument('--result', required=True)
args = parser.parse_args()
build, run, dest = Path(args.build_dir).resolve(strict=True), Path(args.run_dir).resolve(strict=True), Path(args.result).resolve()
assert not dest.exists(), 'preserve existing validation results'
result = {'schemaVersion': 1, 'stage': 'R17', 'verifier': str(Path(__file__).resolve()), 'verifierSHA256': sha(Path(__file__)),
          'nativeLaunchedByVerifier': False, 'historicalR16BinaryDeviceBound': False, 'visualParityAccepted': False}
try:
    provenance = json.loads((build / 'provenance.json').read_text())
    assert provenance['buildExitCode'] == 0 and provenance['status'] == 'built-native-pending'
    assert sha(Path(provenance['binaryPath'])) == provenance['binarySHA256']
    source = json.loads((build / 'source/zeron/crates/ui/examples/renderer-device-source-manifest.json').read_text())
    sidecars = sorted(run.glob('*.capture.json'))
    assert len(sidecars) == args.expected_scenes
    rows, observed = [], set()
    for path in sidecars:
        metadata = json.loads(path.read_text())
        assert metadata['status'] == 'captured' and metadata['captureStarted'] is True
        assert metadata['diagnosticBinary'] is True and metadata['historicalR16BinaryDeviceBound'] is False
        assert 'new instrumented GUI binary' in metadata['renderer']
        assert metadata['binarySHA256'] == provenance['binarySHA256'] and metadata['sourceManifest'] == source
        for name in ['nativeActivityBefore', 'nativeActivityAfter']:
            activity = metadata[name]
            assert activity['nativeWindowActive'] is True
            assert activity['processId'] == metadata['processId'] == activity['foregroundProcessId']
            assert activity['hwnd'].lower() == metadata['hwnd'].lower() == activity['foregroundHWND'].lower()
            assert activity['ownerProcessId'] == metadata['processId']
            assert activity['ownerThreadActiveIsOwn'] is True and activity['queriedOnOwnerUiThread'] is True
            assert activity['ownerThreadId'] == activity['queryThreadId']
        before, after = metadata['rendererDeviceBefore'], metadata['rendererDeviceAfter']
        validate_device(before, metadata)
        validate_device(after, metadata)
        stable = ['processId', 'hwnd', 'ownerThreadId', 'deviceInstanceToken', 'featureLevelHex', 'creationFlagsHex', 'adapter', 'actualRenderTarget', 'actualSwapchain']
        assert all(before[key] == after[key] for key in stable), 'before/after observed device identity changed'
        image = run / metadata['name']
        assert image.is_file(), 'sidecar image missing'
        observed.add((before['adapter']['luid']['hex'], before['featureLevelHex'], before['creationFlagsHex'], before['deviceInstanceToken']))
        rows.append({'sidecar': str(path), 'sidecarSHA256': sha(path), 'image': str(image), 'imageSHA256': sha(image),
                     'processId': before['processId'], 'hwnd': before['hwnd'], 'adapter': before['adapter'],
                     'featureLevelHex': before['featureLevelHex'], 'creationFlagsHex': before['creationFlagsHex'],
                     'actualRenderTarget': before['actualRenderTarget'], 'actualSwapchain': before['actualSwapchain'], 'driver': before['driver']})
    result.update(status='actual-current-renderer-sidecars-verified-no-parity-claim', binarySHA256=provenance['binarySHA256'],
                  sceneCount=len(rows), distinctObservedIdentityCount=len(observed), observedIdentities=[list(value) for value in sorted(observed)], scenes=rows,
                  limitations=['A new diagnostic binary does not retroactively identify old GUI binaries.',
                               'Equal pointer snapshots do not prove uninterrupted lifetime; LUID comparisons are machine/boot scoped.',
                               'This validates device metadata, not WGC timing, every PNG pixel, exact blend internals or visual parity.'])
    code = 0
except Exception as error:
    result.update(status='failed-preserve-evidence', error=f'{type(error).__name__}: {error}')
    code = 1
dest.parent.mkdir(parents=True, exist_ok=True)
dest.write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
print(json.dumps(result, indent=2))
raise SystemExit(code)
