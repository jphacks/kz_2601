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
}

/// `SplashStartGate` を画面の更新(CADisplayLink)で回す。
@MainActor
final class SplashStartWaiter: NSObject {
    private var gate: SplashStartGate
    private var link: CADisplayLink?
    private var continuation: CheckedContinuation<SplashStartGate.Decision, Never>?

    private init(origin: TimeInterval) { gate = SplashStartGate(origin: origin) }

    static func wait() async -> SplashStartGate.Decision {
        let waiter = SplashStartWaiter(origin: CACurrentMediaTime())
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
