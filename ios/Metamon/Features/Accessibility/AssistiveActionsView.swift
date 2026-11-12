import SwiftUI
import MetamonDSL

/// アシスティブアクセス(#1998)で一覧から開いた生成アプリ: 主な動作を大きなボタンで最大 4 つ並べる。
/// 全部の画面を見る道は置かない(戻るのは上の「一覧に戻る」だけ)。
struct AssistiveAccessActionsView: View {
    @Environment(AppRuntime.self) private var rt
    /// 押した直後の表示(保存が返るまで・楽観的 UI)
    @State private var pending: [String: Bool] = [:]
    @State private var notice: AssistiveRecordedNotice.Kind?
    @State private var noticeTask: Task<Void, Never>?
    @State private var taps = 0

    private var actions: [AssistiveActions.Action] {
        AssistiveActions.pick(dsl: rt.dsl, records: rt.records, countUnit: L10n.tr("件")).map { a in
            var a = a
            if let on = pending[a.id] { a.isOn = on }
            return a
        }
    }

    var body: some View {
        ScrollView {
            AssistiveActionGrid(appName: rt.dsl.app.name, actions: actions, onTap: tap)
        }
        .background(Theme.Colors.canvas.ignoresSafeArea())
        .overlay {
            if let notice {
                AssistiveRecordedNotice(kind: notice)
                    .transition(.scale(scale: 0.9).combined(with: .opacity))
                    .onTapGesture { dismissNotice() }
            }
        }
        .animation(.easeOut(duration: 0.2), value: notice)
        .sensoryFeedback(.success, trigger: taps)
        .task { await rt.load(entities: AssistiveActions.entities(in: rt.dsl)) }
    }

    private func tap(_ action: AssistiveActions.Action) {
        guard pending[action.id] == nil else { return } // 保存待ちの間の 2 回目は無視(逆の値を書かない)
        guard let data = AssistiveActions.write(for: action, in: rt.dsl, records: rt.records) else {
            show(.failed)
            return
        }
        if case .check = action.kind { pending[action.id] = !action.isOn } // Task の前に同期で立てる
        Task {
            let saved: Bool
            switch action.kind {
            case .check(let e, let rid, _, _):
                saved = await rt.updateRecord(entity: e, recordId: rid, data: data)
                pending[action.id] = nil
            case .quickRecord(let e):
                saved = await rt.createRecord(entity: e, data: data)
            case .stat:
                return
            }
            if saved { taps += 1 }
            let turnedOff: Bool = if case .check = action.kind { action.isOn } else { false }
            show(!saved ? .failed : turnedOff ? .unchecked : .recorded)
        }
    }

    private func show(_ kind: AssistiveRecordedNotice.Kind) {
        notice = kind
        AccessibilityNotification.Announcement(kind.text).post()
        noticeTask?.cancel()
        noticeTask = Task {
            try? await Task.sleep(for: .seconds(2))
            guard !Task.isCancelled else { return }
            notice = nil
        }
    }

    private func dismissNotice() {
        noticeTask?.cancel()
        notice = nil
    }
}

/// 大きなボタンの並び(見た目だけ・スナップショットテストで描く)。スクロールは置く側(ImageRenderer は ScrollView の中を描かない)。
struct AssistiveActionGrid: View {
    let appName: String
    let actions: [AssistiveActions.Action]
    let onTap: (AssistiveActions.Action) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.lg) {
            Text(appName)
                .font(.largeTitle.weight(.bold))
                .accessibilityAddTraits(.isHeader)
            if actions.isEmpty {
                Text(L10n.tr("まだありません"))
                    .font(.title2)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, minHeight: 120)
            } else {
                ForEach(actions) { a in tile(a) }
            }
        }
        .padding(Theme.Spacing.lg)
    }

    @ViewBuilder
    private func tile(_ a: AssistiveActions.Action) -> some View {
        switch a.kind {
        case .stat:
            content(a)
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("assistive.action.\(a.id)")
        case .check, .quickRecord:
            Button { onTap(a) } label: { content(a) }
                .buttonStyle(.plain)
                .accessibilityValue(accessibilityValue(a))
                .accessibilityIdentifier("assistive.action.\(a.id)")
        }
    }

    private func content(_ a: AssistiveActions.Action) -> some View {
        HStack(spacing: Theme.Spacing.md) {
            Image(systemName: symbol(a))
                .font(.system(size: 40, weight: .semibold))
                .foregroundStyle(symbolColor(a))
                .frame(minWidth: 48)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                Text(title(a))
                    .font(.title2.weight(.semibold))
                    .multilineTextAlignment(.leading)
                if let value = a.value {
                    Text(value)
                        .font(.largeTitle.weight(.bold))
                        .monospacedDigit()
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(Theme.Spacing.lg)
        .frame(maxWidth: .infinity, minHeight: 104)
        .background(Theme.Colors.surface, in: RoundedRectangle(cornerRadius: 24, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 24, style: .continuous).strokeBorder(Theme.Colors.hairline, lineWidth: 2))
        .contentShape(Rectangle())
    }

    private func title(_ a: AssistiveActions.Action) -> String {
        switch a.kind {
        case .quickRecord: L10n.tr("\(a.title)を記録")
        case .check, .stat: a.title
        }
    }

    private func symbol(_ a: AssistiveActions.Action) -> String {
        switch a.kind {
        case .check: a.isOn ? "checkmark.circle.fill" : "circle"
        case .quickRecord: "plus.circle.fill"
        case .stat: "number.circle.fill"
        }
    }

    private func symbolColor(_ a: AssistiveActions.Action) -> Color {
        switch a.kind {
        case .check: a.isOn ? Color.green : Color.secondary
        case .quickRecord: Color.accentColor
        case .stat: Color.secondary
        }
    }

    private func accessibilityValue(_ a: AssistiveActions.Action) -> String {
        guard case .check = a.kind else { return "" }
        return a.isOn ? L10n.tr("済み") : L10n.tr("まだ")
    }
}

/// 押した後に大きく出す「記録しました」(2 秒で消える・タップでも消える)。
struct AssistiveRecordedNotice: View {
    enum Kind: Equatable {
        case recorded, unchecked, failed

        var text: String {
            switch self {
            case .recorded: L10n.tr("記録しました")
            case .unchecked: L10n.tr("チェックを外しました")
            case .failed: L10n.tr("記録できませんでした")
            }
        }

        var symbol: String {
            switch self {
            case .recorded: "checkmark.circle.fill"
            case .unchecked: "arrow.uturn.backward.circle.fill"
            case .failed: "exclamationmark.triangle.fill"
            }
        }
    }

    let kind: Kind

    var body: some View {
        VStack(spacing: Theme.Spacing.md) {
            Image(systemName: kind.symbol)
                .font(.system(size: 72, weight: .bold))
                .foregroundStyle(kind == .failed ? Color.orange : Color.green)
                .accessibilityHidden(true)
            Text(kind.text)
                .font(.largeTitle.weight(.bold))
                .multilineTextAlignment(.center)
        }
        .padding(Theme.Spacing.xl)
        .frame(maxWidth: 360)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 32, style: .continuous))
        .padding(Theme.Spacing.lg)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("assistive.notice")
    }
}
