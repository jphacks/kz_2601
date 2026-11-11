// 審査用ビルドは共有リンクを外す(iOS の ShareAppSheet と同じ)
#if !APP_STORE_REVIEW
import SwiftUI
import AppKit
import MetamonServices

/// ツールバーの「共有」(#1035): 今のアプリのシェアリンクを作り、コピー・共有メニュー・取り消しを出す。
/// iOS の `ShareAppSheet` のリンクの部分の Mac 版(「探す」への公開は後で)。サーバーに保存したアプリだけ。
@MainActor
@Observable
final class MacShareAppModel: Identifiable {
    enum State: Equatable {
        case loading
        case ready(ShareInfo)
        case failed(String)
    }

    let id = UUID()
    let appId: String
    let appName: String
    private(set) var state: State = .loading
    @ObservationIgnored private let sharing: (any SharingClient)?

    init(appId: String, appName: String, sharing: (any SharingClient)?) {
        self.appId = appId
        self.appName = appName
        self.sharing = sharing
    }

    /// ツールバーの「共有」を押せるか: 作ったアプリで、サーバーにあり、サインインしていて、共有の API がある時だけ。
    static func canShare(isCreated: Bool, isOnServer: Bool, signedIn: Bool, sharingAvailable: Bool) -> Bool {
        isCreated && isOnServer && signedIn && sharingAvailable
    }

    /// 押せない時のヘルプのタグ(押せる時は「このアプリを共有する」)
    static func help(canShare: Bool, hasApp: Bool) -> String {
        if canShare { return L10n.tr("このアプリを共有する") }
        return hasApp ? L10n.tr("サーバーに保存した自分のアプリだけ共有できます") : L10n.tr("アプリを選ぶと共有できます")
    }

    /// 渡すリンク。受け取る人がブラウザでも開ける https(`webUrl`)を先に、無ければアプリのリンク。
    static func link(_ info: ShareInfo) -> URL? {
        if let web = URL(string: info.webUrl), web.scheme == "https" { return web }
        return URL(string: info.url)
    }

    var link: URL? {
        if case .ready(let info) = state { return Self.link(info) }
        return nil
    }

    func issue() async {
        guard let sharing else {
            state = .failed(L10n.tr("シェアはサーバー接続時のみ使えます"))
            return
        }
        state = .loading
        do {
            state = .ready(try await sharing.share(appId: appId))
        } catch {
            state = .failed(L10n.tr("リンクを作れませんでした。もう一度お試しください"))
        }
    }

    /// 今のリンクを取り消し、新しいリンクを用意する(iOS と同じ)。
    func revoke() async {
        guard case .ready(let info) = state, let sharing else { return }
        do {
            try await sharing.revokeShare(token: info.token)
            await issue()
        } catch {
            state = .failed(L10n.tr("取り消せませんでした"))
        }
    }

    /// 「リンクをコピー」。コピーできたら true
    @discardableResult
    func copyLink(to board: NSPasteboard = .general) -> Bool {
        guard let link else { return false }
        board.clearContents()
        return board.setString(link.absoluteString, forType: .string)
    }
}

struct MacShareAppSheet: View {
    @State var model: MacShareAppModel
    var onClose: () -> Void
    @State private var copied = false

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("「\(model.appName)」を共有")
                        .font(.title2.weight(.bold))
                        .accessibilityAddTraits(.isHeader)
                    Text("受け取った人は、リンクから自分のアプリとして使えます")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
                Spacer()
            }
            content
            HStack {
                Spacer()
                Button("閉じる", action: onClose)
                    .keyboardShortcut(.cancelAction)
                    .help("閉じる")
            }
        }
        .padding(24)
        .frame(width: 460)
        .task { await model.issue() }
    }

    @ViewBuilder
    private var content: some View {
        switch model.state {
        case .loading:
            HStack(spacing: 8) {
                ProgressView().controlSize(.small)
                Text("リンクを作っています…").foregroundStyle(.secondary)
            }
            .accessibilityElement(children: .combine)
        case .failed(let message):
            HStack {
                Label(message, systemImage: "exclamationmark.triangle")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                Spacer()
                Button("もう一度") { Task { await model.issue() } }
                    .help("リンクをもう一度作る")
            }
        case .ready:
            if let link = model.link {
                VStack(alignment: .leading, spacing: 12) {
                    Text(link.absoluteString)
                        .font(.callout.monospaced())
                        .textSelection(.enabled)
                        .lineLimit(2)
                        .padding(8)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(.quaternary, in: RoundedRectangle(cornerRadius: 6))
                        .accessibilityLabel(Text("シェアリンク"))
                        .accessibilityValue(Text(link.absoluteString))
                    HStack(spacing: 8) {
                        Button {
                            copied = model.copyLink()
                        } label: {
                            Label(copied ? "コピーしました" : "リンクをコピー", systemImage: copied ? "checkmark" : "doc.on.doc")
                        }
                        .help("リンクをクリップボードにコピーする")
                        ShareLink(item: link, subject: Text(model.appName), preview: SharePreview(model.appName)) {
                            Label("共有…", systemImage: "square.and.arrow.up")
                        }
                        .help("メッセージ・メールなどでリンクを送る")
                        Spacer()
                        Button(role: .destructive) {
                            copied = false
                            Task { await model.revoke() }
                        } label: {
                            Text("リンクを取り消す")
                        }
                        .help("今のリンクを開けなくして、新しいリンクを作る")
                    }
                }
            }
        }
    }
}
#endif
