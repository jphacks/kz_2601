import SwiftUI
import MetamonServices
import MetamonRenderer

/// 変身一覧(#51)。マイク FAB の長押し / 「アプリ一覧」で開く。
/// 自分のアプリを横スクロールのカード(サムネ・名前・最終更新・「使用中」)で並べ、タップで切替。
/// 下に「+ 新しいアプリ」「Tailor It に戻る(初期画面)」。
struct AppSwitcherSheet: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.dismiss) private var dismiss
    @State private var thumbs = AppThumbnailStore.shared
    @State private var selected: String?
    #if !APP_STORE_REVIEW
    @State private var sharingUI = SharingUI.shared
    @State private var releaseUI = ReleaseUI.shared
    #endif

    private var apps: [AppSummary] { rt.visibleApps }
    private var showSkeleton: Bool { rt.appsLoading && apps.isEmpty }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header
                .padding(.horizontal, Theme.Spacing.lg)
                .padding(.top, Theme.Spacing.lg)
            cards
                .padding(.top, Theme.Spacing.lg)
            actions
                .padding(.horizontal, Theme.Spacing.lg)
                .padding(.top, Theme.Spacing.lg)
            Spacer(minLength: Theme.Spacing.md)
        }
        .background(Theme.Colors.canvas.ignoresSafeArea())
        .modifier(AppRenameAlert()) // #277
        .presentationDetents([.height(500)])
        .presentationDragIndicator(.visible)
        .sensoryFeedback(.selection, trigger: selected)
        .task { await rt.appsList() }
        #if !APP_STORE_REVIEW // 審査用ビルドはシェア / リリース申請を外す(#245)
        .task { await releaseUI.refresh(rt.services.releaseRequests) }
        .sheet(item: $sharingUI.shareTarget) { ShareAppSheet(app: $0) }
        .sheet(item: $releaseUI.target) { ReleaseRequestSheet(app: $0) }
        #endif
        .screenMapSheet { ScreenMapUI.shared.closeSwitcherThenTalk(rt) } // #116
    }

    // MARK: - Sections

    private var header: some View {
        HStack(alignment: .top, spacing: Theme.Spacing.md) {
            BrandMark(size: 40)
            VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                Text(Greeting.switcherTitle)
                    .font(.system(size: 24, weight: .bold, design: .rounded))
                Text(Greeting.switcherSubtitle)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 0)
            Button { dismiss() } label: {
                Image(systemName: "xmark")
                    .font(.footnote.weight(.bold))
                    .foregroundStyle(.secondary)
                    .padding(Theme.Spacing.sm + 2)
                    .background(Theme.Colors.surface, in: Circle())
                    .overlay(Circle().strokeBorder(Theme.Colors.hairline))
            }
            .accessibilityLabel("閉じる")
            .accessibilityIdentifier("switcher.close")
        }
    }

    @ViewBuilder
    private var cards: some View {
        if showSkeleton {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: Theme.Spacing.md) {
                    ForEach(0..<3, id: \.self) { _ in SkeletonCard() }
                }
                .padding(.horizontal, Theme.Spacing.lg)
            }
            .scrollDisabled(true)
        } else if apps.isEmpty {
            VStack(spacing: Theme.Spacing.sm) {
                CraneMascot(pose: .point, size: 72) // #102
                Text(Greeting.switcherEmpty)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            .frame(maxWidth: .infinity, minHeight: AppCard.height)
            .padding(.horizontal, Theme.Spacing.lg)
        } else {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: Theme.Spacing.md) {
                    ForEach(apps) { app in
                        AppCard(app: app,
                                isCurrent: app.id == rt.document.appId,
                                thumbnail: thumbnail(for: app.id)) {
                            select(app)
                        }
                        .overlay(alignment: .topTrailing) { AppCardActions(app: app) }
                        .overlay(alignment: .bottomTrailing) { if app.id == rt.document.appId { ScreenMapCardButton() } } // #116
                        .modifier(AppCardReorder(app: app)) // #277 長押しでドラッグして並べ替え
                    }
                }
                .padding(.horizontal, Theme.Spacing.lg)
                .padding(.vertical, Theme.Spacing.sm)
            }
            .scrollClipDisabled()
        }
    }

    private var actions: some View {
        HStack(spacing: Theme.Spacing.md) {
            Button {
                selected = "new"
                rt.startFresh(.newApp)
            } label: {
                Label(Greeting.newAppAction, systemImage: "plus")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.white)
                    .frame(maxWidth: .infinity)
                    .frame(height: 48)
                    .background(Theme.brandGradient, in: Capsule())
            }
            .buttonStyle(.plain)
            .accessibilityHint("新しい appId で音声画面から作り始めます")

            Button {
                selected = "home"
                rt.startFresh(.home)
            } label: {
                Label(Greeting.homeAction, systemImage: "house")
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)
                    .frame(maxWidth: .infinity)
                    .frame(height: 48)
                    .background(Theme.Colors.surface, in: Capsule())
                    .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
            }
            .buttonStyle(.plain)
        }
        .disabled(rt.isMorphing)
    }

    // MARK: - Helpers

    private func thumbnail(for appId: String) -> UIImage? {
        _ = thumbs.revision // 撮り直しで再描画
        return thumbs.image(for: appId)
    }

    private func select(_ app: AppSummary) {
        selected = app.id
        if app.id == rt.document.appId {
            dismiss()
            return
        }
        Task { await rt.switchApp(to: app.id) }
    }
}

// MARK: - Card

/// 1 アプリ分のカード。サムネ(無ければイニシャル + ブランドグラデーション)+ 名前 + 最終更新。
struct AppCard: View {
    let app: AppSummary
    let isCurrent: Bool
    let thumbnail: UIImage?
    let action: () -> Void

    static let width: CGFloat = 150
    static let thumbHeight: CGFloat = 210
    static let height: CGFloat = thumbHeight + 64

    var body: some View {
        Button(action: action) {
            VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
                thumb
                    .frame(width: Self.width, height: Self.thumbHeight)
                    .clipShape(RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous)
                            .strokeBorder(isCurrent ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(Theme.Colors.hairline),
                                          lineWidth: isCurrent ? 2.5 : 1)
                    )
                    .overlay(alignment: .topLeading) {
                        if isCurrent { currentBadge.padding(Theme.Spacing.sm) }
                    }
                VStack(alignment: .leading, spacing: 2) {
                    Text(app.name)
                        .font(.subheadline.weight(.semibold))
                        .lineLimit(1)
                    Text(updatedText)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                .padding(.horizontal, Theme.Spacing.xs)
            }
            .frame(width: Self.width)
        }
        .buttonStyle(CardPressStyle())
        .accessibilityLabel(app.name + (isCurrent ? L10n.tr("(使用中)") : ""))
        .accessibilityHint(isCurrent ? "" : L10n.tr("タップで切り替え"))
    }

    @ViewBuilder
    private var thumb: some View {
        if let thumbnail {
            // 画面上部(タイトル・サマリー)が見えるように上揃えで切り出す
            Image(uiImage: thumbnail)
                .resizable()
                .scaledToFill()
                .frame(width: Self.width, height: Self.thumbHeight, alignment: .top)
        } else {
            ZStack {
                Theme.brandGradient
                Circle()
                    .fill(.white.opacity(0.14))
                    .frame(width: 120, height: 120)
                    .offset(x: 40, y: -70)
                Text(initials)
                    .font(.system(size: 44, weight: .bold, design: .rounded))
                    .foregroundStyle(.white)
            }
        }
    }

    private var currentBadge: some View {
        Text(Greeting.currentBadge)
            .font(.caption2.weight(.bold))
            .foregroundStyle(.white)
            .padding(.horizontal, Theme.Spacing.sm)
            .padding(.vertical, 4)
            .background(Theme.brandGradient, in: Capsule())
            .shadow(color: .black.opacity(0.15), radius: 4, y: 2)
    }

    /// 名前の先頭 2 文字(「家計簿」→「家計」)。
    private var initials: String {
        String(app.name.trimmingCharacters(in: .whitespaces).prefix(2))
    }

    private var updatedText: String {
        guard let date = app.updatedDate else { return "v\(app.currentVersion)" }
        let f = RelativeDateTimeFormatter()
        f.unitsStyle = .short
        f.locale = LanguagePreference.shared.locale
        return L10n.tr("\(f.localizedString(for: date, relativeTo: .now))に更新")
    }
}

/// 読み込み中のスケルトン(データ無しと区別するため必ず出す)。
struct SkeletonCard: View {
    @State private var pulse = false

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous)
                .fill(Theme.Colors.surfaceElevated)
                .frame(width: AppCard.width, height: AppCard.thumbHeight)
            RoundedRectangle(cornerRadius: 4).fill(Theme.Colors.surfaceElevated).frame(width: 90, height: 12)
            RoundedRectangle(cornerRadius: 4).fill(Theme.Colors.surfaceElevated).frame(width: 60, height: 10)
        }
        .opacity(pulse ? 0.45 : 1)
        .animation(.easeInOut(duration: 0.9).repeatForever(autoreverses: true), value: pulse)
        .onAppear { pulse = true }
        .accessibilityLabel("読み込み中")
    }
}

/// 押下で少し縮むカード用スタイル。
struct CardPressStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? 0.96 : 1)
            .animation(.spring(duration: 0.25, bounce: 0.2), value: configuration.isPressed)
    }
}

#Preview {
    let services = BuildConfig.make(environment: .mock)
    let rt = AppRuntime(document: .fallback, services: services)
    return Color.clear
        .sheet(isPresented: .constant(true)) { AppSwitcherSheet() }
        .environment(rt)
        .environment(\.services, services)
}
