import SwiftUI
import MetamonServices

/// M5: 新しく作る時の聞き返しは最大 3 回。最新の質問の下に「質問 n / 3」を添え、あと何問かを見せる。
enum QuestionProgress {
    /// backend `internal/ai/chat.go` の `chatMaxTurns`(create の質問の上限)と同じ値
    static let maxQuestions = 3

    /// 今の質問が何問目か。新しく作る会話で、最新の吹き出しが待っている質問の時だけ(edit は上限が無いので出さない)。
    static func number(bubbles: [AppRuntime.ChatBubble], mode: ChatMode, latest: ChatTurn.Kind?) -> Int? {
        guard mode == .create, latest == .question,
              let last = bubbles.last, last.role == .assistant, last.kind == .question else { return nil }
        let n = bubbles.filter { $0.role == .assistant && $0.kind == .question }.count
        return (1...maxQuestions).contains(n) ? n : nil
    }
}

/// 最新の質問の吹き出しの下に出す「質問 n / 3」
struct QuestionProgressLabel: View {
    let number: Int

    var body: some View {
        Text(L10n.tr("質問 \(number) / \(QuestionProgress.maxQuestions)"))
            .font(.caption.weight(.medium))
            .foregroundStyle(.secondary)
            .monospacedDigit()
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.leading, 32) // 吹き出しの本文に合わせる(AI の印の幅)
            .accessibilityIdentifier("chat.questionProgress")
    }
}
