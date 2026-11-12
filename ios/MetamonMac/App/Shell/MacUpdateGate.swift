import SwiftUI
import Observation
import MetamonServices

/// 最低対応版と強制アップデートの Mac 版(#981・iOS の `UpdateGate`)。appconfig `GET /v1/client-config` の
/// `minMacOSBuild` / `latestMacOSBuild` で判定する(iOS の値とは別)。読むのはサービスのお知らせと同じ 1 回(`MacRootView`)。
/// - MIN 未満: 閉じられない「アップデートしてください」のシート
/// - LATEST 未満: 閉じられるお知らせ(同じ latest には 1 起動で 1 回)
/// - 通信失敗・設定なし: 何もしない(fail open)
@MainActor
@Observable
final class MacUpdateGate {
    private(set) var requirement: UpdateRequirement = .none
    private var serverStoreURL: URL?
    private var dismissedLatest: Int?
    @ObservationIgnored private let configuredStoreURL: URL?

    init(configuredStoreURL: URL? = MacAppStoreLink.configured()) {
        self.configuredStoreURL = configuredStoreURL
    }

    /// Mac の版で判定する確認
    static func checker(baseURL: URL) -> UpdateChecker {
        UpdateChecker(client: HTTPClientConfigClient(baseURL: baseURL), platform: .macos)
    }

    func apply(_ result: UpdateChecker.Result) {
        #if DEBUG
        if applyDebugOverride() { return }
        #endif
        requirement = result.requirement
        serverStoreURL = result.appStoreURL
    }

    var isBlocking: Bool {
        if case .required = requirement { return true }
        return false
    }

    var showsNotice: Bool {
        if case .recommended(let latest) = requirement { return dismissedLatest != latest }
        return false
    }

    func dismissNotice() {
        if case .recommended(let latest) = requirement { dismissedLatest = latest }
    }

    /// サーバーの URL → Info.plist → Mac App Store で検索。`https://apps.apple.com` は Mac App Store のアプリで開く
    var storeURL: URL { MacAppStoreLink.macStore(serverStoreURL ?? configuredStoreURL ?? MacAppStoreLink.search) }

    #if DEBUG
    /// `METAMON_FORCE_UPDATE` があれば通信なしでもその状態にする
    @discardableResult
    func applyDebugOverride() -> Bool {
        guard let forced = Self.debugOverride else { return false }
        requirement = forced
        return true
    }

    /// 見た目の確認用: `METAMON_FORCE_UPDATE=required|recommended`(iOS と同じ)
    private static var debugOverride: UpdateRequirement? {
        switch ProcessInfo.processInfo.environment["METAMON_FORCE_UPDATE"] {
        case "required": return .required(minimum: 9999)
        case "recommended": return .recommended(latest: 9999)
        default: return nil
        }
    }
    #endif
}

enum MacAppStoreLink {
    /// Info.plist `MetamonAppStoreURL`(build setting METAMON_APP_STORE_URL・iOS と同じ app id)。未設定なら nil
    static func configured(bundle: Bundle = .main) -> URL? {
        guard let raw = bundle.object(forInfoDictionaryKey: "MetamonAppStoreURL") as? String,
              !raw.isEmpty, !raw.hasPrefix("$("), let url = URL(string: raw) else { return nil }
        return url
    }

    /// TODO(#981): Mac の App Store のページが決まるまでの代わり。Mac App Store で「Tailor It」を検索する
    static let search = URL(string: "macappstore://search.itunes.apple.com/WebObjects/MZSearch.woa/wa/search?media=software&term=Tailor%20It")!

    /// `https://apps.apple.com/...` をブラウザを経ずに Mac App Store で開く形(`macappstore://`)にする。ほかはそのまま
    static func macStore(_ url: URL) -> URL {
        guard url.scheme == "https", url.host == "apps.apple.com",
              var parts = URLComponents(url: url, resolvingAgainstBaseURL: false) else { return url }
        parts.scheme = "macappstore"
        return parts.url ?? url
    }
}

/// MIN 未満の版で出す、閉じられないシート。終了はできる
struct MacUpdateRequiredView: View {
    let storeURL: URL
    @Environment(\.openURL) private var openURL

    var body: some View {
        VStack(spacing: 16) {
            Image(systemName: "arrow.down.app")
                .font(.system(size: 56))
                .foregroundStyle(.tint)
                .accessibilityHidden(true)
            Text("アップデートしてください")
                .font(.title2.bold())
            Text("この版の Tailor It は使えなくなりました。Mac App Store から最新の版にアップデートしてください。")
                .multilineTextAlignment(.center)
                .macSecondaryText()
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 12) {
                Button("Tailor It を終了") { NSApp.terminate(nil) }
                    .accessibilityIdentifier("update.quit")
                Button("Mac App Store を開く") { openURL(storeURL) }
                    .buttonStyle(.borderedProminent)
                    .keyboardShortcut(.defaultAction)
                    .accessibilityIdentifier("update.openStore")
            }
            .controlSize(.large)
            .padding(.top, 8)
        }
        .padding(32)
        .frame(width: 420)
        .interactiveDismissDisabled()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("update.required")
    }
}

/// 閉じられない更新のシートと、閉じられる「新しい版があります」
struct MacUpdateGateModifier: ViewModifier {
    let gate: MacUpdateGate
    @Environment(\.openURL) private var openURL

    func body(content: Content) -> some View {
        content
            .sheet(isPresented: Binding(get: { gate.isBlocking }, set: { _ in })) {
                MacUpdateRequiredView(storeURL: gate.storeURL)
            }
            .alert("新しい版があります", isPresented: Binding(get: { gate.showsNotice && !gate.isBlocking }, set: { if !$0 { gate.dismissNotice() } })) {
                Button("アップデート") {
                    gate.dismissNotice()
                    openURL(gate.storeURL)
                }
                Button("あとで", role: .cancel) { gate.dismissNotice() }
            } message: {
                Text("Mac App Store で最新の版にアップデートできます。")
            }
    }
}
