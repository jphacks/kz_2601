import Foundation

/// コンポーネント種別(schema `type`)。未知は `.unknown` に丸めてプレースホルダ描画する。
/// 新しい種別を足す時はここに case を追加し、Renderer 側で `ComponentRegistry` に View を1つ登録する。
public enum ComponentType: String, Decodable, Sendable, CaseIterable {
    // container(children は Leaf のみ・1段)
    case vstack, hstack, section
    // leaf
    case text, button, textField, numberField, datePicker, picker, toggle
    case photoField, image, list, stat, chart, voiceInput, spacer, divider
    case unknown

    public init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .unknown) }

    public var isContainer: Bool { self == .vstack || self == .hstack || self == .section }
}

/// 全コンポーネントの props をフラットに持つ1ノード(Go の `dsl.Component` と 1:1)。
/// どの props が意味を持つかは `type` 次第(docs/dsl/README.md のコンポーネント表)。
public struct Component: Decodable, Identifiable, Sendable {
    public let id: String
    public var type: ComponentType
    /// デコード時の生の type 文字列(未知 type のプレースホルダ表示用・レジストリのキー)。
    public var rawType: String
    /// 画面グリッド上の矩形 "A1:C3"(Screen 直下のみ有効)。nil = グリッドの下に積む。
    public var area: String?
    public var gridArea: GridArea? { area.flatMap(GridArea.init) }

    // container
    public var title: String?          // section / chart
    public var spacing: String?        // none|small|medium|large
    public var children: [Component]

    // text / button / inputs
    public var text: String?
    public var style: String?          // text: title|headline|body|caption / button: primary|secondary|destructive|plain / picker: menu|segmented|wheel
    public var align: String?          // leading|center|trailing
    public var label: String?
    public var icon: String?
    public var action: Action?
    public var field: String?          // 入力の束縛先 field id / stat・chart の値 field
    public var placeholder: String?
    public var multiline: Bool?
    public var unit: String?
    public var includeTime: Bool?
    public var source: String?         // photoField: camera|library|both
    public var height: String?         // image: small|medium|large

    // data-bound(list / stat / chart)
    public var entity: String?
    public var item: ListItem?
    public var sort: Sort?
    public var filter: Filter?
    public var emptyText: String?
    public var onItemTap: Action?
    public var swipeToDelete: Bool?
    public var aggregate: String?      // count|sum|avg|min|max
    public var format: String?         // plain|currency|percent
    public var kind: String?           // bar|line|pie
    public var groupBy: String?
    public var bucket: String?         // day|week|month

    // voiceInput / spacer
    public var prompt: String?
    public var size: String?           // voiceInput: compact|large / spacer: small|medium|large|flexible

    private enum Keys: String, CodingKey {
        case id, type, area, title, spacing, children, text, style, align, label, icon, action, field, placeholder, multiline, unit
        case includeTime, source, height, entity, item, sort, filter, emptyText, onItemTap, swipeToDelete, aggregate, format
        case kind, groupBy, bucket, prompt, size
    }

    public init(from decoder: Decoder) throws {
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
public struct ListItem: Decodable, Sendable {
    public var title: String
    public var subtitle: String?
    public var trailing: String?
    public var icon: String?
    /// photo field id → サムネイル
    public var image: String?
}

public struct Sort: Decodable, Sendable {
    public var field: String
    public var direction: String  // asc|desc

    public init(field: String, direction: String) {
        self.field = field
        self.direction = direction
    }
}

public struct Filter: Decodable, Sendable {
    public var field: String
    public var op: String          // eq|neq|gt|gte|lt|lte|thisMonth|today
    public var value: JSONValue?

    public init(field: String, op: String, value: JSONValue? = nil) {
        self.field = field
        self.op = op
        self.value = value
    }
}
