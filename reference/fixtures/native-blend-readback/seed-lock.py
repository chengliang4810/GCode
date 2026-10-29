"""Copy only exact package identities/checksums from the existing cached lock.

This is graph extraction, not dependency version resolution. cargo build remains
--offline --locked, so any unsupported lock/manifest change fails closed.
"""
from pathlib import Path
import hashlib
import json
import tomllib

root = Path(__file__).resolve().parents[3]
source = root / "reference/fixtures/native-wgc-bridge/Cargo.lock"
destination = Path(__file__).with_name("Cargo.lock")
if destination.exists():
    raise SystemExit("preserve existing lock; extraction only runs once")
raw = source.read_text(encoding="utf-8")
blocks = raw.split("[[package]]")[1:]
packages = {}
for block in blocks:
    package = tomllib.loads("[[package]]" + block)["package"][0]
    packages[(package["name"], package["version"])] = (package, block)
chosen = set()
def visit(name, version=None):
    matches = [key for key in packages if key[0] == name and (version is None or key[1] == version)]
    if len(matches) != 1:
        raise RuntimeError(f"ambiguous/missing existing identity: {name} {version}: {matches}")
    key = matches[0]
    if key in chosen:
        return
    chosen.add(key)
    for dep in packages[key][0].get("dependencies", []):
        parts = dep.split()
        visit(parts[0], parts[1] if len(parts) > 1 else None)
for name, version in [("anyhow", "1.0.104"), ("serde_json", "1.0.151"), ("sha2", "0.10.9"), ("windows", "0.61.3")]:
    visit(name, version)
output = "# Exact registry identity/checksum graph extracted without resolving new versions.\nversion = 4\n\n"
for key in sorted(chosen):
    output += "[[package]]" + packages[key][1].rstrip() + "\n\n"
output += '[[package]]\nname = "native-blend-readback"\nversion = "0.1.0"\ndependencies = [\n "anyhow",\n "serde_json",\n "sha2",\n "windows",\n]\n'
destination.write_text(output, encoding="utf-8", newline="\n")
Path(__file__).with_name("lock-origin.json").write_text(json.dumps({
    "source": str(source), "sourceSHA256": hashlib.sha256(source.read_bytes()).hexdigest(),
    "lockSHA256": hashlib.sha256(destination.read_bytes()).hexdigest(),
    "method": "exact existing lock graph extraction; no registry query/version selection/download",
    "packages": [{"name":key[0],"version":key[1],"checksum":packages[key][0].get("checksum")} for key in sorted(chosen)],
}, indent=2)+"\n", encoding="utf-8", newline="\n")
