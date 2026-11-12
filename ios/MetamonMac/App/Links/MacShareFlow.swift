// 審査用ビルドは共有リンクの入口を外す(iOS と同じ)
#if !APP_STORE_REVIEW
import Foundation
import Observation
import MetamonDSL
import MetamonServices

/// 共有のリンクから取り込む・参加する(#1287)。iOS の `AppRuntime.openShareURL` と同じ流れ:
/// プレビュー → (記録を共有するアプリなら)参加の確認 / (それ以外は)追加の確認 → 取り込み → そのアプリを開く。
/// - サインインの前に届いたリンクは保留し、ログインできてから 1 回だけ開く(`PendingShareLink`・iOS の `SharingUI.pendingLink`)
/// - サーバーにつながっていない時(モック・plist 無し)は開かずに案内を出す(リンクはコピーできる)
/// - 取り込んだアプリはサーバーにあるので、一覧に足して記録・版をサーバーにつなぐ(`MacRootView` が `adopted` を受けて当てる)
@MainActor
@Observable
final class MacShareFlow {
    static let shared = MacShareFlow(services: .shared, isSignedIn: { MacAccount.shared.session.user != nil })

    struct Pending: Identifiable, Equatable {
        enum Kind: Equatable { case add, join }
        var token: String
        var preview: ShareLinkPreview?
        var kind: Kind
        var id: String { token }
    }

    /// 取り込んだアプリ(一覧に足して開く)。`id` は受け取るたびに変わる
    struct Adopted: Identifiable {
        let id = UUID()
        var document: AppDocument
        var envelope: Data?
        var banner: String
    }

    /// 確認のシート(追加 / 参加)
    var pending: Pending?
    /// サーバーにつながっていない時に届いたリンク(案内を出す)
    var unavailable: URL?
    /// 取り込み終えたアプリ(`MacRootView` が一覧に足して開き、nil に戻す)
    var adopted: Adopted?
    private(set) var link = PendingShareLink()

    @ObservationIgnored private let sharing: (any SharingClient)?
    @ObservationIgnored private let appConfig: (any AppConfigClient)?
    @ObservationIgnored private let isSignedIn: @MainActor () -> Bool
    @ObservationIgnored private let envelope: (String) -> Data?

    init(sharing: (any SharingClient)?, appConfig: (any AppConfigClient)?, isSignedIn: @escaping @MainActor () -> Bool,
         envelope: @escaping (String) -> Data? = { DocumentCache.shared.loadData(appId: $0) }) {
        self.sharing = sharing
        self.appConfig = appConfig
        self.isSignedIn = isSignedIn
        self.envelope = envelope
    }

    convenience init(services: MacServices, isSignedIn: @escaping @MainActor () -> Bool) {
        self.init(sharing: services.sharing, appConfig: services.appConfig, isSignedIn: isSignedIn)
    }

    /// 取り込みができるか(サーバーにつながっている)
    var isAvailable: Bool { sharing != nil && appConfig != nil }

    /// 届いた URL(URL スキーム・Universal Links)。共有のリンクでなければ何もしない
    func receive(_ url: URL) {
        guard ShareURL.token(from: url) != nil else { return }
        guard isAvailable else {
            unavailable = url
            return
        }
        link.receive(url)
    }

    /// 準備(ログイン)ができていれば保留のリンクを開く。`MacLinkReceiver` が届いた時とログインの変化で呼ぶ
    func openPendingIfReady() async {
        guard let url = link.take(isReady: isSignedIn()), let token = ShareURL.token(from: url) else { return }
        await open(token: token)
    }

    /// iOS の `importShared(token:)` と同じ: 記録を共有するアプリ(自分のでない)は参加の確認、それ以外は追加の確認
    func open(token: String) async {
        let preview = try? await (sharing as? any MembershipClient)?.previewShare(token: token)
        if let preview, preview.sharedData, !preview.mine {
            pending = Pending(token: token, preview: preview, kind: .join)
        } else {
            pending = Pending(token: token, preview: preview, kind: .add)
        }
    }

    /// 確認の「追加する」「参加する」。できたら nil(シートを閉じてアプリを開く)、できなければ出す文言(シートは開いたまま)
    func confirm(_ pending: Pending, displayName: String = "") async -> String? {
        guard let sharing, let appConfig else { return L10n.tr("シェアリンクはサーバー接続時のみ開けます") }
        do {
            let app: InstalledApp
            switch pending.kind {
            case .add:
                app = try await sharing.importShare(token: pending.token)
            case .join:
                guard let membership = sharing as? any MembershipClient else { return L10n.tr("シェアリンクはサーバー接続時のみ開けます") }
                MacSharedDisplayName.remember(displayName)
                app = try await membership.importShare(token: pending.token, displayName: displayName)
            }
            let document = try await appConfig.fetchDocument(appId: app.appId)
            self.pending = nil
            adopted = Adopted(document: document, envelope: envelope(app.appId), banner: Self.banner(app, kind: pending.kind))
            return nil
        } catch {
            return Self.failureMessage(error)
        }
    }

    /// iOS の `adopt` と同じ一言(共有データ付きは「みんなと同じデータ」を添える)
    static func banner(_ app: InstalledApp, kind: Pending.Kind) -> String {
        let head = kind == .join ? L10n.tr("「\(app.name)」に参加しました") : L10n.tr("シェアされた「\(app.name)」を追加しました")
        let extra = app.member ? L10n.tr("。参加している人の記録も見られます") : app.joined ? L10n.tr("。みんなの投稿も見られます") : ""
        return head + extra
    }

    /// 参加の表示名に表示できない言葉があった(#321)
    static var displayNameRejected: String { ErrorCopy.textNotAllowed(fieldName: L10n.tr("表示名")) }

    /// iOS の `importFailureMessage` と同じ: 参加の理由(403)・持てるアプリの数・取り消し済み(404)・その他
    static func failureMessage(_ error: Error) -> String {
        if let denial = SharedAccessDenial(error) { return denial.message }
        if let limit = AppLimit(error) { return AppLimitCopy.toast(limit) }
        if (error as? APIError)?.isTextNotAllowed == true { return displayNameRejected }
        return (error as? APIError)?.status == 404
            ? L10n.tr("このシェアリンクは取り消されたか、存在しません")
            : ErrorCopy.failure(L10n.tr("シェアされたアプリを開けませんでした"), error, context: "mac share import")
    }
}

/// 参加者一覧に出す名前。前回入れた名前を次の参加の初期値にする(端末内だけ・iOS の `SharedDisplayName` と同じキー)
enum MacSharedDisplayName {
    static let key = "metamon.sharedDisplayName"
    /// backend の上限(openapi: displayName maxLength 64)
    static let maxLength = 64
    static var last: String { UserDefaults.standard.string(forKey: key) ?? "" }
    static func remember(_ name: String) {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        UserDefaults.standard.set(String(trimmed.prefix(maxLength)), forKey: key)
    }
}
#endif
