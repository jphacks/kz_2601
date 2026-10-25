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
                toolbar
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
            .background(Color(.systemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 12))
            .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(Color(.separator), lineWidth: 0.5))
        } else {
            UnsupportedComponent(label: L("webView: https の URL だけ表示できます"))
        }
    }

    private var toolbar: some View {
        HStack(spacing: 12) {
            Button { state.goBack?() } label: { Image(systemName: "chevron.backward") }
                .disabled(!state.canGoBack)
            Button { state.goForward?() } label: { Image(systemName: "chevron.forward") }
                .disabled(!state.canGoForward)
            Text(c.title ?? state.pageTitle)
                .font(.subheadline.weight(.medium))
                .lineLimit(1)
                .frame(maxWidth: .infinity)
            Button { state.reload?() } label: {
                Image(systemName: state.isLoading ? "xmark" : "arrow.clockwise")
            }
        }
        .buttonStyle(.plain)
        .foregroundStyle(.tint)
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(.bar)
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
    var goBack: (() -> Void)?
    var goForward: (() -> Void)?
    var reload: (() -> Void)?
}

private struct WebViewRepresentable: UIViewRepresentable {
    let url: URL
    let state: WebState

    func makeCoordinator() -> Coordinator { Coordinator(state: state) }

    func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.allowsInlineMediaPlayback = true
        let web = WKWebView(frame: .zero, configuration: config)
        web.navigationDelegate = context.coordinator
        web.allowsBackForwardNavigationGestures = true
        web.scrollView.contentInsetAdjustmentBehavior = .never
        context.coordinator.observe(web)
        state.goBack = { [weak web] in web?.goBack() }
        state.goForward = { [weak web] in web?.goForward() }
        state.reload = { [weak web] in
            guard let web else { return }
            if web.isLoading { web.stopLoading() } else { web.reload() }
        }
        web.load(URLRequest(url: url))
        return web
    }

    func updateUIView(_ web: WKWebView, context: Context) {
        // DSL の url が差し替わった時だけ読み直す(同じ URL での再描画でリロードしない)。
        if context.coordinator.loadedURL != url {
            context.coordinator.loadedURL = url
            web.load(URLRequest(url: url))
        }
    }

    static func dismantleUIView(_ web: WKWebView, coordinator: Coordinator) {
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
            ]
        }

        /// 表示できるのは https だけ。ページ内リンクで他スキーム(tel: / line:// など)に飛ぶ時は allowlist 内なら OS に渡し、それ以外は無視する。
        func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                     decisionHandler: @escaping @MainActor (WKNavigationActionPolicy) -> Void) {
            guard let url = action.request.url, let scheme = url.scheme?.lowercased() else { return decisionHandler(.cancel) }
            if scheme == "https" || scheme == "http" || scheme == "about" { return decisionHandler(.allow) }
            if ExternalURLPolicy.isAllowed(url) { UIApplication.shared.open(url) }
            decisionHandler(.cancel)
        }
    }
}
