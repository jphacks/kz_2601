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
    /// 記録を共有しないアプリのリンクを開いた時の、取り込む前の確認(`AddShareSheet`)
    var pendingImport: PendingImport?

    /// 受け取ったシェアリンク(#278)。サインイン前・bootstrap 前に届いた分も捨てずに持ち、準備ができてから 1 回だけ開く。
    /// 未インストールの人が入れてから開き直すと、サインイン画面の上で届くため(RootView の onOpenURL では取りこぼす)。
    private(set) var pendingLink: URL?
    /// 受け取るたびに増やす番号。開く処理(`SharingHooks` の .task)の id に使う(`takeLink` では変わらないので、開いている途中で取り消されない)
    private(set) var linkSerial = 0
    /// RootView の bootstrap が終わったか(`MetamonApp`)。サインアウトで false に戻す
    var isReady = false

    /// シェアリンクなら保留して true。それ以外の URL は false(触らない)。
    @discardableResult
    func receive(_ url: URL, hold: LaunchConsentHold = .shared) -> Bool {
        guard ShareURL.token(from: url) != nil else { return false }
        pendingLink = url
        linkSerial += 1
        // 起動の途中だけ、開き終わるまで Intent を待たせる(リンク → Intent の順)。
        // 使っている途中に届いたリンクは待たせない(他のシートが出ていると確認が出せず、段が放されないまま残るため)
        hold.beginDuringLaunch(.shareLink)
        return true
    }

    /// Universal Links(#331)。Web 閲覧の activity の URL がシェアリンクなら保留して true。
    @discardableResult
    func receive(activity: NSUserActivity) -> Bool {
        guard activity.activityType == NSUserActivityTypeBrowsingWeb, let url = activity.webpageURL else { return false }
        return receive(url)
    }

    /// 開いたリンクの確認(参加 / 追加)が閉じきったか、出さなかった時。次の段(Intent)へ進む。
    func linkSettled(hold: LaunchConsentHold = .shared) {
        guard pendingLink == nil, pendingJoin == nil, pendingImport == nil else { return }
        hold.end(.shareLink)
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

    /// `tailorit://import/<token>` / `https://tailor-it.app/s/<token>` を受けたら import → そのアプリに変身。それ以外の URL は false。
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
        // #254: 記録を共有するアプリは、参加すると何が見えるかを先に見せて表示名を聞く
        let membership = sharing as? any MembershipClient
        let preview = try? await membership?.previewShare(token: token)
        if let preview, preview.sharedData, !preview.mine {
            // 参加者は無料(正典「課金・Pro」2026-11-06: 共有アプリの費用は作成者の Pro)。課金画面は出さない(H5)
            SharingUI.shared.pendingJoin = PendingJoin(token: token, preview: preview)
            return
        }
        // それ以外も取り込む前に「追加しますか?」を出す。プレビューが取れない時は名前なしで聞く
        SharingUI.shared.pendingImport = PendingImport(token: token, preview: preview)
    }

    /// 確認画面の「追加する」。成功したらシートを閉じて変身し nil、失敗したら画面に出す文言を返す(シートは開いたまま)。
    func confirmImport(_ pending: PendingImport) async -> String? {
        guard let sharing = services.sharing else {
            return L10n.tr("シェアリンクはサーバー接続時のみ開けます")
        }
        do {
            let app = try await sharing.importShare(token: pending.token)
            SharingUI.shared.pendingImport = nil
            await adopt(app, banner: L10n.tr("シェアされた「\(app.name)」を追加しました"))
            return nil
        } catch {
            if let limit = AppLimit(error) { // #630 持てるアプリの数: 追加の確認の上に置き換えシートを出し、空いたら追加し直す
                presentAppLimit(limit) { [weak self] in _ = await self?.confirmImport(pending) }
                return AppLimitCopy.toast(limit)
            }
            return Self.importFailureMessage(error)
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
            if document.appId == app.appId { await setNotifyConsent(true, appId: app.appId) } // 参加の画面の確認つきで有効(#339)
            return nil
        } catch {
            if let limit = AppLimit(error) { // #630 持てるアプリの数: 参加の確認の上に置き換えシートを出し、空いたら同じ名前で参加し直す
                presentAppLimit(limit) { [weak self] in _ = await self?.joinShared(join, displayName: displayName) }
                return AppLimitCopy.toast(limit)
            }
            return Self.importFailureMessage(error)
        }
    }

    /// 参加の表示名に表示できない言葉があった(#321)。参加の確認画面はこの文の時に表示名の欄を目立たせる
    static var displayNameRejected: String { ErrorCopy.textNotAllowed(fieldName: L10n.tr("表示名")) }

    /// import の失敗。403 の参加の理由(#254)・取り消し済み(404)・その他。
    static func importFailureMessage(_ error: Error) -> String {
        if let denial = SharedAccessDenial(error) { return denial.message }
        if (error as? APIError)?.isTextNotAllowed == true { return displayNameRejected } // #321: import で見るのは表示名だけ
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
            if let limit = AppLimit(error) { // #630 持てるアプリの数
                presentAppLimit(limit) { [weak self] in await self?.installPublic(listing) }
                return
            }
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
/// URL スキームは onOpenURL、Universal Links(`https://tailor-it.app/s/<token>`・#331)は Web 閲覧の NSUserActivity で届く。
/// 連係の Handoff(#1851)も同じ所で受ける。
struct ShareLinkReceiver: ViewModifier {
    func body(content: Content) -> some View {
        content
            .onOpenURL { url in SharingUI.shared.receive(url) }
            .onContinueUserActivity(NSUserActivityTypeBrowsingWeb) { SharingUI.shared.receive(activity: $0) }
            // Mac からの Handoff(#1851)。シェアリンクと同じく、サインイン・bootstrap の前に届いたら準備の後に開く(`IntentHost`)
            .onContinueUserActivity(HandoffActivity.type) { activity in
                guard let target = HandoffTarget(activity: activity) else { return }
                Task { await IntentHost.shared.request(.handoff(target)) }
            }
    }
}

/// RootView に 1 行で付ける: URL スキーム受信・「探す」シート・DEBUG の自動操作フック。
struct SharingHooks: ViewModifier {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.assistiveAccess) private var assistiveAccess
    @State private var ui = SharingUI.shared
    private var splash: SplashPresence { .shared }
    private var consentHold: LaunchConsentHold { .shared }

    func body(content: Content) -> some View {
        let shows = assistiveAccess.shows(.shareLinks)
        return content
            // リンクの受信は `ShareLinkReceiver`(サインイン画面の時も届く)。ここでは準備ができたら開く(#278)
            .task(id: "\(ui.isReady)|\(splash.isShowing)|\(consentHold.isHolding(before: .shareLink))|\(ui.linkSerial)") {
                // スプラッシュ → 起動時の同意 → チュートリアルが閉じてから開く(シートを同時に出すと片方が捨てられる)
                guard !splash.isShowing, !consentHold.isHolding(before: .shareLink) else { return }
                guard let url = ui.takeLink() else { return }
                // アシスティブアクセス(#1997)では追加・参加の確認を出さず、リンクは捨てて一言だけ
                guard shows else {
                    print("[Metamon] share link dropped (assistive access)")
                    rt.toast = AssistiveAccessPolicy.unavailableMessage
                    ui.linkSettled()
                    return
                }
                consentHold.beginDuringLaunch(.shareLink) // 起動の途中なら、受信時の段が放されていても Intent を待たせる
                // 取り出したリンクは id が変わっても最後まで開く(.task の取り消しでプレビューの取得を止めない)
                await Task { @MainActor in
                    await rt.openShareURL(url)
                    ui.linkSettled() // 確認を出さなかった(トースト等)時はここで次へ。出した時は閉じた時(onDismiss)
                }.value
            }
            .sheet(isPresented: Binding(get: { shows && ui.showDiscover }, set: { ui.showDiscover = $0 })) { DiscoverSheet() }
            .sheet(item: Binding(get: { shows ? ui.pendingJoin : nil }, set: { ui.pendingJoin = $0 }),
                   onDismiss: { ui.linkSettled() }) { JoinShareSheet(join: $0).onAppear { consentHold.markShown(.shareLink) } }
            .sheet(item: Binding(get: { shows ? ui.pendingImport : nil }, set: { ui.pendingImport = $0 }),
                   onDismiss: { ui.linkSettled() }) { AddShareSheet(pending: $0).onAppear { consentHold.markShown(.shareLink) } }
            #if DEBUG
            .task { await debugHooks() }
            .modifier(ReleaseRequestHooks())
            #endif
    }

    #if DEBUG
    /// ヘッドレス E2E 用(起動 2.5 秒後):
    /// - `METAMON_AUTO_DISCOVER=1` 探すを開く(`METAMON_AUTO_INSTALL=<public app id>` で続けてインストール)
    /// - `METAMON_AUTO_SHARE=1` 変身一覧 → 今のアプリのシェアシート
    /// - `METAMON_AUTO_IMPORT=<token>` シェアリンクを開いたのと同じ経路(追加 / 参加の確認を出す)
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
