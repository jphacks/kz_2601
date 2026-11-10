import SwiftUI
import MetamonDSL

/// 頼み方のコツ(#415・#967)。例文は iOS と同じ `PromptTips`。例を押すと `onPick` に渡す(会話の入力欄に入れる)。
/// ヘルプのメニューから開いた時(シート)は、押すと「直す」会話をその文で始める。
struct PromptTipsList: View {
    var onPick: (String) -> Void

    /// Mac で使える機能。共有・撮影・近くの端末・ヘルスケア・道のりの記録は Mac ではまだ無い
    static let availability = PromptTips.Availability.from(
        allowedComponents: ClientCapabilities.allowedComponents,
        disabledFeatures: ClientCapabilities.disabledFeatures, sharing: false)

    private var categories: [PromptTips.Category] {
        PromptTips.categories(for: Self.availability).map { category in
            var c = category
            c.tips = c.tips.filter { tip in tip.requires.isDisjoint(with: Self.unavailableOnMac) }
            return c
        }
        .filter { !$0.tips.isEmpty }
    }

    private static let unavailableOnMac: Set<PromptTips.Feature> = [.cameraCapture, .nearby, .health, .locationTracking]

    var body: some View {
        List {
            Section {
                Text(PromptTips.intro).foregroundStyle(.secondary)
            }
            ForEach(categories) { category in
                Section {
                    ForEach(category.tips) { tip in
                        Button {
                            onPick(tip.text)
                        } label: {
                            Text(tip.text).frame(maxWidth: .infinity, alignment: .leading).contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .help("入力欄に入れる")
                    }
                } header: {
                    Label(category.title, systemImage: category.symbol)
                }
            }
        }
        .navigationTitle(PromptTips.title)
    }
}

/// ヘルプのメニュー → 頼み方のコツ(シート)
struct PromptTipsSheet: View {
    var onPick: (String) -> Void
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text(PromptTips.title).font(.headline)
                Spacer()
                Button("閉じる") { dismiss() }.keyboardShortcut(.cancelAction)
            }
            .padding(.horizontal, MacTheme.Spacing.window)
            .padding(.vertical, MacTheme.Spacing.group)
            Divider()
            PromptTipsList(onPick: onPick)
        }
        .frame(minWidth: 480, minHeight: 520)
    }
}
