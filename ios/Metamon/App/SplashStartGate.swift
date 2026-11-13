import Foundation
import QuartzCore

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
