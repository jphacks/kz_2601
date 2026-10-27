import SwiftUI

/// 初回のチュートリアル(#274): 3 枚のカード。スキップでき、設定の「使い方を見る」から何度でも見られる。
struct TutorialView: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.colorScheme) private var scheme
    @State private var page = 0

    private var cards: [FirstRunTutorial.Card] {
        FirstRunTutorial.cards(voiceModelMegabytes: AppRuntime.voiceModelMegabytes)
    }
    private var isLast: Bool { page >= cards.count - 1 }

    var body: some View {
        ZStack {
            Theme.backdrop(scheme)
            VStack(spacing: 0) {
                HStack {
                    Spacer()
                    Button("スキップ") { rt.finishTutorial() }
                        .font(.subheadline.weight(.medium))
                        .foregroundStyle(.secondary)
                        .opacity(isLast ? 0 : 1)
                        .disabled(isLast)
                        .accessibilityIdentifier("tutorial.skip")
                }
                .padding(.horizontal, Theme.Spacing.lg)
                .padding(.top, Theme.Spacing.sm)

                TabView(selection: $page) {
                    ForEach(Array(cards.enumerated()), id: \.element.id) { index, card in
                        TutorialCardView(card: card).tag(index)
                    }
                }
                .tabViewStyle(.page(indexDisplayMode: .never))
                .animation(.easeInOut(duration: 0.3), value: page)

                pageDots
                    .padding(.bottom, Theme.Spacing.md)

                Button {
                    if isLast { rt.finishTutorial() } else { withAnimation { page += 1 } }
                } label: {
                    Text(isLast ? "はじめる" : "次へ").frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
                .tint(Theme.Colors.brandA)
                .padding(.horizontal, Theme.Spacing.lg)
                .padding(.bottom, Theme.Spacing.lg)
                .accessibilityIdentifier(isLast ? "tutorial.start" : "tutorial.next")
            }
        }
    }

    private var pageDots: some View {
        HStack(spacing: Theme.Spacing.sm) {
            ForEach(cards.indices, id: \.self) { index in
                Capsule()
                    .fill(index == page ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(Theme.Colors.hairline))
                    .frame(width: index == page ? 22 : 8, height: 8)
            }
        }
        .animation(.spring(duration: 0.3), value: page)
        .accessibilityElement()
        .accessibilityLabel(Text("\(page + 1) / \(cards.count)"))
    }
}

private struct TutorialCardView: View {
    let card: FirstRunTutorial.Card

    var body: some View {
        ScrollView {
            VStack(spacing: Theme.Spacing.lg) {
                ZStack(alignment: .bottomTrailing) {
                    CraneMascot(pose: card.pose, size: 132, sway: 6)
                    Image(systemName: card.symbol)
                        .font(.title2.weight(.semibold))
                        .foregroundStyle(.white)
                        .frame(width: 52, height: 52)
                        .background(Theme.brandGradient, in: Circle())
                        .overlay(Circle().strokeBorder(.white.opacity(0.4), lineWidth: 2))
                        .offset(x: 12, y: 6)
                }
                .padding(.top, Theme.Spacing.xl)
                .accessibilityHidden(true)

                Text(card.title)
                    .font(.system(size: 26, weight: .bold, design: .rounded))
                    .multilineTextAlignment(.center)
                    .accessibilityAddTraits(.isHeader)
                Text(card.body)
                    .font(.body)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                if let note = card.note {
                    Label(note, systemImage: "keyboard")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                        .metamonCard(padding: Theme.Spacing.md)
                }
            }
            .padding(.horizontal, Theme.Spacing.lg)
            .frame(maxWidth: .infinity)
        }
        .scrollBounceBehavior(.basedOnSize)
        .accessibilityIdentifier("tutorial.card.\(card.id)")
    }
}

/// 最初のアプリができた直後に 1 回だけ出す「こう話すと直せます」(#274)。マイク FAB の上に出す。
struct FirstEditHintCard: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.md) {
            HStack(alignment: .top, spacing: Theme.Spacing.sm + 2) {
                CraneMascot(pose: .point, size: 48, sway: 4)
                VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                    Text("こう話すと直せます")
                        .font(.headline)
                    Text("画面のマイクをタップして、話しかけてください。")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                Spacer(minLength: 0)
            }
            Text(FirstRunTutorial.editHintExamples.map { L10n.tr("「\($0)」") }.joined(separator: " "))
                .font(.subheadline.weight(.medium))
                .foregroundStyle(Theme.Colors.brandText)
                .fixedSize(horizontal: false, vertical: true)
            HStack {
                Button("とじる") { withAnimation { rt.showFirstEditHint = false } }
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("editHint.close")
                Spacer()
                Button { withAnimation { rt.openVoiceFromEditHint() } } label: {
                    Label("話してみる", systemImage: "mic.fill")
                }
                .buttonStyle(.borderedProminent)
                .tint(Theme.Colors.brandA)
                .accessibilityIdentifier("editHint.talk")
            }
            .font(.subheadline.weight(.medium))
        }
        .metamonCard(padding: Theme.Spacing.md)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("editHint.card")
    }
}
