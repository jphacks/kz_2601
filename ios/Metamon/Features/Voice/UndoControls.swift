import SwiftUI

/// 「1 つ前に戻す」/「やり直す」の入口(#209)。仕組みは `AppRuntime.undo()` / `redo()`。
enum UndoCopy {
    static let undo: LocalizedStringKey = "元に戻す"
    static let undoPrevious: LocalizedStringKey = "1つ前に戻す"
    static let redo: LocalizedStringKey = "やり直す"
}

/// 変身直後の要約バナーに添えるボタン。修正の直後は「元に戻す」、戻した直後は「やり直す」。
struct SummaryBannerActionButton: View {
    @Environment(AppRuntime.self) private var rt
    let action: AppRuntime.SummaryBanner.Action

    var body: some View {
        Button {
            let version = rt.document.version // 押した時点の版(連打の 2 回目を捨てる)
            Task {
                switch action {
                case .undo: await rt.undo(expectedVersion: version)
                case .redo: await rt.redo(expectedVersion: version)
                }
            }
        } label: {
            Text(action == .undo ? UndoCopy.undo : UndoCopy.redo)
                .font(.subheadline.weight(.semibold))
                .lineLimit(1)
                .padding(.horizontal, Theme.Spacing.md)
                .padding(.vertical, Theme.Spacing.sm)
                .background(Theme.Colors.surface, in: Capsule())
                .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
        }
        .buttonStyle(.plain)
        .disabled(rt.isMorphing || (action == .undo && !rt.canUndo) || (action == .redo && rt.redoPoint == nil))
        .accessibilityIdentifier(action == .undo ? "banner.undo" : "banner.redo")
    }
}

/// 音声シート(作ったアプリの修正チャット)の上部に置く「1 つ前に戻す」。戻せる版が無い・戻している最中は押せない。
/// 戻した直後(シートを開き直した時)は「やり直す」も並べる。
struct UndoHeaderButtons: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        HStack(spacing: Theme.Spacing.sm) {
            Button { let v = rt.document.version; Task { await rt.undo(expectedVersion: v) } } label: {
                Label(UndoCopy.undoPrevious, systemImage: "arrow.uturn.backward")
                    .modifier(HeaderCapsule())
            }
            .disabled(!rt.canUndo || rt.isMorphing)
            .accessibilityIdentifier("voice.undo")
            if rt.redoPoint != nil {
                Button { let v = rt.document.version; Task { await rt.redo(expectedVersion: v) } } label: {
                    Label(UndoCopy.redo, systemImage: "arrow.uturn.forward")
                        .modifier(HeaderCapsule())
                }
                .disabled(rt.isMorphing)
                .accessibilityIdentifier("voice.redo")
            }
        }
    }
}

private struct HeaderCapsule: ViewModifier {
    @Environment(\.isEnabled) private var isEnabled

    func body(content: Content) -> some View {
        content
            .opacity(isEnabled ? 1 : 0.4) // 戻せない時は押せないと分かるよう薄く
            .font(.subheadline.weight(.medium))
            .foregroundStyle(.secondary)
            .lineLimit(1)
            .padding(.horizontal, Theme.Spacing.md)
            .padding(.vertical, Theme.Spacing.sm)
            .background(Theme.Colors.surface, in: Capsule())
            .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
    }
}
