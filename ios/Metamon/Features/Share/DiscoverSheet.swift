// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonRenderer
import MetamonServices

/// 「探す」(#50): 他の人が公開したアプリを検索して、インストール → そのアプリに変身。
/// 初期(音声)画面の下部の `DiscoverEntry` から開く(present は RootView の `SharingHooks`)。
struct DiscoverSheet: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.services) private var services
    @State private var query = ""
    @State private var sort = "popular"
    @State private var items: [PublicAppListing] = []
    @State private var loading = true
    @State private var error: String?
    @State private var moderation = ModerationUI.shared

    var body: some View {
        NavigationStack {
            ScrollView {
                LazyVStack(spacing: Theme.Spacing.md) {
                    Picker("並び順", selection: $sort) {
                        ForEach(DiscoverSort.allCases) { Text($0.label).tag($0.rawValue) }
                    }
                    .pickerStyle(.segmented)
                    content
                }
                .padding(.horizontal, Theme.Spacing.lg)
                .padding(.bottom, Theme.Spacing.lg)
            }
            .background(Theme.Colors.canvas.ignoresSafeArea())
            .navigationTitle("アプリを探す")
            .navigationBarTitleDisplayMode(.inline)
            .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .always), prompt: "家計簿、筋トレ、散歩…")
            .toolbar {
                SheetCloseToolbarItem { SharingUI.shared.showDiscover = false }
            }
        }
        .presentationDetents([.large])
        .appLimitSheets() // #630 入れる時に持てるアプリの数に達したら、この上で置き換える
        .task(id: "\(sort)|\(query)|\(moderation.blockCount)") { await search() } // ブロックしたら読み直す(#321)
        .modifier(ModerationDialogs(discover: true))
    }

    @ViewBuilder
    private var content: some View {
        if loading && items.isEmpty {
            ForEach(0..<3, id: \.self) { _ in
                RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous)
                    .fill(Theme.Colors.surface)
                    .frame(height: 132)
                    .redacted(reason: .placeholder)
            }
            .accessibilityLabel("読み込み中")
        } else if let error, items.isEmpty {
            VStack(spacing: Theme.Spacing.sm) {
                Text(error).font(.footnote).foregroundStyle(.secondary).multilineTextAlignment(.center)
                Button("もう一度") { Task { await search() } }.buttonStyle(.bordered)
            }
            .padding(.top, Theme.Spacing.xl)
        } else if items.isEmpty {
            VStack(spacing: Theme.Spacing.sm) {
                Image(systemName: "magnifyingglass").font(.title).foregroundStyle(Theme.brandGradient)
                Text(DiscoverCopy.emptyMessage(query: query))
                    .font(.subheadline).foregroundStyle(.secondary)
            }
            .padding(.top, Theme.Spacing.xl)
        } else {
            ForEach(items) { item in
                PublicAppCard(item: item) { install(item) }
                    .contextMenu {
                        if !item.mine, rt.moderation != nil { // #321
                            Button("通報する", systemImage: "exclamationmark.bubble") { rt.reportListing(item) }
                            Button("公開した人をブロック", systemImage: "hand.raised", role: .destructive) { rt.blockPublisher(item) }
                        }
                    }
            }
        }
    }

    private func search() async {
        guard let sharing = services.sharing else {
            loading = false
            error = DiscoverCopy.unavailable
            return
        }
        if !query.isEmpty { try? await Task.sleep(for: .milliseconds(250)) } // 入力中の連打を間引く
        guard !Task.isCancelled else { return }
        loading = true
        defer { loading = false }
        do {
            items = try await sharing.searchPublicApps(query: query, sort: sort)
            error = nil
        } catch is CancellationError {
        } catch {
            if (error as? URLError)?.code == .cancelled { return }
            self.error = ErrorCopy.failure(L10n.tr("読み込めませんでした"), error, context: "DiscoverSheet")
        }
    }

    private func install(_ item: PublicAppListing) {
        SharingUI.shared.showDiscover = false
        Task { await rt.installPublic(item) }
    }
}

/// 初期(音声)画面の下部に置く小さな導線。音声中心の見た目を崩さないよう 1 行のテキストボタンだけ。
struct DiscoverEntry: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        if rt.canShare {
            Button { SharingUI.shared.showDiscover = true } label: {
                Label("みんなが作ったアプリを探す", systemImage: "magnifyingglass")
                    .font(.footnote.weight(.medium))
                    .foregroundStyle(.secondary)
                    .padding(.vertical, Theme.Spacing.xs)
            }
            .buttonStyle(.plain)
            .padding(.top, Theme.Spacing.sm)
            .disabled(rt.isMorphing)
        }
    }
}
#endif
