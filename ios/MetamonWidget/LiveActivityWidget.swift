import ActivityKit
import SwiftUI
import WidgetKit

// ライブアクティビティ(#423)。ロック画面のバナーと Dynamic Island(compact / expanded / minimal)。
// 中身は本体が `TailorActivityAttributes.ContentState` で渡す。時間は日付から OS が描くのでアプリが止まっていても進む。

struct TailorLiveActivity: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: TailorActivityAttributes.self) { context in
            LockScreenActivityView(attributes: context.attributes, state: context.state, isStale: context.isStale)
                .activityBackgroundTint(nil)
                .activitySystemActionForegroundColor(WidgetTheme.brandA)
                .widgetURL(URL(string: "metamon://widget"))
        } dynamicIsland: { context in
            let state = context.state
            return DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    ActivityIcon(name: context.attributes.icon, size: 36)
                }
                DynamicIslandExpandedRegion(.trailing) {
                    ActivityClockText(clock: state.clock, finished: state.finished || context.isStale)
                        .font(.title2.weight(.semibold).monospacedDigit())
                        .multilineTextAlignment(.trailing)
                        .frame(maxWidth: 110, alignment: .trailing)
                }
                DynamicIslandExpandedRegion(.center) {
                    Text(state.title)
                        .font(.headline)
                        .lineLimit(1)
                }
                DynamicIslandExpandedRegion(.bottom) {
                    ActivityDetail(appName: context.attributes.appName, state: state)
                }
            } compactLeading: {
                ActivityIcon(name: context.attributes.icon, size: 22)
            } compactTrailing: {
                if state.clock != nil {
                    ActivityClockText(clock: state.clock, finished: state.finished || context.isStale)
                        .font(.caption.weight(.semibold).monospacedDigit())
                        .frame(maxWidth: 52)
                } else {
                    Text(state.value ?? "")
                        .font(.caption.weight(.semibold))
                        .lineLimit(1)
                        .frame(maxWidth: 52)
                }
            } minimal: {
                ActivityIcon(name: context.attributes.icon, size: 22)
            }
            .widgetURL(URL(string: "metamon://widget"))
            .keylineTint(WidgetTheme.brandA)
        }
    }
}

/// ロック画面のバナー: アイコン・タイトル・時間、下に小さく数字と進み具合。
struct LockScreenActivityView: View {
    let attributes: TailorActivityAttributes
    let state: TailorActivityAttributes.ContentState
    let isStale: Bool

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            ActivityIcon(name: attributes.icon, size: 44)
            VStack(alignment: .leading, spacing: 2) {
                Text(state.title)
                    .font(.headline)
                    .lineLimit(1)
                if let subtitle = state.subtitle {
                    Text(subtitle)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                ActivityDetail(appName: attributes.appName, state: state)
            }
            Spacer(minLength: 8)
            ActivityClockText(clock: state.clock, finished: state.finished || isStale)
                .font(.system(size: 34, weight: .semibold, design: .rounded).monospacedDigit())
                .multilineTextAlignment(.trailing)
                .frame(maxWidth: 130, alignment: .trailing)
        }
        .padding(16)
    }
}

/// 数字(stat)と進み具合(progressBar)。無ければアプリ名。
struct ActivityDetail: View {
    let appName: String
    let state: TailorActivityAttributes.ContentState

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            if let value = state.value {
                HStack(spacing: 4) {
                    if let label = state.valueLabel {
                        Text(label).foregroundStyle(.secondary)
                    }
                    Text(value).fontWeight(.semibold).monospacedDigit()
                }
                .font(.caption)
                .lineLimit(1)
            } else {
                Text(appName)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            if let progress = state.progress {
                ProgressView(value: min(max(progress, 0), 1))
                    .tint(progress > 1 ? .orange : WidgetTheme.brandB)
            }
        }
    }
}

/// 時間の表示。countdown / 経過は OS が日付から描く(アプリが止まっていても進む)。
struct ActivityClockText: View {
    let clock: TailorActivityAttributes.Clock?
    let finished: Bool

    var body: some View {
        switch clock {
        case .countdown(let start, let end) where !finished:
            Text(timerInterval: start...max(start, end), countsDown: true)
        case .elapsed(let since):
            Text(since, style: .timer)
        case .paused(let seconds):
            Text(TailorActivityAttributes.format(seconds: seconds))
        case .countdown:
            Text(TailorActivityAttributes.format(seconds: 0))
        case nil:
            EmptyView()
        }
    }
}

struct ActivityIcon: View {
    let name: String
    let size: CGFloat

    var body: some View {
        ZStack {
            Circle().fill(WidgetTheme.brandGradient)
            Image(systemName: name)
                .font(.system(size: size * 0.45, weight: .semibold))
                .foregroundStyle(.white)
        }
        .frame(width: size, height: size)
    }
}

// MARK: - Previews

#Preview("Lock screen", as: .content, using: TailorActivityAttributes.preview) {
    TailorLiveActivity()
} contentStates: {
    TailorActivityAttributes.ContentState.previewCountdown
    TailorActivityAttributes.ContentState.previewElapsed
}

#Preview("Island expanded", as: .dynamicIsland(.expanded), using: TailorActivityAttributes.preview) {
    TailorLiveActivity()
} contentStates: {
    TailorActivityAttributes.ContentState.previewCountdown
}

#Preview("Island compact", as: .dynamicIsland(.compact), using: TailorActivityAttributes.preview) {
    TailorLiveActivity()
} contentStates: {
    TailorActivityAttributes.ContentState.previewCountdown
}

#Preview("Island minimal", as: .dynamicIsland(.minimal), using: TailorActivityAttributes.preview) {
    TailorLiveActivity()
} contentStates: {
    TailorActivityAttributes.ContentState.previewElapsed
}

extension TailorActivityAttributes {
    static let preview = TailorActivityAttributes(appId: "preview", appName: "ラーメンタイマー", icon: "flame", key: "timer:preview", statId: nil)
}

extension TailorActivityAttributes.ContentState {
    static let previewCountdown = Self(title: "カップ麺", subtitle: nil, clock: .countdown(start: .now, end: .now.addingTimeInterval(180)),
                                       value: nil, valueLabel: nil, progress: nil, finished: false)
    static let previewElapsed = Self(title: "行列に並び中", subtitle: nil, clock: .elapsed(since: .now.addingTimeInterval(-420)),
                                     value: "12 件", valueLabel: "今月の杯数", progress: 0.4, finished: false)
}
