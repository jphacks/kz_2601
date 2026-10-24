import SwiftUI
import MetamonServices

/// リリース申請(#76)の UI 状態。変身一覧のカードのバッジとシートで共有する。
@MainActor
@Observable
final class ReleaseUI {
    static let shared = ReleaseUI()

    /// 申請シートを開く対象
    var target: AppSummary?
    /// appId → いま見せる申請(申請中があればそれ)
    var byApp: [String: ReleaseRequest] = [:]
    #if DEBUG
    /// E2E: シートが開いたらこの内容を入力して送る(`ReleaseRequestHooks`)
    var autoSend: (purpose: String, contact: String)?
    #endif

    func refresh(_ client: (any ReleaseRequestClient)?) async {
        guard let client, let items = try? await client.myRequests() else { return }
        byApp = ReleaseRequest.latestByApp(items)
    }
}

extension ReleaseStatus {
    var label: String {
        switch self {
        case .received: return L10n.tr("受付")
        case .reviewing: return L10n.tr("検討中")
        case .planned: return L10n.tr("リリース予定")
        case .declined: return L10n.tr("見送り")
        }
    }

    var symbol: String {
        switch self {
        case .received: return "tray.and.arrow.down"
        case .reviewing: return "magnifyingglass"
        case .planned: return "shippingbox"
        case .declined: return "xmark.circle"
        }
    }
}

/// 変身一覧のカード右上: 「…」メニュー(シェア・公開 / リリース申請)+ 申請済みなら status の小さなバッジ。
struct AppCardActions: View {
    let app: AppSummary
    @Environment(AppRuntime.self) private var rt
    @State private var sharingUI = SharingUI.shared
    @State private var releaseUI = ReleaseUI.shared

    var body: some View {
        VStack(alignment: .trailing, spacing: Theme.Spacing.xs) {
            if rt.canShare || rt.services.releaseRequests != nil {
                Menu {
                    if rt.canShare {
                        Button { sharingUI.shareTarget = app } label: {
                            Label("シェア・公開", systemImage: "square.and.arrow.up")
                        }
                    }
                    if rt.services.releaseRequests != nil {
                        Button { releaseUI.target = app } label: {
                            Label("単体アプリとしてリリースを申請", systemImage: "shippingbox")
                        }
                    }
                } label: {
                    Image(systemName: "ellipsis")
                        .font(.footnote.weight(.bold))
                        .foregroundStyle(.white)
                        .frame(width: 32, height: 32)
                        .background(.black.opacity(0.35), in: Circle())
                        .overlay(Circle().strokeBorder(.white.opacity(0.3)))
                }
                .accessibilityLabel(L10n.tr("\(app.name)のメニュー"))
            }
            if let req = releaseUI.byApp[app.id] {
                ReleaseStatusBadge(status: req.kind)
            }
        }
        .padding(Theme.Spacing.sm)
    }
}

struct ReleaseStatusBadge: View {
    let status: ReleaseStatus

    var body: some View {
        Label(status.label, systemImage: status.symbol)
            .font(.caption2.weight(.bold))
            .foregroundStyle(.white)
            .padding(.horizontal, Theme.Spacing.sm)
            .padding(.vertical, 4)
            .background(.black.opacity(status == .declined ? 0.35 : 0.55), in: Capsule())
            .accessibilityLabel(L10n.tr("リリース申請: \(status.label)"))
    }
}

/// 「単体アプリとしてリリースを申請」シート。用途の一言(必須)と連絡先(任意)を送る。
struct ReleaseRequestSheet: View {
    let app: AppSummary
    @Environment(\.services) private var services
    @Environment(\.dismiss) private var dismiss
    @State private var ui = ReleaseUI.shared

    @State private var purpose = ""
    @State private var contact = ""
    @State private var busy = false
    @State private var error: String?
    /// 今回送って受け付けられた(「受け付けました」表示)
    @State private var justSent = false

    private static let maxPurpose = 200
    private static let maxContact = 100

    /// 申請中(見送り以外)があればフォームの代わりに状況を出す
    private var open: ReleaseRequest? {
        guard let r = ui.byApp[app.id], r.kind != .declined else { return nil }
        return r
    }

    private var canSend: Bool { !purpose.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !busy }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Theme.Spacing.lg) {
                header
                if let open {
                    statusSection(open)
                } else {
                    formSection
                }
            }
            .padding(Theme.Spacing.lg)
        }
        .background(Theme.Colors.canvas.ignoresSafeArea())
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .sensoryFeedback(.success, trigger: justSent)
        .task {
            await ui.refresh(services.releaseRequests)
            #if DEBUG
            if let auto = ui.autoSend, open == nil {
                ui.autoSend = nil
                purpose = auto.purpose
                contact = auto.contact
                try? await Task.sleep(for: .seconds(2))
                await send()
                try? await Task.sleep(for: .seconds(3))
                dismiss() // カードのバッジを見せる
            }
            #endif
        }
    }

    private var header: some View {
        HStack(alignment: .top) {
            VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                Text("「\(app.name)」を単体アプリに")
                    .font(.system(size: 22, weight: .bold, design: .rounded))
                Text("Tailor It の外でも使える 1 つのアプリとして公開してほしい、と運営に申請します。みんなに需要があると判断したら、運営がリリース作業を行います。")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer()
            Button { dismiss() } label: {
                Image(systemName: "xmark")
                    .font(.footnote.weight(.bold))
                    .foregroundStyle(.secondary)
                    .padding(Theme.Spacing.sm + 2)
                    .background(Theme.Colors.surface, in: Circle())
            }
            .accessibilityLabel("閉じる")
        }
    }

    private var formSection: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.md) {
            Text("どんな用途で使っていますか?")
                .font(.headline)
            TextField("用途の一言(必須) 例: 毎朝の体調記録に使っています", text: $purpose, axis: .vertical)
                .lineLimit(2...4)
                .textFieldStyle(.roundedBorder)
                .onChange(of: purpose) { _, v in if v.count > Self.maxPurpose { purpose = String(v.prefix(Self.maxPurpose)) } }
            TextField("連絡先(任意) メールや SNS など", text: $contact)
                .textFieldStyle(.roundedBorder)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .onChange(of: contact) { _, v in if v.count > Self.maxContact { contact = String(v.prefix(Self.maxContact)) } }
            Text("今のバージョンの画面構成が一緒に送られます(記録の中身は送られません)")
                .font(.caption)
                .foregroundStyle(.secondary)
            if let error {
                Label(error, systemImage: "exclamationmark.triangle")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            Button { Task { await send() } } label: {
                Group {
                    if busy { ProgressView().tint(.white) } else { Text("申請する") }
                }
                .font(.subheadline.weight(.semibold))
                .frame(maxWidth: .infinity)
                .frame(height: 44)
            }
            .buttonStyle(.borderedProminent)
            .disabled(!canSend)
        }
        .metamonCard()
    }

    private func statusSection(_ r: ReleaseRequest) -> some View {
        VStack(spacing: Theme.Spacing.md) {
            Image(systemName: justSent ? "checkmark.circle.fill" : r.kind.symbol)
                .font(.system(size: 44))
                .foregroundStyle(Theme.brandGradient)
            Text(justSent ? L10n.tr("受け付けました") : L10n.tr("申請済みです"))
                .font(.title3.weight(.bold))
            HStack(spacing: Theme.Spacing.sm) {
                Text("いまの状況")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                ReleaseStatusBadge(status: r.kind)
            }
            if !r.purpose.isEmpty {
                Text("「\(r.purpose)」")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            Text("状況が変わると変身一覧のカードに表示されます")
                .font(.caption)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
            Button { dismiss() } label: {
                Text("閉じる").frame(maxWidth: .infinity).frame(height: 44)
            }
            .buttonStyle(.bordered)
        }
        .frame(maxWidth: .infinity)
        .metamonCard()
    }

    private func send() async {
        guard let client = services.releaseRequests else {
            error = L10n.tr("リリース申請はサーバー接続時のみ使えます")
            return
        }
        busy = true
        error = nil
        defer { busy = false }
        let c = contact.trimmingCharacters(in: .whitespacesAndNewlines)
        do {
            let r = try await client.request(appId: app.id,
                                             purpose: purpose.trimmingCharacters(in: .whitespacesAndNewlines),
                                             contact: c.isEmpty ? nil : c)
            ui.byApp[app.id] = r
            justSent = true
            print("[Metamon] release request: id=\(r.id) status=\(r.status)")
        } catch {
            self.error = L10n.tr("申請できませんでした: \(error.localizedDescription)")
        }
    }
}

#if DEBUG
/// ヘッドレス E2E 用(起動 2.5 秒後): `METAMON_AUTO_RELEASE=1` 変身一覧 → 今のアプリの申請シート。
/// `METAMON_AUTO_RELEASE_APP=<appId>` で対象を指定、`METAMON_AUTO_RELEASE_PURPOSE=<文>` を付けると入力して送信 → 閉じてバッジまで。
struct ReleaseRequestHooks: ViewModifier {
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        content.task {
            let env = ProcessInfo.processInfo.environment
            guard env["METAMON_AUTO_RELEASE"] == "1" else { return }
            try? await Task.sleep(for: .seconds(2.5))
            rt.openSwitcher()
            let want = env["METAMON_AUTO_RELEASE_APP"] ?? rt.document.appId
            for _ in 0..<20 where !rt.visibleApps.contains(where: { $0.id == want }) {
                try? await Task.sleep(for: .seconds(0.5)) // 一覧の読み込み待ち
            }
            try? await Task.sleep(for: .seconds(1.5))
            let current = rt.visibleApps.first { $0.id == want }
                ?? AppSummary(id: rt.document.appId, name: rt.dsl.app.name, currentVersion: rt.document.version)
            if let purpose = env["METAMON_AUTO_RELEASE_PURPOSE"], !purpose.isEmpty {
                ReleaseUI.shared.autoSend = (purpose, env["METAMON_AUTO_RELEASE_CONTACT"] ?? "")
            }
            ReleaseUI.shared.target = current
            print("[Metamon] auto release: target=\(current.id)")
        }
    }
}
#endif
