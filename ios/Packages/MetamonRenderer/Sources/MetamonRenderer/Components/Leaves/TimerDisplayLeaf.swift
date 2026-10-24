import SwiftUI
import MetamonDSL

/// `timerDisplay`: ストップウォッチ(mode=stopwatch・カウントアップ)/ タイマー(mode=countdown・`seconds` から 0 へ)。
/// 開始 / 一時停止 / リセット。一時停止・終了時に経過秒を `field`(integer/number)へ保存する(フォーム画面のみ)。
struct TimerDisplayLeaf: View {
    let c: Component
    let ctx: RenderContext
    @State private var elapsed: Int = 0
    @State private var running = false
    @State private var startedAt: Date?
    @State private var base: Int = 0

    private let tick = Timer.publish(every: 0.25, on: .main, in: .common).autoconnect()

    private var isCountdown: Bool { c.mode == "countdown" }
    private var total: Int { max(c.seconds ?? 0, 0) }
    private var shown: Int { isCountdown ? max(total - elapsed, 0) : elapsed }
    private var finished: Bool { isCountdown && elapsed >= total }

    var body: some View {
        VStack(spacing: 10) {
            if let label = c.label, !label.isEmpty {
                Text(label).font(.caption).foregroundStyle(.secondary)
            }
            Text(Self.format(shown))
                .font(.system(size: 44, weight: .semibold, design: .rounded).monospacedDigit())
                .foregroundStyle(finished ? AnyShapeStyle(RenderTheme.Colors.negative) : AnyShapeStyle(RenderTheme.brandGradient))
                .contentTransition(.numericText())
            HStack(spacing: 12) {
                Button(running ? "一時停止" : (elapsed > 0 && !finished ? "再開" : "開始"), systemImage: running ? "pause.fill" : "play.fill") {
                    running ? pause() : start()
                }
                .buttonStyle(RenderButtonStyle(kind: .primary))
                .disabled(finished)
                Button("リセット", systemImage: "arrow.counterclockwise") { reset() }
                    .buttonStyle(RenderButtonStyle(kind: .secondary))
                    .disabled(elapsed == 0 && !running)
            }
        }
        .frame(maxWidth: .infinity)
        .renderCard()
        .onReceive(tick) { now in
            guard running, let startedAt else { return }
            elapsed = base + Int(now.timeIntervalSince(startedAt))
            if finished {
                running = false
                self.startedAt = nil
                base = total
                save()
            }
        }
    }

    private func start() {
        startedAt = Date()
        running = true
    }

    private func pause() {
        running = false
        base = elapsed
        startedAt = nil
        save()
    }

    private func reset() {
        running = false
        startedAt = nil
        base = 0
        elapsed = 0
    }

    /// 経過秒を number field へ(stopwatch=計測秒、countdown=消費秒)。
    private func save() {
        guard let form = ctx.form, let field = c.field else { return }
        form.values[field] = .number(Double(elapsed))
    }

    static func format(_ seconds: Int) -> String {
        let h = seconds / 3600, m = (seconds % 3600) / 60, s = seconds % 60
        return h > 0 ? String(format: "%d:%02d:%02d", h, m, s) : String(format: "%02d:%02d", m, s)
    }
}
