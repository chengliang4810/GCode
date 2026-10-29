# R17 isolated feature-level control (source only)

This is a separate diagnostic project. The original R16 source, executable and readbacks are preserved byte-for-byte. No Cargo command or binary was executed during preparation.

`--requested-feature-level baseline` is the default and preserves the original ordered `[11_1,11_0]` device request. Explicit `11_1` or `11_0` requests exactly one level and rejects any different actual `ID3D11Device::GetFeatureLevel`. The creation out-parameter is cross-checked. The driver query uses the actual device's `IDXGIDevice` adapter and the correct `IDXGIDevice::IID`. Creation flags remain BGRA_SUPPORT; actual flags are separately queried. Canonical IUnknown identity of this device and immediate-context GetDevice is compared and recorded with PID; local RAII references do not escape, and address equality is a per-process snapshot only.

After owner coordination, build using `build.ps1 -BuildDirectory output/native-blend-readback-r17-build-attempt01`. It uses a fresh private Cargo cache/target, only existing locked cached dependencies, offline/locked and two jobs. It never runs the probe. No shared target or old export is changed.

Use the same new EXE for two fresh no-window output directories, with `--requested-feature-level 11_1` and `--requested-feature-level 11_0`. Retain actual chosen level, LUID, PCI identity, driver and shader blob hashes. Reject unsupported levels. Only compare outputs when adapter identity, compiler DLL, shader profiles/flags, all54 cases, format/state/input bits and actual blend descriptors match; differing debug shader bytes must be reported rather than hidden.

HLSL, BGRA8_UNORM/RGBA32_FLOAT targets, 8x8 geometry, clears, washes, all three blend descriptions, VS/PS profiles/flags and 54-case loops remain unchanged. This new hardware device is an isolated feature-level control, not the GUI's actual device or a WGC producer-frame query. Default fallback semantics are retained; improved driver metadata is an explicit R17 diagnostic change.

Pending: Rust compilation/type checking, strict CLI failure checks, actual runs, dependency tree/source/binary seal and comparison of both levels. No numerical result is predicted or claimed here.
