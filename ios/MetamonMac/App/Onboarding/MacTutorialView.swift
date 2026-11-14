import SwiftUI

extension FirstRunTutorial {
    /// Mac のカード(#971)。iOS の `cards` と同じ 1 枚で、「タップ」を「クリック」に。
    /// 音声モデルのダウンロードの補足は Mac に無いので付けない。
    static var macCards: [Card] {
        [
            Card(id: "create", symbol: "mic.fill", pose: .happy,
                 title: L10n.pick(L10n.tr("話すと、アプリができる"), review: L10n.tr("話すと、記録帳が用意できる")),
                 body: L10n.pick(L10n.tr("マイクをクリックして「家計簿が欲しい」のように話すだけ。文字で入力しても作れます。"), review: L10n.tr("マイクをクリックして「家計簿をつけたい」のように話すだけ。文字で入力しても用意できます。")),
                 note: nil),
        ]
    }
}

/// 初回のチュートリアル(#274)の Mac 版(#971)。シートで出し、設定の「使い方を見る」からも開ける。
/// iOS の横にめくる TabView は Mac に無いので「次へ」で進める。Return で進む・Esc でスキップ。
struct MacTutorialView: View {
    let onFinish: () -> Void
    @Environment(\.colorScheme) private var scheme
    @State private var page = 0

    private let cards = FirstRunTutorial.macCards
    private var isLast: Bool { page >= cards.count - 1 }

    var body: some View {
        VStack(spacing: MacTheme.Spacing.group) {
            if cards.indices.contains(page) {
                card(cards[page])
            }
            if cards.count > 1 {
                Text(verbatim: "\(page + 1) / \(cards.count)")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            HStack {
                if !isLast {
                    Button("スキップ") { onFinish() }
                        .keyboardShortcut(.cancelAction)
                        .accessibilityIdentifier("tutorial.skip")
                }
                Spacer()
                Button {
                    if isLast { onFinish() } else { withMacAnimation { page += 1 } }
                } label: {
                    Text(isLast ? "はじめる" : "次へ").frame(minWidth: 96)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
                .keyboardShortcut(.defaultAction)
                .accessibilityIdentifier(isLast ? "tutorial.start" : "tutorial.next")
            }
        }
        .padding(MacTheme.Spacing.window)
        .frame(width: 460)
        .background(Theme.backdrop(scheme))
        .interactiveDismissDisabled()
    }

    private func card(_ card: FirstRunTutorial.Card) -> some View {
        VStack(spacing: MacTheme.Spacing.group) {
            ZStack(alignment: .bottomTrailing) {
                CraneMascot(pose: card.pose, size: 112, sway: 6)
                Image(systemName: card.symbol)
                    .font(.title2.weight(.semibold))
                    .foregroundStyle(.white)
                    .frame(width: 44, height: 44)
                    .background(Theme.brandGradient, in: Circle())
                    .overlay(Circle().strokeBorder(.white.opacity(0.4), lineWidth: 2))
                    .offset(x: 10, y: 4)
            }
            .padding(.top, MacTheme.Spacing.group)
            .accessibilityHidden(true)

            Text(card.title)
                .font(.system(size: 22, weight: .bold, design: .rounded))
                .multilineTextAlignment(.center)
                .accessibilityAddTraits(.isHeader)
            Text(card.body)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
            if let note = card.note {
                Label(note, systemImage: "keyboard")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .metamonCard(padding: Theme.Spacing.md)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("tutorial.card.\(card.id)")
    }
}
