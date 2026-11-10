// 審査用ビルド(APP_STORE_REVIEW・#452 G3)ではバイナリから外す(審査メモの「WebView を使わない」)。
#if !APP_STORE_REVIEW
import SwiftUI
import WebKit
import MetamonDSL

/// `webView`: https URL をアプリ内の WKWebView で表示。上に 戻る/進む/再読込 の小ツールバー + 読み込み中インジケータ。
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
            }
            .background(Color.platformBackground)
            .clipShape(RoundedRectangle(cornerRadius: 12))
            .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(Color.platformSeparator, lineWidth: 0.5))
        } else {
            UnsupportedComponent(label: L("webView: https の URL だけ表示できます"))
        }
    }

    private func toolbar(_ url: URL) -> some View {
        HStack(spacing: 12) {
            Button { state.goBack?() } label: { Image(systemName: "chevron.backward") }
                .disabled(!state.canGoBack)
                .accessibilityLabel(L("前のページ"))
            Button { state.goForward?() } label: { Image(systemName: "chevron.forward") }
                .disabled(!state.canGoForward)
                .accessibilityLabel(L("次のページ"))
            Text(c.title ?? state.pageTitle)
                .font(.subheadline.weight(.medium))
                .lineLimit(1)
                .frame(maxWidth: .infinity)
            Button { state.reload?() } label: {
                Image(systemName: state.isLoading ? "xmark" : "arrow.clockwise")
            }
            .accessibilityLabel(state.isLoading ? L("読み込みを止める") : L("再読み込み"))
            #if os(macOS)
            // Mac(#961): 今のページを既定のブラウザで開く(NSWorkspace)。
            Button { PlatformSystem.open(state.currentURL ?? url) } label: { Image(systemName: "safari") }
                .help(L("ブラウザで開く"))
                .accessibilityLabel(L("ブラウザで開く"))
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
    case ignore

    /// 表示できるのは https だけ。ページ内リンクで他スキーム(tel: / line:// など)に飛ぶ時は allowlist 内なら OS に渡し、それ以外は無視する。
    /// Mac は新しいウインドウで開くリンク(target=_blank)を既定のブラウザに渡す(#961)。iOS は今まで通り。
    static func decide(_ url: URL?, opensNewWindow: Bool, externalNewWindows: Bool = isMac) -> WebLinkPolicy {
        guard let url, let scheme = url.scheme?.lowercased() else { return .ignore }
        if scheme == "https" || scheme == "http" || scheme == "about" {
            if externalNewWindows, opensNewWindow, scheme == "https" { return .openExternally(url) }
            return .allow
        }
        return ExternalURLPolicy.isAllowed(url) ? .openExternally(url) : .ignore
    }

    static var isMac: Bool {
        #if os(macOS)
        true
        #else
        false
        #endif
    }
}

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
        let web = WKWebView(frame: .zero, configuration: config)
        web.navigationDelegate = coordinator
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
    final class Coordinator: NSObject, WKNavigationDelegate {
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
            switch WebLinkPolicy.decide(action.request.url, opensNewWindow: action.targetFrame == nil) {
            case .allow:
                decisionHandler(.allow)
            case .openExternally(let url):
                PlatformSystem.open(url)
                decisionHandler(.cancel)
            case .ignore:
                decisionHandler(.cancel)
            }
        }
    }
}
#endif
