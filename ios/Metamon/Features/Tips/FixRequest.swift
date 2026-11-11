import Foundation

/// 「動かない時は話しかけると直せる」の案内と、「直してと頼む」で入力欄に入れる文(テスターの声: 直せると知らなかった)。
/// 直し方の例(`FirstEditHintCard`)・頼み方のコツ・動きの失敗のトーストから使う。送信はしない(入力欄に入れるだけ)。
enum FixRequest {
    /// 案内の一文(頼み方のコツと同じ文。Mac と共有する `PromptTips` に置く)
    static var hint: String { PromptTips.fixRequest }

    /// 入力欄に入れる依頼文。画面の名前・部品が分かれば入れ、出た文言があれば添える。
    static func draft(screenTitle: String?, part: String?, problem: String? = nil) -> String {
        let screen = screenTitle.flatMap(nonEmpty)
        let part = part.flatMap(nonEmpty)
        let base: String
        switch (screen, part) {
        case let (s?, p?): base = L10n.tr("「\(s)」の画面の「\(p)」が動かないから直して")
        case let (s?, nil): base = L10n.tr("「\(s)」の画面がうまく動かないから直して")
        case let (nil, p?): base = L10n.tr("「\(p)」が動かないから直して")
        case (nil, nil): base = L10n.tr("うまく動かない所があるから直して")
        }
        guard let problem = problem.flatMap(nonEmpty) else { return base }
        return base + L10n.tr("(「\(problem)」と出ます)")
    }

    private static func nonEmpty(_ s: String) -> String? {
        let t = s.trimmingCharacters(in: .whitespacesAndNewlines)
        return t.isEmpty ? nil : t
    }
}

/// 失敗のトーストに付ける「直してと頼む」(`AppRuntime.fixOffer`)。`message` がいまのトーストと同じ間だけ出す。
struct FixOffer: Equatable {
    var message: String
    var draft: String
}
