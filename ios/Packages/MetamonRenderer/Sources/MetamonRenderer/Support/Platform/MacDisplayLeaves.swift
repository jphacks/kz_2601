import Foundation
#if os(macOS)
import AppKit
#endif

/// Mac の表示 leaf・一覧 leaf のキー操作と記号の決まり(#941・#944)。判定だけの純粋な関数。iOS の描画からは呼ばない。

/// leaf が受けるキー(SwiftUI の `KeyPress` から作る。テストでは直接作る)
enum MacLeafKey: Equatable {
    case left, right, up, down, space, digit(Int), other
}

/// rating: ←↓ で 1 つ減らす・→↑ で 1 つ増やす・数字キーでその数(0 で未評価)。
enum MacRatingKeys {
    /// 押したキーの後の値。そのキーを使わなければ nil(ほかへ渡す)
    static func value(after key: MacLeafKey, current: Int, count: Int) -> Int? {
        let cur = min(max(current, 0), count)
        switch key {
        case .left, .down: return max(cur - 1, 0)
        case .right, .up: return min(cur + 1, count)
        case .digit(let d) where (0...9).contains(d): return min(d, count)
        default: return nil
        }
    }

    /// VoiceOver の値「5 段階中 3」の数字の部分
    static func adjusted(increment: Bool, current: Int, count: Int) -> Int {
        value(after: increment ? .right : .left, current: current, count: count) ?? current
    }
}

/// flashcards: Space でめくる・← で「まだ」・→ で「覚えた」(iOS の左右スワイプと同じ向き)。
enum MacFlashcardKeys {
    enum Command: Equatable { case flip, notYet, learned }

    static func command(for key: MacLeafKey) -> Command? {
        switch key {
        case .space: return .flip
        case .left: return .notYet
        case .right: return .learned
        default: return nil
        }
    }
}

/// calendar: 矢印で選んだ日を動かす(←→ 1 日・↑↓ 1 週)。月をまたいだら表示の月も付いていく。
enum MacCalendarKeys {
    static func move(_ selected: Date, by key: MacLeafKey, calendar cal: Calendar) -> Date? {
        let days: Int
        switch key {
        case .left: days = -1
        case .right: days = 1
        case .up: days = -7
        case .down: days = 7
        default: return nil
        }
        return cal.date(byAdding: .day, value: days, to: cal.startOfDay(for: selected))
    }
}

/// calendar の月の表を広いウインドウに合わせる。幅が広いほど日の枠を高くし、その日の記録の題を数件だけ出す。
enum MacCalendarLayout {
    /// 日の枠の高さ(iOS と同じ 36pt から、列の幅の 0.75 倍・最大 96pt まで)
    static func cellHeight(gridWidth: CGFloat) -> CGFloat {
        guard gridWidth.isFinite, gridWidth > 0 else { return 36 }
        return min(max((gridWidth / 7) * 0.75, 36), 96)
    }

    /// 枠の中に出す記録の題の数(高さ 60pt 未満は点だけ)
    static func titleLines(cellHeight: CGFloat) -> Int {
        guard cellHeight >= 60 else { return 0 }
        return min(Int((cellHeight - 28) / 16), 3)
    }
}

/// SF Symbols の名前を Mac で描ける名前にする。DSL(AI)が出す名前には Material 風の名前や
/// iOS でしか無い名前が混じるので、言い換えの表 → それでも無ければ部品ごとの既定の記号。
enum MacSymbolFallback {
    /// 無い名前 → Mac にある SF Symbol
    static let aliases: [String: String] = [
        "add": "plus",
        "baby": "figure.and.child.holdinghands",
        "calendar.fill": "calendar",
        "chart": "chart.bar",
        "chat": "bubble.left",
        "check": "checkmark",
        "close": "xmark",
        "delete": "trash",
        "edit": "pencil",
        "home": "house",
        "image": "photo",
        "jpysign": "yensign",
        "list": "list.bullet",
        "money": "banknote",
        "money.yen": "yensign",
        "music": "music.note",
        "people": "person.2",
        "search": "magnifyingglass",
        "settings": "gearshape",
        "settings.fill": "gearshape.fill",
        "share": "square.and.arrow.up",
        "smile": "face.smiling",
        "sunny": "sun.max",
        "user": "person",
        "wallet": "wallet.pass",
        "warning": "exclamationmark.triangle",
        "yen": "yensign",
    ]

    /// 描ける名前。`exists` は記号があるか(Mac は `NSImage(systemSymbolName:)`・テストで差し替える)
    static func resolve(_ name: String?, fallback: String, exists: (String) -> Bool) -> String {
        guard let name, !name.isEmpty else { return fallback }
        if exists(name) { return name }
        if let alias = aliases[name.lowercased()], exists(alias) { return alias }
        return fallback
    }

    #if os(macOS)
    static func exists(_ name: String) -> Bool {
        NSImage(systemSymbolName: name, accessibilityDescription: nil) != nil
    }
    #endif
}

/// 部品の記号の名前。iOS は DSL の名前をそのまま(無ければ既定)、Mac は描けない名前を言い換える。
enum PlatformSymbol {
    static func name(_ name: String?, fallback: String) -> String {
        #if os(macOS)
        MacSymbolFallback.resolve(name, fallback: fallback, exists: MacSymbolFallback.exists)
        #else
        name ?? fallback
        #endif
    }
}
