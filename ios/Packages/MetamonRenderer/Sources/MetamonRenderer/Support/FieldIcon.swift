import SwiftUI
import MetamonDSL
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif

/// 入力・stat の左に置く SF Symbol を field 型(と id / ラベルの語)から推定する。component に `icon` があればそれを優先。
/// 対応表は `docs/ios/design-system.md` §2.6。
enum FieldIcon {
    static func infer(component c: Component, field: Field?) -> String {
        let inferred = infer(field: field, hint: [c.field ?? "", c.label ?? "", field?.name ?? ""].joined(separator: " "))
        // DSL の icon は iOS 向けに書かれる。Mac に無い名前はこちらで推定した記号に落とす(#941)
        return resolve(c.icon, fallback: inferred)
    }

    static func infer(field: Field?, hint: String = "") -> String {
        let h = hint.lowercased()
        switch field?.type {
        case .currency: return "yensign"
        case .date: return "calendar"
        case .datetime: return h.contains("time") || h.contains("時") ? "clock" : "calendar.badge.clock"
        case .integer, .number:
            if h.contains("weight") || h.contains("重") { return "scalemass" }
            if h.contains("rep") || h.contains("回") { return "repeat" }
            if h.contains("step") || h.contains("歩") { return "figure.walk" }
            return "number"
        case .enum: return "list.bullet"
        case .photo: return "camera"
        case .video: return "video"
        case .audio: return "waveform"
        case .location: return "mappin.and.ellipse"
        case .boolean: return "checkmark.circle"
        case .text: return "note.text"
        default: break
        }
        if h.contains("memo") || h.contains("note") || h.contains("comment") || h.contains("メモ") || h.contains("ひとこと") { return "note.text" }
        if h.contains("pin") || h.contains("password") || h.contains("pass") || h.contains("ロック") || h.contains("暗証") { return "lock" }
        if h.contains("mail") || h.contains("メール") { return "envelope" }
        if h.contains("url") || h.contains("link") || h.contains("リンク") { return "link" }
        if h.contains("phone") || h.contains("tel") || h.contains("電話") { return "phone" }
        if h.contains("name") || h.contains("title") || h.contains("名") { return "textformat" }
        if h.contains("search") || h.contains("検索") { return "magnifyingglass" }
        return "pencil.line"
    }

    /// stat: aggregate から(sum は field 型のアイコン)。
    static func stat(aggregate: String?, field: Field?) -> String {
        switch aggregate {
        case "count": return "number"
        case "avg": return "chart.bar"
        case "max": return "arrow.up.right"
        case "min": return "arrow.down.right"
        case "distance": return "point.bottomleft.forward.to.point.topright.scurvepath"
        case "duration": return "clock"
        default: return infer(field: field, hint: field?.name ?? "")
        }
    }
}

// MARK: - その OS に無い記号の代わり(#941)

extension FieldIcon {
    /// `name` がこの OS の SF Symbols にあればそのまま、無ければ `fallback`(DSL は iOS 向けに書かれるので
    /// Mac に無い記号が来る)。空・nil も `fallback`。
    static func resolve(_ name: String?, fallback: String) -> String {
        guard let name, !name.isEmpty else { return fallback }
        return exists(name) ? name : fallback
    }

    /// この OS に実在する SF Symbol か。名前ごとに覚える(描画のたびに画像を作らない)。
    static func exists(_ name: String) -> Bool {
        if let known = symbolCache.value(name) { return known }
        #if canImport(UIKit)
        let found = UIImage(systemName: name) != nil
        #else
        let found = NSImage(systemSymbolName: name, accessibilityDescription: nil) != nil
        #endif
        symbolCache.set(name, found)
        return found
    }
}

/// 記号の有無の覚え書き。どのスレッドから引かれてもよいように鍵をかける。
private final class SymbolCache: @unchecked Sendable {
    private var known: [String: Bool] = [:]
    private let lock = NSLock()

    func value(_ name: String) -> Bool? {
        lock.lock()
        defer { lock.unlock() }
        return known[name]
    }

    func set(_ name: String, _ found: Bool) {
        lock.lock()
        defer { lock.unlock() }
        known[name] = found
    }
}

private let symbolCache = SymbolCache()
