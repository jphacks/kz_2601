import SwiftUI

/// 何も選んでいない時の中央: アプリを「作る」入口(iOS の起動画面「どんなアプリが欲しいですか?」に合わせる)。
/// 大きなボタンで話して / 文字で作る。例のチップは押すと文字で作る会話にその文を入れて開く(送らない)。
struct MacCreateLanding: View {
    var enabled = true
    var returning = Greeting.launchIsReturning
    let speak: () -> Void
    let type: () -> Void
    let example: (String) -> Void

    @Environment(\.colorScheme) private var scheme
    @State private var category = "record"

    private var copy: Greeting.Copy { Greeting.copy(for: returning ? .returning : .firstVisit) }
    private var categories: [Greeting.ExampleCategory] { Greeting.createExampleCategories() }
    private var examples: [String] { categories.first { $0.id == category }?.examples ?? [] }

    var body: some View {
        ScrollView {
            VStack(spacing: Theme.Spacing.lg) {
                CraneMascot(pose: .idea, size: MacTheme.Mascot.empty)
                VStack(spacing: Theme.Spacing.sm) {
                    Text(copy.title)
                        .font(.largeTitle.weight(.bold))
                        .multilineTextAlignment(.center)
                    // 下の一言は初回の文言(再訪の「マイクを長押し」は Mac に無い)
                    Text(Greeting.copy(for: .firstVisit).subtitle)
                        .font(.title3)
                        .macSecondaryText()
                        .multilineTextAlignment(.center)
                }
                .accessibilityElement(children: .combine)
                .accessibilityAddTraits(.isHeader)

                HStack(spacing: Theme.Spacing.md) {
                    Button(action: speak) {
                        Label("話して作る", systemImage: "mic.fill")
                            .frame(minWidth: 168)
                    }
                    .buttonStyle(LandingButtonStyle(prominent: true))
                    .help("声でアプリを作る")
                    .accessibilityIdentifier("landing.speak")
                    Button(action: type) {
                        Label("文字で作る", systemImage: "keyboard")
                            .frame(minWidth: 168)
                    }
                    .buttonStyle(LandingButtonStyle(prominent: false))
                    .help("文字でアプリを作る")
                    .accessibilityIdentifier("landing.type")
                }
                .disabled(!enabled)

                examplePicker
            }
            .padding(MacTheme.Spacing.window * 2)
            .frame(maxWidth: 640)
            .frame(maxWidth: .infinity)
        }
        .scrollBounceBehavior(.basedOnSize)
        .defaultScrollAnchor(.center)
        .background { Theme.backdrop(scheme) }
        .accessibilityIdentifier("shell.landing")
    }

    /// 例文: 上の行がカテゴリ(記録・習慣・お金・家族/友達と・ゲーム)、下の行がその例(iOS と同じ `Greeting.createExampleCategories`)
    private var examplePicker: some View {
        VStack(spacing: Theme.Spacing.sm) {
            Text("例えば")
                .font(.subheadline)
                .macSecondaryText()
            HStack(spacing: Theme.Spacing.xs) {
                ForEach(categories) { item in
                    let selected = item.id == category
                    Button {
                        withMacAnimation { category = item.id }
                    } label: {
                        Text(item.title)
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(selected ? Color.white : Color.secondary)
                            .padding(.horizontal, Theme.Spacing.sm + 4)
                            .padding(.vertical, Theme.Spacing.xs + 1)
                            .background { if selected { Capsule().fill(Theme.brandGradient) } }
                            .contentShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(selected ? .isSelected : [])
                    .accessibilityIdentifier("landing.category.\(item.id)")
                }
            }
            HStack(spacing: Theme.Spacing.sm) {
                ForEach(examples, id: \.self) { text in
                    Button { example(text) } label: {
                        Text(text)
                            .font(.body)
                            .lineLimit(1)
                            .padding(.horizontal, Theme.Spacing.md)
                            .padding(.vertical, Theme.Spacing.sm)
                            .background(Theme.Colors.surface, in: Capsule())
                            .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
                            .contentShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .help("この文で作り始める")
                    .accessibilityIdentifier("landing.example")
                }
            }
            .disabled(!enabled)
            .id(category)
            .transition(.opacity)
        }
    }
}

/// 作る入口の大きなボタン。主はブランドの塗り、副は面の色
private struct LandingButtonStyle: ButtonStyle {
    let prominent: Bool
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.title3.weight(.semibold))
            .foregroundStyle(prominent ? AnyShapeStyle(Color.white) : AnyShapeStyle(Theme.Colors.brandText))
            .padding(.horizontal, Theme.Spacing.lg)
            .padding(.vertical, Theme.Spacing.md - 2)
            .background {
                let shape = RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous)
                if prominent {
                    shape.fill(Theme.brandGradient)
                } else {
                    shape.fill(Theme.Colors.surface).overlay(shape.strokeBorder(Theme.Colors.hairline))
                }
            }
            .shadow(color: Theme.Shadow.softColor, radius: prominent ? 12 : 6, y: 4)
            .opacity(isEnabled ? (configuration.isPressed ? 0.8 : 1) : 0.5)
            .contentShape(RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
    }
}
