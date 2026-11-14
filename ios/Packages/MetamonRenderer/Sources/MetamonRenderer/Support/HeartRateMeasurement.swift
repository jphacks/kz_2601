import Foundation

/// Heart Rate Measurement(0x2A37)の 1 回分(#2237)。Bluetooth の仕様(Heart Rate Service 1.0)のバイト列を読む純関数。
/// 1 バイト目のフラグ: bit0 = 心拍が 16bit / bit1 = 接触あり / bit2 = 接触の検出に対応 / bit3 = 消費エネルギーあり / bit4 = RR 間隔あり。
/// 以降はリトルエンディアン。CoreBluetooth に依存しない(審査用ビルドにも残してよい)。
public struct HeartRateMeasurement: Sendable, Equatable {
    public enum Contact: Sendable, Equatable {
        /// 機器が接触の検出に対応していない
        case unsupported
        /// 肌に触れていない(値はあてにならない)
        case notDetected
        case detected
    }

    /// 拍/分
    public var beatsPerMinute: Int
    public var contact: Contact
    /// 前回のリセットからの消費エネルギー(kJ)。載っていなければ nil
    public var energyExpended: Int?
    /// RR 間隔(秒・古い順)。仕様の単位は 1/1024 秒
    public var rrIntervals: [TimeInterval]

    public init(beatsPerMinute: Int, contact: Contact, energyExpended: Int? = nil, rrIntervals: [TimeInterval] = []) {
        self.beatsPerMinute = beatsPerMinute
        self.contact = contact
        self.energyExpended = energyExpended
        self.rrIntervals = rrIntervals
    }

    /// 足りない・壊れたバイト列は nil。RR 間隔の端数(1 バイト余り)は捨てる
    public init?(_ data: Data) {
        let b = [UInt8](data)
        guard let flags = b.first else { return nil }
        var i = 1
        func u16() -> Int? {
            guard i + 1 < b.count else { return nil }
            defer { i += 2 }
            return Int(b[i]) | Int(b[i + 1]) << 8
        }
        if flags & 0x01 != 0 {
            guard let v = u16() else { return nil }
            beatsPerMinute = v
        } else {
            guard i < b.count else { return nil }
            beatsPerMinute = Int(b[i])
            i += 1
        }
        contact = flags & 0x04 == 0 ? .unsupported : (flags & 0x02 != 0 ? .detected : .notDetected)
        energyExpended = nil
        if flags & 0x08 != 0 {
            guard let e = u16() else { return nil }
            energyExpended = e
        }
        rrIntervals = []
        if flags & 0x10 != 0 {
            while let rr = u16() { rrIntervals.append(Double(rr) / 1024) }
        }
    }
}
