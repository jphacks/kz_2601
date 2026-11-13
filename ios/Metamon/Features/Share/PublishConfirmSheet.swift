// 審査用ビルド(APP_STORE_REVIEW・#245)ではシェアごと外す。
#if !APP_STORE_REVIEW
import SwiftUI

/// 公開の確認(注意の文言は Mac と共有の `PublishConfirmCopy`・PublishCopy.swift)。「公開する」で `onPublish`、閉じる・「やめる」で何もしない。
struct PublishConfirmSheet: View {
    let appName: String
    let onPublish: () -> Void
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Theme.Spacing.lg) {
                Text(PublishConfirmCopy.title(appName: appName))
                    .font(.system(.title2, design: .rounded).weight(.bold))
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
