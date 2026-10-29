#pragma once
#include <stddef.h>
#include <stdint.h>

/* Test-only, synchronous; call on a background thread and retain this DLL
 * until process exit. UTF-8 path is length-delimited and must not contain NUL.
 * error is caller-owned, NUL-terminated, and at most error_capacity bytes.
 * 0=success, 1=error, 2=contained bridge panic, 3=invalid buffer size.
 * The first-frame timeout does not bound WGC initialization or stop/join. */
uint32_t __stdcall zeron_fixture_wgc_abi_version(void);
uint32_t __stdcall zeron_fixture_wgc_capture(
    uintptr_t hwnd, const uint8_t *path, size_t path_len,
    uint32_t timeout_ms, uint8_t *error, size_t error_capacity);
