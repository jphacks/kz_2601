// 審査用ビルド(APP_STORE_REVIEW)は「探す」を外す(iOS の DiscoverSheet と同じ)
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonRenderer
import MetamonServices

/// 「アプリを探す」(#2069): 他の人が公開したアプリを検索して入れ、そのアプリを開く。iOS の `DiscoverSheet` の Mac 版。
/// カード(`PublicAppCard`)・並び順・文言は iOS と共有し、Mac の幅ではカードを横に並べる。
/// 右クリックで通報・ブロック。入れる時に持てるアプリの数に達していれば、このシートの上で置き換える。
struct MacDiscoverView: View {
    @Bindable var model: MacDiscoverModel
    /// 置き換えに使うアプリと消し方(`MacRootView` の置き換えと同じ)
    let replace: MacAppReplaceContext
    let onClose: () -> Void

    private let columns = [GridItem(.adaptive(minimum: 300, maximum: 420), spacing: 16, alignment: .top)]

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            ScrollView {
                content
                    .padding(20)
                    .frame(maxWidth: .infinity, alignment: .top)
            }
            .background(Theme.Colors.canvas)
        }
        .frame(minWidth: 680, idealWidth: 860, minHeight: 480, idealHeight: 620)
        .task(id: model.searchKey) { await model.search() }
        .sheet(item: $model.limitRequest) { request in
            MacAppLimitSheet(limit: request.limit, context: replace, onFreed: request.retry)
        }
        .modifier(MacDiscoverDialogs(model: model))
    }

    private var header: some View {
        HStack(spacing: MacTheme.Spacing.group) {
            Text("みんなのアプリを探す")
                .font(.title2.weight(.bold))
                .accessibilityAddTraits(.isHeader)
            Picker("並び順", selection: $model.sort) {
                ForEach(DiscoverSort.allCases) { Text($0.label).tag($0) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .fixedSize()
            .accessibilityIdentifier("discover.sort")
            TextField("家計簿、筋トレ、散歩…", text: $model.query)
                .textFieldStyle(.roundedBorder)
                .frame(minWidth: 180, maxWidth: 320)
                .accessibilityLabel(Text("アプリを検索"))
                .accessibilityIdentifier("discover.search")
            Spacer(minLength: 0)
            Button("閉じる", action: onClose)
                .keyboardShortcut(.cancelAction)
                .accessibilityIdentifier("discover.close")
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 14)
    }

    @ViewBuilder
    private var content: some View {
        switch model.phase {
        case .loading:
            LazyVGrid(columns: columns, spacing: 16) {
                ForEach(0..<6, id: \.self) { _ in
                    RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous)
                        .fill(Theme.Colors.surface)
                        .frame(height: 132)
                        .redacted(reason: .placeholder)
                }
            }
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text("読み込み中"))
            .accessibilityIdentifier("discover.loading")
        case .failed(let message):
            VStack(spacing: MacTheme.Spacing.group) {
                Text(message)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                Button("もう一度") { Task { await model.search() } }
                    .accessibilityIdentifier("discover.retry")
            }
            .frame(maxWidth: 420)
            .padding(.top, 60)
            .accessibilityIdentifier("discover.error")
        case .empty(let message):
            VStack(spacing: MacTheme.Spacing.group) {
                Image(systemName: "magnifyingglass")
                    .font(.largeTitle)
                    .foregroundStyle(Theme.brandGradient)
                    .accessibilityHidden(true)
                Text(message).foregroundStyle(.secondary)
            }
            .padding(.top, 60)
            .accessibilityIdentifier("discover.empty")
        case .results:
            LazyVGrid(columns: columns, spacing: 16) {
                ForEach(model.items) { item in
                    PublicAppCard(item: item, busy: model.installing == item.id) {
                        Task { await model.install(item) }
                    }
                    .disabled(model.installing != nil && model.installing != item.id)
                    .contextMenu { moderationMenu(item) }
                    .accessibilityIdentifier("discover.card.\(item.id)")
                }
            }
        }
    }

    @ViewBuilder
    private func moderationMenu(_ item: PublicAppListing) -> some View {
        if model.canModerate(item) {
            Button("通報する", systemImage: "exclamationmark.bubble") { model.reporting = item }
            Button("公開した人をブロック", systemImage: "hand.raised", role: .destructive) { model.blocking = item }
        }
    }
}

/// `MacRootView` に 1 行で付ける: シートを出し、入れ終えたら閉じて `onInstalled` に渡す(本体の式を型検査の上限に収めるため分ける)
struct MacDiscoverPresenter: ViewModifier {
    @Binding var model: MacDiscoverModel?
    let replace: MacAppReplaceContext
    let onInstalled: (MacDiscoverModel.Installed) -> Void

    func body(content: Content) -> some View {
        content
            .sheet(item: $model) { model in
                MacDiscoverView(model: model, replace: replace, onClose: { self.model = nil })
            }
            .onChange(of: model?.installed?.id) {
                guard let installed = model?.installed else { return }
                model = nil
                onInstalled(installed)
            }
    }
}

/// 通報の理由・ブロックの確認・結果・入れられなかった理由(`MacSocialDialogs` と同じ文言)
private struct MacDiscoverDialogs: ViewModifier {
    @Bindable var model: MacDiscoverModel

    func body(content: Content) -> some View {
        content
            .confirmationDialog(Text("通報の理由"), isPresented: Binding(
                get: { model.reporting != nil }, set: { if !$0 { model.reporting = nil } }
            ), titleVisibility: .visible, presenting: model.reporting) { item in
                ForEach(ReportReason.allCases) { reason in
                    Button(reason.macLabel) { Task { await model.report(item, reason: reason) } }
                }
                Button("キャンセル", role: .cancel) {}
            } message: { _ in
                Text("運営が内容を確認します。通報したことは相手に知らされません。")
            }
            .alert(Text("「\(model.blocking.map(Self.authorName) ?? "")」さんをブロックしますか?"), isPresented: Binding(
                get: { model.blocking != nil }, set: { if !$0 { model.blocking = nil } }
            ), presenting: model.blocking) { item in
                Button("ブロックする", role: .destructive) { Task { await model.block(item) } }
                Button("キャンセル", role: .cancel) {}
            } message: { _ in
                Text("この人の投稿と公開したアプリが、あなたに表示されなくなります。相手には知らされません。設定からいつでも解除できます。")
            }
            .alert(Text(model.notice?.title ?? ""), isPresented: Binding(
                get: { model.notice != nil }, set: { if !$0 { model.notice = nil } }
            ), presenting: model.notice) { _ in
                Button("OK", role: .cancel) {}
            } message: { notice in
                Text(notice.message)
            }
            .alert(Text("インストールできませんでした"), isPresented: Binding(
                get: { model.installError != nil }, set: { if !$0 { model.installError = nil } }
            ), presenting: model.installError) { _ in
                Button("OK", role: .cancel) {}
            } message: { message in
                Text(message)
            }
    }

    private static func authorName(_ item: PublicAppListing) -> String {
        item.authorName.isEmpty ? L10n.tr("匿名") : item.authorName
    }
}
#endif
