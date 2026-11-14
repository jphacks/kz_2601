import Foundation
import PencilKit
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif

/// Mac のマウス・トラックパッドの点の列 → `PKStroke`(PKCanvasView が無いので自分で集める)。
/// 見た目は iOS の `PKInkingTool(.pen, color: .black, width: 6)` に合わせる。
struct DrawingStrokeBuilder {
    static let penWidth: CGFloat = 6

    private(set) var points: [PKStrokePoint] = []
    private var startTime: TimeInterval?

    var locations: [CGPoint] { points.map(\.location) }
    var isEmpty: Bool { points.isEmpty }

    /// `pressure` はペンタブレットの筆圧(0...1)。無ければ一定の太さ。
    mutating func add(_ location: CGPoint, time: TimeInterval, pressure: CGFloat? = nil) {
        let start = startTime ?? time
        startTime = start
        let width = Self.width(pressure: pressure)
        points.append(PKStrokePoint(location: location, timeOffset: max(0, time - start),
                                    size: CGSize(width: width, height: width), opacity: 1,
                                    force: pressure ?? 1, azimuth: 0, altitude: .pi / 2))
    }

    /// 筆圧 0 で半分・1 で `penWidth`。
    static func width(pressure: CGFloat?) -> CGFloat {
        guard let pressure else { return penWidth }
        return penWidth * (0.5 + 0.5 * min(1, max(0, pressure)))
    }

    /// 離した時の線。クリックだけ(点 1 つ)でも点が残るよう、同じ所にもう 1 点足す。
    func stroke(creationDate: Date = Date()) -> PKStroke? {
        guard let first = points.first else { return nil }
        var controls = points
        if controls.count == 1 {
            controls.append(PKStrokePoint(location: CGPoint(x: first.location.x + 0.5, y: first.location.y),
                                          timeOffset: first.timeOffset + 0.01, size: first.size, opacity: first.opacity,
                                          force: first.force, azimuth: first.azimuth, altitude: first.altitude))
        }
        let path = PKStrokePath(controlPoints: controls, creationDate: creationDate)
        return PKStroke(ink: PKInk(.pen, color: .black), path: path)
    }
}
