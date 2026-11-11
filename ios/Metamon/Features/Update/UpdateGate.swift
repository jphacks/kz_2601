import SwiftUI
import MetamonServices

/// 最低対応版と強制アップデート(#302)。起動時と前面に戻った時に appconfig `GET /v1/client-config` を読む。
/// - MIN 未満: 閉じられない「アップデートしてください」画面
/// - LATEST 未満: 閉じられるお知らせ(同じ latest には 1 起動で 1 回)
/// - 通信失敗・設定なし・モック: 何もしない(fail open)
/// 同じ応答に載るアプリ内のお知らせ(#333)も持つ。審査用の版でも出す(運用の機能)。
@MainActor
@Observable
final class UpdateGate {
    private(set) var requirement: UpdateRequirement = .none
    private var serverStoreURL: URL?
    private var dismissedLatest: Int?
    private var checking = false
    private let checker: UpdateChecker
    /// サーバーのお知らせ(この build 向けに絞ったもの)。閉じたかは `dismissals` で見る
    private(set) var notice: ServiceNotice?
    private var dismissedNoticeID: String?
    private let dismissals: NoticeDismissals

    init(checker: UpdateChecker, dismissals: NoticeDismissals = NoticeDismissals()) {
        self.checker = checker
        self.dismissals = dismissals
    }

    convenience init() {
        self.init(checker: UpdateChecker(client: BuildConfig.forceMock ? nil : BuildConfig.appConfigBaseURL.map { HTTPClientConfigClient(baseURL: $0) }))
    }

    func check() async {
        #if DEBUG
        if let forced = Self.debugOverride { requirement = forced; return }
        if let forced = Self.debugNotice { notice = forced; return }
        #endif
        guard !checking else { return }
        checking = true
        defer { checking = false }
        let result = await checker.check()
        requirement = result.requirement
        serverStoreURL = result.appStoreURL
        notice = result.notice
    }

    /// バナーに出すお知らせ。閉じられない更新画面の間は出さない。
    var visibleNotice: ServiceNotice? {
        guard let notice, !isBlocking, notice.id != dismissedNoticeID, !dismissals.isDismissed(notice.id) else { return nil }
        return notice
    }

    /// 閉じた id を端末に覚える(同じ id は二度と出さない)。
    func dismissServiceNotice() {
        guard let notice else { return }
        dismissals.dismiss(notice.id)
        dismissedNoticeID = notice.id // @Observable に変化を伝える(UserDefaults は追跡されない)
    }

    var isBlocking: Bool {
        if case .required = requirement { return true }
        return false
    }

    var showsNotice: Bool {
        if case .recommended(let latest) = requirement { return dismissedLatest != latest }
        return false
    }

    /// 「新しい版があります」のアラートを出すか。スプラッシュの上には出さない(閉じてから出す)。
    var canShowNoticeAlert: Bool {
        showsNotice && !isBlocking && !SplashPresence.shared.isShowing
    }

    func dismissNotice() {
        if case .recommended(let latest) = requirement { dismissedLatest = latest }
    }

    var storeURL: URL { serverStoreURL ?? AppStoreLink.configured ?? AppStoreLink.search }

    #if DEBUG
    /// 見た目の確認用: `METAMON_FORCE_UPDATE=required|recommended`
    private static var debugOverride: UpdateRequirement? {
        switch ProcessInfo.processInfo.environment["METAMON_FORCE_UPDATE"] {
        case "required": return .required(minimum: 9999)
        case "recommended": return .recommended(latest: 9999)
        default: return nil
        }
    }

    /// 見た目の確認用: `METAMON_FORCE_NOTICE=info|warning`(閉じた記録は残らないよう毎回違う id)
    private static var debugNotice: ServiceNotice? {
        guard let raw = ProcessInfo.processInfo.environment["METAMON_FORCE_NOTICE"], let severity = ServiceNotice.Severity(rawValue: raw) else { return nil }
        return ServiceNotice(
            id: "debug-\(UUID().uuidString)", severity: severity,
            message: .init(ja: "AI の生成を一時的に止めています。作ったアプリと記録はそのまま使えます。", en: "AI generation is paused for now. Your apps and records still work."),
            linkUrl: "https://example.com")
    }
    #endif
}

enum AppStoreLink {
    /// Info.plist `MetamonAppStoreURL`(build setting METAMON_APP_STORE_URL)。未設定なら nil。
    static var configured: URL? {
        guard let raw = Bundle.main.object(forInfoDictionaryKey: "MetamonAppStoreURL") as? String,
              !raw.isEmpty, !raw.hasPrefix("$("), let url = URL(string: raw) else { return nil }
        return url
    }

    /// TODO(#302): app id が決まるまでの代わり。App Store で「Tailor It」を検索する。
    static let search = URL(string: "itms-apps://search.itunes.apple.com/WebObjects/MZSearch.woa/wa/search?media=software&term=Tailor%20It")!
}

/// MIN 未満の版で出す、閉じられない画面。
struct UpdateRequiredView: View {
    let storeURL: URL
    @Environment(\.openURL) private var openURL

    var body: some View {
        VStack(spacing: 20) {
            Spacer()
            Image(systemName: "arrow.down.app")
                .font(.system(size: 64))
                .foregroundStyle(.tint)
                .accessibilityHidden(true)
            Text("アップデートしてください")
                .font(.title2.bold())
            Text("この版の Tailor It は使えなくなりました。App Store から最新の版にアップデートしてください。")
                .multilineTextAlignment(.center)
                .foregroundStyle(.secondary)
            Spacer()
            Button {
                openURL(storeURL)
            } label: {
                Text("App Store を開く")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.large)
            .accessibilityIdentifier("update.openStore")
        }
        .padding(32)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color(.systemBackground))
        .accessibilityIdentifier("update.required")
    }
}

/// 起動時・前面に戻った時の確認と、画面 / お知らせの表示。
struct UpdateGateModifier: ViewModifier {
    let gate: UpdateGate
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.openURL) private var openURL

    func body(content: Content) -> some View {
        content
            .overlay {
                if gate.isBlocking {
                    UpdateRequiredView(storeURL: gate.storeURL)
                        .transition(.opacity)
                }
            }
            .safeAreaInset(edge: .top, spacing: 0) {
                if let notice = gate.visibleNotice {
                    ServiceNoticeBanner(notice: notice) { gate.dismissServiceNotice() }
                        .transition(.move(edge: .top).combined(with: .opacity))
                }
            }
            .animation(.easeInOut(duration: 0.25), value: gate.visibleNotice)
            .alert("新しい版があります", isPresented: Binding(get: { gate.canShowNoticeAlert }, set: { if !$0 { gate.dismissNotice() } })) {
                Button("アップデート") {
                    gate.dismissNotice()
                    openURL(gate.storeURL)
                }
                Button("あとで", role: .cancel) { gate.dismissNotice() }
            } message: {
                Text("App Store で最新の版にアップデートできます。")
            }
            .task { await gate.check() }
            .onChange(of: scenePhase) { _, phase in
                if phase == .active { Task { await gate.check() } }
            }
    }
}
