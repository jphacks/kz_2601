// 審査用ビルドは「探す」への公開を外す(iOS の ShareAppSheet と同じ)
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 共有シートの「みんなに公開」(#2070)。iOS の `ShareAppSheet` の `publishSection`・`publish()`・`unpublish()` の Mac 版。
/// 説明・タグを入れて「公開する」→ 注意の確認(`PublishConfirmCopy`)→ 公開。公開中は状態と「公開をやめる」を出す。
/// 失敗した時は表示を変えず(元の表示のまま)、iOS と同じ一言を返す。
@MainActor
@Observable
final class MacPublishModel {
    enum Phase: Equatable {
        case loading
        case unpublished
        case published(PublicAppListing)
    }

    let appId: String
    let appName: String
    private(set) var phase: Phase = .loading
    var description = ""
    var tags = ""
    private(set) var busy = false
    /// 表示できない言葉があった欄(#321 text_not_allowed の details.field)
    private(set) var flaggedField: String?
    @ObservationIgnored private let sharing: any SharingClient

    init(appId: String, appName: String, sharing: any SharingClient) {
        self.appId = appId
        self.appName = appName
        self.sharing = sharing
    }

    var isPublished: Bool {
        if case .published = phase { return true }
        return false
    }

    /// 自分の掲載があるか(iOS と同じく新着の一覧から探す。読めなければ公開していない扱い)
    func load() async {
        let found = try? await sharing.searchPublicApps(query: "", sort: "new").first { $0.id == appId && $0.mine }
        phase = found.map(Phase.published) ?? .unpublished
    }

    /// 入力を直したら、その欄の印を外す
    func edited(_ field: String) {
        if flaggedField == field { flaggedField = nil }
    }

    /// 公開する(確認の後)。出す一言を返す
    func publish() async -> String {
        guard !busy else { return "" }
        busy = true
        defer { busy = false }
        do {
            let listing = try await sharing.publish(appId: appId, description: description,
                                                    tags: PublishCopy.tags(from: tags), authorName: nil)
            flaggedField = nil
            phase = .published(listing)
            return L10n.tr("「探す」に公開しました")
        } catch let api as APIError where api.isTextNotAllowed { // #321
            ErrorLog.record(api, context: "MacPublish publish")
            flaggedField = api.field
            return PublishCopy.textNotAllowedMessage(field: api.field)
        } catch {
            return ErrorCopy.failure(L10n.tr("公開できませんでした"), error, context: "MacPublish")
        }
    }

    /// 公開をやめる。出す一言を返す
    func unpublish() async -> String {
        guard !busy else { return "" }
        busy = true
        defer { busy = false }
        do {
            try await sharing.unpublish(appId: appId)
            phase = .unpublished
            return L10n.tr("公開をやめました")
        } catch {
            return ErrorCopy.failure(L10n.tr("非公開にできませんでした"), error, context: "MacPublish")
        }
    }
}

/// 共有シートの「みんなに公開」の欄
struct MacPublishSection: View {
    @Bindable var model: MacPublishModel
    /// 公開・公開をやめた後の一言(シートの下に出す)
    var onNotice: (String) -> Void
    @State private var confirming = false

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("みんなに公開", systemImage: "globe.asia.australia")
                .font(.headline)
                .accessibilityAddTraits(.isHeader)
            switch model.phase {
            case .loading:
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("公開の状態を確かめています…").foregroundStyle(.secondary)
                }
                .accessibilityElement(children: .combine)
            case .published(let listing):
                Text("「探す」に公開中・\(listing.installs) 人がインストール")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                HStack {
                    Spacer()
                    Button(role: .destructive) {
                        Task { report(await model.unpublish()) }
                    } label: {
                        Text("公開をやめる")
                    }
                    .disabled(model.busy)
                    .help("「探す」に出さないようにする")
                }
            case .unpublished:
                Text("「探す」から誰でも見つけてインストールできるようになります")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                TextField("ひとこと説明(任意)", text: $model.description, axis: .vertical)
                    .lineLimit(2...5)
                    .textFieldStyle(.roundedBorder)
                    .overlay(flagBorder("description"))
                    .onChange(of: model.description) { model.edited("description") }
                TextField("タグ(スペース区切り・任意) 例: 健康 習慣", text: $model.tags)
                    .textFieldStyle(.roundedBorder)
                    .overlay(flagBorder("tags"))
                    .onChange(of: model.tags) { model.edited("tags") }
                HStack {
                    Spacer()
                    Button("公開する…") { confirming = true }
                        .buttonStyle(.borderedProminent)
                        .disabled(model.busy)
                        .help("公開の前の注意を見てから公開する")
                }
            }
        }
        .sheet(isPresented: $confirming) {
            MacPublishConfirmSheet(appName: model.appName,
                                   onPublish: { Task { report(await model.publish()) } },
                                   onClose: { confirming = false })
        }
    }

    private func report(_ message: String) {
        if !message.isEmpty { onNotice(message) }
    }

    /// 表示できない言葉があった欄を赤く囲む
    @ViewBuilder
    private func flagBorder(_ field: String) -> some View {
        if model.flaggedField == field {
            RoundedRectangle(cornerRadius: 6).stroke(.red, lineWidth: 1.5).allowsHitTesting(false)
        }
    }
}

/// 公開の前の注意(iOS の `PublishConfirmSheet` と同じ 4 つ・文言は `PublishConfirmCopy` を共有)
struct MacPublishConfirmSheet: View {
    let appName: String
    let onPublish: () -> Void
    let onClose: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text(PublishConfirmCopy.title(appName: appName))
                .font(.title2.weight(.bold))
                .accessibilityAddTraits(.isHeader)
            Label("公開すると", systemImage: "globe.asia.australia")
                .font(.headline)
            VStack(alignment: .leading, spacing: 10) {
                ForEach(PublishConfirmCopy.points, id: \.text) { p in
                    HStack(alignment: .firstTextBaseline, spacing: 8) {
                        Image(systemName: p.icon)
                            .foregroundStyle(.tint)
                            .frame(width: 22)
                            .accessibilityHidden(true)
                        Text(verbatim: p.text)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
            }
            HStack {
                Spacer()
                Button("やめる", action: onClose)
                    .keyboardShortcut(.cancelAction)
                Button("公開する") {
                    onClose()
                    onPublish()
                }
                .keyboardShortcut(.defaultAction)
                .buttonStyle(.borderedProminent)
            }
        }
        .padding(24)
        .frame(width: 420)
    }
}
#endif
