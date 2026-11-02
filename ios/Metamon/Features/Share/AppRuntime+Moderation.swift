// 審査用ビルド(APP_STORE_REVIEW・#245)はシェアを外しているので、通報・ブロックも外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// 通報・ブロック(#321・審査 1.2)の UI 状態。AppRuntime(@Observable)には拡張で保存プロパティを足せないので別に持つ。
@MainActor
@Observable
final class ModerationUI {
    static let shared = ModerationUI()

    /// ブロック中の uid → 表示名。共有アプリの投稿(`records(for:)`)はここで隠す
    var blocked: [String: String] = [:]
    /// 理由を選ぶ通報(nil = 出していない)
    var reporting: ModerationRequest?
    /// ブロックの確認
    var blocking: ModerationRequest?
    /// 通報・ブロックの結果
    var notice: ModerationNotice?
    /// ブロックで増える。「探す」がこれを見て読み直す(掲載者の他の掲載も消える)
    var blockCount = 0
    /// サーバーの一覧を読んだ uid(サインインし直したら読み直す)
    @ObservationIgnored var loadedFor: String?
}

/// 通報 / ブロックする対象。投稿は作者の uid、掲載は掲載 id(uid はサーバーが引く)。
struct ModerationRequest: Identifiable {
    let id = UUID()
    let target: ReportTarget
    let authorName: String
    /// 投稿の作者(掲載は nil・サーバーが引く)
    var authorUID: String?
    /// 「探す」のシートから(確認はそのシートの上に出す)
    var fromDiscover = false
}

struct ModerationNotice: Identifiable {
    let id = UUID()
    let title: String
    let message: String
    let fromDiscover: Bool
}

extension ReportReason {
    var label: String {
        switch self {
        case .spam: L10n.tr("スパム・宣伝")
        case .sexual: L10n.tr("性的な内容")
        case .violence: L10n.tr("暴力的・危険な内容")
        case .harassment: L10n.tr("嫌がらせ・差別")
        case .illegal: L10n.tr("違法な内容")
        case .privacy: L10n.tr("個人情報の公開")
        case .other: L10n.tr("その他")
        }
    }
}

extension AppRuntime {
    /// 通報・ブロックの API(live のシェアがある時だけ)。
    var moderation: (any ModerationClient)? { services.sharing as? any ModerationClient }

    // MARK: DSLRuntime(Renderer の長押しメニュー)

    /// 他人の共有レコードだけ(自分の投稿・private の entity・作者の分からない行は出さない)。
    func canReport(entity: String, record: Record) -> Bool {
        guard moderation != nil, dsl.entity(entity)?.isShared == true,
              let author = record.author, !author.isEmpty, let me = currentUserId else { return false }
        return author != me
    }

    func reportRecord(entity: String, record: Record) {
        ModerationUI.shared.reporting = recordRequest(entity: entity, record: record)
    }

    func blockAuthor(entity: String, record: Record) {
        ModerationUI.shared.blocking = recordRequest(entity: entity, record: record)
    }

    private func recordRequest(entity: String, record: Record) -> ModerationRequest {
        ModerationRequest(target: .record(appId: document.appId, entityId: entity, recordId: record.id),
                          authorName: record.authorName ?? L10n.tr("匿名"), authorUID: record.author)
    }

    // MARK: 「探す」

    func reportListing(_ item: PublicAppListing) {
        ModerationUI.shared.reporting = ModerationRequest(target: .listing(id: item.id), authorName: item.authorName, fromDiscover: true)
    }

    func blockPublisher(_ item: PublicAppListing) {
        ModerationUI.shared.blocking = ModerationRequest(target: .listing(id: item.id), authorName: item.authorName, fromDiscover: true)
    }

    // MARK: 実行

    func submitReport(_ req: ModerationRequest, reason: ReportReason) async {
        guard let moderation else { return }
        let ui = ModerationUI.shared
        do {
            try await moderation.report(req.target, reason: reason)
            ui.notice = ModerationNotice(title: L10n.tr("通報しました"),
                                         message: L10n.tr("運営が内容を確認し、利用規約に反するものは非公開にします。この人を表示したくない時はブロックもできます。"),
                                         fromDiscover: req.fromDiscover)
        } catch {
            let message = (error as? APIError)?.status == 429
                ? L10n.tr("今日はこれ以上通報できません。明日またお試しください")
                : L10n.tr("通報できませんでした: \(error.localizedDescription)")
            ui.notice = ModerationNotice(title: L10n.tr("通報できませんでした"), message: message, fromDiscover: req.fromDiscover)
        }
    }

    /// 投稿の作者は先に端末で隠してから送る(失敗したら戻す)。掲載者は応答の uid で隠し、「探す」は読み直す。
    func confirmBlock(_ req: ModerationRequest) async {
        guard let moderation else { return }
        let ui = ModerationUI.shared
        if let uid = req.authorUID { ui.blocked[uid] = req.authorName }
        do {
            let user: BlockedUser
            switch req.target {
            case .record:
                user = try await moderation.block(uid: req.authorUID ?? "", name: req.authorName)
            case .listing(let id):
                user = try await moderation.block(listingId: id)
            }
            ui.blocked[user.uid] = user.name
            ui.blockCount += 1
            ui.notice = ModerationNotice(title: L10n.tr("ブロックしました"),
                                         message: L10n.tr("「\(user.name)」さんの投稿と公開したアプリは表示されません。設定から解除できます。"),
                                         fromDiscover: req.fromDiscover)
        } catch {
            if let uid = req.authorUID { ui.blocked[uid] = nil }
            ui.notice = ModerationNotice(title: L10n.tr("ブロックできませんでした"), message: error.localizedDescription, fromDiscover: req.fromDiscover)
        }
    }

    func unblock(_ user: BlockedUser) async {
        guard let moderation else { return }
        let ui = ModerationUI.shared
        ui.blocked[user.uid] = nil
        do {
            try await moderation.unblock(uid: user.uid)
            for e in dsl.entities where e.isShared { await load(entities: [e.id], force: true) } // サーバーが除いていた投稿を取り直す
        } catch {
            ui.blocked[user.uid] = user.name
            toast = L10n.tr("解除できませんでした: \(error.localizedDescription)")
        }
    }

    /// サインイン中の uid のブロック一覧を 1 回読む(失敗しても投稿はサーバーが除くので、次の起動で読み直す)。
    func loadBlockedUsers() async {
        guard let moderation, let me = currentUserId, ModerationUI.shared.loadedFor != me else { return }
        guard let users = try? await moderation.blockedUsers() else { return }
        ModerationUI.shared.blocked = Dictionary(users.map { ($0.uid, $0.name) }, uniquingKeysWith: { a, _ in a })
        ModerationUI.shared.loadedFor = me
    }

    /// 共有の entity から、ブロック中の人の投稿を除く(`records(for:)`)。
    func hidingBlocked(_ rows: [Record], entity: String) -> [Record] {
        let blocked = ModerationUI.shared.blocked
        guard !blocked.isEmpty, dsl.entity(entity)?.isShared == true else { return rows }
        return rows.filter { $0.author.map { blocked[$0] == nil } ?? true }
    }
}

/// 通報の理由・ブロックの確認・結果を出す。`onModal` は NearbySheets と同じ(DSL のシートの上か下か)。
/// 「探す」のシートは `ModerationDialogs(discover: true)` を自分に付ける。
struct ModerationDialogs: ViewModifier {
    var onModal = false
    var discover = false
    @Environment(AppRuntime.self) private var rt
    @State private var ui = ModerationUI.shared

    private func mine(_ fromDiscover: Bool) -> Bool {
        if discover || fromDiscover { return discover && fromDiscover }
        return onModal == (rt.sheet != nil || rt.fullScreen != nil)
    }

    func body(content: Content) -> some View {
        content
            .confirmationDialog(Text("通報の理由"), isPresented: Binding(
                get: { ui.reporting.map { mine($0.fromDiscover) } ?? false },
                set: { if !$0 { ui.reporting = nil } }
            ), titleVisibility: .visible, presenting: ui.reporting) { req in
                ForEach(ReportReason.allCases) { reason in
                    Button(reason.label) { Task { await rt.submitReport(req, reason: reason) } }
                }
                Button("キャンセル", role: .cancel) {}
            } message: { _ in
                Text("運営が内容を確認します。通報したことは相手に知らされません。")
            }
            .alert(Text("「\(ui.blocking?.authorName ?? "")」さんをブロックしますか?"), isPresented: Binding(
                get: { ui.blocking.map { mine($0.fromDiscover) } ?? false },
                set: { if !$0 { ui.blocking = nil } }
            ), presenting: ui.blocking) { req in
                Button("ブロックする", role: .destructive) { Task { await rt.confirmBlock(req) } }
                Button("キャンセル", role: .cancel) {}
            } message: { _ in
                Text("この人の投稿と公開したアプリが、あなたに表示されなくなります。相手には知らされません。設定からいつでも解除できます。")
            }
            .alert(Text(ui.notice?.title ?? ""), isPresented: Binding(
                get: { ui.notice.map { mine($0.fromDiscover) } ?? false },
                set: { if !$0 { ui.notice = nil } }
            ), presenting: ui.notice) { _ in
                Button("OK", role: .cancel) {}
            } message: { notice in
                Text(notice.message)
            }
            .task(id: rt.currentUserId) { await rt.loadBlockedUsers() }
    }
}

/// 設定の「ブロック中のユーザー」(#321)。シェアが使える時だけ。
struct BlockedUsersSettingsSection: View {
    @Environment(AppRuntime.self) private var rt
    @State private var ui = ModerationUI.shared

    var body: some View {
        if rt.moderation != nil {
            Section {
                if ui.blocked.isEmpty {
                    Text("ブロックしている人はいません").foregroundStyle(.secondary)
                } else {
                    ForEach(ui.blocked.sorted { $0.value < $1.value }, id: \.key) { uid, name in
                        HStack {
                            Text(name)
                            Spacer()
                            Button("解除") { Task { await rt.unblock(BlockedUser(uid: uid, name: name)) } }
                                .buttonStyle(.borderless)
                        }
                    }
                }
            } header: {
                Text("ブロック中のユーザー")
            } footer: {
                Text("ブロックした人の投稿と公開したアプリは表示されません。")
            }
        }
    }
}
#endif
