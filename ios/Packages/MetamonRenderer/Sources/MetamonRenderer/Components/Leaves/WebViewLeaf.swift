// 審査用ビルド(APP_STORE_REVIEW・#452 G3)ではバイナリから外す(審査メモの「WebView を使わない」)。
#if !APP_STORE_REVIEW
import SwiftUI
import WebKit
import MetamonDSL

/// `webView`: https URL をアプリ内の WKWebView で表示。上に 戻る/進む/再読込 の小ツールバー + 読み込み中インジケータ。
/// 新しいウインドウ・ダウンロードの扱い(#1289): リンクで開く新しいウインドウは既定のブラウザへ、スクリプトが開くウインドウは同じ枠で開く。
/// ダウンロードは保存せず一言で知らせる(Mac はツールバーの「ブラウザで開く」から保存できる)。黙って何も起きない動きは作らない。
/// WKWebView 自身が縦スクロールするので `GridScreenLayout.scrollsInside` には入れない(セルの外側で二重スクロールさせない)。
struct WebViewLeaf: View {
    let c: Component
    @State private var state = WebState()

    var body: some View {
        if let url = ExternalURLPolicy.webViewURL(c.url) {
            VStack(spacing: 0) {
                toolbar(url)
                Divider()
                ZStack(alignment: .top) {
                    WebViewRepresentable(url: url, state: state)
                    if state.isLoading {
                        ProgressView(value: state.progress)
                            .progressViewStyle(.linear)
                            .tint(.accentColor)
                    }
                }
                .overlay(alignment: .bottom) { noticeBanner }
            }
            .background(Color.platformBackground)
            .clipShape(RoundedRectangle(cornerRadius: 12))
            .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(Color.platformSeparator, lineWidth: 0.5))
        } else {
            UnsupportedComponent(label: L("webView: https の URL だけ表示できます"))
        }
    }

    /// ダウンロードを止めた時の一言(数秒で消す)
    @ViewBuilder private var noticeBanner: some View {
        if let notice = state.notice {
            Text(notice)
                .font(.footnote)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
                .background(.regularMaterial, in: Capsule())
                .padding(12)
                .accessibilityAddTraits(.isStaticText)
                .task(id: notice) {
                    try? await Task.sleep(for: .seconds(4))
                    if state.notice == notice { state.notice = nil }
                }
        }
    }

    private func toolbar(_ url: URL) -> some View {
        HStack(spacing: 12) {
            Button { state.goBack?() } label: { Image(systemName: "chevron.backward") }
                .iconButtonLabel(L("前のページ"))
                .disabled(!state.canGoBack)
            Button { state.goForward?() } label: { Image(systemName: "chevron.forward") }
                .iconButtonLabel(L("次のページ"))
                .disabled(!state.canGoForward)
            Text(c.title ?? state.pageTitle)
                .font(.subheadline.weight(.medium))
                .lineLimit(1)
                .frame(maxWidth: .infinity)
            Button { state.reload?() } label: {
                Image(systemName: state.isLoading ? "xmark" : "arrow.clockwise")
            }
            .iconButtonLabel(state.isLoading ? L("読み込みを止める") : L("再読み込み"))
            #if os(macOS)
            // Mac(#961): 今のページを既定のブラウザで開く(NSWorkspace)。
            Button { PlatformSystem.open(state.currentURL ?? url) } label: { Image(systemName: "safari") }
                .iconButtonLabel(L("ブラウザで開く"))
            #endif
        }
        .buttonStyle(.plain)
        .foregroundStyle(.tint)
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(.bar)
    }
}

/// webView の中のリンクの扱い(iOS と Mac で同じ判定・テストから使う)。
enum WebLinkPolicy: Equatable {
    case allow
    /// アプリの外(ブラウザ・電話・LINE など)に渡す。iOS は UIApplication、Mac は NSWorkspace。
    case openExternally(URL)
    /// 新しいウインドウを開かず、今の枠で開く(スクリプトの window.open など・#1289)
    case openInPlace(URL)
    case ignore

    /// 表示できるのは https だけ。ページ内リンクで他スキーム(tel: / line:// など)に飛ぶ時は allowlist 内なら OS に渡し、それ以外は無視する。
    /// 新しいウインドウで開くリンク(target=_blank)は既定のブラウザに渡す(Mac は #961・iOS も #1289 で揃えた)。
    /// ブラウザに渡すのは https のリンクを押した時だけ(`linkActivated`)。スクリプトが開くウインドウと http は今の枠で開く。
    static func decide(_ url: URL?, opensNewWindow: Bool, linkActivated: Bool = true) -> WebLinkPolicy {
        guard let url, let scheme = url.scheme?.lowercased() else { return .ignore }
        if scheme == "https" || scheme == "http" || scheme == "about" {
            guard opensNewWindow else { return .allow }
            if scheme == "about" { return .ignore } // 中身の無い窓(about:blank)は開かない
            return linkActivated && scheme == "https" ? .openExternally(url) : .openInPlace(url)
        }
        return ExternalURLPolicy.isAllowed(url) ? .openExternally(url) : .ignore
    }
}

/// webView のダウンロードの扱い(#1289)。保存はしない(アプリの中にファイルを置く場所が無い)。止めた時は一言で知らせる。
enum WebDownloadPolicy {
    /// 応答を表示せず保存に回すべきか: 表示できない種類か、添付(Content-Disposition: attachment)なら止める
    static func blocks(canShowMIMEType: Bool, contentDisposition: String?) -> Bool {
        if !canShowMIMEType { return true }
        guard let value = contentDisposition?.trimmingCharacters(in: .whitespaces).lowercased() else { return false }
        return value.hasPrefix("attachment")
    }

    static var notice: String {
        #if os(macOS)
        L("ファイルはアプリの中では保存できません。「ブラウザで開く」から保存してください")
        #else
        L("ファイルはアプリの中では保存できません")
        #endif
    }
}

#if os(macOS)
/// Mac の右クリックのメニュー(#1289): 新しいウインドウで開く・ダウンロードの項目を外す(押しても何も起きないため)。
/// リンクをブラウザで開きたい時は、リンクを押す(新しいウインドウのリンク)か、ツールバーの「ブラウザで開く」を使う。
enum WebContextMenuPolicy {
    static let removed: Set<String> = [
        "WKMenuItemIdentifierOpenLinkInNewWindow",
        "WKMenuItemIdentifierOpenImageInNewWindow",
        "WKMenuItemIdentifierOpenMediaInNewWindow",
        "WKMenuItemIdentifierOpenFrameInNewWindow",
        "WKMenuItemIdentifierDownloadLinkedFile",
        "WKMenuItemIdentifierDownloadImage",
        "WKMenuItemIdentifierDownloadMedia",
    ]

    static func keeps(_ identifier: String?) -> Bool {
        guard let identifier else { return true }
        return !removed.contains(identifier)
    }
}

/// 右クリックのメニューから `WebContextMenuPolicy` の項目を外す WKWebView
final class MenuFilteringWebView: WKWebView {
    override func willOpenMenu(_ menu: NSMenu, with event: NSEvent) {
        super.willOpenMenu(menu, with: event)
        for item in menu.items where !WebContextMenuPolicy.keeps(item.identifier?.rawValue) {
            menu.removeItem(item)
        }
    }
}
#endif

/// WKWebView の観測値と操作。`@Observable` なので SwiftUI がツールバーの有効/無効を追従する。
@MainActor @Observable
final class WebState {
    var isLoading = false
    var progress: Double = 0
    var canGoBack = false
    var canGoForward = false
    var pageTitle = ""
    /// 今表示しているページ(Mac の「ブラウザで開く」が使う)。
    var currentURL: URL?
    /// ダウンロードを止めた時の一言(#1289)
    var notice: String?
    var goBack: (() -> Void)?
    var goForward: (() -> Void)?
    var reload: (() -> Void)?
}

#if os(iOS)
extension WebViewRepresentable: UIViewRepresentable {
    func makeUIView(context: Context) -> WKWebView { makeWeb(context.coordinator) }
    func updateUIView(_ web: WKWebView, context: Context) { updateWeb(web, context.coordinator) }
    static func dismantleUIView(_ web: WKWebView, coordinator: Coordinator) { dismantle(web, coordinator) }
}
#else
extension WebViewRepresentable: NSViewRepresentable {
    func makeNSView(context: Context) -> WKWebView { makeWeb(context.coordinator) }
    func updateNSView(_ web: WKWebView, context: Context) { updateWeb(web, context.coordinator) }
    static func dismantleNSView(_ web: WKWebView, coordinator: Coordinator) { dismantle(web, coordinator) }
}
#endif

@MainActor private struct WebViewRepresentable {
    let url: URL
    let state: WebState

    func makeCoordinator() -> Coordinator { Coordinator(state: state) }

    func makeWeb(_ coordinator: Coordinator) -> WKWebView {
        let config = WKWebViewConfiguration()
        #if os(iOS)
        config.allowsInlineMediaPlayback = true
        #endif
        #if os(macOS)
        let web: WKWebView = MenuFilteringWebView(frame: .zero, configuration: config)
        #else
        let web = WKWebView(frame: .zero, configuration: config)
        #endif
        web.navigationDelegate = coordinator
        web.uiDelegate = coordinator
        web.allowsBackForwardNavigationGestures = true
        #if os(iOS)
        web.scrollView.contentInsetAdjustmentBehavior = .never
        #endif
        coordinator.observe(web)
        state.goBack = { [weak web] in web?.goBack() }
        state.goForward = { [weak web] in web?.goForward() }
        state.reload = { [weak web] in
            guard let web else { return }
            if web.isLoading { web.stopLoading() } else { web.reload() }
        }
        web.load(URLRequest(url: url))
        return web
    }

    func updateWeb(_ web: WKWebView, _ coordinator: Coordinator) {
        // DSL の url が差し替わった時だけ読み直す(同じ URL での再描画でリロードしない)。
        if coordinator.loadedURL != url {
            coordinator.loadedURL = url
            web.load(URLRequest(url: url))
        }
    }

    static func dismantle(_ web: WKWebView, _ coordinator: Coordinator) {
        coordinator.observations.removeAll()
        web.stopLoading()
    }

    @MainActor
    final class Coordinator: NSObject, WKNavigationDelegate, WKUIDelegate {
        let state: WebState
        var loadedURL: URL?
        var observations: [NSKeyValueObservation] = []

        init(state: WebState) { self.state = state }

        func observe(_ web: WKWebView) {
            observations = [
                web.observe(\.isLoading, options: [.initial, .new]) { [state] w, _ in
                    Task { @MainActor in state.isLoading = w.isLoading }
                },
                web.observe(\.estimatedProgress, options: [.initial, .new]) { [state] w, _ in
                    Task { @MainActor in state.progress = w.estimatedProgress }
                },
                web.observe(\.canGoBack, options: [.initial, .new]) { [state] w, _ in
                    Task { @MainActor in state.canGoBack = w.canGoBack }
                },
                web.observe(\.canGoForward, options: [.initial, .new]) { [state] w, _ in
                    Task { @MainActor in state.canGoForward = w.canGoForward }
                },
                web.observe(\.title, options: [.initial, .new]) { [state] w, _ in
                    Task { @MainActor in state.pageTitle = w.title ?? "" }
                },
                web.observe(\.url, options: [.initial, .new]) { [state] w, _ in
                    Task { @MainActor in state.currentURL = w.url }
                },
            ]
        }

        func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                     decisionHandler: @escaping @MainActor (WKNavigationActionPolicy) -> Void) {
            if action.shouldPerformDownload { // <a download> など(#1289)
                state.notice = WebDownloadPolicy.notice
                decisionHandler(.cancel)
                return
            }
            switch WebLinkPolicy.decide(action.request.url, opensNewWindow: action.targetFrame == nil,
                                        linkActivated: action.navigationType == .linkActivated) {
            case .allow:
                decisionHandler(.allow)
            case .openExternally(let url):
                PlatformSystem.open(url)
                decisionHandler(.cancel)
            case .openInPlace(let url):
                decisionHandler(.cancel)
                webView.load(URLRequest(url: url))
            case .ignore:
                decisionHandler(.cancel)
            }
        }

        /// 表示できない応答・添付は保存せず止めて知らせる(#1289)
        func webView(_ webView: WKWebView, decidePolicyFor response: WKNavigationResponse,
                     decisionHandler: @escaping @MainActor (WKNavigationResponsePolicy) -> Void) {
            let disposition = (response.response as? HTTPURLResponse)?.value(forHTTPHeaderField: "Content-Disposition")
            guard WebDownloadPolicy.blocks(canShowMIMEType: response.canShowMIMEType, contentDisposition: disposition) else {
                decisionHandler(.allow)
                return
            }
            state.notice = WebDownloadPolicy.notice
            decisionHandler(.cancel)
        }

        /// 新しいウインドウは作らない。ここまで来た要求(許した新しいウインドウ)は今の枠で開く(#1289)
        func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration,
                     for action: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
            if let url = action.request.url, case .openInPlace = WebLinkPolicy.decide(url, opensNewWindow: true, linkActivated: false) {
                webView.load(URLRequest(url: url))
            }
            return nil
        }
    }
}
#endif
