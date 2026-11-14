import Foundation

/// 生成中の経過段階(docs/backend/ai-eval 2026-10-24 §5 と 01 §4 の予算に対応)。
/// - 0〜10s  normal   : create p50 6.6s / p95 8.7s。数秒ごとに文言を回す
/// - 10〜20s slow     : 「もう少しかかっています」(area 再生成 1 回 ≒ +7s)
/// - 20〜30s warning  : 警告色。backend は 20s で警告ライン
/// - 30s〜   fallback : backend がテンプレ DSL にフォールバック中。キャンセルを枠付きで目立たせる(HTTP は 45s で timeout)
/// キャンセルは最初から小さく出す(言い間違いに気づいたらすぐ止められる・UX 点検 M6)。
/// Mac と共有する(#967)。
enum GeneratingStage: Equatable {
    case normal, slow, warning, fallback

    init(elapsed: TimeInterval) {
        switch elapsed {
        case ..<10: self = .normal
        case ..<20: self = .slow
        case ..<30: self = .warning
        default: self = .fallback
        }
    }

    /// 文言表(docs/ios/architecture.md §6 と同期)
    static var normalMessages: [String] { [
        L10n.pick(L10n.tr("どんなアプリにするか考えています…"), review: L10n.tr("どんな記録帳にするか考えています…")),
        L10n.tr("必要な画面とデータを決めています…"),
        L10n.tr("画面を組み立てています…"),
        L10n.tr("細部を整えています…"),
    ] }

    func message(elapsed: TimeInterval, edit: Bool) -> String {
        switch self {
        case .normal:
            if edit { return L10n.tr("変更を反映しています…") }
            let i = Int(elapsed / 3) % Self.normalMessages.count
            return Self.normalMessages[i]
        case .slow: return L10n.tr("もう少しかかっています…")
        case .warning: return L10n.tr("時間がかかっています。もう少しだけお待ちください")
        case .fallback: return L10n.tr("混み合っています。安全なテンプレートに切り替えています…")
        }
    }

    /// キャンセルを枠付きで目立たせるか(M6)。それまでは小さい文字のボタン
    var cancelIsProminent: Bool { self == .fallback }

    /// 待ち時間の目安(M6)。create の最初の段だけ(create p50 6.6s / p95 8.7s)。過ぎたら段階文言に任せる
    func estimate(edit: Bool) -> String? {
        self == .normal && !edit ? L10n.tr("ふつう 10 秒ほど") : nil
    }
}
