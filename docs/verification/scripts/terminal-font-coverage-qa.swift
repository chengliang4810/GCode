#!/usr/bin/env swift

// Deterministic CoreText coverage probe for the native terminal's likely
// prompt glyphs. It never starts a shell, reads shell configuration, reads
// environment variables, opens a window, or captures prompt bytes.

import AppKit
import CoreText
import Foundation

private let pointSize: CGFloat = 13

private struct Probe {
    let label: String
    let scalar: UnicodeScalar
    let expectedInPowerline: Bool
}

private struct FontCase {
    let label: String
    let font: CTFont
}

private struct Options {
    var requirePUA = false
    var fontFile: URL?
    var cascadeFamily: String?
}

private struct Coverage {
    let glyph: CGGlyph
    let direct: Bool
    let resolvedGlyph: CGGlyph
    let resolved: Bool
    let resolvedFamily: String?
    let shapedFamily: String?
    let cascadeFamily: String?
    let cascadeCount: Int
}

private let probes: [Probe] = [
    Probe(label: "ASCII A", scalar: UnicodeScalar(0x0041)!, expectedInPowerline: true),
    Probe(label: "CJK 中", scalar: UnicodeScalar(0x4E2D)!, expectedInPowerline: false),
    Probe(label: "Powerline E0A0", scalar: UnicodeScalar(0xE0A0)!, expectedInPowerline: true),
    Probe(label: "Powerline E0B0", scalar: UnicodeScalar(0xE0B0)!, expectedInPowerline: true),
    Probe(label: "FontAwesome F07C", scalar: UnicodeScalar(0xF07C)!, expectedInPowerline: false),
    Probe(label: "FontAwesome F120", scalar: UnicodeScalar(0xF120)!, expectedInPowerline: false),
]

private let requiredPUAScalars: Set<UInt32> = [0xE0A0, 0xE0B0, 0xF07C, 0xF120]

private func usage() {
    print("usage: terminal-font-coverage-qa.swift [--require-pua] [--font-file TTF] [--cascade-family FAMILY]")
    print("  --require-pua       fail when system monospace leaves any target PUA glyph missing")
    print("  --font-file TTF     register a font URL with CTFontManagerScope.process")
    print("  --cascade-family F  add F as an explicit cascade after system monospace")
}

private func parseOptions() -> Options {
    var options = Options()
    let arguments = CommandLine.arguments
    var index = 1
    while index < arguments.count {
        switch arguments[index] {
        case "--help", "-h":
            usage()
            exit(0)
        case "--require-pua":
            options.requirePUA = true
        case "--font-file":
            guard index + 1 < arguments.count else {
                fputs("RED missing value for --font-file\n", stderr)
                exit(2)
            }
            index += 1
            options.fontFile = URL(fileURLWithPath: arguments[index])
        case "--cascade-family":
            guard index + 1 < arguments.count else {
                fputs("RED missing value for --cascade-family\n", stderr)
                exit(2)
            }
            index += 1
            options.cascadeFamily = arguments[index]
        default:
            fputs("RED unknown option: \(arguments[index])\n", stderr)
            usage()
            exit(2)
        }
        index += 1
    }
    return options
}

private func registerProcessFont(_ url: URL) {
    var error: Unmanaged<CFError>?
    guard CTFontManagerRegisterFontsForURL(url as CFURL, .process, &error) else {
        let detail = error?.takeRetainedValue().localizedDescription ?? "unknown CoreText error"
        fputs("RED font-file-registration scope=process path=\(url.path) error=\(detail)\n", stderr)
        exit(2)
    }
    print("font-file-registration=GREEN scope=process path=\(url.path)")
}

private func familyName(_ font: CTFont) -> String {
    CTFontCopyFamilyName(font) as String
}

private func postScriptName(_ font: CTFont) -> String {
    CTFontCopyPostScriptName(font) as String
}

private func glyphID(_ font: CTFont, scalar: UnicodeScalar) -> CGGlyph {
    var character = UniChar(scalar.value)
    var glyph: CGGlyph = 0
    _ = CTFontGetGlyphsForCharacters(font, &character, &glyph, 1)
    return glyph
}

private func defaultCascade(_ font: CTFont) -> [(font: CTFont, family: String)] {
    guard let descriptors = CTFontCopyDefaultCascadeListForLanguages(font, ["en-US"] as CFArray) else {
        return []
    }
    let array = descriptors as NSArray
    return array.compactMap { value in
        let descriptor = value as! CTFontDescriptor
        let cascadeFont = CTFontCreateWithFontDescriptor(descriptor, pointSize, nil)
        return (cascadeFont, familyName(cascadeFont))
    }
}

private func fallbackFont(_ font: CTFont, scalar: UnicodeScalar) -> CTFont {
    CTFontCreateForStringWithLanguage(
        font,
        String(scalar) as CFString,
        CFRangeMake(0, 1),
        nil
    )
}

private func shapedFont(_ font: CTFont, scalar: UnicodeScalar) -> (glyph: CGGlyph, family: String?) {
    let text = String(scalar)
    let attributed = NSAttributedString(string: text, attributes: [
        NSAttributedString.Key(kCTFontAttributeName as String): font,
    ])
    let line = CTLineCreateWithAttributedString(attributed)
    let runs = CTLineGetGlyphRuns(line) as NSArray
    guard let rawRun = runs.firstObject else {
        return (0, nil)
    }
    let run = rawRun as! CTRun
    guard CTRunGetGlyphCount(run) > 0 else { return (0, nil) }
    var glyph: CGGlyph = 0
    CTRunGetGlyphs(run, CFRangeMake(0, 1), &glyph)
    let attributes = CTRunGetAttributes(run) as NSDictionary
    let key = NSAttributedString.Key(kCTFontAttributeName as String)
    let runFont: CTFont?
    if let rawFont = attributes[key] {
        runFont = (rawFont as! CTFont)
    } else {
        runFont = nil
    }
    return (glyph, runFont.map(familyName))
}

private func coverage(_ font: CTFont, scalar: UnicodeScalar) -> Coverage {
    let glyph = glyphID(font, scalar: scalar)
    let cascade = defaultCascade(font)
    let cascadeHit = cascade.first(where: { glyphID($0.font, scalar: scalar) != 0 })?.family
    let resolvedFont = fallbackFont(font, scalar: scalar)
    let resolvedGlyph = glyphID(resolvedFont, scalar: scalar)
    let resolvedFamily = familyName(resolvedFont)
    let shaped = shapedFont(font, scalar: scalar)
    let resolved = resolvedGlyph != 0 && resolvedFamily != ".LastResort"
    return Coverage(
        glyph: glyph,
        direct: glyph != 0,
        resolvedGlyph: resolvedGlyph,
        resolved: resolved,
        resolvedFamily: resolvedFamily,
        shapedFamily: shaped.glyph == 0 ? nil : shaped.family,
        cascadeFamily: cascadeHit,
        cascadeCount: cascade.count
    )
}

private func displayScalar(_ scalar: UnicodeScalar) -> String {
    String(format: "U+%04X", scalar.value)
}

private func status(_ value: Bool) -> String {
    value ? "GREEN" : "RED"
}

private func explicitCascadeFont(_ base: CTFont, family: String) -> CTFont? {
    guard let cascade = CTFontCreateWithName(family as CFString, pointSize, nil) as CTFont? else {
        return nil
    }
    let cascadeDescriptor = CTFontCopyFontDescriptor(cascade)
    let attributes = [kCTFontCascadeListAttribute as String: [cascadeDescriptor]]
    let descriptor = CTFontDescriptorCreateWithAttributes(attributes as CFDictionary)
    return CTFontCreateCopyWithAttributes(base, pointSize, nil, descriptor)
}

private func makeFontCases(options: Options) -> [FontCase] {
    let systemFont = NSFont.monospacedSystemFont(ofSize: pointSize, weight: .regular)
    let systemCTFont = CTFontCreateWithName(systemFont.fontName as CFString, pointSize, nil)
    var cases = [FontCase(label: "system monospace", font: systemCTFont)]
    if let menlo = CTFontCreateWithName("Menlo" as CFString, pointSize, nil) as CTFont? {
        cases.append(FontCase(label: "Menlo", font: menlo))
    }
    if let cascadeFamily = options.cascadeFamily {
        if let cascaded = explicitCascadeFont(systemCTFont, family: cascadeFamily) {
            cases.append(FontCase(label: "system monospace + explicit cascade \(cascadeFamily)", font: cascaded))
        } else {
            fputs("RED cascade-family-not-found family=\(cascadeFamily)\n", stderr)
            exit(2)
        }
    }

    // Use the CoreText font registry only. This avoids filesystem scans,
    // shell configuration, environment access, and user prompt data.
    let families = (CTFontManagerCopyAvailableFontFamilyNames() as NSArray)
        .compactMap { $0 as? String }
        .filter { $0.localizedCaseInsensitiveContains("powerline") }
        .sorted { $0.localizedCaseInsensitiveCompare($1) == .orderedAscending }
    let preferred = ["Meslo LG S for Powerline", "Source Code Pro for Powerline"]
    let selected = preferred.filter { families.contains($0) }
    for family in selected.isEmpty ? Array(families.prefix(2)) : selected {
        if let font = CTFontCreateWithName(family as CFString, pointSize, nil) as CTFont? {
            cases.append(FontCase(label: "installed Powerline \(family)", font: font))
        }
    }
    return cases
}

private let options = parseOptions()
if let fontFile = options.fontFile {
    registerProcessFont(fontFile)
}
private let fontCases = makeFontCases(options: options)
print("probe=CoreText-glyph-coverage")
print("macOS=\(ProcessInfo.processInfo.operatingSystemVersionString)")
print("size=13pt")
print("fixed-probes=\(probes.map { displayScalar($0.scalar) }.joined(separator: ","))")
print("font-cases=\(fontCases.count)")
let requirePUATargetLabel = options.cascadeFamily.map { "system monospace + explicit cascade \($0)" } ?? "system monospace"
print("require-pua=\(options.requirePUA)")
print("require-pua-target=\(requirePUATargetLabel)")
if let cascadeFamily = options.cascadeFamily {
    print("explicit-cascade-family=\(cascadeFamily)")
}
let selectedPowerlineFamilies = fontCases.filter { $0.label.hasPrefix("installed Powerline ") }.map { familyName($0.font) }
print("selected-Powerline-families=\(selectedPowerlineFamilies.joined(separator: ","))")

var requiredFailure = false
var requirePUAFailure = false
var powerlineCaseCount = 0
var powerlineSeparatorCoverage = Set<UInt32>()

for fontCase in fontCases {
    let family = familyName(fontCase.font)
    let postScript = postScriptName(fontCase.font)
    let isInstalledPowerline = fontCase.label.hasPrefix("installed Powerline ")
    let isExplicitCascade = fontCase.label.hasPrefix("system monospace + explicit cascade ")
    let isPUAFont = isInstalledPowerline || isExplicitCascade
    if isPUAFont { powerlineCaseCount += 1 }
    print("\nFONT label=\"\(fontCase.label)\" family=\"\(family)\" postscript=\"\(postScript)\"")
    for probe in probes {
        let result = coverage(fontCase.font, scalar: probe.scalar)
        let cascade = result.cascadeFamily ?? "none"
        let shaped = result.shapedFamily ?? "none"
        let resolvedFamily = result.resolvedFamily ?? "none"
        let directStatus = status(result.direct)
        let resolvedStatus = status(result.resolved)
        print("  direct=\(directStatus) resolved=\(resolvedStatus) \(displayScalar(probe.scalar)) \(probe.label) glyph=\(result.glyph) resolvedGlyph=\(result.resolvedGlyph) resolvedFamily=\"\(resolvedFamily)\" ctLineFamily=\"\(shaped)\" cascadeHit=\"\(cascade)\" cascadeCount=\(result.cascadeCount)")

        if !isPUAFont && (probe.label == "ASCII A" || probe.label == "CJK 中") && !result.resolved {
            requiredFailure = true
        }
        if options.requirePUA && fontCase.label == requirePUATargetLabel && requiredPUAScalars.contains(probe.scalar.value) {
            let directGlyphRequired = options.cascadeFamily == nil
            if (directGlyphRequired && result.glyph == 0) || result.resolvedGlyph == 0 || !result.resolved || result.resolvedFamily == ".LastResort" {
                requirePUAFailure = true
            }
        }
        if isPUAFont && probe.expectedInPowerline && result.direct {
            powerlineSeparatorCoverage.insert(probe.scalar.value)
        }
    }
}

if powerlineCaseCount == 0 {
    requiredFailure = true
    print("\nRED summary=no-installed-Powerline-family")
} else if !powerlineSeparatorCoverage.contains(0xE0A0) || !powerlineSeparatorCoverage.contains(0xE0B0) {
    requiredFailure = true
    print("\nRED summary=installed-Powerline-family-missing-E0A0-or-E0B0")
} else {
    print("\nGREEN summary=system-and-Powerline-probes-complete")
}

if options.requirePUA {
    if requirePUAFailure {
        requiredFailure = true
        print("RED require-pua=\(requirePUATargetLabel)-missing-target-pua")
    } else {
        print("GREEN require-pua=\(requirePUATargetLabel)-resolves-all-target-pua")
    }
}

if requiredFailure {
    print("RESULT=RED")
    exit(1)
}
print("RESULT=GREEN")
