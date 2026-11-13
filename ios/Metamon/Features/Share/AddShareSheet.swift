// 審査用ビルド(APP_STORE_REVIEW・#245 / #284)ではシェアごと外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 記録を共有しないアプリのシェアリンクを開いた時の、取り込む前の確認。プレビューが取れない時は名前なしで聞く。
struct PendingImport: Identifiable, Equatable {
    var token: String
    var preview: ShareLinkPreview?
    var id: String { token }
}

/// 「「<アプリ名>」を追加しますか?」。追加すると import し、そのアプリに変身する。
struct AddShareSheet: View {
    let pending: PendingImport
    @Environment(AppRuntime.self) private var rt
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.lg) {
            VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                title
                    .font(.system(.title2, design: .rounded).weight(.bold))
                    .accessibilityIdentifier("addShare.title")
                if let description = pending.preview?.description, !description.isEmpty {
                    Text(description)
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            Text("追加すると、このアプリに切り替わります")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            if let error {
                Label(error, systemImage: "exclamationmark.triangle")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("addShare.error")
            }
            Spacer(minLength: 0)
            buttons
        }
        .padding(Theme.Spacing.lg)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.Colors.canvas.ignoresSafeArea())
        .presentationDetents([.medium])
        .presentationDragIndicator(.visible)
        .interactiveDismissDisabled(busy)
        .appLimitSheets() // #630 追加で持てるアプリの数に達したら、この上で置き換える
    }

    private var title: Text {
        if let name = pending.preview?.name, !name.isEmpty {
            return Text("「\(name)」を追加しますか?")
        }
        return Text("シェアされたアプリを追加しますか?")
    }

    private var buttons: some View {
        VStack(spacing: Theme.Spacing.md) {
            Button { Task { await doAdd() } } label: {
                HStack(spacing: Theme.Spacing.sm) {
                    if busy { ProgressView().tint(.white) }
                    Text("追加する")
                }
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(.white)
                .frame(maxWidth: .infinity)
                .frame(height: 48)
                .background(Theme.brandGradient, in: Capsule())
            }
            .disabled(busy)
            .accessibilityIdentifier("addShare.confirm")
            Button("追加しない") { SharingUI.shared.pendingImport = nil }
                .font(.subheadline)
                .disabled(busy)
                .accessibilityIdentifier("addShare.cancel")
        }
    }

    private func doAdd() async {
        busy = true
        error = nil
        defer { busy = false }
        error = await rt.confirmImport(pending)
    }
}
#endif
