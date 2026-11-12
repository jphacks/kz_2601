import SwiftUI
import MetamonServices

/// 数え方は共有の `QuestionProgress`(Shared/Voice・#1211)。
extension AppRuntime.ChatBubble: QuestionProgressBubble {
    var isAssistantQuestion: Bool { role == .assistant && kind == .question }
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
