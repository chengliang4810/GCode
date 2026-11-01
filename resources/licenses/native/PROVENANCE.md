# Ghostty-VT native library provenance

The terminal uses the native library selected by the pinned MyGo's
`plugins/terminal/mygo-plugin.json`, copied verbatim as `ghostty-vt-manifest.json`.
Its source is **ghostty-org/ghostty commit
`befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028`**. The exact upstream root license is
MIT, copyright 2024 Mitchell Hashimoto, Ghostty contributors; see `Ghostty-MIT.txt`.

Primary root license:
https://raw.githubusercontent.com/ghostty-org/ghostty/befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028/LICENSE

MyGo builds the library with `zig build -Demit-lib-vt -Doptimize=ReleaseFast
-Dstrip=true`, at the source commit above. Its manifest pins the separate Windows
amd64 and macOS amd64/arm64 binary hashes; Universal packaging combines the two
Mac libraries. The app's release verifier checks architecture and the delivered
file checksum independently of this notice.

The pinned `GhosttyZig.zig` VT build imports uucode, and enables Wuffs for Kitty
graphics when configured; SIMD builds use simdutf and Highway. The following
original license texts are included for those identified source components:

| Component | Identified source | Included license |
| --- | --- | --- |
| uucode | `jacobsandlund/uucode` at `9d55524551411b493cca41ca06363625d90aff1e` | `uucode-LICENSE.md`; the linked Bjoern Hoehrmann MIT and Unicode V3 notices are also included |
| Highway | `google/highway` at `66486a10623fa0d72fe91260f96c892e41aceb06` | `Highway-Apache-2.0.txt` |
| simdutf | vendored `pkg/simdutf/vendor/simdutf.h` reports **9.0.0** | `simdutf-9.0.0-MIT.txt`, `simdutf-9.0.0-Apache-2.0.txt`, retrieved from the official v9.0.0 tag |
| Wuffs | mirror commit `7411f488fe2e2c205c3d3b3d28638b7356522930` | `Wuffs-MIT.txt`, `Wuffs-Apache-2.0.txt`, retrieved from `google/wuffs-mirror-release-c` at that exact commit |

The Ghostty simdutf package description still labels itself 5.2.8; that stale
description is not used to identify the bundled source. The actual vendored
header's `SIMDUTF_VERSION` value is 9.0.0. No 5.2.8 license is substituted here.
The Wuffs `pixels` archive is used only by its unit-test target in the inspected
build file and is not identified as a VT runtime component.

Source inspection paths at the pinned Ghostty commit:
`src/build/GhosttyZig.zig`, `src/build/SharedDeps.zig`, `build.zig.zon`,
`pkg/highway/build.zig.zon`, `pkg/wuffs/build.zig{,.zon}` and
`pkg/simdutf/vendor/simdutf.h`.

This is an inventory of the root license and identified VT components, not a
publisher-provided complete binary SBOM. Remaining toolchain/runtime attribution
limits are recorded in `../OPEN-ITEMS.md`.
