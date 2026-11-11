import SwiftUI
import Observation

/// 完成の後のお知らせ(H7・#968)を Mac で 1 つずつ出す。順番は iOS と同じ `PostBuildNotice`(Shared)。
/// 今の Mac にあるのは要約(「元に戻す / やり直す」付き)と、最初のアプリの後の直し方の例(#1278)。
/// 公開の確認・保存期間・引き継ぎはそれぞれの機能が Mac に来たら `pending` に足す(順番はここで決まる)。
@MainActor
@Observable
final class MacPostBuildNotices {
    /// 要約のお知らせ。action はアプリの版の「元に戻す / やり直す」
    struct Summary: Identifiable, Equatable {
        enum Action: Equatable { case undo, redo }
        let id = UUID()
        var appId: String
        var text: String
        var action: Action?
    }

    var summary: Summary?
    /// 最初のアプリを作った後に 1 回だけ出す「こう頼むと直せます」(iOS の `FirstEditHintCard`・#1278)。要約が閉じてから出る
    var showsEditHint = false

    /// 要約を出しておく秒数。iOS の要約バナー(RootView)と同じ: 文字数に応じて 3〜5 秒・ボタン付きは 6 秒以上・VoiceOver の時は長く
    static func holdSeconds(_ summary: Summary) -> Double {
        MacVoiceOver.holdSeconds(max(summary.action == nil ? 3.0 : 6.0, min(5.0, Double(summary.text.count) / 12)))
    }

    var pending: Set<PostBuildNotice> {
        var pending: Set<PostBuildNotice> = []
        if summary != nil { pending.insert(.summary) }
        if showsEditHint { pending.insert(.editHint) }
        return pending
    }

    var active: PostBuildNotice? { PostBuildNotice.current(pending) }

    /// いま出ているお知らせを閉じる(閉じるボタン・Esc・時間)。閉じたら次の番のものが出る
    func dismissActive() {
        switch active {
        case .summary: summary = nil
        case .editHint: showsEditHint = false
        default: break
        }
    }
}

/// 中央の列の上に出す要約のお知らせ。閉じるボタン・Esc・時間で閉じる。
struct MacSummaryNoticeBar: View {
    let summary: MacPostBuildNotices.Summary
    var canUndo: Bool
    var canRedo: Bool
    var undo: () -> Void
    var redo: () -> Void
    var dismiss: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "checkmark.circle.fill").foregroundStyle(Theme.brandGradient)
            Text(summary.text)
                .font(.callout)
                .lineLimit(2)
                .textSelection(.enabled)
            switch summary.action {
            case .undo?:
                Button("元に戻す", action: undo)
                    .disabled(!canUndo)
                    .help(Text(verbatim: "⌥⌘Z"))
            case .redo?:
                Button("やり直す", action: redo)
                    .disabled(!canRedo)
                    .help(Text(verbatim: "⌥⇧⌘Z"))
            case nil:
                EmptyView()
            }
            Button(action: dismiss) { Image(systemName: "xmark") }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .keyboardShortcut(.cancelAction) // Esc で閉じる
                .help("閉じる")
                .accessibilityLabel("閉じる")
        }
        .padding(.horizontal, 14).padding(.vertical, 8)
        .background(.regularMaterial, in: Capsule())
        .overlay(Capsule().strokeBorder(.separator))
        .frame(maxWidth: 560)
        .transition(.move(edge: .top).combined(with: .opacity))
        .accessibilityElement(children: .contain)
    }
}

/// 最初のアプリの後の「こう頼むと直せます」(iOS の `FirstEditHintCard`・#1278)。iOS の「マイクをタップ」を ⌘K / ⇧⌘M の案内に言い換える。
/// 例文は iOS と同じ `FirstRunTutorial.editHintExamples`。時間では消さない(閉じるか、頼んでみるまで出す)
struct MacEditHintBar: View {
    var ask: () -> Void
    var dismiss: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            CraneMascot(pose: .point, size: 40, sway: 3)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 4) {
                Text("こう頼むと直せます").font(.headline)
                Text("⌘K で文字で、⇧⌘M で声で頼めます。")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                Text(FirstRunTutorial.editHintExamples.map { L10n.tr("「\($0)」") }.joined(separator: " "))
                    .font(.callout.weight(.medium))
                    .foregroundStyle(Theme.Colors.brandText)
                    .fixedSize(horizontal: false, vertical: true)
            }
            VStack(alignment: .trailing, spacing: 6) {
                Button("頼んでみる", action: ask)
                    .buttonStyle(.borderedProminent)
                    .accessibilityIdentifier("editHint.talk")
                Button("とじる", action: dismiss)
                    .keyboardShortcut(.cancelAction) // Esc で閉じる
                    .accessibilityIdentifier("editHint.close")
            }
        }
        .padding(.horizontal, 16).padding(.vertical, 12)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).strokeBorder(.separator))
        .frame(maxWidth: 560)
        .transition(.move(edge: .top).combined(with: .opacity))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("editHint.card")
    }
}
