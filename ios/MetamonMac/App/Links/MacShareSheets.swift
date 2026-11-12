// 審査用ビルドは共有リンクの入口を外す(iOS と同じ)
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 共有のリンクを開いた時の確認(#1287)。iOS の `AddShareSheet`(追加)と `JoinShareSheet`(参加)の Mac 版。
/// 参加は「参加すると何が見えるか」と表示名(任意)を出す。Mac には共有シート・通知の切り替えがまだ無いので、その 2 行は出さない。
struct MacShareConfirmSheet: View {
    let pending: MacShareFlow.Pending
    let flow: MacShareFlow
    @State private var name = MacSharedDisplayName.last
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            VStack(alignment: .leading, spacing: 4) {
                title
                    .font(.title2.weight(.bold))
                    .accessibilityIdentifier(pending.kind == .join ? "join.title" : "addShare.title")
                if let description = pending.preview?.description, !description.isEmpty {
                    Text(description).font(.callout).foregroundStyle(.secondary)
                }
            }
            if pending.kind == .join {
                whatYouSee
                nameField
            } else {
                Text("追加すると、このアプリに切り替わります").foregroundStyle(.secondary)
            }
            if let error {
                Label(error, systemImage: "exclamationmark.triangle")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("share.error")
            }
            HStack {
                Spacer()
                Button(pending.kind == .join ? "参加しない" : "追加しない", role: .cancel) { flow.pending = nil }
                    .keyboardShortcut(.cancelAction)
                    .disabled(busy)
                Button {
                    Task { await confirm() }
                } label: {
                    HStack(spacing: 6) {
                        if busy { ProgressView().controlSize(.small) }
                        Text(pending.kind == .join ? "参加する" : "追加する")
                    }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(busy)
                .accessibilityIdentifier(pending.kind == .join ? "join.confirm" : "addShare.confirm")
            }
        }
        .padding(24)
        .frame(width: 460)
        .interactiveDismissDisabled(busy)
    }

    private var title: Text {
        if pending.kind == .join, let name = pending.preview?.name { return Text("「\(name)」に参加しますか?") }
        if let name = pending.preview?.name, !name.isEmpty { return Text("「\(name)」を追加しますか?") }
        return Text("シェアされたアプリを追加しますか?")
    }

    private var whatYouSee: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("参加すると", systemImage: "person.2.fill").font(.headline)
            point("eye", "参加している人が付けた記録が見られます")
            point("square.and.pencil", "あなたが付けた記録も、参加している人に見えます")
            point("list.bullet", "参加している人の一覧に、下の名前で出ます")
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 8))
    }

    private func point(_ icon: String, _ text: LocalizedStringKey) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Image(systemName: icon).foregroundStyle(.tint).frame(width: 20)
            Text(text)
        }
    }

    private var nameField: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("表示名(任意)").font(.headline)
            TextField("例: 父、たろう", text: $name)
                .textFieldStyle(.roundedBorder)
                .accessibilityIdentifier("join.name")
                .onChange(of: name) { _, new in
                    if new.count > MacSharedDisplayName.maxLength { name = String(new.prefix(MacSharedDisplayName.maxLength)) }
                    if error == MacShareFlow.displayNameRejected { error = nil } // 直し始めたら警告を消す
                }
            Text("空欄のままなら、名前なしで一覧に出ます").font(.caption).foregroundStyle(.secondary)
        }
    }

    private func confirm() async {
        busy = true
        error = nil
        defer { busy = false }
        error = await flow.confirm(pending, displayName: name)
    }
}
#endif
