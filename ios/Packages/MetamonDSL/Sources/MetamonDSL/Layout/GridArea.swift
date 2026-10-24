import Foundation

/// `area` の解析結果。列は 0 始まり(A=0)、行は 1 始まり、両端含む。
/// 画面は横 6 列(A-F)× 縦 N 行(1 行 = 可視高さ ÷ 12、13 行目以降は画面全体がスクロール)。
public struct GridArea: Equatable, Sendable {
    public static let columns = 6
    public static let visibleRows = 12

    public var col1: Int, row1: Int, col2: Int, row2: Int

    public init?(_ s: String) {
        let parts = s.split(separator: ":")
        guard parts.count == 2, let a = Self.cell(parts[0]), let b = Self.cell(parts[1]),
              a.col <= b.col, a.row <= b.row else { return nil }
        (col1, row1, col2, row2) = (a.col, a.row, b.col, b.row)
    }

    private static func cell(_ s: Substring) -> (col: Int, row: Int)? {
        guard let first = s.first, let ascii = first.asciiValue, (65...70).contains(ascii),
              let row = Int(s.dropFirst()), row >= 1 else { return nil }
        return (Int(ascii) - 65, row)
    }

    public var colSpan: Int { col2 - col1 + 1 }
    public var rowSpan: Int { row2 - row1 + 1 }

    public func overlaps(_ o: GridArea) -> Bool {
        col1 <= o.col2 && o.col1 <= col2 && row1 <= o.row2 && o.row1 <= row2
    }
}

public extension Screen {
    /// `area` 付きの直下コンポーネント(絶対配置)と、無いもの(下に積む)に分ける。
    var gridPlacement: (placed: [(component: Component, area: GridArea)], stacked: [Component]) {
        let placed = components.compactMap { c in c.gridArea.map { (component: c, area: $0) } }
        let stacked = components.filter { $0.gridArea == nil }
        return (placed, stacked)
    }

    /// 兄弟同士で `area` が重なっている組(backend も検証するが、クライアントでも検出できるように)。
    var overlappingAreas: [(Component, Component)] {
        let placed = gridPlacement.placed
        var out: [(Component, Component)] = []
        for i in placed.indices {
            for j in placed.indices where j > i && placed[i].area.overlaps(placed[j].area) {
                out.append((placed[i].component, placed[j].component))
            }
        }
        return out
    }
}
