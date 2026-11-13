#if !APP_STORE_REVIEW // 審査用の版は OAuth の provider を外す(決定 6・iOS と同じ)
import AppKit
import AuthenticationServices
import Observation
import SwiftUI
import MetamonRenderer
import MetamonServices

/// 設定の「つないだサービス」タブ(#1714)。iOS の `ConnectedServicesSection`(#1493)の Mac 版。
/// 状態と操作は iOS と同じ `ConnectedServicesModel`(共有のファイル)。トークンは runner だけが持ち、端末には来ない。
/// - つなぐ: `MacWebAuthenticator`(ASWebAuthenticationSession・前面のウインドウに出す)
/// - 戻り先 `tailorit://connected?…` が外から届いた時は `MacLinkReceiver` → `MacConnectCallbackInbox` で一覧を取り直す
/// - サーバーにつながっていない(appconfig の URL 無し)時は「この端末では使えません」
struct MacConnectedServicesSettings: View {
    var client: (any ExternalServicesClient)? = MacServices.shared.external

    var body: some View {
        Form {
            if let client {
                MacConnectedServicesSection(client: client)
            } else {
                Section {
                    MacUnavailableSettingRow(title: "つないだサービス")
                } footer: {
                    Text("カレンダーやタスクなどの外のサービスとつなぐと、アプリから使えます。Pro で使えます。")
                }
            }
        }
        .formStyle(.grouped)
    }
}

/// 一覧・つなぐ・つなぎ直す・切る(iOS の `ConnectedServicesSection` と同じ文言・同じ出し分け)。
struct MacConnectedServicesSection: View {
    let client: any ExternalServicesClient
    @State private var model = ConnectedServicesModel()
    @State private var confirmDisconnect: ConnectedServiceRow?
    @State private var showPaywall = false
    private let inbox = MacConnectCallbackInbox.shared

    var body: some View {
        Group {
            switch model.state {
            case .hidden:
                // サーバーが外の API を止めている(ext_unavailable)。iOS はセクションごと隠すが、Mac はタブなので理由を出す
                Section { MacUnavailableSettingRow(title: "つないだサービス") }
            case .loading:
                section { HStack { ProgressView().controlSize(.small); Text("読み込み中").foregroundStyle(.secondary) } }
            case .failed(let text):
                section {
                    Text(text).foregroundStyle(.secondary)
                    Button("再試行") { Task { await model.load(client) } }
                        .accessibilityIdentifier("settings.connections.retry")
                }
            case .loaded(let rows):
                section { ForEach(rows) { row($0) } }
            }
        }
        .task { await model.load(client) }
        // 戻り先が外(ブラウザ)から届いた: 一覧を取り直す。失敗の戻り先なら一言出す
        .onChange(of: inbox.revision) { _, _ in
            Task {
                await model.load(client)
                if inbox.last.map(MacConnectCallbackInbox.isFailure) == true {
                    model.message = L10n.tr("つなげませんでした。もう一度試してください。")
                }
            }
        }
        .sheet(isPresented: $showPaywall) { MacPaywallView(reason: .paidFeature([Self.paidFeature])) }
        .confirmationDialog(confirmTitle, isPresented: Binding(get: { confirmDisconnect != nil }, set: { if !$0 { confirmDisconnect = nil } }),
                            titleVisibility: .visible, presenting: confirmDisconnect) { row in
            Button("切る", role: .destructive) {
                Task { await model.disconnect(row.provider, client: client) }
            }
            .accessibilityIdentifier("settings.connections.disconnect.confirm")
            Button("キャンセル", role: .cancel) {}
        } message: { _ in
            Text("切ると、このサービスを使うアプリの部品が動かなくなります。もう一度つなげば戻ります。")
        }
    }

    private var confirmTitle: String {
        L10n.tr("\(confirmDisconnect?.name ?? "") とのつなぎを切りますか？")
    }

    private func section(@ViewBuilder _ content: () -> some View) -> some View {
        Section {
            content()
        } header: {
            Text("つないだサービス")
        } footer: {
            Text(model.message ?? L10n.tr("カレンダーやタスクなどの外のサービスとつなぐと、アプリから使えます。Pro で使えます。"))
                .foregroundStyle(model.message == nil ? AnyShapeStyle(.secondary) : AnyShapeStyle(Theme.Colors.warning))
        }
        .accessibilityIdentifier("settings.connections")
    }

    private func row(_ row: ConnectedServiceRow) -> some View {
        LabeledContent {
            if model.busy == row.provider {
                ProgressView().controlSize(.small)
            } else {
                actions(row)
            }
        } label: {
            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: row.name)
                    .foregroundStyle(row.isUnavailable ? AnyShapeStyle(.secondary) : AnyShapeStyle(.primary))
                Text(MacConnectedServicesCopy.status(row))
                    .font(.footnote)
                    .foregroundStyle(row.status == .needsReconnect ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(.secondary))
            }
        }
        .disabled(row.isUnavailable || (model.busy != nil && model.busy != row.provider))
        .accessibilityIdentifier("settings.connections.\(row.provider)")
    }

    @ViewBuilder
    private func actions(_ row: ConnectedServiceRow) -> some View {
        switch row.status {
        case .connected:
            Button("切る", role: .destructive) { confirmDisconnect = row }
                .accessibilityIdentifier("settings.connections.\(row.provider).disconnect")
        case .needsReconnect:
            HStack {
                if row.enabled {
                    Button("つなぎ直す") { connect(row.provider) }
                        .accessibilityIdentifier("settings.connections.\(row.provider).reconnect")
                }
                Button("切る", role: .destructive) { confirmDisconnect = row }
                    .accessibilityIdentifier("settings.connections.\(row.provider).disconnect")
            }
        case .notConnected: // 今つなげない時は行ごと押せない(row の .disabled)
            Button("つなぐ") { connect(row.provider) }
                .accessibilityIdentifier("settings.connections.\(row.provider).connect")
        }
    }

    private func connect(_ provider: String) {
        // 外の API は作成者の Pro を確かめるので、Mac も Pro そのものを見る(iOS と同じ)
        guard MacPro.shared.isPro else {
            showPaywall = true
            return
        }
        Task {
            await model.connect(provider, client: client) { url in
                try await MacWebAuthenticator().authenticate(url: url, callbackScheme: ConnectCallback.scheme)
            }
        }
    }

    static var paidFeature: PaidFeature { ExtDenialCopy.externalServicesFeature }
}

/// 行の状態の文言(iOS の `ConnectedServicesCopy` と同じ)
enum MacConnectedServicesCopy {
    /// 今つなげない(つないでいない)行は「いまは使えません」
    static func status(_ row: ConnectedServiceRow) -> String {
        row.isUnavailable ? L10n.tr("いまは使えません") : status(row.status)
    }

    static func status(_ s: ConnectedServiceStatus) -> String {
        switch s {
        case .connected: L10n.tr("つないでいます")
        case .needsReconnect: L10n.tr("つなぎ直してください")
        case .notConnected: L10n.tr("つないでいません")
        }
    }
}

/// OAuth の同意をシステムのブラウザの画面(ASWebAuthenticationSession)で開き、戻り先の URL を返す。
/// 前にサインインしていた人のブラウザのログインを使い回さない(ephemeral・iOS と同じ)。
@MainActor
final class MacWebAuthenticator: NSObject, ASWebAuthenticationPresentationContextProviding {
    /// 出すウインドウ(前面 → 最初のウインドウ)。テストで差し替える
    var anchor: @MainActor () -> ASPresentationAnchor = { MacWebAuthenticator.keyWindowAnchor() }
    /// 終わるまで自分を持っておく(session は context provider を弱く持つ)
    private var session: ASWebAuthenticationSession?

    func authenticate(url: URL, callbackScheme: String) async throws -> URL {
        try await withCheckedThrowingContinuation { continuation in
            let session = makeSession(url: url, callbackScheme: callbackScheme) { [weak self] result in
                continuation.resume(with: result)
                self?.session = nil
            }
            self.session = session
            if !session.start() {
                self.session = nil
                continuation.resume(throwing: ASWebAuthenticationSessionError(.presentationContextInvalid))
            }
        }
    }

    /// 開く前の session(テストで中身を確かめる)
    func makeSession(url: URL, callbackScheme: String, completion: @escaping @MainActor (Result<URL, Error>) -> Void) -> ASWebAuthenticationSession {
        let session = ASWebAuthenticationSession(url: url, callbackURLScheme: callbackScheme) { callback, error in
            let result: Result<URL, Error> = if let callback { .success(callback) } else { .failure(error ?? URLError(.badServerResponse)) }
            Task { @MainActor in completion(result) }
        }
        session.prefersEphemeralWebBrowserSession = true
        session.presentationContextProvider = self
        return session
    }

    nonisolated func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor {
        MainActor.assumeIsolated { anchor() }
    }

    static func keyWindowAnchor() -> ASPresentationAnchor {
        NSApp.keyWindow ?? NSApp.mainWindow ?? NSApp.windows.first(where: \.isVisible) ?? ASPresentationAnchor()
    }
}

/// OAuth の戻り先 `tailorit://connected?…` が外から届いた時の受け口(`MacLinkReceiver` が入れ、設定のタブが見る)。
/// ふだんは ASWebAuthenticationSession が戻り先を受けるが、ブラウザから直接アプリが開かれた時もここで一覧を取り直す。
@MainActor
@Observable
final class MacConnectCallbackInbox {
    static let shared = MacConnectCallbackInbox()

    /// 最後に届いた戻り先
    private(set) var last: ConnectCallback?
    /// 届くたびに増える(同じ戻り先が 2 回届いても取り直す)
    private(set) var revision = 0

    /// つなぐの戻り先なら受けて true(共有のリンクの流れに渡さない)。ほかの URL は false
    @discardableResult
    func receive(_ url: URL) -> Bool {
        let callback = ConnectCallback.parse(url)
        guard callback != .invalid else { return false }
        last = callback
        revision += 1
        return true
    }

    static func isFailure(_ callback: ConnectCallback) -> Bool {
        if case .error = callback { return true }
        return false
    }
}
#endif
