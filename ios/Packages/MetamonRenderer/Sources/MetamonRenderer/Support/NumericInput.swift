import Foundation

/// 数値入力(slider / stepper / rating)の DSL 値の丸め。AI が作る DSL は min > max・step ≤ 0・巨大な数が来うる。
enum NumericInput {
    /// min...max。省略・非有限は既定値。max ≤ min なら min のすぐ上まで(幅 0 の範囲にしない)。
    static func range(min: Double?, max: Double?, defaultMin: Double, defaultMax: Double) -> ClosedRange<Double> {
        let lo = min.flatMap { $0.isFinite ? $0 : nil } ?? defaultMin
        let rawHi = max.flatMap { $0.isFinite ? $0 : nil } ?? defaultMax
        let hi = rawHi > lo ? rawHi : Swift.max(lo + 0.001, lo.nextUp)
        return lo...hi
    }

    /// 刻み。0 以下・非有限は nil(slider は連続、stepper は呼び出し側の既定)。
    static func step(_ raw: Double?) -> Double? {
        guard let raw, raw.isFinite, raw > 0 else { return nil }
        return raw
    }

    /// rating の星の数(1〜10・既定 5)。巨大な値も Int に変換する前に丸める。
    static func ratingCount(_ raw: Double?) -> Int {
        guard let raw, raw.isFinite else { return 5 }
        return Int(Swift.min(Swift.max(raw, 1), 10))
    }

    /// rating の保存値 → 塗る星の数(0〜count)。巨大・非有限な値で落ちない。
    static func ratingValue(_ v: Double, count: Int) -> Int {
        guard v.isFinite else { return 0 }
        return Int(Swift.min(Swift.max(v, 0), Double(count)).rounded())
    }

    /// 表示用に Int へ丸める。範囲外・非有限は端に寄せる(`Int(Double)` は範囲外で落ちる)。
    static func clampedInt(_ v: Double) -> Int {
        guard v.isFinite else { return v > 0 ? Int.max : (v < 0 ? Int.min : 0) }
        let r = v.rounded()
        if r >= 9.2e18 { return Int.max }
        if r <= -9.2e18 { return Int.min }
        return Int(r)
    }
}
