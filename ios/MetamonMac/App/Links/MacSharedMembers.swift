// 審査用ビルドは共有を外す(iOS の SharedMembersSection と同じ)
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonRenderer
import MetamonServices

/// 共有シートの「参加している人」(#974)。iOS の `SharedMembersModel` の Mac 版。
/// 外す・管理者にする・抜けるは楽観的 UI(先に反映し、失敗したら戻して文言を返す)。
/// 変わったら `MacSocial.memberEntries` も合わせる(開いているアプリの管理者だけのボタン・削除の可否 = `MembersHost`・#963)。
@MainActor
@Observable
final class MacSharedMembersModel {
    enum Phase: Equatable {
        /// 共有の記録があるか確かめ中(何も出さない)
        case probing
        case loading
        /// 共有の記録が無いアプリ(欄ごと出さない)
        case hidden
        case loaded
        /// 自分が参加者ではない(403 not_member)
        case notMember
        case failed(String)
        /// 自分で抜けた
        case left
    }

    let appId: String
    private(set) var phase: Phase = .probing
    private(set) var list: SharedMemberList?
    /// 外した直後の人。リンクの取り消しへ誘導する(作成者のリンクは取り消すまで有効なので)
    var justRemoved: SharedMember?
    @ObservationIgnored private let client: any MembershipClient
    @ObservationIgnored private let social: MacSocial?

    init(appId: String, client: any MembershipClient, social: MacSocial? = nil) {
        self.appId = appId
        self.client = client
        self.social = social
    }

    /// 欄を出すか(確かめ中・共有の記録が無いアプリでは出さない)
    var isVisible: Bool { phase != .probing && phase != .hidden }

    func load() async {
        do {
            if phase == .probing {
                guard try await client.hasMembersOnlyData(appId: appId) else {
                    phase = .hidden
                    return
                }
                phase = .loading
            }
            let loaded = try await client.members(appId: appId)
            list = loaded
            phase = .loaded
            sync()
        } catch {
            if SharedAccessDenial(error) == .notMember {
                phase = .notMember
                social?.memberEntries[appId] = ([], false)
            } else if phase == .probing {
                phase = .hidden // 共有かどうか分からない時は出さない
            } else {
                phase = .failed(L10n.tr("参加している人を読み込めませんでした"))
            }
        }
    }

    /// 作成者だけが外せる(自分・作成者は外せない)
    func canRemove(_ member: SharedMember) -> Bool {
        list?.role == .owner && !member.you && member.role != .owner
    }

    /// 管理者だけが管理者を変えられる(自分・作成者は変えられない)
    func canSetAdmin(_ member: SharedMember) -> Bool {
        list?.isAdmin == true && !member.you && member.role != .owner
    }

    /// 参加者(作成者でない)だけが抜けられる
    var canLeave: Bool { list?.role == .member }

    /// 外す。先に一覧から消し、失敗したら元の位置に戻して文言を返す
    func remove(_ member: SharedMember) async -> String? {
        guard var current = list, let index = current.items.firstIndex(of: member) else { return nil }
        current.items.remove(at: index)
        list = current
        justRemoved = member
        do {
            try await client.removeMember(appId: appId, uid: member.uid)
            sync()
            return nil
        } catch let e as APIError where e.status == 404 {
            sync()
            return nil // もう参加していない = 目的は達成
        } catch {
            if var back = list, !back.items.contains(member) {
                back.items.insert(member, at: min(index, back.items.count))
                list = back
            }
            if justRemoved == member { justRemoved = nil }
            return L10n.tr("外せませんでした。元に戻しました")
        }
    }

    /// 管理者にする / 外す。先に一覧に反映し、失敗したら戻して文言を返す
    func setAdmin(_ member: SharedMember, _ admin: Bool) async -> String? {
        guard var current = list, let index = current.items.firstIndex(of: member) else { return nil }
        current.items[index].isAdmin = admin
        list = current
        do {
            try await client.setAdmin(appId: appId, uid: member.uid, admin: admin)
            sync()
            return nil
        } catch {
            if var back = list, let i = back.items.firstIndex(where: { $0.uid == member.uid }) {
                back.items[i].isAdmin = member.isAdmin
                list = back
            }
            return L10n.tr("管理者を変更できませんでした。元に戻しました")
        }
    }

    /// 自分が抜ける。先に「抜けました」にし、失敗したら戻して文言を返す
    func leave() async -> String? {
        let before = phase
        phase = .left
        do {
            try await client.leave(appId: appId)
            social?.memberEntries[appId] = ([], false)
            return nil
        } catch {
            phase = before
            return L10n.tr("抜けられませんでした。元に戻しました")
        }
    }

    /// 開いているアプリの参加者(`MacAppRuntime.appMembers`・`isAppAdmin`)を一覧に合わせる
    private func sync() {
        guard let social, let list else { return }
        social.memberEntries[appId] = (list.items.map { AppMember(uid: $0.uid, displayName: $0.displayName, isAdmin: $0.isAdmin) },
                                       list.isAdmin)
    }
}

extension SharedMember {
    /// 一覧の名前。作成者は表示名を持たない(backend 仕様)ので「作成者」
    var macLabel: String {
        if !displayName.isEmpty { return displayName }
        return role == .owner ? L10n.tr("作成者") : L10n.tr("名前なし")
    }
}

/// 共有シートの「参加している人」の欄。作成者なら各行に「外す」、管理者なら「管理者にする / 外す」、参加者なら「この共有から抜ける」。
/// 外す・抜けるは確認してから。
struct MacSharedMembersSection: View {
    let model: MacSharedMembersModel
    /// 外した後の誘導「リンクも取り消す」
    var onRevokeLink: () async -> Void
    /// 抜けた後(シートの発行済みリンクはもう使えない)
    var onLeft: () -> Void
    /// 失敗の文言
    var onError: (String) -> Void

    @State private var confirmRemove: SharedMember?
    @State private var confirmLeave = false

    var body: some View {
        switch model.phase {
        case .probing, .hidden:
            EmptyView()
        case .loading:
            section {
                ForEach(0..<3, id: \.self) { _ in
                    RoundedRectangle(cornerRadius: 6).fill(.quaternary).frame(height: 28)
                }
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(Text("読み込み中"))
            }
        case .notMember:
            section { note(SharedAccessDenial.notMember.message) }
        case .failed(let message):
            section {
                HStack {
                    note(message)
                    Spacer()
                    Button("もう一度") { Task { await model.load() } }
                }
            }
        case .left:
            section {
                Label("この共有から抜けました。もう一度参加するには、参加している人に招待リンクをもらってください", systemImage: "checkmark.circle")
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }
        case .loaded:
            if let list = model.list { loaded(list) }
        }
    }

    private func loaded(_ list: SharedMemberList) -> some View {
        section(count: list.items.count) {
            note(L10n.tr("この共有の記録を見られるのは、ここにいる人だけです"))
            ScrollView {
                VStack(spacing: 0) {
                    ForEach(list.items) { member in
                        row(member)
                        if member != list.items.last { Divider() }
                    }
                }
            }
            .frame(maxHeight: 220)
            if list.isFull {
                note(L10n.tr("参加している人が上限(100 人)に達しています。これ以上は参加できません"))
            }
            if let removed = model.justRemoved { removedNotice(removed) }
            if model.canLeave {
                Button(role: .destructive) { confirmLeave = true } label: { Text("この共有から抜ける") }
                    .help("この共有から抜ける")
            }
        }
        .confirmationDialog(L10n.tr("「\(confirmRemove?.macLabel ?? "")」を外しますか?"), isPresented: Binding(
            get: { confirmRemove != nil }, set: { if !$0 { confirmRemove = nil } }
        )) {
            Button("外す", role: .destructive) {
                guard let target = confirmRemove else { return }
                Task { if let msg = await model.remove(target) { onError(msg) } }
            }
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text("外した人は、この共有の記録を見られなくなります。その人の端末のアプリと、これまでの記録は残ります。")
        }
        .confirmationDialog(L10n.tr("この共有から抜けますか?"), isPresented: $confirmLeave) {
            Button("抜ける", role: .destructive) {
                Task {
                    if let msg = await model.leave() { onError(msg) } else { onLeft() }
                }
            }
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text("抜けると、参加している人の記録は見られなくなります。もう一度参加するには招待リンクが要ります。")
        }
    }

    private func row(_ member: SharedMember) -> some View {
        HStack(spacing: 10) {
            Image(systemName: member.role == .owner ? "crown.fill" : "person.crop.circle.fill")
                .foregroundStyle(.tint)
                .frame(width: 22)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 2) {
                Text(member.macLabel).font(.body.weight(.medium))
                HStack(spacing: 6) {
                    if member.role == .owner, !member.displayName.isEmpty { Text("作成者") }
                    if member.isAdmin { Text("管理者") }
                    if member.you { Text("あなた") }
                }
                .font(.caption)
                .foregroundStyle(.secondary)
            }
            .accessibilityElement(children: .combine)
            Spacer()
            if model.canSetAdmin(member) {
                Button(member.isAdmin ? L10n.tr("管理者から外す") : L10n.tr("管理者にする")) {
                    Task { if let msg = await model.setAdmin(member, !member.isAdmin) { onError(msg) } }
                }
                .controlSize(.small)
                .help(member.isAdmin ? L10n.tr("管理者から外す") : L10n.tr("管理者にする"))
            }
            if model.canRemove(member) {
                Button("外す", role: .destructive) { confirmRemove = member }
                    .controlSize(.small)
                    .help("この人を共有から外す")
            }
        }
        .padding(.vertical, 6)
    }

    private func removedNotice(_ removed: SharedMember) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("「\(removed.macLabel)」を外しました。あなたのシェアリンクはまだ使えるので、そのリンクからまた参加できます。リンクも止めますか?")
                .font(.callout)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 8) {
                Button(role: .destructive) {
                    Task {
                        await onRevokeLink()
                        model.justRemoved = nil
                    }
                } label: { Text("リンクを取り消す") }
                Button("そのままにする") { model.justRemoved = nil }
            }
        }
        .padding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.quaternary, in: RoundedRectangle(cornerRadius: 6))
    }

    private func note(_ text: String) -> some View {
        Text(text).font(.callout).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
    }

    private func section<Content: View>(count: Int? = nil, @ViewBuilder _ content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Label("参加している人", systemImage: "person.2").font(.headline)
                    .accessibilityAddTraits(.isHeader)
                Spacer()
                if let count { Text("\(count) 人").foregroundStyle(.secondary) }
            }
            content()
        }
    }
}
#endif
