// 審査用ビルド(APP_STORE_REVIEW・#245 / #284)ではシェアごと外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 記録を共有するアプリのシェアリンクを開いた時の、import 前の確認(#254)。
struct PendingJoin: Identifiable, Equatable {
    var token: String
    var preview: ShareLinkPreview
    var id: String { token }
}

/// 参加者一覧に出す名前。前回入れた名前を次の参加の初期値にする(端末内だけ)。
enum SharedDisplayName {
    static let key = "metamon.sharedDisplayName"
    static var last: String { UserDefaults.standard.string(forKey: key) ?? "" }
    static func remember(_ name: String) {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        UserDefaults.standard.set(String(trimmed.prefix(maxLength)), forKey: key)
    }
    /// backend の上限(openapi: displayName maxLength 64)
    static let maxLength = 64
}

/// 「参加すると何が見えるか」と表示名(任意)。参加すると import し、そのアプリに変身する。
struct JoinShareSheet: View {
    let join: PendingJoin
    @Environment(AppRuntime.self) private var rt
    @State private var name = SharedDisplayName.last
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Theme.Spacing.lg) {
                VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                    Text("「\(join.preview.name)」に参加しますか?")
                        .font(.system(size: 22, weight: .bold, design: .rounded))
                        .accessibilityIdentifier("join.title")
                    if !join.preview.description.isEmpty {
                        Text(join.preview.description)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }
                whatYouSee
                nameField
                if let error {
                    Label(error, systemImage: "exclamationmark.triangle")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("join.error")
                }
                buttons
            }
            .padding(Theme.Spacing.lg)
        }
        .scrollDismissesKeyboard(.interactively)
        .background(Theme.Colors.canvas.ignoresSafeArea())
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .interactiveDismissDisabled(busy)
    }

    private var whatYouSee: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.md) {
            Label("参加すると", systemImage: "person.2.fill")
                .font(.headline)
            point("eye", "参加している人が付けた記録が見られます")
            point("square.and.pencil", "あなたが付けた記録も、参加している人に見えます")
            point("list.bullet", "参加している人の一覧に、下の名前で出ます")
            point("rectangle.portrait.and.arrow.right", "共有シートから、いつでも抜けられます")
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .metamonCard()
    }

    private func point(_ icon: String, _ text: LocalizedStringKey) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.sm) {
            Image(systemName: icon)
                .foregroundStyle(Theme.brandGradient)
                .frame(width: 22)
            Text(text).font(.subheadline)
        }
    }

    private var nameField: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            Text("表示名(任意)").font(.subheadline.weight(.semibold))
            TextField("例: 父、たろう", text: $name)
                .submitLabel(.done)
                .textInputAutocapitalization(.never)
                .metamonInputField()
                .accessibilityIdentifier("join.name")
                .onChange(of: name) { _, new in
                    if new.count > SharedDisplayName.maxLength { name = String(new.prefix(SharedDisplayName.maxLength)) }
                }
            Text("空欄のままなら、名前なしで一覧に出ます")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    private var buttons: some View {
        VStack(spacing: Theme.Spacing.md) {
            Button { Task { await doJoin() } } label: {
                HStack(spacing: Theme.Spacing.sm) {
                    if busy { ProgressView().tint(.white) }
                    Text("参加する")
                }
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(.white)
                .frame(maxWidth: .infinity)
                .frame(height: 48)
                .background(Theme.brandGradient, in: Capsule())
            }
            .disabled(busy)
            .accessibilityIdentifier("join.confirm")
            Button("参加しない") { SharingUI.shared.pendingJoin = nil }
                .font(.subheadline)
                .disabled(busy)
                .accessibilityIdentifier("join.cancel")
        }
    }

    private func doJoin() async {
        busy = true
        error = nil
        defer { busy = false }
        error = await rt.joinShared(join, displayName: name)
    }
}
#endif
