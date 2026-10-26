import SwiftUI
import MetamonDSL

/// フォーム画面(`screen.entity` あり)の toolbar にある `saveRecord` を、ナビバーではなく画面下の固定バー(primary ボタン)に出す。
/// `safeAreaInset` なのでセル内スクロールの外に常に見え、キーボードが出てもその上に逃げる。
/// 押下中はスピナー、完了後は 0.6 秒チェックを見せてから元に戻る(画面が閉じればそのまま消える)。
/// 他人の共有レコード(#249・`FormDraft.isReadOnly`)ではボタンの代わりに「作った人だけが変更できます」を出す。
struct FormSaveBar: ViewModifier {
    @Environment(\.dslRuntime) private var rt
    let screen: Screen
    let ctx: RenderContext
    @State private var phase: Phase = .idle
    private let keyboard = KeyboardState.shared

    private enum Phase { case idle, saving, done }

    /// 画面下に出す toolbar 項目(= ナビバーからは外す)。
    static func pinned(in screen: Screen) -> [ToolbarButton] {
        guard screen.entity != nil else { return [] }
        return screen.toolbar.filter { $0.action.type == .saveRecord }
    }

    /// 画面のどこかに saveRecord がある(= 編集フォーム)。詳細表示だけの画面には理由を出さない。
    static func hasSave(in screen: Screen) -> Bool {
        screen.toolbar.contains { $0.action.type == .saveRecord }
            || screen.components.contains { c in
                c.action?.type == .saveRecord || c.children.contains { $0.action?.type == .saveRecord }
            }
    }

    func body(content: Content) -> some View {
        let pinned = Self.pinned(in: screen)
        content
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if ctx.form?.isReadOnly == true, Self.hasSave(in: screen) {
                    readOnlyNotice
                } else if let save = pinned.first {
                    bar(save)
                }
            }
    }

    private func bar(_ save: ToolbarButton) -> some View {
        Button {
            guard phase == .idle else { return }
            Task { await run(save.action) }
        } label: {
            HStack(spacing: RenderTheme.Spacing.sm) {
                switch phase {
                case .idle:
                    Image(systemName: save.icon ?? "checkmark")
                    Text(save.label)
                case .saving:
                    ProgressView().tint(.white)
                    Text(save.label)
                case .done:
                    Image(systemName: "checkmark.circle.fill")
                        .transition(.scale.combined(with: .opacity))
                    Text(L("保存しました"))
                }
            }
            .frame(maxWidth: .infinity)
        }
        .buttonStyle(RenderButtonStyle(kind: .primary))
        .disabled(phase != .idle)
        .animation(RenderTheme.Motion.select, value: phase)
        .sensoryFeedback(.success, trigger: phase) { _, new in new == .done }
        .accessibilityLabel(save.label)
        .padding(.horizontal, RenderTheme.Spacing.screenInset)
        .padding(.top, RenderTheme.Spacing.sm)
        .padding(.bottom, RenderTheme.Spacing.sm + (keyboard.isVisible ? KeyboardState.toolbarClearance : 0))
        .background {
            RenderTheme.Colors.canvas
                .overlay(alignment: .top) { Hairline() }
                .ignoresSafeArea(edges: .bottom) // ホームインジケータ下と、iOS 26 の透けるキーボードツールバーの裏まで塗る(下の欄が透けて見えていた・#196)
        }
    }

    private var readOnlyNotice: some View {
        Label(L("このデータは作った人だけが変更できます"), systemImage: "lock")
            .font(.subheadline)
            .foregroundStyle(.secondary)
            .frame(maxWidth: .infinity)
            .padding(.horizontal, RenderTheme.Spacing.screenInset)
            .padding(.vertical, RenderTheme.Spacing.md)
            .background {
                RenderTheme.Colors.canvas
                    .overlay(alignment: .top) { Hairline() }
                    .ignoresSafeArea(edges: .bottom)
            }
            .accessibilityIdentifier("form.readOnly")
    }

    private func run(_ action: Action) async {
        phase = .saving
        await rt.run(action, ctx.actionContext())
        phase = .done
        try? await Task.sleep(for: .seconds(RenderTheme.Motion.checkSeconds))
        phase = .idle
    }
}
