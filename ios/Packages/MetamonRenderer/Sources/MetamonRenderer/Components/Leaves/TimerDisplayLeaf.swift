import SwiftUI
import MetamonDSL

/// `timerDisplay`: ストップウォッチ(mode=stopwatch・カウントアップ)/ タイマー(mode=countdown・`seconds` から 0 へ)。
/// 開始 / 一時停止 / リセット。一時停止・終了時に経過秒を `field`(integer/number)へ保存する(フォーム画面のみ)。
/// 時間は開始時刻から都度計算し `TimerRunStore` に保存(#38): 背面・再起動を挟んでも正しい残り時間を出し、
/// countdown を開始した時点で終了時刻にローカル通知を予約し(部品が画面から消えても残る)、一時停止・リセットで取り消す。
/// 前面でこの部品が見えている間は、アプリの通知 delegate がバナーを出さない(`TimerVisibility`)。
struct TimerDisplayLeaf: View {
    let c: Component
    let ctx: RenderContext
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.dslRuntime) private var rt
    @State private var elapsed: Int = 0
    @State private var run = TimerRun()
    @ScaledMetric(relativeTo: .largeTitle) private var timeSize: CGFloat = 44
    private var running: Bool { run.isRunning }

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
                .font(.system(size: timeSize, weight: .semibold, design: .rounded).monospacedDigit())
                .lineLimit(1)
                .minimumScaleFactor(0.4)
                .foregroundStyle(finished ? AnyShapeStyle(RenderTheme.Colors.negative) : AnyShapeStyle(RenderTheme.brandFill))
                .contentTransition(.numericText())
            HStack(spacing: 12) {
                Button(running ? L("一時停止") : (elapsed > 0 && !finished ? L("再開") : L("開始")), systemImage: running ? "pause.fill" : "play.fill") {
                    running ? pause() : start()
                }
                .buttonStyle(RenderButtonStyle(kind: .primary))
                .disabled(finished)
                Button(L("リセット"), systemImage: "arrow.counterclockwise") { reset() }
                    .buttonStyle(RenderButtonStyle(kind: .secondary))
                    .disabled(elapsed == 0 && !running)
            }
        }
        .frame(maxWidth: .infinity)
        .renderCard()
        .onReceive(tick) { now in
            guard running else { return }
            catchUp(now)
        }
        .onAppear {
            TimerVisibility.appeared(storeKey)
            restore()
        }
        .onDisappear { TimerVisibility.disappeared(storeKey) }
        .onChange(of: scenePhase) { _, phase in
            if phase == .active { catchUp(.now) }
        }
    }

    // MARK: - 時間の扱い(#38)

    /// 画面 / アプリ / 部品で一意。同じ名前のアプリが 2 つあっても混ざらないよう appId を使う。
    private var storeKey: String { TimerRunStore.key(appId: rt.appId, screen: ctx.route.screen, component: c.id) }

    private func restore() {
        #if DEBUG
        // ヘッドレス E2E 用: `METAMON_E2E_TIMER_START=<部品 id>` で表示時に開始(#38 の背面・通知の確認)
        if ProcessInfo.processInfo.environment["METAMON_E2E_TIMER_START"] == c.id, TimerRunStore.shared.load(storeKey)?.isRunning != true {
            print("[Metamon] E2E: timer start id=\(c.id)")
            elapsed = 0
            start()
            return
        }
        #endif
        guard let saved = TimerRunStore.shared.load(storeKey) else { return }
        run = saved
        catchUp(.now)
        if !running { elapsed = run.elapsed(at: .now) }
        if running { persist() } // 予約とロック画面の表示を取り直す(キーの移行で消えた分も戻る)
    }

    /// 開始時刻から経過を出し直す。背面中に終わっていれば完了として止める。
    private func catchUp(_ now: Date) {
        guard running else { return }
        elapsed = run.elapsed(at: now, total: isCountdown ? total : nil)
        if finished {
            run = TimerRun(base: total)
            persist()
            save()
        }
    }

    private func persist() {
        TimerRunStore.shared.save(run, for: storeKey)
        notify()
        if c.liveActivity == true {
            rt.liveActivity?.timerActivity(TimerActivityUpdate(key: storeKey, label: c.label.flatMap { $0.isEmpty ? nil : $0 } ?? L("タイマー"),
                                                 total: isCountdown ? total : nil, run: run))
        }
    }

    private func notify() {
        let plan = TimerNotificationPlan.decide(isCountdown: isCountdown, run: run, total: total, now: .now)
        TimerNotifier.apply(plan, key: storeKey, title: rt.dsl.app.name, name: c.label.flatMap { $0.isEmpty ? nil : $0 } ?? L("タイマー"))
    }

    private func start() {
        run = TimerRun(base: elapsed, startedAt: Date())
        persist() // countdown なら終了通知をここで予約する(初回は許可を求める)
    }

    private func pause() {
        elapsed = run.elapsed(at: .now, total: isCountdown ? total : nil)
        run = TimerRun(base: elapsed)
        persist()
        save()
    }

    private func reset() {
        run = TimerRun()
        elapsed = 0
        persist()
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
