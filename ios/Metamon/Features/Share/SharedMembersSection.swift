// 審査用ビルド(APP_STORE_REVIEW・#245 / #284)ではシェアごと外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 共有シートの「参加している人」(#254)の状態。外す・抜けるは楽観的 UI(先に反映し、失敗したら戻して知らせる)。
@MainActor
@Observable
final class SharedMembersModel {
    enum Phase: Equatable {
        /// scope:shared があるか確かめ中(何も出さない)
        case probing
        /// 参加者を読み込み中(スケルトン)
        case loading
        /// 共有の記録が無いアプリ(セクションごと出さない)
        case hidden
        case loaded
        /// 自分が参加者ではない(403 not_member)
        case notMember
        case failed(String)
        /// 自分で抜けた
        case left
    }

    let appId: String
    private let client: any MembershipClient
    private(set) var phase: Phase = .probing
    private(set) var list: SharedMemberList?
    /// 外した直後の人。リンクの取り消しへ誘導する(作成者のリンクは取り消すまで有効なので)
    var justRemoved: SharedMember?

    init(appId: String, client: any MembershipClient) {
        self.appId = appId
        self.client = client
    }

    func load() async {
        do {
            if phase == .probing {
                guard try await client.hasMembersOnlyData(appId: appId) else {
                    phase = .hidden
                    return
                }
                phase = .loading
            }
            list = try await client.members(appId: appId)
            phase = .loaded
        } catch {
            if SharedAccessDenial(error) == .notMember {
                phase = .notMember
            } else if phase == .probing {
                phase = .hidden // 共有かどうか分からない時は出さない
            } else {
                phase = .failed(L10n.tr("参加している人を読み込めませんでした: \(error.localizedDescription)"))
            }
        }
    }

    /// 作成者が他の人を外す。先に一覧から消し、失敗したら元の位置に戻して文言を返す。
    func remove(_ member: SharedMember) async -> String? {
        guard var current = list, let index = current.items.firstIndex(of: member) else { return nil }
        current.items.remove(at: index)
        list = current
        justRemoved = member
        do {
            try await client.removeMember(appId: appId, uid: member.uid)
            return nil
        } catch let e as APIError where e.status == 404 {
            return nil // もう参加していない = 目的は達成
        } catch {
            if var back = list, !back.items.contains(member) {
                back.items.insert(member, at: min(index, back.items.count))
                list = back
            }
            if justRemoved == member { justRemoved = nil }
            return L10n.tr("外せませんでした。元に戻しました: \(error.localizedDescription)")
        }
    }

    /// 参加者が自分で抜ける。先に「抜けました」にし、失敗したら一覧に戻して文言を返す。
    func leave() async -> String? {
        let before = phase
        phase = .left
        do {
            try await client.leave(appId: appId)
            return nil
        } catch {
            phase = before
            return L10n.tr("抜けられませんでした。元に戻しました: \(error.localizedDescription)")
        }
    }
}

extension SharedMembersModel {
    /// 共有シートに出すか(確かめ中・共有の記録が無いアプリでは欄ごと出さない)
    var isVisible: Bool { phase != .probing && phase != .hidden }
}

extension SharedMember {
    /// 一覧の名前。作成者は表示名を持たない(backend 仕様)ので「作成者」。
    var label: String {
        if !displayName.isEmpty { return displayName }
        return role == .owner ? L10n.tr("作成者") : L10n.tr("名前なし")
    }
}

/// 共有シートの中段: 参加している人の一覧。作成者なら各行に「外す」、参加者なら「この共有から抜ける」。
struct SharedMembersSection: View {
    let model: SharedMembersModel
    /// 外した後の誘導「リンクも取り消す」
    var onRevokeLink: () async -> Void
    /// 抜けた後(自分が出したリンクはもう使えないので、シート側の発行済みリンクを捨てる)
    var onLeft: () -> Void

    @Environment(AppRuntime.self) private var rt
    @State private var confirmRemove: SharedMember?
    @State private var confirmLeave = false

    var body: some View {
        content(model)
    }

    @ViewBuilder
    private func content(_ model: SharedMembersModel) -> some View {
        switch model.phase {
        case .probing, .hidden:
            EmptyView()
        case .loading:
            card {
                title(count: nil)
                ForEach(0..<3, id: \.self) { _ in
                    RoundedRectangle(cornerRadius: 6).fill(Theme.Colors.surfaceElevated).frame(height: 36)
                }
                .accessibilityLabel("読み込み中")
            }
        case .notMember:
            card {
                title(count: nil)
                Text(SharedAccessDenial.notMember.message)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        case .failed(let message):
            card {
                title(count: nil)
                Text(message).font(.footnote).foregroundStyle(.secondary)
                Button("もう一度") { Task { await model.load() } }.buttonStyle(.bordered)
            }
        case .left:
            card {
                title(count: nil)
                Label("この共有から抜けました。もう一度参加するには、参加している人に招待リンクをもらってください", systemImage: "checkmark.circle")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("members.left")
            }
        case .loaded:
            if let list = model.list {
                loaded(model, list)
            }
        }
    }

    private func loaded(_ model: SharedMembersModel, _ list: SharedMemberList) -> some View {
        card {
            title(count: list.items.count)
            Text("この共有の記録を見られるのは、ここにいる人だけです")
                .font(.footnote)
                .foregroundStyle(.secondary)
            VStack(spacing: 0) {
                ForEach(list.items) { member in
                    row(member, canRemove: list.role == .owner && !member.you && member.role != .owner)
                    if member != list.items.last { Divider() }
                }
            }
            if let removed = model.justRemoved {
                removedNotice(model, removed)
            }
            if list.role == .member {
                Button(role: .destructive) { confirmLeave = true } label: {
                    Text("この共有から抜ける").frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
                .accessibilityIdentifier("members.leave")
            }
        }
        .alert(L10n.tr("「\(confirmRemove?.label ?? "")」を外しますか?"), isPresented: Binding(
            get: { confirmRemove != nil }, set: { if !$0 { confirmRemove = nil } }
        )) {
            Button("外す", role: .destructive) {
                guard let target = confirmRemove else { return }
                Task { if let msg = await model.remove(target) { rt.toast = msg } }
            }
            .accessibilityIdentifier("members.remove.confirm")
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text("外した人は、この共有の記録を見られなくなります。その人の端末のアプリと、これまでの記録は残ります。")
        }
        .alert(L10n.tr("この共有から抜けますか?"), isPresented: $confirmLeave) {
            Button("抜ける", role: .destructive) {
                Task {
                    if let msg = await model.leave() {
                        rt.toast = msg
                    } else {
                        onLeft()
                    }
                }
            }
            .accessibilityIdentifier("members.leave.confirm")
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text("抜けると、参加している人の記録は見られなくなります。もう一度参加するには招待リンクが要ります。")
        }
    }

    private func row(_ member: SharedMember, canRemove: Bool) -> some View {
        HStack(spacing: Theme.Spacing.md) {
            Image(systemName: member.role == .owner ? "crown.fill" : "person.crop.circle.fill")
                .font(.title3)
                .foregroundStyle(Theme.brandGradient)
                .frame(width: 28)
            VStack(alignment: .leading, spacing: 2) {
                Text(member.label).font(.subheadline.weight(.medium))
                    .accessibilityIdentifier("members.row")
                HStack(spacing: Theme.Spacing.xs) {
                    if member.role == .owner, !member.displayName.isEmpty { Text("作成者") }
                    if member.you { Text("あなた") }
                }
                .font(.caption)
                .foregroundStyle(.secondary)
            }
            Spacer()
            if canRemove {
                Button("外す", role: .destructive) { confirmRemove = member }
                    .font(.footnote.weight(.medium))
                    .buttonStyle(.bordered)
                    .accessibilityIdentifier("members.remove")
            }
        }
        .padding(.vertical, Theme.Spacing.sm + 2)
    }

    private func removedNotice(_ model: SharedMembersModel, _ removed: SharedMember) -> some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            Text("「\(removed.label)」を外しました。あなたのシェアリンクはまだ使えるので、そのリンクからまた参加できます。リンクも止めますか?")
                .font(.footnote)
            HStack(spacing: Theme.Spacing.md) {
                Button(role: .destructive) {
                    Task {
                        await onRevokeLink()
                        model.justRemoved = nil
                    }
                } label: {
                    Text("リンクを取り消す")
                }
                .buttonStyle(.borderedProminent)
                .accessibilityIdentifier("members.revokeLink")
                Button("そのままにする") { model.justRemoved = nil }
                    .buttonStyle(.bordered)
            }
            .font(.footnote.weight(.medium))
        }
        .padding(Theme.Spacing.md)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.Colors.surfaceElevated, in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
        .accessibilityIdentifier("members.removedNotice")
    }

    private func title(count: Int?) -> some View {
        HStack {
            Label("参加している人", systemImage: "person.2")
                .font(.headline)
            Spacer()
            if let count {
                Text("\(count) 人").font(.subheadline).foregroundStyle(.secondary)
            }
        }
    }

    private func card<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.md, content: content)
            .frame(maxWidth: .infinity, alignment: .leading)
            .metamonCard()
    }
}
#endif
