import SwiftUI

/// 画面の並びの編集(#2323)の上部バー。iOS の `GridEditBar` と同じく「キャンセル」で捨て、「完了」で保存する。
/// Esc は「キャンセル」(cancelAction)、Return は「完了」(defaultAction)。
struct MacGridEditBar: View {
    let runtime: MacAppRuntime

    var body: some View {
        let changed = runtime.gridEdit?.draft.hasChanges ?? false
        HStack(spacing: 12) {
            Button("キャンセル") { runtime.cancelGridEdit() }
                .keyboardShortcut(.cancelAction)
                .disabled(runtime.gridEditSaving)
                .accessibilityIdentifier("gridEdit.cancel")
            Spacer(minLength: 0)
            Text("ドラッグで動かす・角で大きさ・右クリックで操作")
                .font(.callout)
                .macSecondaryText()
                .lineLimit(2)
                .multilineTextAlignment(.center)
            Spacer(minLength: 0)
            Button {
                Task { await runtime.finishGridEdit() }
            } label: {
                if runtime.gridEditSaving {
                    ProgressView().controlSize(.small)
                } else {
                    Text("完了").fontWeight(.semibold)
                }
            }
            .keyboardShortcut(.defaultAction)
            .buttonStyle(.borderedProminent)
            .tint(Theme.Colors.brandA)
            .disabled(runtime.gridEditSaving)
            .accessibilityIdentifier("gridEdit.done")
            .accessibilityHint(changed ? Text("配置を保存します") : Text(""))
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 8)
        .macRaisedSurface(in: Rectangle())
        .overlay(alignment: .bottom) { Divider() }
        .macTopTransition()
        .task { MacVoiceOver.announce(L10n.tr("画面の並びを編集しています")) }
    }
}

/// ツールバーの「画面の並びを編集」(#2323)。表示メニューと同じ条件で押せる
struct MacGridEditToolbar: ToolbarContent {
    let runtime: MacAppRuntime

    var body: some ToolbarContent {
        ToolbarItem(placement: .secondaryAction) {
            Button { runtime.startGridEdit() } label: {
                Label("画面の並びを編集", systemImage: "hand.draw")
            }
            .help("部品をドラッグして画面の並びを変える")
            .disabled(!runtime.canStartGridEdit)
            .accessibilityIdentifier("toolbar.gridEdit")
        }
    }
}
