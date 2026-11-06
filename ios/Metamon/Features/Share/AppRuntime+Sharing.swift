// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// シェア / 「探す」(#49 #50)の UI 状態。AppRuntime(@Observable)には拡張で保存プロパティを足せないので別に持つ。
@MainActor
@Observable
final class SharingUI {
    static let shared = SharingUI()

    /// 「探す」シート(RootView の `SharingHooks` が present)
    var showDiscover = false
    /// シェアシートを開く対象(変身一覧 `AppSwitcherSheet` が present)
    var shareTarget: AppSummary?
    /// appId → 発行済みのシェアリンク(開くたびにトークンを増やさない)
    var issued: [String: ShareInfo] = [:]
    /// 記録を共有するアプリのリンクを開いた時の参加の確認(#254・`JoinShareSheet`)
    var pendingJoin: PendingJoin?

    /// 受け取ったシェアリンク(#278)。サインイン前・bootstrap 前に届いた分も捨てずに持ち、準備ができてから 1 回だけ開く。
    /// 未インストールの人が入れてから開き直すと、サインイン画面の上で届くため(RootView の onOpenURL では取りこぼす)。
    private(set) var pendingLink: URL?
    /// RootView の bootstrap が終わったか(`MetamonApp`)。サインアウトで false に戻す
    var isReady = false

    /// シェアリンクなら保留して true。それ以外の URL は false(触らない)。
    @discardableResult
    func receive(_ url: URL) -> Bool {
        guard ShareURL.token(from: url) != nil else { return false }
        pendingLink = url
        return true
    }

    /// 準備済みなら保留中のリンクを渡して空にする。まだなら nil(保留は残す)。
    func takeLink() -> URL? {
        guard isReady, let url = pendingLink else { return nil }
        pendingLink = nil
        return url
    }
}

extension AppRuntime {
    /// appconfig が live で、シェア / 探すが使えるか。
    var canShare: Bool { services.sharing != nil }

    /// `tailorit://import/<token>` を受けたら import → そのアプリに変身。それ以外の URL は false。
    @discardableResult
    func openShareURL(_ url: URL) async -> Bool {
        guard let token = ShareURL.token(from: url) else { return false }
        await importShared(token: token)
        return true
    }

    func importShared(token: String) async {
        guard let sharing = services.sharing else {
            toast = L10n.tr("シェアリンクはサーバー接続時のみ開けます")
            return
        }
        // #254: 記録を共有するアプリは、参加すると何が見えるかを先に見せて表示名を聞く(プレビューが取れなければ従来どおり)
        let membership = sharing as? any MembershipClient
        let preview = try? await membership?.previewShare(token: token)
        if let preview, preview.sharedData, !preview.mine {
            // 参加者は無料(正典「課金・Pro」2026-11-06: 共有アプリの費用は作成者の Pro)。課金画面は出さない(H5)
            SharingUI.shared.pendingJoin = PendingJoin(token: token, preview: preview)
            return
        }
        do {
            let app = try await sharing.importShare(token: token)
            await adopt(app, banner: L10n.tr("シェアされた「\(app.name)」を追加しました"))
        } catch {
            toast = Self.importFailureMessage(error)
        }
    }

    /// 確認画面の「参加する」。成功したらシートを閉じて変身し nil、失敗したら画面に出す文言を返す(シートは開いたまま)。
    func joinShared(_ join: PendingJoin, displayName: String) async -> String? {
        guard let membership = services.sharing as? any MembershipClient else {
            return L10n.tr("シェアリンクはサーバー接続時のみ開けます")
        }
        SharedDisplayName.remember(displayName)
        do {
            let app = try await membership.importShare(token: join.token, displayName: displayName)
            SharingUI.shared.pendingJoin = nil
            await adopt(app, banner: L10n.tr("「\(app.name)」に参加しました"))
            return nil
        } catch {
            return Self.importFailureMessage(error)
        }
    }

    /// import の失敗。403 の参加の理由(#254)・取り消し済み(404)・その他。
    static func importFailureMessage(_ error: Error) -> String {
        if let denial = SharedAccessDenial(error) { return denial.message }
        return (error as? APIError)?.status == 404
            ? L10n.tr("このシェアリンクは取り消されたか、存在しません")
            : ErrorCopy.failure(L10n.tr("シェアされたアプリを開けませんでした"), error, context: "share import")
    }

    func installPublic(_ listing: PublicAppListing) async {
        guard let sharing = services.sharing else { return }
        do {
            let app = try await sharing.install(publicAppId: listing.id)
            await adopt(app, banner: L10n.tr("「\(app.name)」をインストールしました"))
        } catch {
            toast = ErrorCopy.failure(L10n.tr("インストールできませんでした"), error, context: "install public")
        }
    }

    /// import / install の結果に切り替える(変身アニメは switchApp)。共有データ付きは「みんなと同じデータ」を一言添える。
    /// 参加者・インストールした人は無料(H5)。作成者が Pro でない時の書き込みは 402 `creator_needs_pro` の案内(`ProRequired.writeMessage`)だけ。
    private func adopt(_ app: InstalledApp, banner: String) async {
        print("[Metamon] adopt shared app: appId=\(app.appId) joined=\(app.joined)")
        if app.appId != document.appId {
            await switchApp(to: app.appId, afterSheetDismiss: true)
        }
        guard document.appId == app.appId else { return } // 切替に失敗(トーストは switchApp が出す)
        let extra = app.member ? L10n.tr("。参加している人の記録も見られます") : app.joined ? L10n.tr("。みんなの投稿も見られます") : ""
        summaryBanner = SummaryBanner(text: banner + extra, highlight: nil)
        await appsList()
    }
}

/// WindowGroup の根に付ける(#278): サインイン画面の時に届いたシェアリンクも `SharingUI` に保留する。
struct ShareLinkReceiver: ViewModifier {
    func body(content: Content) -> some View {
        content.onOpenURL { url in SharingUI.shared.receive(url) }
    }
}

/// RootView に 1 行で付ける: URL スキーム受信・「探す」シート・DEBUG の自動操作フック。
struct SharingHooks: ViewModifier {
    @Environment(AppRuntime.self) private var rt
    @State private var ui = SharingUI.shared

    func body(content: Content) -> some View {
        content
            // リンクの受信は `ShareLinkReceiver`(サインイン画面の時も届く)。ここでは準備ができたら開く(#278)
            .task(id: "\(ui.isReady)|\(ui.pendingLink?.absoluteString ?? "")") {
                if let url = ui.takeLink() { await rt.openShareURL(url) }
            }
            .sheet(isPresented: $ui.showDiscover) { DiscoverSheet() }
            .sheet(item: $ui.pendingJoin) { JoinShareSheet(join: $0) }
            #if DEBUG
            .task { await debugHooks() }
            .modifier(ReleaseRequestHooks())
            #endif
    }

    #if DEBUG
    /// ヘッドレス E2E 用(起動 2.5 秒後):
    /// - `METAMON_AUTO_DISCOVER=1` 探すを開く(`METAMON_AUTO_INSTALL=<public app id>` で続けてインストール)
    /// - `METAMON_AUTO_SHARE=1` 変身一覧 → 今のアプリのシェアシート
    /// - `METAMON_AUTO_IMPORT=<token>` シェアリンクを開いたのと同じ import → 変身
    private func debugHooks() async {
        let env = ProcessInfo.processInfo.environment
        let importToken = env["METAMON_AUTO_IMPORT"] ?? ""
        guard env["METAMON_AUTO_DISCOVER"] == "1" || env["METAMON_AUTO_SHARE"] == "1" || !importToken.isEmpty else { return }
        try? await Task.sleep(for: .seconds(2.5))
        if !importToken.isEmpty {
            // simctl openurl は OS の「開きますか?」確認で止まるので、同じ経路(openShareURL)を直接呼ぶ
            await rt.openShareURL(URL(string: ShareURL.scheme + "://import/" + importToken)!)
        } else if env["METAMON_AUTO_DISCOVER"] == "1" {
            ui.showDiscover = true
            guard let id = env["METAMON_AUTO_INSTALL"], !id.isEmpty else { return }
            try? await Task.sleep(for: .seconds(3))
            ui.showDiscover = false
            await rt.installPublic(PublicAppListing(id: id, name: id))
        } else {
            rt.openSwitcher()
            try? await Task.sleep(for: .seconds(1.5))
            let current = rt.visibleApps.first { $0.id == rt.document.appId }
                ?? AppSummary(id: rt.document.appId, name: rt.dsl.app.name, currentVersion: rt.document.version)
            ui.shareTarget = current
        }
    }
    #endif
}
#endif
