import SwiftUI
import UIKit

/// 「通知がオフ」バーの出し分け(UX 点検 L3)。View から切り離してテストする。
enum NotificationsOffBarModel {
    /// 通知を使うアプリで許可が無く、閉じていない時。生成中・要約の表示中は重ねない(完成直後の割り込みを増やさない・H7)
    static func isVisible(denied: Bool, dismissed: Bool, busy: Bool, summaryShowing: Bool) -> Bool {
        denied && !dismissed && !busy && !summaryShowing
    }

    static var message: String { L10n.tr("通知がオフのため、このアプリの通知は届きません") }
}

/// 上に重ねる「通知がオフ」バー: 文言 [設定を開く] [×]。3 秒のトーストでは読み切れず、設定を開く手段も無かった(L3)。
struct NotificationsOffBar: View {
    let onOpenSettings: () -> Void
    let onDismiss: () -> Void

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "bell.slash")
                .foregroundStyle(Theme.Colors.warning)
                .accessibilityHidden(true)
            Text(NotificationsOffBarModel.message)
                .font(.footnote.weight(.medium))
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
            Button("設定を開く", action: onOpenSettings)
                .font(.footnote.weight(.semibold))
                .buttonStyle(.bordered)
                .controlSize(.small)
                .frame(minHeight: 44)
                .accessibilityIdentifier("notificationsOff.openSettings")
            Button(action: onDismiss) {
                Image(systemName: "xmark")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(.secondary)
                    .frame(width: 44, height: 44)
                    .contentShape(Rectangle())
            }
            .accessibilityLabel("閉じる")
            .accessibilityIdentifier("notificationsOff.dismiss")
        }
        .padding(.leading, 16)
        .padding(.trailing, 4)
        .background {
            Rectangle()
                .fill(Color(.secondarySystemBackground))
                .overlay(alignment: .bottom) { Rectangle().fill(Theme.Colors.hairline).frame(height: 1) }
                .ignoresSafeArea(edges: .top)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("notificationsOff")
    }
}

/// 通知を使うアプリで通知が許可されていない時、どの画面でも上に「通知がオフ」バーを出す(L3)。
/// 閉じたらそのアプリの間は出さない(別のアプリに切り替えたら、また判定する)。許可されたら自動で消える。
struct NotificationsOffBarHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt
    @State private var dismissedAppId: String?

    func body(content: Content) -> some View {
        let visible = NotificationsOffBarModel.isVisible(
            denied: rt.notificationsDenied,
            dismissed: dismissedAppId == rt.document.appId,
            busy: rt.buildStage != nil || rt.morphJob != nil,
            summaryShowing: rt.summaryBanner != nil
        )
        content
            .safeAreaInset(edge: .top, spacing: 0) {
                if visible {
                    NotificationsOffBar(onOpenSettings: openSettings, onDismiss: dismiss)
                        .transition(.move(edge: .top).combined(with: .opacity))
                        .onAppear { AccessibilityNotification.Announcement(NotificationsOffBarModel.message).post() }
                }
            }
            .animation(.easeInOut(duration: 0.25), value: visible)
    }

    private func openSettings() {
        if let url = URL(string: UIApplication.openNotificationSettingsURLString) { UIApplication.shared.open(url) }
    }

    private func dismiss() {
        dismissedAppId = rt.document.appId
    }
}
