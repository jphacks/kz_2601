import SwiftUI

/// 「頼み方のコツ」(#415)。例をタップすると `onPick` に文を渡す(送らない。呼び出し側が入力欄に入れる)。
/// 設定からは push、音声画面の「頼み方のコツ」からは `PromptTipsSheet` で開く。
struct PromptTipsView: View {
    var availability: PromptTips.Availability = .current
    let onPick: (String) -> Void

    var body: some View {
        List {
            Section {
                Text(PromptTips.intro)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets(top: 0, leading: Theme.Spacing.xs, bottom: 0, trailing: Theme.Spacing.xs))
            }
            ForEach(PromptTips.categories(for: availability)) { category in
                Section {
                    ForEach(category.tips) { tip in
                        let text = tip.text
                        Button { onPick(text) } label: {
                            HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.sm) {
                                Text(text)
                                    .foregroundStyle(.primary)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                                Image(systemName: "arrow.down.left")
                                    .font(.caption.weight(.semibold))
                                    .foregroundStyle(Theme.Colors.brandText)
                                    .accessibilityHidden(true)
                            }
                            .contentShape(Rectangle())
                        }
                        .accessibilityHint(Text("入力欄に入れる"))
                        .accessibilityIdentifier("tips.example.\(tip.id)")
                    }
                } header: {
                    Label(category.title, systemImage: category.symbol)
                        .accessibilityIdentifier("tips.category.\(category.id)")
                }
            }
        }
        .navigationTitle(PromptTips.title)
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityIdentifier("tips.list")
    }
}

/// 音声画面から開くとき(閉じるボタン付き)。
struct PromptTipsSheet: View {
    @Environment(\.dismiss) private var dismiss
    let onPick: (String) -> Void

    var body: some View {
        NavigationStack {
            PromptTipsView(onPick: onPick)
                .toolbar {
                    ToolbarItem(placement: .topBarTrailing) { Button("閉じる") { dismiss() } }
                }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }
}

/// 音声画面の例文の見出しの横に置く小さなリンク。
struct PromptTipsLink: View {
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Label(PromptTips.title, systemImage: "lightbulb")
                .font(.caption.weight(.semibold))
                .foregroundStyle(Theme.Colors.brandText)
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("voice.tips")
    }
}

#Preview {
    NavigationStack { PromptTipsView { print($0) } }
}
