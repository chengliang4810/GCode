"""Refine only the new R17 example metadata after retaining build01 bytes."""
import argparse
import hashlib
import json
from pathlib import Path

root = Path(__file__).resolve().parents[3]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--attempt-root', required=True)
args = parser.parse_args()
attempt = Path(args.attempt_root).resolve(strict=True)
assert attempt.parent == root / 'output' and attempt.name.startswith('renderer-device-r17-')
old_build = attempt / 'build-attempt01'
provenance = json.loads((old_build / 'provenance.json').read_text())
assert 'buildExitCode' in provenance, 'do not modify a running build input'
evidence = attempt / 'evidence/metadata-refinement'
assert not evidence.exists(), 'preserve previous refinement evidence'
example = attempt / 'zeron/crates/ui/examples/renderer-device-native.rs'
before = example.read_bytes()
assert before == (old_build / 'source/zeron/crates/ui/examples/renderer-device-native.rs').read_bytes()
text = before.decode('utf-8')
replacements = [
    ('let (hwnd, mut metadata) = lossless_transport::describe(window)?;',
     'let (hwnd, mut metadata) = lossless_transport::describe(window)?;\n        metadata["renderer"] = "frozen production GPUI rendering methods with append-only actual-device diagnostic; new instrumented GUI binary".into();\n        metadata["diagnosticBinary"] = true.into();\n        metadata["historicalR16BinaryDeviceBound"] = false.into();'),
    ('"failed-native-activity-or-style-input-before"', '"failed-renderer-device-or-native-activity-or-style-input-before"'),
    ('"failed-native-activity-before"', '"failed-renderer-device-or-native-activity-before"'),
    ('"raw native activity before WGC is not own foreground/owner-active; scene {name}"', '"current renderer device query/crosscheck or native activity/style gate failed before WGC; scene {name}"'),
    ('"failed-native-activity-or-style-input-after"', '"failed-renderer-device-or-native-activity-or-style-input-after"'),
    ('"failed-native-activity-after"', '"failed-renderer-device-or-native-activity-after"'),
    ('"own foreground/owner-active gate failed after WGC; preserve raw PNG as failed evidence"', '"observed renderer device identity/crosscheck or native activity/style gate failed after WGC; preserve raw PNG as failed evidence"'),
    ('"raw native activity after WGC is not own foreground/owner-active; scene {name}"', '"observed renderer device identity/crosscheck or native activity/style gate failed after WGC; scene {name}"'),
]
for old, new in replacements:
    assert text.count(old) == 1, old
    text = text.replace(old, new, 1)
after = text.encode('utf-8')
evidence.mkdir()
(evidence / 'renderer-device-native.rs.before').write_bytes(before)
(evidence / 'renderer-device-native.rs.after').write_bytes(after)
example.write_bytes(after)
result = {'schemaVersion': 1, 'stage': 'R17', 'scope': 'only new diagnostic example labels; render logic and production method bytes unchanged',
          'beforeSHA256': hashlib.sha256(before).hexdigest(), 'afterSHA256': hashlib.sha256(after).hexdigest(),
          'helperSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'replacements': replacements,
          'build01Preserved': True, 'nativeRun': False}
(evidence / 'result.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
print(json.dumps(result, indent=2))
