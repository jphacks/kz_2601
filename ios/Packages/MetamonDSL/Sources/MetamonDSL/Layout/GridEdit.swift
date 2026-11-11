import Foundation

// 手で動かす編集モード(#1442)の計算。UI に依存しない(Renderer の編集レイヤーとホストの保存が使う)。
// 決まりは backend `dsl.checkGrid` と同じ: 6 列(A〜F)・行 1〜48・`area` を持つ兄弟同士でセルを共有しない。

public extension GridArea {
    init(col1: Int, row1: Int, col2: Int, row2: Int) {
        (self.col1, self.row1, self.col2, self.row2) = (col1, row1, col2, row2)
    }

    /// スプレッドシート記法(`A1:C3`)。backend `Area.String()` と同じ形。
    var notation: String {
        func col(_ c: Int) -> String { String(UnicodeScalar(UInt8(65 + c))) }
        return "\(col(col1))\(row1):\(col(col2))\(row2)"
    }
}

/// ドラッグ量をセル単位に丸める。
public enum GridEditMath {
    /// 動かす: 大きさはそのまま、左上をドラッグ量ぶん(セル単位に四捨五入)ずらし、グリッドの外に出ないよう寄せる。
    public static func moved(_ a: GridArea, dx: Double, dy: Double, cellW: Double, cellH: Double) -> GridArea {
        let dc = steps(dx, cellW), dr = steps(dy, cellH)
        let col1 = clamp(a.col1 + dc, 0, GridArea.columns - a.colSpan)
        let row1 = clamp(a.row1 + dr, 1, GridArea.maxRows - a.rowSpan + 1)
        return GridArea(col1: col1, row1: row1, col2: col1 + a.colSpan - 1, row2: row1 + a.rowSpan - 1)
    }

    /// 大きさを変える: 左上は固定、右下の角をドラッグ量ぶん動かす。1 × 1 より小さくせず、グリッドの外に出さない。
    public static func resized(_ a: GridArea, dx: Double, dy: Double, cellW: Double, cellH: Double) -> GridArea {
        let col2 = clamp(a.col2 + steps(dx, cellW), a.col1, GridArea.columns - 1)
        let row2 = clamp(a.row2 + steps(dy, cellH), a.row1, GridArea.maxRows)
        return GridArea(col1: a.col1, row1: a.row1, col2: col2, row2: row2)
    }

    static func steps(_ d: Double, _ cell: Double) -> Int {
        guard cell > 0, d.isFinite else { return 0 }
        return Int((d / cell).rounded())
    }

    static func clamp(_ v: Int, _ lo: Int, _ hi: Int) -> Int { min(max(v, lo), max(lo, hi)) }
}

/// RFC 6902 の 1 操作(backend `dsl.PatchOp`)。
public struct JSONPatchOp: Encodable, Equatable, Sendable {
    public var op: String
    public var path: String
    public var value: JSONValue?

    public init(op: String, path: String, value: JSONValue? = nil) {
        self.op = op
        self.path = path
        self.value = value
    }
}

/// 1 画面ぶんの編集中の配置。`area` を持つ直下の部品だけを扱う(area 無しで積まれた部品・Container の中は v1 では動かさない)。
public struct GridEditDraft: Equatable, Sendable {
    public struct Item: Equatable, Sendable, Identifiable {
        /// `screen.components` の中の位置(patch の path に使う)
        public var index: Int
        public var componentId: String
        /// DSL に書かれていた `area` の文字列(patch の `test` に使う)
        public var originalText: String
        public var original: GridArea
        public var area: GridArea
        public var removed = false
        /// 読み上げに使う部品の種類と見出し(title → label → text → placeholder の最初の空でないもの)
        public var type: ComponentType = .unknown
        public var title: String?

        public var id: String { componentId }
        public var changed: Bool { removed || area != original }
    }

    public var screenId: String
    /// 画面の位置(`dsl.screens` の中)
    public var screenIndex: Int
    public private(set) var items: [Item]
    /// 画面の部品の数(area 無しも含む)。最後の 1 つは消させない(空の画面は DSL として不正)
    public var componentCount: Int

    public init(screen: Screen, screenIndex: Int) {
        screenId = screen.id
        self.screenIndex = screenIndex
        componentCount = screen.components.count
        items = screen.components.enumerated().compactMap { i, c in
            guard let text = c.area, let a = c.gridArea else { return nil }
            let title = [c.title, c.label, c.text, c.placeholder]
                .lazy.compactMap { $0?.trimmingCharacters(in: .whitespacesAndNewlines) }.first { !$0.isEmpty }
            return Item(index: i, componentId: c.id, originalText: text, original: a, area: a, type: c.type, title: title)
        }
    }

    /// 消していない部品
    public var placed: [Item] { items.filter { !$0.removed } }
    public var hasChanges: Bool { items.contains(where: \.changed) }
    public var canRemove: Bool { componentCount - items.filter(\.removed).count > 1 }

    public func item(_ componentId: String) -> Item? { items.first { $0.componentId == componentId } }

    /// `area` を `candidate` にした時に重なる、ほかの部品の id。
    public func conflicts(for componentId: String, at candidate: GridArea) -> [String] {
        placed.filter { $0.componentId != componentId && $0.area.overlaps(candidate) }.map(\.componentId)
    }

    /// 置けるなら置いて true。重なる時は何もせず false(backend の重なり禁止と同じ)。
    @discardableResult
    public mutating func place(_ componentId: String, at area: GridArea) -> Bool {
        guard let i = items.firstIndex(where: { $0.componentId == componentId && !$0.removed }),
              conflicts(for: componentId, at: area).isEmpty else { return false }
        items[i].area = area
        return true
    }

    @discardableResult
    public mutating func remove(_ componentId: String) -> Bool {
        guard canRemove, let i = items.firstIndex(where: { $0.componentId == componentId && !$0.removed }) else { return false }
        items[i].removed = true
        return true
    }

    /// 元の DSL からこの配置にする JSON Patch。
    /// - 先頭で画面の id を `test`(端末とサーバーで画面の並びがずれていたら当てない)
    /// - 動かした部品: 元の `area` を `test` → `replace`
    /// - 消した部品: 元の `area` を `test` → `remove`。後ろから消す(前の部品の位置がずれない)。replace は全部その前に置く
    public var patch: [JSONPatchOp] {
        guard hasChanges else { return [] }
        let screen = "/screens/\(screenIndex)"
        var ops = [JSONPatchOp(op: "test", path: "\(screen)/id", value: .string(screenId))]
        for item in items where !item.removed && item.area != item.original {
            let path = "\(screen)/components/\(item.index)/area"
            ops.append(JSONPatchOp(op: "test", path: path, value: .string(item.originalText)))
            ops.append(JSONPatchOp(op: "replace", path: path, value: .string(item.area.notation)))
        }
        for item in items.filter(\.removed).sorted(by: { $0.index > $1.index }) {
            let path = "\(screen)/components/\(item.index)"
            ops.append(JSONPatchOp(op: "test", path: "\(path)/area", value: .string(item.originalText)))
            ops.append(JSONPatchOp(op: "remove", path: path))
        }
        return ops
    }

    /// 端末だけで当てる(サーバーの版が無い時の保存・編集中の描画)。
    public func applied(to screen: Screen) -> Screen {
        var out = screen
        var comps = screen.components
        for item in items where item.index < comps.count && comps[item.index].id == item.componentId {
            comps[item.index].area = item.area.notation
        }
        let removed = Set(items.filter(\.removed).map(\.componentId))
        comps.removeAll { removed.contains($0.id) }
        out.components = comps
        return out
    }

    public func applied(to dsl: AppDSL) -> AppDSL {
        var out = dsl
        guard screenIndex < out.screens.count, out.screens[screenIndex].id == screenId else { return dsl }
        out.screens[screenIndex] = applied(to: out.screens[screenIndex])
        return out
    }
}
