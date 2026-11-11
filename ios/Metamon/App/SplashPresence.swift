import Foundation
import Observation
import QuartzCore

/// スプラッシュが前に出ているか。出ている間はシート・吹き出し・保留中のリンク/通知を出さない(スプラッシュの上に重なるため)。
@MainActor
@Observable
final class SplashPresence {
    static let shared = SplashPresence()

    private(set) var isShowing = false
    @ObservationIgnored private var waiters: [CheckedContinuation<Void, Never>] = []

    func begin() { isShowing = true }

    func end() {
        guard isShowing else { return }
        isShowing = false
        let pending = waiters
        waiters = []
        pending.forEach { $0.resume() }
    }

    /// スプラッシュが出ていなければすぐ戻る。出ていれば終わるまで待つ。
    func waitUntilHidden() async {
        guard isShowing else { return }
        await withCheckedContinuation { waiters.append($0) }
    }

    #if DEBUG
    func resetForTesting() { isShowing = false; waiters = [] }
    #endif
}

/// 起動時に出す画面の順番待ち: 同意(`ConsentHost`)→ チュートリアル(`TutorialHost`)→ シェアリンク(`SharingHooks`)→ Intent(開く/話しかける)。
/// シートを同時に出すと SwiftUI が片方を捨て、状態が残ったまま止まるため、前の画面が閉じきってから(出さないと決まってから)次へ進む。
/// 各段は出さなかった・閉じた・画面が消えた時に必ず `end(_:)` する(待たせたままにしない)。全部消えた時は `endAll()`。
@MainActor
@Observable
final class LaunchConsentHold {
    static let shared = LaunchConsentHold()

    /// 出す順。後ろの段は前の段がすべて終わるまで待つ。Intent は全段の後。
    enum Step: Int, Comparable, CaseIterable {
        case consent, tutorial, shareLink
        static func < (a: Step, b: Step) -> Bool { a.rawValue < b.rawValue }
    }

    private(set) var pending: Set<Step> = []
    @ObservationIgnored private var waiters: [(before: Step?, continuation: CheckedContinuation<Void, Never>)] = []

    /// どれかの段が残っている(Intent を待たせる)。
    var isHolding: Bool { !pending.isEmpty }

    /// `step` より前の段が残っているか(`step` の番がまだ来ていない)。
    func isHolding(before step: Step) -> Bool { pending.contains { $0 < step } }

    func begin(_ step: Step = .consent) { pending.insert(step) }

    func end(_ step: Step = .consent) {
        guard pending.remove(step) != nil else { return }
        resumeReleased()
    }

    /// 画面が消えた(サインアウト等)時: 残りを全部放す。
    func endAll() {
        guard !pending.isEmpty else { return }
        pending = []
        resumeReleased()
    }

    /// 全段が終わるまで待つ(Intent)。待たせていなければすぐ戻る。
    func waitUntilReleased() async { await wait(before: nil) }

    /// `step` より前の段が終わるまで待つ。
    func waitUntilReleased(before step: Step) async { await wait(before: step) }

    private func wait(before step: Step?) async {
        guard isBlocked(before: step) else { return }
        await withCheckedContinuation { waiters.append((step, $0)) }
    }

    private func isBlocked(before step: Step?) -> Bool {
        guard let step else { return isHolding }
        return isHolding(before: step)
    }

    private func resumeReleased() {
        var still: [(before: Step?, continuation: CheckedContinuation<Void, Never>)] = []
        var ready: [CheckedContinuation<Void, Never>] = []
        for w in waiters {
            if isBlocked(before: w.before) { still.append(w) } else { ready.append(w.continuation) }
        }
        waiters = still
        ready.forEach { $0.resume() }
    }

    #if DEBUG
    func resetForTesting() {
        pending = []
        let pending = waiters
        waiters = []
        pending.forEach { $0.continuation.resume() }
    }
    #endif
}

/// アニメを始める合図: 最初のコマが描かれ、メインスレッドが空いた(画面の更新が続けて間に合った)ら始める。
/// `cap` 秒待っても空かなければ、動かさずに完成形を出してフェードする。
struct SplashStartGate {
    enum Decision: Equatable { case wait, start, fallback }

    static let cap: TimeInterval = 1.5
    /// 続けて間に合ったコマ数
    static let smoothFrames = 3
    /// 1 コマの間隔がこれ以内なら「間に合った」(60Hz で約 3 コマ)
    static let maxFrameGap: TimeInterval = 0.05

    let origin: TimeInterval
    private var last: TimeInterval?
    private var smooth = 0

    init(origin: TimeInterval) { self.origin = origin }

    mutating func frame(at time: TimeInterval) -> Decision {
        if time - origin >= Self.cap { return .fallback }
        if let last, time - last <= Self.maxFrameGap { smooth += 1 } else { smooth = 0 }
        last = time
        return smooth >= Self.smoothFrames ? .start : .wait
    }

    /// 起動からフェードの終わりまでの上限(秒)
    static let totalBudget: TimeInterval = 3.0
    /// フェードアウトの長さ(`SplashOverlay`)
    static let fadeOut: TimeInterval = 0.3

    /// 開始を `waited` 秒待った時の、アニメの速さの倍率(1 = そのまま・小さいほど速い)。
    /// 待った分だけ縮めて、待つ + アニメ + フェードを `totalBudget` に収める。
    static func pace(waited: TimeInterval, animation: TimeInterval) -> Double {
        let room = totalBudget - fadeOut - max(0, waited)
        guard animation > 0, room < animation else { return 1 }
        return max(0.1, room / animation)
    }
}

/// `SplashStartGate` を画面の更新(CADisplayLink)で回す。
@MainActor
final class SplashStartWaiter: NSObject {
    private var gate: SplashStartGate
    private var link: CADisplayLink?
    private var continuation: CheckedContinuation<SplashStartGate.Decision, Never>?

    private init(origin: TimeInterval) { gate = SplashStartGate(origin: origin) }

    /// 合図と、それまで待った秒数。
    static func wait() async -> (decision: SplashStartGate.Decision, waited: TimeInterval) {
        let origin = CACurrentMediaTime()
        let decision = await waitForDecision(origin: origin)
        return (decision, CACurrentMediaTime() - origin)
    }

    private static func waitForDecision(origin: TimeInterval) async -> SplashStartGate.Decision {
        let waiter = SplashStartWaiter(origin: origin)
        return await withCheckedContinuation { c in
            waiter.continuation = c
            let link = CADisplayLink(target: waiter, selector: #selector(tick(_:)))
            link.add(to: .main, forMode: .common)
            waiter.link = link
        }
    }

    @objc private func tick(_ link: CADisplayLink) {
        let decision = gate.frame(at: CACurrentMediaTime())
        guard decision != .wait else { return }
        link.invalidate() // CADisplayLink が target を強参照するので、ここで切れる
        self.link = nil
        continuation?.resume(returning: decision)
        continuation = nil
    }
}
