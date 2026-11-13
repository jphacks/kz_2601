import SwiftUI
import MetamonServices
import MetamonRenderer

/// 上部のお知らせが重ならないように、新着のバナー(#268)が出ているかを共有する。
@MainActor
@Observable
final class TopNoticeSlot {
    static let shared = TopNoticeSlot()
    var newItemsVisible = false
}

/// 上限に近い時の 1 行のお知らせ(#317)。落ち着いた見た目で、閉じられる。「Pro にする」は控えめなボタンで、押した時だけ課金画面を開く。
struct UsageNoticeBanner: View {
    let notice: UsageNotice
    /// nil = 「Pro にする」を出さない(Pro・審査用の版・上がらない上限)
    var onUpgrade: (() -> Void)?
    let onDismiss: () -> Void

    var body: some View {
        HStack(spacing: Theme.Spacing.sm) {
            Image(systemName: "gauge.with.dots.needle.67percent")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .accessibilityHidden(true)
            Text(UsageCopy.notice(notice))
                .font(.footnote)
                .lineLimit(2)
                .minimumScaleFactor(0.9)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityIdentifier("usage.notice.text")
            if let onUpgrade {
                Button(action: onUpgrade) {
                    Text("Pro にする").font(.footnote.weight(.semibold))
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .accessibilityIdentifier("usage.notice.upgrade")
            }
            Button(action: onDismiss) {
                Image(systemName: "xmark")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.secondary)
                    .padding(Theme.Spacing.xs)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(Text("閉じる"))
            .accessibilityIdentifier("usage.notice.close")
        }
        .metamonCard(padding: Theme.Spacing.sm + 2)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("usage.notice.\(notice.resource.rawValue)")
    }
}

/// アプリ一覧の上に出す「作れるアプリの数」のお知らせ(1 日 1 回)。
struct AppsUsageNotice: View {
    @Environment(AppRuntime.self) private var rt
    @State private var notice: UsageNotice?
    #if !APP_STORE_REVIEW
    @State private var showPaywall = false
    #endif
    var log = UsageNoticeLog()

    var body: some View {
        VStack(spacing: 0) {
            Color.clear.frame(height: 0) // 出していない間も `.task` が動くように
            if let notice {
                UsageNoticeBanner(notice: notice, onUpgrade: upgradeAction(notice)) {
                    withAnimation(.easeInOut(duration: 0.2)) { self.notice = nil }
                }
                .padding(.top, Theme.Spacing.md)
                .transition(.opacity)
                .onAppear { log.markShown(notice, uid: rt.currentUserId) } // 出した時点で数える(閉じても同じ日はもう出さない)
            }
        }
        .task(id: PlanUsageStore.shared.revision) { await check() }
        #if !APP_STORE_REVIEW
        .sheet(isPresented: $showPaywall) { PaywallSheet(reason: .settings) }
        #endif
    }

    private func check() async {
        await rt.refreshPlanUsage()
        guard notice == nil, let next = UsageNoticeRules.appsNotice(PlanUsageStore.shared.me),
              log.shouldShow(next, uid: rt.currentUserId) else { return }
        withAnimation(.easeInOut(duration: 0.2)) { notice = next }
    }

    private func upgradeAction(_ notice: UsageNotice) -> (() -> Void)? {
        #if APP_STORE_REVIEW
        return nil
        #else
        guard UsageCopy.showsProButton(notice, isPro: RemainingTodayPlan.isPro) else { return nil }
        return { showPaywall = true }
        #endif
    }
}

/// アプリの中に出す「記録・写真」のお知らせ(作成者のアプリだけ・1 日 1 回)。開いた直後の新着・要約・カードが消えてから出し、数秒で消える。
struct UsageNoticeHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.assistiveAccess) private var assistiveAccess
    @State private var notice: UsageNotice?
    var log = UsageNoticeLog()

    private var trigger: String { "\(rt.document.appId)|\(rt.dsl.isVoiceOnly)|\(PlanUsageStore.shared.revision)" }

    private var canShow: Bool {
        rt.summaryBanner == nil && rt.errorCard == nil && rt.morphJob == nil && !TopNoticeSlot.shared.newItemsVisible
    }

    func body(content: Content) -> some View {
        content
            .overlay(alignment: .top) {
                if let notice, canShow {
                    UsageNoticeBanner(notice: notice, onUpgrade: upgradeAction(notice)) { self.notice = nil }
                        .padding(.horizontal, Theme.Spacing.md)
                        .padding(.top, Theme.Spacing.sm)
                        .transition(.move(edge: .top).combined(with: .opacity))
                        .onAppear { log.markShown(notice, uid: rt.currentUserId) } // 見えた時点で数える
                        .task(id: notice.id) {
                            try? await Task.sleep(for: .seconds(8))
                            guard !Task.isCancelled, self.notice?.id == notice.id else { return }
                            self.notice = nil
                        }
                }
            }
            .animation(.easeInOut(duration: 0.3), value: notice)
            .task(id: trigger) { await check() }
    }

    private func check() async {
        guard !rt.dsl.isVoiceOnly else { notice = nil; return }
        try? await Task.sleep(for: .seconds(1.5)) // 開いた直後のバナーと重ねない
        guard !Task.isCancelled else { return }
        let appId = rt.document.appId
        await rt.refreshPlanUsage()
        guard !Task.isCancelled, rt.document.appId == appId, notice == nil,
              let next = UsageNoticeRules.appNotices(rt.currentAppUsage).first(where: { log.shouldShow($0, uid: rt.currentUserId) }) else { return }
        notice = next
    }

    private func upgradeAction(_ notice: UsageNotice) -> (() -> Void)? {
        #if APP_STORE_REVIEW
        return nil
        #else
        // アシスティブアクセス(#1997)では課金の画面を出さない
        guard assistiveAccess.shows(.paywall), UsageCopy.showsProButton(notice, isPro: RemainingTodayPlan.isPro) else { return nil }
        return {
            self.notice = nil
            Task { _ = await rt.proGate.requestPro(.settings) }
        }
        #endif
    }
}
