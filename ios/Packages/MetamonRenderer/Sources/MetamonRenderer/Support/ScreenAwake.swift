import UIKit

/// 画面を消さない(`isIdleTimerDisabled`)を、使う所ごとに持つ。持っている所が 1 つでもあれば消さない。
/// 直接 `isIdleTimerDisabled = false` にすると、ドラレコ・道案内・文字起こしの他の使い手の分まで戻してしまうため。
@MainActor
final class ScreenAwake {
    static let shared = ScreenAwake { UIApplication.shared.isIdleTimerDisabled = $0 }

    private(set) var holders = 0
    private let apply: (Bool) -> Void

    init(apply: @escaping (Bool) -> Void) { self.apply = apply }

    /// 持つ。やめる時は返り値の `release()`(何度呼んでもよい)。
    func hold() -> Hold {
        holders += 1
        if holders == 1 { apply(true) }
        return Hold(owner: self)
    }

    fileprivate func drop() {
        guard holders > 0 else { return }
        holders -= 1
        if holders == 0 { apply(false) }
    }

    @MainActor
    final class Hold {
        private weak var owner: ScreenAwake?

        fileprivate init(owner: ScreenAwake) { self.owner = owner }

        func release() {
            owner?.drop()
            owner = nil
        }
    }
}

/// 背面に回った時、今の処理を区切りよく止めるまでの猶予(`beginBackgroundTask`)。`end()` は何度呼んでもよい。
@MainActor
final class BackgroundGrace {
    private var id: UIBackgroundTaskIdentifier = .invalid

    func begin(_ name: String) {
        guard id == .invalid else { return }
        id = UIApplication.shared.beginBackgroundTask(withName: name) { [weak self] in
            MainActor.assumeIsolated { self?.end() }
        }
    }

    func end() {
        guard id != .invalid else { return }
        UIApplication.shared.endBackgroundTask(id)
        id = .invalid
    }
}
