import SwiftUI
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

    var body: some View {
        NavigationStack {
            ScrollView {
                LazyVStack(spacing: Theme.Spacing.md) {
                    Picker("並び順", selection: $sort) {
                        Text("人気").tag("popular")
                        Text("新着").tag("new")
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
                ToolbarItem(placement: .cancellationAction) {
                    Button("閉じる") { SharingUI.shared.showDiscover = false }
                }
            }
        }
        .presentationDetents([.large])
        .task(id: "\(sort)|\(query)") { await search() }
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
                Text(query.isEmpty ? "まだ公開されたアプリがありません" : "「\(query)」に合うアプリは見つかりませんでした")
                    .font(.subheadline).foregroundStyle(.secondary)
            }
            .padding(.top, Theme.Spacing.xl)
        } else {
            ForEach(items) { item in
                PublicAppCard(item: item) { install(item) }
            }
        }
    }

    private func search() async {
        guard let sharing = services.sharing else {
            loading = false
            error = "「探す」はサーバー接続時のみ使えます"
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
            self.error = "読み込めませんでした: \(error.localizedDescription)"
        }
    }

    private func install(_ item: PublicAppListing) {
        SharingUI.shared.showDiscover = false
        Task { await rt.installPublic(item) }
    }
}

/// 検索結果カード: 名前・説明・作者・使っている部品のアイコン・インストール数。
struct PublicAppCard: View {
    let item: PublicAppListing
    let install: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            HStack(alignment: .top, spacing: Theme.Spacing.md) {
                Text(String(item.name.prefix(1)))
                    .font(.system(size: 22, weight: .bold, design: .rounded))
                    .foregroundStyle(.white)
                    .frame(width: 48, height: 48)
                    .background(Theme.brandGradient, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                VStack(alignment: .leading, spacing: 2) {
                    Text(item.name).font(.headline).lineLimit(1)
                    Text(byline).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                }
                Spacer(minLength: 0)
                Button(action: install) {
                    Text(item.mine ? "コピー" : "入れる")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(.white)
                        .padding(.horizontal, Theme.Spacing.md)
                        .frame(height: 34)
                        .background(Theme.brandGradient, in: Capsule())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("\(item.name)をインストール")
            }
            if !item.description.isEmpty {
                Text(item.description)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
                    .fixedSize(horizontal: false, vertical: true)
            }
            HStack(spacing: Theme.Spacing.sm) {
                ForEach(item.components.prefix(6), id: \.self) { type in
                    Image(systemName: ComponentIcon.symbol(for: type))
                        .font(.caption)
                        .foregroundStyle(Theme.Colors.brandA)
                        .frame(width: 26, height: 26)
                        .background(Theme.Colors.brandA.opacity(0.12), in: Circle())
                        .accessibilityLabel(type)
                }
                Spacer(minLength: 0)
                ForEach(item.tags.prefix(2), id: \.self) { tag in
                    Text("#\(tag)").font(.caption2.weight(.medium)).foregroundStyle(.secondary)
                }
            }
        }
        .metamonCard()
    }

    private var byline: String {
        let author = item.authorName.isEmpty ? "匿名" : item.authorName
        return "\(author) ・ \(item.installs) 人が利用" + (item.mine ? " ・ あなたの公開" : "")
    }
}

/// DSL の component type → SF Symbol(カードの部品アイコン)。
enum ComponentIcon {
    static func symbol(for type: String) -> String {
        switch type {
        case "stat": "number"
        case "chart": "chart.bar.fill"
        case "list", "table": "list.bullet"
        case "button", "shareButton", "openUrlButton": "hand.tap.fill"
        case "textField": "character.cursor.ibeam"
        case "numberField": "textformat.123"
        case "picker", "segmented": "filemenu.and.selection"
        case "datePicker", "calendar": "calendar"
        case "toggle": "switch.2"
        case "photoField", "camera": "camera.fill"
        case "image": "photo"
        case "map", "mapView", "locationButton": "map.fill"
        case "timer", "timerDisplay": "timer"
        case "webView": "safari"
        case "qrScanner": "qrcode.viewfinder"
        case "progress", "gauge": "gauge.with.dots.needle.50percent"
        case "text": "text.alignleft"
        default: "square.grid.2x2"
        }
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
