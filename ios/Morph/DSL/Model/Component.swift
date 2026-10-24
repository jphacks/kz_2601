import Foundation

/// コンポーネント種別(schema `type`)。未知は `.unknown` に丸めてプレースホルダ描画する。
enum ComponentType: String, Decodable, Sendable {
    // container(children は Leaf のみ・1段)
    case vstack, hstack, section
    // leaf
    case text, button, textField, numberField, datePicker, picker, toggle
    case photoField, image, list, stat, chart, voiceInput, spacer, divider
    case unknown

    init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .unknown) }

    var isContainer: Bool { self == .vstack || self == .hstack || self == .section }
}

/// 全コンポーネントの props をフラットに持つ1ノード(Go の `dsl.Component` と 1:1)。
/// どの props が意味を持つかは `type` 次第(docs/dsl/README.md のコンポーネント表)。
struct Component: Decodable, Identifiable, Sendable {
    let id: String
    var type: ComponentType
    /// デコード時の生の type 文字列(未知 type のプレースホルダ表示用)。
    var rawType: String
    /// 画面グリッド上の矩形 "A1:C3"(Screen 直下のみ有効)。nil = グリッドの下に積む。
    var area: String?
    var gridArea: GridArea? { area.flatMap(GridArea.init) }

    // container
    var title: String?          // section / chart
    var spacing: String?        // none|small|medium|large
    var children: [Component]

    // text / button / inputs
    var text: String?
    var style: String?          // text: title|headline|body|caption / button: primary|secondary|destructive|plain / picker: menu|segmented|wheel
    var align: String?          // leading|center|trailing
    var label: String?
    var icon: String?
    var action: Action?
    var field: String?          // 入力の束縛先 field id / stat・chart の値 field
    var placeholder: String?
    var multiline: Bool?
    var unit: String?
    var includeTime: Bool?
    var source: String?         // photoField: camera|library|both
    var height: String?         // image: small|medium|large

    // data-bound(list / stat / chart)
    var entity: String?
    var item: ListItem?
    var sort: Sort?
    var filter: Filter?
    var emptyText: String?
    var onItemTap: Action?
    var swipeToDelete: Bool?
    var aggregate: String?      // count|sum|avg|min|max
    var format: String?         // plain|currency|percent
    var kind: String?           // bar|line|pie
    var groupBy: String?
    var bucket: String?         // day|week|month

    // voiceInput / spacer
    var prompt: String?
    var size: String?           // voiceInput: compact|large / spacer: small|medium|large|flexible

    private enum Keys: String, CodingKey {
        case id, type, area, title, spacing, children, text, style, align, label, icon, action, field, placeholder, multiline, unit
        case includeTime, source, height, entity, item, sort, filter, emptyText, onItemTap, swipeToDelete, aggregate, format
        case kind, groupBy, bucket, prompt, size
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        rawType = try c.decodeIfPresent(String.self, forKey: .type) ?? ""
        type = ComponentType(rawValue: rawType) ?? .unknown
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? UUID().uuidString
        area = try? c.decodeIfPresent(String.self, forKey: .area)
        title = try? c.decodeIfPresent(String.self, forKey: .title)
        spacing = try? c.decodeIfPresent(String.self, forKey: .spacing)
        children = (try? c.decode(Lossy<Component>.self, forKey: .children).wrappedValue) ?? []
        text = try? c.decodeIfPresent(String.self, forKey: .text)
        style = try? c.decodeIfPresent(String.self, forKey: .style)
        align = try? c.decodeIfPresent(String.self, forKey: .align)
        label = try? c.decodeIfPresent(String.self, forKey: .label)
        icon = try? c.decodeIfPresent(String.self, forKey: .icon)
        action = try? c.decodeIfPresent(Action.self, forKey: .action)
        field = try? c.decodeIfPresent(String.self, forKey: .field)
        placeholder = try? c.decodeIfPresent(String.self, forKey: .placeholder)
        multiline = try? c.decodeIfPresent(Bool.self, forKey: .multiline)
        unit = try? c.decodeIfPresent(String.self, forKey: .unit)
        includeTime = try? c.decodeIfPresent(Bool.self, forKey: .includeTime)
        source = try? c.decodeIfPresent(String.self, forKey: .source)
        height = try? c.decodeIfPresent(String.self, forKey: .height)
        entity = try? c.decodeIfPresent(String.self, forKey: .entity)
        item = try? c.decodeIfPresent(ListItem.self, forKey: .item)
        sort = try? c.decodeIfPresent(Sort.self, forKey: .sort)
        filter = try? c.decodeIfPresent(Filter.self, forKey: .filter)
        emptyText = try? c.decodeIfPresent(String.self, forKey: .emptyText)
        onItemTap = try? c.decodeIfPresent(Action.self, forKey: .onItemTap)
        swipeToDelete = try? c.decodeIfPresent(Bool.self, forKey: .swipeToDelete)
        aggregate = try? c.decodeIfPresent(String.self, forKey: .aggregate)
        format = try? c.decodeIfPresent(String.self, forKey: .format)
        kind = try? c.decodeIfPresent(String.self, forKey: .kind)
        groupBy = try? c.decodeIfPresent(String.self, forKey: .groupBy)
        bucket = try? c.decodeIfPresent(String.self, forKey: .bucket)
        prompt = try? c.decodeIfPresent(String.self, forKey: .prompt)
        size = try? c.decodeIfPresent(String.self, forKey: .size)
    }
}

/// list の行テンプレート。値は全部 field id。
struct ListItem: Decodable, Sendable {
    var title: String
    var subtitle: String?
    var trailing: String?
    var icon: String?
    /// photo field id → サムネイル
    var image: String?
}

struct Sort: Decodable, Sendable {
    var field: String
    var direction: String  // asc|desc
}

struct Filter: Decodable, Sendable {
    var field: String
    var op: String          // eq|neq|gt|gte|lt|lte|thisMonth|today
    var value: JSONValue?
}

/// `area` の解析結果。列は 0 始まり(A=0)、行は 1 始まり、両端含む。
/// 画面は横 6 列(A-F)× 縦 N 行(1 行 = 可視高さ ÷ 12、13 行目以降は画面全体がスクロール)。
struct GridArea: Equatable, Sendable {
    static let columns = 6
    static let visibleRows = 12

    var col1: Int, row1: Int, col2: Int, row2: Int

    init?(_ s: String) {
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

    var colSpan: Int { col2 - col1 + 1 }
    var rowSpan: Int { row2 - row1 + 1 }

    func overlaps(_ o: GridArea) -> Bool {
        col1 <= o.col2 && o.col1 <= col2 && row1 <= o.row2 && o.row1 <= row2
    }
}
