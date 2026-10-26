import SwiftUI
import MetamonRenderer

/// 新着(#268)のバナー。アプリを開いた直後(起動・前面復帰・切替)に「新着 N 件」を上部に出す。
/// 変身の要約・状態カード・生成中と重なる間は待ち、消えてから出す。タップか数秒で消える。
struct NewItemsHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt
    @Environment(\.scenePhase) private var scenePhase
    @State private var notice: Notice?

    struct Notice: Identifiable, Equatable {
        let id = UUID()
        var count: Int
    }

    /// entity が変わったら(初回の復元・切替・生成)数え直す。前回時刻はその都度進むので二重には出ない。
    private var trigger: String {
        ([rt.document.appId] + rt.dsl.entities.map(\.id)).joined(separator: "|")
    }

    private var canShow: Bool {
        rt.summaryBanner == nil && rt.errorCard == nil && rt.morphJob == nil
    }

    func body(content: Content) -> some View {
        content
            .overlay(alignment: .top) {
                if let notice, canShow {
                    banner(notice)
                }
            }
            .animation(.easeInOut(duration: 0.3), value: notice)
            .task(id: trigger) { await check() }
            .onChange(of: scenePhase) { old, phase in
                if phase == .active, old == .background { Task { await check() } }
            }
    }

    private func check() async {
        notice = nil
        guard let count = await rt.checkNewItems(), !Task.isCancelled else { return }
        notice = Notice(count: count)
    }

    private func banner(_ notice: Notice) -> some View {
        HStack(spacing: Theme.Spacing.sm + 2) {
            Image(systemName: "bell.badge.fill")
                .font(.title3)
                .foregroundStyle(RenderTheme.Colors.brandA)
            VStack(alignment: .leading, spacing: 2) {
                Text("新着 \(notice.count) 件")
                    .font(.subheadline.weight(.semibold))
                Text("ほかの人が追加・更新しました")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer(minLength: 0)
        }
        .metamonCard(padding: Theme.Spacing.md)
        .padding(.horizontal, Theme.Spacing.md)
        .padding(.top, Theme.Spacing.sm)
        .transition(.move(edge: .top).combined(with: .opacity))
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("newItems.banner")
        .onTapGesture { self.notice = nil }
        .task(id: notice.id) {
            try? await Task.sleep(for: .seconds(6))
            guard !Task.isCancelled, self.notice?.id == notice.id else { return }
            self.notice = nil
        }
    }
}
