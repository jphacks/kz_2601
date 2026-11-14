// 審査用ビルド(APP_STORE_REVIEW)ではフィードバックと一緒に外す(#374・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import SwiftUI
import Observation
import MetamonServices

/// 設定の「お知らせ」(#374): 送ったフィードバックが直った時の「あなたの声で直りました」。
@MainActor
@Observable
final class NoticesModel {
    enum Phase: Equatable {
        case loading
        case loaded
        case failed
    }

    private(set) var phase: Phase = .loading
    private(set) var items: [LoopNotice] = []
    /// すべてのページの未読の数(設定の行のバッジ)
    private(set) var unreadCount = 0
    private(set) var nextPageToken: String?
    private(set) var isLoadingMore = false
    /// push の同意(nil = まだ読んでいない・読めなかった)
    private(set) var pushEnabled: Bool?
    /// 同意の切り替えに失敗した(元に戻した)
    private(set) var pushFailed = false

    @ObservationIgnored private let client: any LoopNoticesClient
    @ObservationIgnored private let now: () -> Date

    init(client: any LoopNoticesClient, now: @escaping () -> Date = Date.init) {
        self.client = client
        self.now = now
    }

    /// 最初のページ(引っぱって更新も同じ)。失敗しても前の一覧は残す。
    func load() async {
        do {
            let page = try await client.notices(pageToken: nil)
            items = page.items
            unreadCount = page.unreadCount
            nextPageToken = page.nextPageToken
            phase = .loaded
        } catch {
            if phase != .loaded { phase = .failed }
        }
    }

    func loadMore() async {
        guard let token = nextPageToken, !isLoadingMore else { return }
        isLoadingMore = true
        defer { isLoadingMore = false }
        guard let page = try? await client.notices(pageToken: token) else { return }
        let known = Set(items.map(\.id))
        items += page.items.filter { !known.contains($0.id) }
        unreadCount = page.unreadCount
        nextPageToken = page.nextPageToken
    }

    /// 1 件を既読に。先に見た目を変え、失敗したら戻す。
    func markRead(_ id: String) async {
        guard let i = items.firstIndex(where: { $0.id == id }), items[i].isUnread else { return }
        items[i].readAt = now()
        unreadCount = max(0, unreadCount - 1)
        do {
            try await client.markRead(id: id)
        } catch {
            if let j = items.firstIndex(where: { $0.id == id }) { items[j].readAt = nil }
            unreadCount += 1
        }
    }

    /// 一覧を閉じた時: 見えていた未読をまとめて既読に(開いている間は未読の点を残す)。
    func markAllSeen() async {
        for id in items.filter(\.isUnread).map(\.id) { await markRead(id) }
    }

    func loadConsent() async {
        pushEnabled = try? await client.feedbackFixedConsent()
    }

    /// push の同意のオン / オフ。楽観的に変え、失敗したら戻す。
    func setPush(_ enabled: Bool) async {
        guard let before = pushEnabled, before != enabled else { return }
        pushEnabled = enabled
        pushFailed = false
        do {
            try await client.setFeedbackFixedConsent(enabled)
        } catch {
            pushEnabled = before
            pushFailed = true
        }
    }
}

/// 設定の行。未読があればバッジ。live の時だけ出す。
struct NoticesSettingsRow: View {
    let client: any LoopNoticesClient
    @State private var model: NoticesModel?

    var body: some View {
        NavigationLink {
            if let model { NoticesView(model: model) }
        } label: {
            Label("お知らせ", systemImage: "bell")
        }
        .badge(model?.unreadCount ?? 0)
        .accessibilityIdentifier("settings.notices")
        .task {
            if model == nil { model = NoticesModel(client: client) }
            if model?.phase != .loaded { await model?.load() }
        }
    }
}

/// お知らせの一覧: 題名・本文・日付・未読の点。読み込み中はスケルトン、無ければ空の案内、引っぱって更新。
struct NoticesView: View {
    @Bindable var model: NoticesModel

    var body: some View {
        List {
            switch model.phase {
            case .loading where model.items.isEmpty:
                Section {
                    ForEach(0..<3, id: \.self) { _ in NoticeRow.placeholder }
                }
                .redacted(reason: .placeholder)
                .accessibilityLabel(Text("読み込み中"))
            case .failed where model.items.isEmpty:
                ContentUnavailableView {
                    Label("読み込めませんでした", systemImage: "wifi.exclamationmark")
                } description: {
                    Text("つながる場所で、引っぱって読み込み直してください。")
                } actions: {
                    Button("もう一度") { Task { await model.load() } }
                }
                .listRowBackground(Color.clear)
            default:
                if model.items.isEmpty {
                    ContentUnavailableView {
                        Label("お知らせはありません", systemImage: "bell")
                    } description: {
                        Text("送ったフィードバックで直ったものがあると、ここでお知らせします。")
                    }
                    .listRowBackground(Color.clear)
                    .accessibilityIdentifier("notices.empty")
                } else {
                    Section {
                        ForEach(model.items) { n in
                            NoticeRow(notice: n)
                                .contentShape(Rectangle())
                                .onTapGesture { Task { await model.markRead(n.id) } }
                                .onAppear {
                                    if n.id == model.items.last?.id { Task { await model.loadMore() } }
                                }
                        }
                        if model.isLoadingMore {
                            ProgressView().frame(maxWidth: .infinity)
                        }
                    }
                }
            }
            if let enabled = model.pushEnabled {
                Section {
                    Toggle("直った時に通知でも知らせる", isOn: Binding(get: { enabled }, set: { on in Task { await model.setPush(on) } }))
                        .accessibilityIdentifier("notices.pushConsent")
                } footer: {
                    if model.pushFailed {
                        Text("変更できませんでした。元に戻しました")
                    } else {
                        Text("オフにしても、お知らせはこの画面に届きます。")
                    }
                }
            }
        }
        .navigationTitle("お知らせ")
        .refreshable { await model.load() }
        .task {
            await model.load()
            await model.loadConsent()
        }
        .onDisappear { Task { await model.markAllSeen() } }
    }
}

struct NoticeRow: View {
    let notice: LoopNotice

    static var placeholder: some View {
        NoticeRow(notice: LoopNotice(id: "placeholder", title: "直りました: 写真の欄で落ちる",
                                     body: "あなたの声で直りました。ありがとうございます。", createdAt: Date(), readAt: Date()))
    }

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.sm) {
            Circle()
                .fill(notice.isUnread ? Color.accentColor : .clear)
                .frame(width: 8, height: 8)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 4) {
                Text(verbatim: notice.title)
                    .font(.subheadline.weight(notice.isUnread ? .semibold : .regular))
                Text(verbatim: notice.body)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Text(notice.createdAt, format: .dateTime.year().month().day())
                    .font(.caption)
                    .foregroundStyle(.tertiary)
            }
        }
        .padding(.vertical, 2)
        .accessibilityElement(children: .combine)
        .accessibilityValue(notice.isUnread ? Text("未読") : Text(verbatim: ""))
        .accessibilityIdentifier("notices.row.\(notice.id)")
    }
}
#endif
