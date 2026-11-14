// 審査用ビルド(APP_STORE_REVIEW)は「探す」を外す(iOS の DiscoverSheet と同じ)
#if !APP_STORE_REVIEW
import Foundation
import Observation
import MetamonDSL
import MetamonServices

/// 「アプリを探す」(#2069)の状態。iOS の `DiscoverSheet` の検索・入れる・通報 / ブロックの Mac 版。
/// 検索は iOS と同じ `SharingClient.searchPublicApps`、入れるのは `install(publicAppId:)` → 中身を取って一覧に足す(`MacRootView`)。
/// 持てるアプリの数に達したら置き換えのシート(`MacAppLimitSheet`)を出し、1 つ空いたら入れ直す。
@MainActor
@Observable
final class MacDiscoverModel: Identifiable {
    /// 本文の出し分け(iOS と同じ順: 最初の読み込み → 失敗 → 0 件 → 結果)
    enum Phase: Equatable {
        case loading
        case failed(String)
        case empty(String)
        case results
    }

    /// 入れ終えたアプリ(`MacRootView` が一覧に足して開く)
    struct Installed: Identifiable {
        let id = UUID()
        var document: AppDocument
        var envelope: Data?
        var banner: String
        /// 共有データに参加した(サイドバーの「共有されたアプリ」に出す)。自分のコピー・自分の公開は出さない
        var shared: Bool
    }

    let id = UUID()
    var query = ""
    var sort: DiscoverSort = .popular
    private(set) var items: [PublicAppListing] = []
    private(set) var loading = true
    private(set) var error: String?
    /// 入れている途中の掲載 id(カードのボタンを輪にする)
    private(set) var installing: String?
    /// 入れられなかった時の次の一手(シートの上のアラートの本文)
    var installError: String?
    /// 持てるアプリの数の置き換え
    var limitRequest: MacAppLimitRequest?
    /// 入れ終えた(シートを閉じてそのアプリを開く)
    private(set) var installed: Installed?
    /// 通報の理由を選ぶ・ブロックの確認・結果
    var reporting: PublicAppListing?
    var blocking: PublicAppListing?
    var notice: MacModerationNotice?
    /// ブロックで増える。検索の `task(id:)` に入れて読み直す(掲載者の他の掲載も消える・iOS と同じ)
    private(set) var blockCount = 0

    @ObservationIgnored private let sharing: (any SharingClient)?
    @ObservationIgnored private let appConfig: (any AppConfigClient)?
    @ObservationIgnored private let moderation: (any ModerationClient)?
    @ObservationIgnored private let social: MacSocial?
    @ObservationIgnored private let envelope: (String) -> Data?
    @ObservationIgnored private let debounce: Duration

    init(sharing: (any SharingClient)?, appConfig: (any AppConfigClient)?, moderation: (any ModerationClient)? = nil,
         social: MacSocial? = nil, envelope: @escaping (String) -> Data? = { DocumentCache.shared.loadData(appId: $0) },
         debounce: Duration = .milliseconds(250)) {
        self.sharing = sharing
        self.appConfig = appConfig
        self.moderation = moderation
        self.social = social
        self.envelope = envelope
        self.debounce = debounce
    }

    convenience init(services: MacServices) {
        self.init(sharing: services.sharing, appConfig: services.appConfig, moderation: MacSocial.shared.moderation, social: .shared)
    }

    /// メニューの「みんなのアプリを探す」を押せるか: サインインしていて、サーバーにつながっている時だけ
    static func isAvailable(signedIn: Bool, services: MacServices) -> Bool {
        signedIn && services.sharing != nil && services.appConfig != nil
    }

    var phase: Phase {
        if loading && items.isEmpty { return .loading }
        if let error, items.isEmpty { return .failed(error) }
        if items.isEmpty { return .empty(DiscoverCopy.emptyMessage(query: query)) }
        return .results
    }

    /// 通報・ブロックを出せるか(自分の掲載には出さない)
    func canModerate(_ item: PublicAppListing) -> Bool { moderation != nil && !item.mine }

    /// 検索をやり直すきっかけ(並び順・検索語・ブロック)
    var searchKey: String { "\(sort.rawValue)|\(query)|\(blockCount)" }

    // MARK: - 検索

    /// iOS の `DiscoverSheet.search` と同じ。入力中は間引き、取り消された検索の結果は書かない
    func search() async {
        guard let sharing else {
            loading = false
            error = DiscoverCopy.unavailable
            return
        }
        if !query.isEmpty, debounce > .zero { try? await Task.sleep(for: debounce) }
        guard !Task.isCancelled else { return }
        loading = true
        defer { loading = false }
        do {
            let found = try await sharing.searchPublicApps(query: query, sort: sort.rawValue)
            guard !Task.isCancelled else { return }
            items = found
            error = nil
        } catch is CancellationError {
        } catch {
            if (error as? URLError)?.code == .cancelled { return }
            self.error = ErrorCopy.failure(L10n.tr("読み込めませんでした"), error, context: "MacDiscover")
        }
    }

    // MARK: - 入れる

    /// カードの「入れる」。できたら `installed`、上限なら `limitRequest`、それ以外は `installError`
    func install(_ item: PublicAppListing) async {
        guard installing == nil else { return }
        guard let sharing, let appConfig else {
            installError = DiscoverCopy.unavailable
            return
        }
        installing = item.id
        installError = nil
        defer { installing = nil }
        do {
            let app = try await sharing.install(publicAppId: item.id)
            let document = try await appConfig.fetchDocument(appId: app.appId)
            installed = Installed(document: document, envelope: envelope(app.appId),
                                  banner: L10n.tr("「\(app.name)」をインストールしました") + MacShareFlow.bannerExtra(app),
                                  shared: app.joined && !app.mine)
        } catch {
            if let limit = AppLimit(error) { // #630 持てるアプリの数
                limitRequest = MacAppLimitRequest(limit: limit) { [weak self] in
                    Task { await self?.install(item) }
                }
                return
            }
            ErrorLog.record(error, context: "mac install public")
            installError = ErrorCopy.nextStep(for: error) // 題(「インストールできませんでした」)はアラート側
        }
    }

    // MARK: - 通報・ブロック(#321)

    func report(_ item: PublicAppListing, reason: ReportReason) async {
        guard let moderation else { return }
        reporting = nil
        do {
            try await moderation.report(.listing(id: item.id), reason: reason)
            notice = MacModerationNotice(
                title: L10n.tr("通報しました"),
                message: L10n.tr("運営が内容を確認し、利用規約に反するものは非公開にします。この人を表示したくない時はブロックもできます。"))
        } catch {
            let message = (error as? APIError)?.status == 429
                ? L10n.tr("今日はこれ以上通報できません。明日またお試しください")
                : L10n.tr("通信できる状態でもう一度お試しください")
            notice = MacModerationNotice(title: L10n.tr("通報できませんでした"), message: message)
        }
    }

    /// 掲載者をブロックする(uid はサーバーが引く)。できたら投稿も隠し、検索を読み直す
    func block(_ item: PublicAppListing) async {
        guard let moderation else { return }
        blocking = nil
        do {
            let user = try await moderation.block(listingId: item.id)
            social?.blocked[user.uid] = user.name
            blockCount += 1
            notice = MacModerationNotice(
                title: L10n.tr("ブロックしました"),
                message: L10n.tr("「\(user.name)」さんの投稿と公開したアプリは表示されません。設定から解除できます。"))
        } catch {
            notice = MacModerationNotice(title: L10n.tr("ブロックできませんでした"),
                                         message: L10n.tr("通信できる状態でもう一度お試しください"))
        }
    }
}
#endif
