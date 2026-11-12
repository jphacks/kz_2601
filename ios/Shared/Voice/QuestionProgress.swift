import Foundation
import MetamonServices

/// 「質問 n / 3」を数えられる吹き出し(iOS の `AppRuntime.ChatBubble`・Mac の `MacComposeSession.Bubble`)
protocol QuestionProgressBubble {
    /// AI の質問(kind == .question)の吹き出しか
    var isAssistantQuestion: Bool { get }
}

/// M5: 新しく作る時の聞き返しは最大 3 回。最新の質問の下に「質問 n / 3」を添え、あと何問かを見せる。
/// iOS と Mac の会話(#1211)で共有する。見た目(`QuestionProgressLabel`)は各 OS。
enum QuestionProgress {
    /// backend `internal/ai/chat.go` の `chatMaxTurns`(create の質問の上限)と同じ値
    static let maxQuestions = 3

    /// 今の質問が何問目か。新しく作る会話で、最新の吹き出しが待っている質問の時だけ(edit は上限が無いので出さない)。
    static func number<B: QuestionProgressBubble>(bubbles: [B], mode: ChatMode, latest: ChatTurn.Kind?) -> Int? {
        guard mode == .create, latest == .question, let last = bubbles.last, last.isAssistantQuestion else { return nil }
        let n = bubbles.filter(\.isAssistantQuestion).count
        return (1...maxQuestions).contains(n) ? n : nil
    }

    /// 「質問 2 / 3」
    static func label(_ number: Int) -> String {
        L10n.tr("質問 \(number) / \(maxQuestions)")
    }
}
