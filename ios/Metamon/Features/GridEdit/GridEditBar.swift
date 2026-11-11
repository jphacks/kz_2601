import SwiftUI

/// 手で動かす編集モード(#1442)の上部バー。「キャンセル」で捨て、「完了」で保存する(新しい版になり「元に戻す」で戻せる)。
struct GridEditBar: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        let changed = rt.gridEdit?.draft.hasChanges ?? false
        HStack(spacing: Theme.Spacing.sm) {
            Button(L10n.tr("キャンセル")) { rt.cancelGridEdit() }
                .disabled(rt.gridEditSaving)
                .accessibilityIdentifier("gridEdit.cancel")
            Spacer(minLength: 0)
            Text(L10n.tr("ドラッグで動かす・角で大きさ・✕で消す"))
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(2)
                .multilineTextAlignment(.center)
            Spacer(minLength: 0)
            Button {
                Task { await rt.finishGridEdit() }
            } label: {
                if rt.gridEditSaving {
                    ProgressView().controlSize(.small)
                } else {
                    Text(L10n.tr("完了")).fontWeight(.semibold)
                }
            }
            .buttonStyle(.borderedProminent)
            .tint(Theme.Colors.brandA)
            .disabled(rt.gridEditSaving)
            .accessibilityIdentifier("gridEdit.done")
            .accessibilityHint(changed ? Text(L10n.tr("配置を保存します")) : Text(""))
        }
        .padding(.horizontal, Theme.Spacing.md)
        .padding(.vertical, Theme.Spacing.sm)
        .background(.regularMaterial)
        .overlay(alignment: .bottom) { Divider() }
    }
}

/// 修正チャット(音声シート)の上部に置く「編集」。押すとシートを閉じて、今の画面を手で動かすモードに入る。
/// 動かせる部品が無い画面(フォーム・area の無い画面)では出さない。
struct GridEditEntryButton: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        if rt.canStartGridEdit {
            Button { rt.startGridEdit() } label: {
                // 上部のボタンが多い時(対話中・狭い端末)はアイコンだけにする
                ViewThatFits(in: .horizontal) {
                    capsule(Label(L10n.tr("編集"), systemImage: "hand.draw").fixedSize())
                    capsule(Image(systemName: "hand.draw"))
                }
            }
            .disabled(rt.isMorphing)
            .accessibilityLabel(Text(L10n.tr("編集")))
            .accessibilityHint(Text(L10n.tr("部品を指で動かして配置を変えます")))
            .accessibilityIdentifier("voice.gridEdit")
        }
    }

    private func capsule(_ content: some View) -> some View {
        content
            .font(.subheadline.weight(.medium))
            .foregroundStyle(.secondary)
            .lineLimit(1)
            .padding(.horizontal, Theme.Spacing.md)
            .padding(.vertical, Theme.Spacing.sm)
            .background(Theme.Colors.surface, in: Capsule())
            .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
    }
}
