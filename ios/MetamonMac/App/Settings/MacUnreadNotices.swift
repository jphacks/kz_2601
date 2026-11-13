// 審査用ビルド(APP_STORE_REVIEW)ではお知らせと一緒に外す(#374)。
#if !APP_STORE_REVIEW
import AppKit
import SwiftUI
import Observation
import MetamonServices

/// 未読の直ったお知らせの数(#3245)。起動時とウインドウが前に来た時に読み、サイドバーの設定の行と Dock のバッジに出す。0 なら出さない。
@MainActor
@Observable
final class MacUnreadNotices {
    static let shared = MacUnreadNotices(client: MacServices.shared.loopNotices,
                                         uid: { MacAccount.shared.session.user?.uid },
                                         applyDockBadge: { NSApp.dockTile.badgeLabel = $0 })

    /// 前に来るたびに問い合わせない(ウインドウの切り替えは多い)
    nonisolated static let minInterval: TimeInterval = 30

    private(set) var count = 0

    @ObservationIgnored let client: (any LoopNoticesClient)?
    @ObservationIgnored private let uid: () -> String?
    @ObservationIgnored private let applyDockBadge: (String?) -> Void
    @ObservationIgnored private var lastRefresh: Date?
    @ObservationIgnored private var generation = 0
    @ObservationIgnored private var observers: [NSObjectProtocol] = []

    init(client: (any LoopNoticesClient)?, uid: @escaping () -> String?, applyDockBadge: @escaping (String?) -> Void) {
        self.client = client
        self.uid = uid
        self.applyDockBadge = applyDockBadge
    }

    /// 読み直す。サインインしていない・モックなら 0。読めない時は前の数のまま。
    func refresh(force: Bool = false, now: Date = Date()) async {
        guard let client, uid() != nil else {
            set(0)
            return
        }
        guard Self.shouldRefresh(last: lastRefresh, now: now, force: force) else { return }
        lastRefresh = now
        generation += 1
        let mine = generation
        guard let page = try? await client.notices(pageToken: nil) else { return }
        guard mine == generation else { return } // 後から始めた読み直しの方を使う
        set(page.unreadCount)
    }

    /// 一覧で既読にした時など、数が分かっている時
    func set(_ count: Int) {
        self.count = max(0, count)
        applyDockBadge(Self.badgeLabel(self.count))
    }

    /// 起動時に 1 回: 前面に来た時・ウインドウが前に来た時に読み直す
    func install() {
        guard observers.isEmpty else { return }
        let center = NotificationCenter.default
        for name in [NSApplication.didBecomeActiveNotification, NSWindow.didBecomeMainNotification] {
            observers.append(center.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated {
                    guard let self else { return }
                    Task { await self.refresh() }
                }
            })
        }
    }

    nonisolated static func shouldRefresh(last: Date?, now: Date, force: Bool) -> Bool {
        guard !force, let last else { return true }
        return now.timeIntervalSince(last) >= minInterval || now < last
    }

    /// Dock とサイドバーのバッジの文字。0 なら出さない
    nonisolated static func badgeLabel(_ count: Int) -> String? {
        guard count > 0 else { return nil }
        return count > 99 ? "99+" : "\(count)"
    }

    /// 読み上げ: 「未読のお知らせ N 件」。0 なら nil
    static func accessibilityLabel(_ count: Int) -> String? {
        count > 0 ? L10n.tr("未読のお知らせ \(count) 件") : nil
    }
}

/// サイドバーの下の「設定」の行(#3245)。未読のお知らせがあれば数を出す。押すと設定のウインドウ。
struct MacSidebarSettingsRow: View {
    @State private var unread = MacUnreadNotices.shared
    private let account = MacAccount.shared

    var body: some View {
        SettingsLink {
            HStack(spacing: 8) {
                Label("設定", systemImage: "gearshape")
                Spacer(minLength: 4)
                if let badge = MacUnreadNotices.badgeLabel(unread.count) {
                    Text(verbatim: badge)
                        .font(.caption2.weight(.semibold).monospacedDigit())
                        .foregroundStyle(.white)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1)
                        .background(Capsule().fill(Color.accentColor))
                        .accessibilityHidden(true)
                }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.horizontal, MacTheme.Spacing.group)
        .padding(.vertical, 8)
        .accessibilityElement(children: .combine)
        .accessibilityLabel(Text(verbatim: [L10n.tr("設定"), MacUnreadNotices.accessibilityLabel(unread.count)].compactMap { $0 }.joined(separator: ", ")))
        .help(MacUnreadNotices.accessibilityLabel(unread.count) ?? L10n.tr("設定を開く"))
        .accessibilityIdentifier("shell.sidebar.settings")
        // サインインした・アカウントが変わった時(起動直後の匿名ログインの後も)
        .task(id: account.session.user?.uid) { await unread.refresh(force: true) }
    }
}
#endif
