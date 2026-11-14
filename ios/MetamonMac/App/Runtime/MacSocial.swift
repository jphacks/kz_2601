// 審査用ビルド(APP_STORE_REVIEW)は共有を外しているので、通報・参加者・仲間探しも外す(iOS と同じ)。
#if !APP_STORE_REVIEW
import Foundation
import Observation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// 人とつながる部品(#963)の問い合わせ先と状態。ウインドウをまたいで 1 つ(ブロックした人はどのウインドウでも隠す)。
/// iOS の `ModerationUI`・`MatchingUI`・`AppMembersStore` の Mac 版で、クライアントは iOS と同じ `HTTPSharingClient`・`HTTPMatchingClient`。
/// モック(サーバーにつながっていない)の時はクライアントが nil で、印・通報・参加者・仲間探しのボタンは出ない。
@MainActor
@Observable
final class MacSocial {
    static let shared = MacSocial(
        currentUserId: { MacServices.shared.isLive ? MacServices.shared.auth.currentUser?.uid : nil },
        members: MacServices.shared.members, moderation: MacServices.shared.moderation, matching: MacServices.shared.matching)

    /// サインイン中の uid。モックでは nil(作者の判定をしない・今までの Mac と同じ)
    @ObservationIgnored let currentUserId: @MainActor () -> String?
    @ObservationIgnored let members: (any MembershipClient)?
    @ObservationIgnored let moderation: (any ModerationClient)?
    @ObservationIgnored let matching: (any MatchingClient)?

    /// ブロック中の uid → 表示名。共有の entity の投稿はここで隠す
    var blocked: [String: String] = [:]
    /// 理由を選ぶ通報・ブロックの確認・結果(`MacSocialDialogs`)
    var reporting: MacModerationRequest?
    var blocking: MacModerationRequest?
    var notice: MacModerationNotice?
    /// 18 歳以上の確認を待っている操作
    var askingAge: MacPendingInterest?
    /// 「18 歳未満です」と答えた(開いた時・読み込みでは聞き直さない)。メモリだけ
    @ObservationIgnored var declinedAge = false
    /// appId → 参加者と自分が管理者か(nil = 分からない)
    var memberEntries: [String: (members: [AppMember], isAdmin: Bool?)] = [:]
    /// ブロック一覧を読んだ uid
    @ObservationIgnored var blockedLoadedFor: String?
    @ObservationIgnored var ageGate: MatchingAgeGate
    /// "appId|uid" → 仲間探しの一覧
    @ObservationIgnored private var stores: [String: MatchingStore] = [:]

    init(currentUserId: @escaping @MainActor () -> String? = { nil }, members: (any MembershipClient)? = nil,
         moderation: (any ModerationClient)? = nil, matching: (any MatchingClient)? = nil,
         ageGate: MatchingAgeGate = MatchingAgeGate()) {
        self.currentUserId = currentUserId
        self.members = members
        self.moderation = moderation
        self.matching = matching
        self.ageGate = ageGate
    }

    func matchingStore(appId: String, uid: String) -> MatchingStore? {
        guard let matching else { return nil }
        let key = "\(appId)|\(uid)"
        if let s = stores[key] { return s }
        let s = MatchingStore(appId: appId, client: matching)
        stores[key] = s
        return s
    }

    /// サインアウト・アカウント削除(`MacLocalWipe`): 前のアカウントのブロック・参加者・仲間探しの一覧と申告の記録を消す
    func wipe() {
        blocked = [:]
        reporting = nil
        blocking = nil
        notice = nil
        askingAge = nil
        declinedAge = false
        memberEntries = [:]
        blockedLoadedFor = nil
        stores = [:]
        ageGate.forgetSent()
    }
}

/// 通報 / ブロックする投稿。
struct MacModerationRequest: Identifiable {
    let id = UUID()
    let target: ReportTarget
    let authorName: String
    let authorUID: String?
}

struct MacModerationNotice: Identifiable {
    let id = UUID()
    let title: String
    let message: String
}

/// 18 歳以上の確認のあとに続ける操作。
enum MacPendingInterest: Identifiable {
    case send(entity: String, record: Record)
    case accept(MatchInterest)
    case refresh(Set<MatchBox>)

    var id: String {
        switch self {
        case .send(let entity, let record): "send/\(entity)/\(record.id)"
        case .accept(let item): "accept/\(item.uid)"
        case .refresh: "refresh"
        }
    }
}

extension ReportReason {
    var macLabel: String {
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
#endif
