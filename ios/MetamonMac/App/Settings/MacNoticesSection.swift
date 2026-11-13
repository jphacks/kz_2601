// 審査用ビルド(APP_STORE_REVIEW)ではフィードバックと一緒に外す(#374)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 今のアカウントのお知らせの状態(#364)。uid が変わったら(サインアウト・別のアカウント)前の一覧を捨てる。
@MainActor
final class MacNoticesStore {
    let client: (any LoopNoticesClient)?
    private var current: (uid: String, model: NoticesModel)?

    init(client: (any LoopNoticesClient)?) {
        self.client = client
    }

    /// サーバーにつながっていない・サインインしていない時は nil
    func model(for uid: String?) -> NoticesModel? {
        guard let client, let uid else {
            current = nil
            return nil
        }
        if let current, current.uid == uid { return current.model }
        let model = NoticesModel(client: client)
        current = (uid, model)
        return model
    }
}

/// 設定の「お知らせ」のタブ(iOS の `NoticesView` の Mac 版・#364)。状態と行は iOS の `NoticesModel`・`NoticeRow`。
struct MacNoticesSettings: View {
    @State private var store = MacNoticesStore(client: MacServices.shared.loopNotices)
    @State private var model: NoticesModel?
    private let account = MacAccount.shared

    var body: some View {
        Group {
            if store.client == nil {
                Form { MacUnavailableSettingRow(title: "お知らせ") }
                    .formStyle(.grouped)
            } else if let model {
                MacNoticesList(model: model)
                    .id(ObjectIdentifier(model))
            } else {
                Form { MacNoticesSkeleton() }
                    .formStyle(.grouped)
            }
        }
        .task(id: account.session.user?.uid) { model = store.model(for: account.session.user?.uid) }
    }
}

/// 読み込み中(スケルトン)
private struct MacNoticesSkeleton: View {
    var body: some View {
        Section {
            ForEach(0..<3, id: \.self) { _ in NoticeRow.placeholder }
        }
        .redacted(reason: .placeholder)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("読み込み中"))
    }
}

/// 一覧: 押したお知らせを既読に(`POST /v1/me/notices/{id}/read`)。タブを離れたら見えていた未読をまとめて既読に(iOS と同じ)。
private struct MacNoticesList: View {
    @Bindable var model: NoticesModel

    var body: some View {
        Form {
            switch model.phase {
            case .loading where model.items.isEmpty:
                MacNoticesSkeleton()
            case .failed where model.items.isEmpty:
                ContentUnavailableView {
                    Label("読み込めませんでした", systemImage: "wifi.exclamationmark")
                } description: {
                    Text("つながる場所で、もう一度読み込んでください。")
                } actions: {
                    Button("もう一度") { Task { await model.load() } }
                }
            default:
                if model.items.isEmpty {
                    ContentUnavailableView {
                        Label("お知らせはありません", systemImage: "bell")
                    } description: {
                        Text("送ったフィードバックで直ったものがあると、ここでお知らせします。")
                    }
                    .accessibilityIdentifier("notices.empty")
                } else {
                    Section {
                        ForEach(model.items) { n in
                            Button { Task { await model.markRead(n.id) } } label: {
                                NoticeRow(notice: n).contentShape(Rectangle())
                            }
                            .buttonStyle(.plain)
                            .onAppear {
                                if n.id == model.items.last?.id { Task { await model.loadMore() } }
                            }
                        }
                        if model.isLoadingMore {
                            ProgressView().controlSize(.small).frame(maxWidth: .infinity)
                        }
                    } footer: {
                        HStack {
                            Spacer()
                            Button("読み込み直す") { Task { await model.load() } }
                                .accessibilityIdentifier("notices.reload")
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
        .formStyle(.grouped)
        .task {
            await model.load()
            await model.loadConsent()
        }
        .onDisappear { Task { await model.markAllSeen() } }
    }
}
#endif
