#if METAMON_STG
import Foundation

/// 開発用メニューのアカウントの枠(#2084)。枠ごとに Firebase の名前付きアプリを使い、サインインの状態を別々に持つ。
/// 枠 0 = いつものアカウント(既定のアプリ)。1〜4 = 試験用。
/// 保存先は `DebugKnobs` のスイート。ただしキーは `metamon.debug.*` の外に置く
/// (「全部元に戻す」で端末のデータを消さずに枠 0 へ戻ると、前の枠のデータが残ったまま別の uid になるため)。
enum DebugAuthSlot {
    static let slots = 0...4
    static let key = "metamon.debugAuthSlot"

    /// 保存した値から枠を決める。無し・数でない・範囲外は枠 0
    static func slot(from raw: String?) -> Int {
        guard let raw, let n = Int(raw), slots.contains(n) else { return 0 }
        return n
    }

    /// Firebase のアプリの名前。枠 0 は nil(既定のアプリ)
    static func appName(for slot: Int) -> String? {
        slot == 0 || !slots.contains(slot) ? nil : "metamon-slot-\(slot)"
    }

    static func current(in defaults: UserDefaults) -> Int { slot(from: defaults.string(forKey: key)) }

    static func save(_ slot: Int, in defaults: UserDefaults) {
        if slot == 0 || !slots.contains(slot) { defaults.removeObject(forKey: key) } else { defaults.set(String(slot), forKey: key) }
    }

    /// 起動中の値(`BuildConfig.makeAuth` は起動時に 1 回だけ読む)
    static var current: Int { current(in: DebugKnobs.store.defaults) }

    /// 枠を切り替える: 端末のデータを消す(サインインは残す)→ 選んだ枠を保存 → 終了して開き直す
    @MainActor
    static func switchTo(_ slot: Int, defaults: UserDefaults, wipe: @MainActor () async -> Void, relaunch: @MainActor () -> Void) async {
        await wipe()
        save(slot, in: defaults)
        relaunch()
    }
}
#endif
