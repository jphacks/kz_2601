// 審査用ビルド(APP_STORE_REVIEW・#245)ではシェアごと外す。
#if !APP_STORE_REVIEW
import SwiftUI

/// 「みんなに公開」の前に出す注意(Android #1297 と同じ 4 つ)。
enum PublishConfirmCopy {
    struct Point: Equatable {
        var icon: String
        var text: String
    }

    static func title(appName: String) -> String { L10n.tr("「\(appName)」を公開しますか?") }

    static var points: [Point] {
        [
            Point(icon: "magnifyingglass", text: L10n.tr("「探す」から誰でも見つけて、自分のアプリとして入れられます")),
            Point(icon: "lock", text: L10n.tr("あなただけの記録は含まれません")),
            Point(icon: "person.crop.circle.badge.exclamationmark", text: L10n.tr("名前・説明・タグに、住所や電話番号などの個人の情報を書かないでください")),
            Point(icon: "globe.badge.chevron.backward", text: L10n.tr("このシェアの画面から、いつでも公開をやめられます")),
        ]
    }
}

/// 公開の確認。「公開する」で `onPublish`、閉じる・「やめる」で何もしない。
struct PublishConfirmSheet: View {
    let appName: String
    let onPublish: () -> Void
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Theme.Spacing.lg) {
                Text(PublishConfirmCopy.title(appName: appName))
                    .font(.system(size: 22, weight: .bold, design: .rounded))
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityIdentifier("publish.confirm.title")
                VStack(alignment: .leading, spacing: Theme.Spacing.md) {
                    Label("公開すると", systemImage: "globe.asia.australia")
                        .font(.headline)
                    ForEach(PublishConfirmCopy.points, id: \.text) { p in
                        HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.sm) {
                            Image(systemName: p.icon)
                                .foregroundStyle(Theme.brandGradient)
                                .frame(width: 22)
                                .accessibilityHidden(true)
                            Text(verbatim: p.text).font(.subheadline)
                        }
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .metamonCard()
                VStack(spacing: Theme.Spacing.md) {
                    Button {
                        dismiss()
                        onPublish()
                    } label: {
                        Text("公開する")
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(.white)
                            .frame(maxWidth: .infinity)
                            .frame(height: 48)
                            .background(Theme.brandGradient, in: Capsule())
                    }
                    .accessibilityIdentifier("publish.confirm.publish")
                    Button("やめる") { dismiss() }
                        .font(.subheadline)
                        .accessibilityIdentifier("publish.confirm.cancel")
                }
            }
            .padding(Theme.Spacing.lg)
        }
        .background(Theme.Colors.canvas.ignoresSafeArea())
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }
}
#endif
