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
        do {
            let app = try await sharing.importShare(token: token)
            await adopt(app, banner: L10n.tr("シェアされた「\(app.name)」を追加しました"))
        } catch {
            toast = (error as? APIError)?.status == 404
                ? L10n.tr("このシェアリンクは取り消されたか、存在しません")
                : L10n.tr("シェアされたアプリを開けませんでした: \(error.localizedDescription)")
        }
    }

    func installPublic(_ listing: PublicAppListing) async {
        guard let sharing = services.sharing else { return }
        do {
            let app = try await sharing.install(publicAppId: listing.id)
            await adopt(app, banner: L10n.tr("「\(app.name)」をインストールしました"))
        } catch {
            toast = L10n.tr("インストールできませんでした: \(error.localizedDescription)")
        }
    }

    /// import / install の結果に切り替える(変身アニメは switchApp)。共有データ付きは「みんなと同じデータ」を一言添える。
    private func adopt(_ app: InstalledApp, banner: String) async {
        print("[Metamon] adopt shared app: appId=\(app.appId) joined=\(app.joined)")
        if app.appId != document.appId {
            await switchApp(to: app.appId, afterSheetDismiss: true)
        }
        guard document.appId == app.appId else { return } // 切替に失敗(トーストは switchApp が出す)
        summaryBanner = SummaryBanner(text: banner + (app.joined ? L10n.tr("。みんなの投稿も見られます") : ""), highlight: nil)
        await appsList()
    }
}

/// RootView に 1 行で付ける: URL スキーム受信・「探す」シート・DEBUG の自動操作フック。
struct SharingHooks: ViewModifier {
    @Environment(AppRuntime.self) private var rt
    @State private var ui = SharingUI.shared

    func body(content: Content) -> some View {
        content
            .onOpenURL { url in Task { await rt.openShareURL(url) } }
            .sheet(isPresented: $ui.showDiscover) { DiscoverSheet() }
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
