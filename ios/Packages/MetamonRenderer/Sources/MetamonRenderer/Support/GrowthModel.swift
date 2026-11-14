import CoreGraphics
import Foundation

/// `growth` の育ち具合(純粋な計算・テストはここを見る)。
/// `step` ごとに実・花・葉が 1 つ増え、`target` で育ち切る。実の数は `target ÷ step`(最大 30)まで。
public struct GrowthModel: Equatable, Sendable {
    public enum Kind: String, Sendable, CaseIterable {
        case tree, flower, plant
        /// 未知・省略は tree(iOS は寛容)
        public init(_ raw: String?) { self = raw.flatMap(Kind.init(rawValue:)) ?? .tree }
    }

    public static let maxItems = 30

    public let value: Double
    public let target: Double
    public let step: Double

    /// - Parameters:
    ///   - target: 0 以下・NaN は 1 として扱う(0 で割らない)
    ///   - step: 省略・0 以下は target の 1/10。実の数が 30 を超える小ささは 30 に収まる大きさに上げる
    public init(value: Double, target: Double, step: Double?) {
        let t = target.isFinite && target > 0 ? target : 1
        self.value = value.isFinite ? max(value, 0) : 0
        self.target = t
        var s = step.flatMap { $0.isFinite && $0 > 0 ? $0 : nil } ?? t / 10
        if t / s > Double(Self.maxItems) { s = t / Double(Self.maxItems) }
        self.step = s
    }

    /// 育ち切った時の実・花・葉の数(1〜30)
    public var capacity: Int { min(max(Int((target / step - 1e-9).rounded(.up)), 1), Self.maxItems) }

    /// 今ついている実・花・葉の数。target に届いたら必ず capacity
    public var items: Int {
        if isComplete { return capacity }
        return min(max(Int((value / step + 1e-9).rounded(.down)), 0), capacity)
    }

    /// 0〜1 の達成率
    public var progress: Double { min(value / target, 1) }

    public var isComplete: Bool { value >= target }

    /// 次の実がなる値(育ち切ったら nil)
    public var nextAt: Double? { isComplete ? nil : min(step * Double(items + 1), target) }

    /// 木・草の体の大きさ(0.55〜1)。最初に大きく伸び、後はゆっくり
    public var bodyScale: Double { 0.55 + 0.45 * progress.squareRoot() }

    /// 実・花・葉を置く位置(描く領域を 0〜1 にした座標)。capacity だけで決まるので、増えても前の実は動かない。
    public static func slots(_ kind: Kind, count: Int) -> [CGPoint] {
        let n = max(count, 0)
        let golden = 2.399963229728653 // 黄金角(rad)
        switch kind {
        case .tree:
            // 樹冠(中心 0.5, 0.34・半径 0.33×0.2)の中にひまわりの種の並び
            return (0..<n).map { i in
                let r = ((Double(i) + 0.5) / Double(n)).squareRoot() * 0.86
                let a = Double(i) * golden
                return CGPoint(x: 0.5 + r * 0.33 * cos(a), y: 0.34 + r * 0.2 * sin(a))
            }
        case .flower:
            // 地面の上の野原。左右は黄金比の数列でばらし、高さは 3 段
            return (0..<n).map { i in
                let x = 0.1 + 0.8 * (Double(i) * 0.6180339887).truncatingRemainder(dividingBy: 1)
                let y = 0.42 + 0.3 * (Double(i % 3) / 2) + 0.04 * (Double(i) * 0.381966).truncatingRemainder(dividingBy: 1)
                return CGPoint(x: x, y: y)
            }
        case .plant:
            // 茎に沿って下から左右交互(y は茎の長さに対する割合・x は -1 左 / 1 右)
            return (0..<n).map { i in
                CGPoint(x: i.isMultiple(of: 2) ? -1 : 1, y: (Double(i) + 1) / (Double(n) + 1))
            }
        }
    }
}
