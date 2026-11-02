import SwiftUI

#if !APP_STORE_REVIEW
import MetamonRenderer

/// 「記録中」バーの出し分けと表示の文字(#559)。View から切り離してテストする。
enum TrackingBarModel {
    static func isVisible(_ status: LocationTrackingStatus) -> Bool { status.isTracking }

    /// 経過時間 `12:34` / `1:02:03`。許可待ちは 0。
    static func elapsed(_ status: LocationTrackingStatus, now: Date) -> TimeInterval {
        guard let startedAt = status.startedAt else { return 0 }
        return max(0, now.timeIntervalSince(startedAt))
    }

    static func clock(_ seconds: TimeInterval) -> String {
        let s = Int(max(0, seconds))
        return s >= 3600 ? String(format: "%d:%02d:%02d", s / 3600, s / 60 % 60, s % 60) : String(format: "%02d:%02d", s / 60, s % 60)
    }

    /// VoiceOver 用の経過時間(「12分34秒」/ "12 minutes, 34 seconds")
    static func spokenDuration(_ seconds: TimeInterval, locale: Locale) -> String {
        let f = DateComponentsFormatter()
        f.unitsStyle = .full
        f.allowedUnits = seconds >= 3600 ? [.hour, .minute, .second] : [.minute, .second]
        f.zeroFormattingBehavior = .dropLeading
        var calendar = Calendar.current
        calendar.locale = locale
        f.calendar = calendar
        return f.string(from: max(0, seconds.rounded(.down))) ?? clock(seconds)
    }
}

/// アプリ全体の「記録中」バー: ● 記録中 12:34 [停止]。停止は確認してから。
struct TrackingBar: View {
    let status: LocationTrackingStatus
    let onStop: () -> Void
    @Environment(\.locale) private var locale
    @State private var confirmsStop = false

    var body: some View {
        HStack(spacing: 10) {
            TimelineView(.periodic(from: .now, by: 1)) { context in
                let elapsed = TrackingBarModel.elapsed(status, now: context.date)
                HStack(spacing: 8) {
                    Circle().fill(.red).frame(width: 8, height: 8)
                    Text(status.isRequesting ? "位置情報の許可を待っています" : "記録中")
                        .font(.subheadline.weight(.semibold))
                        .lineLimit(1)
                    if !status.isRequesting {
                        Text(verbatim: TrackingBarModel.clock(elapsed))
                            .font(.subheadline.monospacedDigit())
                            .foregroundStyle(.secondary)
                    }
                }
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(status.isRequesting
                    ? Text("位置情報の許可を待っています")
                    : Text("位置を記録中・経過 \(TrackingBarModel.spokenDuration(elapsed, locale: locale))"))
                .accessibilityIdentifier("trackingBar.status")
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Button("停止", systemImage: "stop.fill") { confirmsStop = true }
                .font(.subheadline.weight(.semibold))
                .buttonStyle(.bordered)
                .tint(.red)
                .controlSize(.small)
                .frame(minHeight: 44) // タップ領域 44pt
                .accessibilityLabel(Text("位置の記録を停止"))
                .accessibilityIdentifier("trackingBar.stop")
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 2)
        .background {
            Rectangle()
                .fill(Color(.secondarySystemBackground))
                .overlay(alignment: .bottom) { Rectangle().fill(Color.red.opacity(0.5)).frame(height: 1) }
                .ignoresSafeArea(edges: .top)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("trackingBar")
        .confirmationDialog("位置の記録を停止しますか?", isPresented: $confirmsStop, titleVisibility: .visible) {
            Button("停止", role: .destructive, action: onStop)
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text("ここまでに記録した地点はそのまま残ります。")
        }
    }
}
#endif

/// 位置の記録中(`locationTracker`)だけ、どの画面でも上に「記録中」バーを重ねる(#559)。
/// 審査用の版は位置の記録ごと外すので何もしない(docs/ios/review-build.md)。
struct TrackingBarHost: ViewModifier {
    #if !APP_STORE_REVIEW
    /// 状態の読み出しと停止(テストで差し替える)
    var status: @MainActor () -> LocationTrackingStatus = { LocationTrackingControl.status }
    var stop: @MainActor () -> Void = { LocationTrackingControl.stop() }

    func body(content: Content) -> some View {
        let status = status()
        content
            .safeAreaInset(edge: .top, spacing: 0) {
                if TrackingBarModel.isVisible(status) {
                    TrackingBar(status: status, onStop: stop)
                        .transition(.move(edge: .top).combined(with: .opacity))
                }
            }
            .animation(.easeInOut(duration: 0.25), value: status.isTracking)
    }
    #else
    func body(content: Content) -> some View { content }
    #endif
}
