// 審査用ビルドはリリース申請を外す(iOS の ReleaseRequestSheet と同じ・#245)
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 自分のリリース申請(#76)を appId ごとに持つ(全ウインドウで 1 つ)。iOS の `ReleaseUI.byApp` に当たる。
/// 申請のシート(#2249)とサイドバーの状況のバッジが読む。
@MainActor
@Observable
final class MacReleaseRequests {
    static let shared = MacReleaseRequests()

    /// appId → いま見せる申請(申請中があればそれ・`ReleaseRequest.latestByApp`)
    private(set) var byApp: [String: ReleaseRequest] = [:]

    /// 自分の申請を読み直す。読めなければ今のまま(iOS と同じ)
    func refresh(_ client: (any ReleaseRequestClient)?) async {
        guard let client, let items = try? await client.myRequests() else { return }
        byApp = ReleaseRequest.latestByApp(items)
    }

    /// 送れた申請を覚える(読み直しを待たずにシートとバッジへ出す)
    func record(_ request: ReleaseRequest) { byApp[request.appId] = request }

    /// サインアウト・アカウント削除
    func clear() { byApp = [:] }
}

/// サイドバーの右クリックの「リリースを申請…」(#2249)のシートの中身。用途の一言(必須)と連絡先(任意)を送り、
/// 申請中(見送り以外)ならフォームの代わりに状況を出す。入力の決まりと文言は iOS と共有(`ReleaseRequestForm`)。
@MainActor
@Observable
final class MacReleaseRequestModel: Identifiable {
    let id = UUID()
    let appId: String
    let appName: String
    var purpose = ""
    var contact = ""
    private(set) var busy = false
    private(set) var error: String?
    /// 今回送って受け付けられた(「受け付けました」)
    private(set) var justSent = false
    @ObservationIgnored private let client: (any ReleaseRequestClient)?
    @ObservationIgnored private let store: MacReleaseRequests

    init(appId: String, appName: String, client: (any ReleaseRequestClient)?, store: MacReleaseRequests = .shared) {
        self.appId = appId
        self.appName = appName
        self.client = client
        self.store = store
    }

    /// 右クリックに「リリースを申請…」を出すか: サーバーに保存した自分のアプリで、サインインしていて、申請の API がある時だけ
    static func canRequest(isCreated: Bool, isOnServer: Bool, isSharedWithMe: Bool, signedIn: Bool, available: Bool) -> Bool {
        isCreated && isOnServer && !isSharedWithMe && signedIn && available
    }

    /// 申請中(見送り以外)なら状況を出す
    var open: ReleaseRequest? { ReleaseRequestForm.open(store.byApp[appId]) }

    var canSend: Bool { ReleaseRequestForm.canSend(purpose: purpose, busy: busy) }

    func load() async { await store.refresh(client) }

    /// 送る。受け付けられたら true
    @discardableResult
    func send() async -> Bool {
        guard canSend else { return false }
        guard let client else {
            error = L10n.tr("リリース申請はサーバー接続時のみ使えます")
            return false
        }
        busy = true
        error = nil
        defer { busy = false }
        let values = ReleaseRequestForm.values(purpose: purpose, contact: contact)
        do {
            let request = try await client.request(appId: appId, purpose: values.purpose, contact: values.contact)
            store.record(request)
            justSent = true
            return true
        } catch {
            self.error = ErrorCopy.failure(L10n.tr("申請できませんでした"), error, context: "MacReleaseRequestSheet")
            return false
        }
    }
}

struct MacReleaseRequestSheet: View {
    @State var model: MacReleaseRequestModel
    var onClose: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            VStack(alignment: .leading, spacing: 4) {
                Text("「\(model.appName)」を単体アプリに")
                    .font(.title2.weight(.bold))
                    .accessibilityAddTraits(.isHeader)
                Text("Tailor It の外でも使える 1 つのアプリとして公開してほしい、と運営に申請します。みんなに需要があると判断したら、運営がリリース作業を行います。")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if let open = model.open {
                status(open)
            } else {
                form
            }
        }
        .padding(24)
        .frame(width: 460)
        .task { await model.load() }
    }

    private var form: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("どんな用途で使っていますか?")
                .font(.headline)
            TextField("用途の一言(必須) 例: 毎朝の体調記録に使っています", text: $model.purpose, axis: .vertical)
                .lineLimit(2...5)
                .textFieldStyle(.roundedBorder)
                .onChange(of: model.purpose) { _, v in model.purpose = ReleaseRequestForm.clipped(v, max: ReleaseRequestForm.maxPurpose) }
            TextField("連絡先(任意) メールや SNS など", text: $model.contact)
                .textFieldStyle(.roundedBorder)
                .autocorrectionDisabled()
                .onChange(of: model.contact) { _, v in model.contact = ReleaseRequestForm.clipped(v, max: ReleaseRequestForm.maxContact) }
            Text("今のバージョンの画面構成が一緒に送られます(記録の中身は送られません)")
                .font(.caption)
                .foregroundStyle(.secondary)
            if let error = model.error {
                Label(error, systemImage: "exclamationmark.triangle")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .accessibilityAddTraits(.updatesFrequently)
            }
            HStack {
                Spacer()
                Button("キャンセル", action: onClose)
                    .keyboardShortcut(.cancelAction)
                Button {
                    Task { await model.send() }
                } label: {
                    if model.busy { ProgressView().controlSize(.small) } else { Text("申請する") }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(!model.canSend)
            }
        }
    }

    private func status(_ request: ReleaseRequest) -> some View {
        VStack(spacing: 12) {
            Image(systemName: model.justSent ? "checkmark.circle.fill" : request.kind.symbol)
                .font(.system(size: 40))
                .foregroundStyle(.tint)
                .accessibilityHidden(true)
            Text(model.justSent ? L10n.tr("受け付けました") : L10n.tr("申請済みです"))
                .font(.title3.weight(.bold))
            HStack(spacing: 8) {
                Text("いまの状況")
                    .foregroundStyle(.secondary)
                MacReleaseStatusLabel(status: request.kind)
            }
            .accessibilityElement(children: .combine)
            if !request.purpose.isEmpty {
                Text("「\(request.purpose)」")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            HStack {
                Spacer()
                Button("閉じる", action: onClose)
                    .keyboardShortcut(.defaultAction)
            }
        }
        .frame(maxWidth: .infinity)
    }
}

/// 状況の小さな札(iOS の `ReleaseStatusBadge` と同じ色と語)
struct MacReleaseStatusLabel: View {
    let status: ReleaseStatus

    var body: some View {
        Label(status.label, systemImage: status.symbol)
            .font(.caption2.weight(.bold))
            .foregroundStyle(.white)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(.black.opacity(status == .declined ? 0.35 : 0.55), in: Capsule())
            .accessibilityLabel(Text(verbatim: status.accessibilityLabel))
    }
}
#endif
