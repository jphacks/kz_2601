import SwiftUI
import MetamonDSL

/// `disclosure`: 折りたたみ。`title` がトグル行、`expanded` が初期状態(既定 false)。
struct DisclosureContainer: View {
    let c: Component
    let ctx: RenderContext
    @State private var isExpanded: Bool
    @Environment(\.formTabCoordinator) private var tabs

    init(c: Component, ctx: RenderContext) {
        self.c = c
        self.ctx = ctx
        _isExpanded = State(initialValue: c.expanded ?? false)
    }

    var body: some View {
        DisclosureGroup(isExpanded: $isExpanded) {
            VStack(alignment: .leading, spacing: c.containerSpacing) {
                ForEach(c.children) { DSLView(component: $0, ctx: ctx) }
            }
            .padding(.top, RenderTheme.Spacing.sm)
        } label: {
            Text(c.title ?? L("詳細")).font(RenderTheme.Fonts.cardTitle)
        }
        .tint(RenderTheme.Colors.brandA)
        #if os(macOS)
        .disclosureGroupStyle(MacDisclosureStyle())
        #endif
        .renderCard()
        // Mac の Tab の並び(#2065): 閉じている間は中の欄を飛ばす
        .onAppear { tabs?.noteDisclosure(c.id, expanded: isExpanded) }
        .onChange(of: isExpanded) { _, open in tabs?.noteDisclosure(c.id, expanded: open) }
    }
}

#if os(macOS)
/// Mac の標準の折りたたみは小さな三角だけが押せて、見出しの文字は押せない(#940)。
/// 行全体を 1 つのボタンにして、クリック・Tab で止めて Space / Return・VoiceOver の操作で開閉する(#937)。
struct MacDisclosureStyle: DisclosureGroupStyle {
    func makeBody(configuration: Configuration) -> some View {
        MacDisclosureBody(configuration: configuration)
    }
}

private struct MacDisclosureBody: View {
    let configuration: DisclosureGroupStyleConfiguration
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Button {
                withAnimation(reduceMotion ? nil : RenderTheme.Motion.select) { configuration.isExpanded.toggle() }
            } label: {
                HStack(spacing: RenderTheme.Spacing.sm) {
                    configuration.label
                    Spacer(minLength: 0)
                    Image(systemName: "chevron.right")
                        .font(.callout.weight(.semibold))
                        .foregroundStyle(RenderTheme.Colors.brandA)
                        .rotationEffect(.degrees(configuration.isExpanded ? 90 : 0))
                        .accessibilityHidden(true)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityValue(DisclosureAccessibility.value(expanded: configuration.isExpanded))
            .accessibilityHint(DisclosureAccessibility.hint(expanded: configuration.isExpanded))
            if configuration.isExpanded {
                configuration.content
            }
        }
    }
}
#endif

/// 折りたたみの VoiceOver の値とヒント(Mac の自前の行で使う)。
enum DisclosureAccessibility {
    static func value(expanded: Bool) -> String { expanded ? L("開いています") : L("閉じています") }
    static func hint(expanded: Bool) -> String { expanded ? L("閉じます") : L("開きます") }
}
