import Foundation

/// コンポーネント種別(schema `type`)。未知は `.unknown` に丸めてプレースホルダ描画する。
/// 新しい種別を足す時はここに case を追加し、Renderer 側で `ComponentRegistry` に View を1つ登録する。
public enum ComponentType: String, Decodable, Sendable, CaseIterable {
    // container(children は Leaf のみ・1段)
    case vstack, hstack, section
    // leaf
    case text, button, textField, numberField, datePicker, picker, toggle
    case photoField, image, list, stat, chart, voiceInput, spacer, divider
    // container(構造系: card/disclosure/carousel/grid)
    case card, disclosure, carousel, grid
    // leaf(ナビ・ネイティブ系)
    case searchBar, calendar, shareButton, openUrlButton, mapView, locationButton, qrScanner, timerDisplay
    case webView
    // leaf: 入力系バッチ
    case slider, stepper, textEditor, secureField, timePicker, colorPicker, rating
    // leaf(表示系)
    case label
    case icon
    case progressBar
    case gauge
    case badge
    case link
    case markdown
    case avatar
    case table
    case timeline
    case emptyState
    // leaf(計算・#101)
    case settlement
    case unknown

    public init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .unknown) }

    public var isContainer: Bool {
        switch self {
        case .vstack, .hstack, .section, .card, .disclosure, .carousel, .grid: true
        default: false
        }
    }
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
    /// `filter` が配列(AND)で来た時も含む全条件。単体なら [filter]。
    public var filters: [Filter]
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
    public var size: String?           // voiceInput: compact|large / spacer: small|medium|large|flexible / icon・avatar: small|medium|large

    // 表示系(label / icon / progressBar / gauge / badge / link / markdown / avatar / table / timeline / emptyState)
    public var name: String?           // icon: SF Symbols 名 / avatar: イニシャル元
    public var color: String?          // SemanticColor: primary|secondary|accent|red|green|orange|blue
    public var value: Double?          // progressBar(0..1)/ gauge のリテラル値
    public var valueExpr: Expr?        // stat: `value` に Expr(aggregate の代替・#101)。数値リテラルも入る
    // settlement(#101)
    public var nameField: String?
    public var balanceField: String?
    public var target: Double?         // progressBar entity モードの分母
    public var url: String?            // link
    public var image: String?          // avatar リテラル(https / data URL)
    public var message: String?        // emptyState
    public var columns: [TableColumnSpec]? // table
    public var limit: Int?             // table / timeline
    public var dateField: String?      // timeline
    public var titleField: String?     // timeline
    public var subtitleField: String?  // timeline

    // 構造系 container
    public var expanded: Bool?         // disclosure: 初期展開
    public var itemWidth: String?      // carousel: small|medium|large
    public var columnCount: Int?       // grid: 2..4(`columns` は table の列定義)

    // ナビ・ネイティブ系 leaf(url / dateField / titleField は link / timeline と共用)
    public var searchField: String?    // list: 同画面の searchBar.field
    public var onDateTap: Action?      // calendar
    public var locationField: String?  // mapView
    public var zoom: String?           // mapView: near|city|wide
    public var mode: String?           // timerDisplay: stopwatch|countdown
    public var seconds: Int?           // timerDisplay countdown 初期秒
    public var shareTo: String?        // shareButton: any|line|slack|discord(`target` は progressBar の数値)
    // 入力系バッチ(slider / stepper / rating / textEditor)
    public var min: Double?            // slider / stepper / gauge
    public var max: Double?            // rating: 星の数(既定 5) / gauge
    public var step: Double?           // slider: 省略で連続 / stepper: 既定 1
    public var minLines: Int?          // textEditor: 最低表示行数(既定 4)

    private enum Keys: String, CodingKey {
        case id, type, area, title, spacing, children, text, style, align, label, icon, action, field, placeholder, multiline, unit
        case includeTime, source, height, entity, item, sort, filter, emptyText, onItemTap, swipeToDelete, aggregate, format
        case kind, groupBy, bucket, prompt, size
        case expanded, itemWidth, columnCount
        case searchField, onDateTap, locationField, zoom, mode, seconds, shareTo
        case name, color, value, target, url, image, message, columns, limit, dateField, titleField, subtitleField
        case min, max, step, minLines
        case nameField, balanceField
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
        filters = Filter.decodeOneOrMany(c, forKey: .filter)
        filter = filters.first
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
        expanded = try? c.decodeIfPresent(Bool.self, forKey: .expanded)
        itemWidth = try? c.decodeIfPresent(String.self, forKey: .itemWidth)
        columnCount = try? c.decodeIfPresent(Int.self, forKey: .columnCount)
        searchField = try? c.decodeIfPresent(String.self, forKey: .searchField)
        onDateTap = try? c.decodeIfPresent(Action.self, forKey: .onDateTap)
        locationField = try? c.decodeIfPresent(String.self, forKey: .locationField)
        zoom = try? c.decodeIfPresent(String.self, forKey: .zoom)
        mode = try? c.decodeIfPresent(String.self, forKey: .mode)
        seconds = try? c.decodeIfPresent(Int.self, forKey: .seconds)
        shareTo = try? c.decodeIfPresent(String.self, forKey: .shareTo)
        min = try? c.decodeIfPresent(Double.self, forKey: .min)
        max = try? c.decodeIfPresent(Double.self, forKey: .max)
        step = try? c.decodeIfPresent(Double.self, forKey: .step)
        minLines = try? c.decodeIfPresent(Int.self, forKey: .minLines)
        name = try? c.decodeIfPresent(String.self, forKey: .name)
        color = try? c.decodeIfPresent(String.self, forKey: .color)
        value = try? c.decodeIfPresent(Double.self, forKey: .value)
        valueExpr = try? c.decodeIfPresent(Expr.self, forKey: .value)
        nameField = try? c.decodeIfPresent(String.self, forKey: .nameField)
        balanceField = try? c.decodeIfPresent(String.self, forKey: .balanceField)
        target = try? c.decodeIfPresent(Double.self, forKey: .target)
        url = try? c.decodeIfPresent(String.self, forKey: .url)
        image = try? c.decodeIfPresent(String.self, forKey: .image)
        message = try? c.decodeIfPresent(String.self, forKey: .message)
        columns = try? c.decode(Lossy<TableColumnSpec>.self, forKey: .columns).wrappedValue
        limit = try? c.decodeIfPresent(Int.self, forKey: .limit)
        dateField = try? c.decodeIfPresent(String.self, forKey: .dateField)
        titleField = try? c.decodeIfPresent(String.self, forKey: .titleField)
        subtitleField = try? c.decodeIfPresent(String.self, forKey: .subtitleField)
    }
}

/// `table` の1列。`title` 省略時は Field.name を見出しにする。
public struct TableColumnSpec: Decodable, Sendable {
    public var field: String
    public var title: String?

    public init(field: String, title: String? = nil) {
        self.field = field
        self.title = title
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
    /// boolean field id → 行頭のチェック丸。タップでそのレコードの値を反転して保存(#108・フォームを開かない)
    public var toggle: String?

    public init(title: String, subtitle: String? = nil, trailing: String? = nil, icon: String? = nil, image: String? = nil, toggle: String? = nil) {
        self.title = title
        self.subtitle = subtitle
        self.trailing = trailing
        self.icon = icon
        self.image = image
        self.toggle = toggle
    }
}

public struct Sort: Decodable, Sendable {
    public var field: String
    public var direction: String  // asc|desc

    public init(field: String, direction: String) {
        self.field = field
        self.direction = direction
    }
}

/// list / stat / chart / agg の絞り込み。`value` はリテラル(後方互換)か Expr(#101: 例 `{"ref":"id"}`)。
/// Expr の場合は `value` が nil で `valueExpr` に入り、`ExprEvaluator.resolve` が外側の行コンテキストで評価してから照合する。
public struct Filter: Codable, Hashable, Sendable {
    public var field: String
    public var op: String          // eq|neq|gt|gte|lt|lte|today|thisWeek|thisMonth|thisYear
    public var value: JSONValue?
    public var valueExpr: Expr?

    public init(field: String, op: String, value: JSONValue? = nil) {
        self.field = field
        self.op = op
        self.value = value
    }

    public init(field: String, op: String, valueExpr: Expr) {
        self.field = field
        self.op = op
        if let lit = valueExpr.literalValue { value = lit } else { self.valueExpr = valueExpr }
    }

    private enum Keys: String, CodingKey { case field, op, value }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        field = try c.decode(String.self, forKey: .field)
        op = try c.decode(String.self, forKey: .op)
        if let e = try c.decodeIfPresent(Expr.self, forKey: .value) {
            if let lit = e.literalValue { value = lit } else { valueExpr = e }
        }
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: Keys.self)
        try c.encode(field, forKey: .field)
        try c.encode(op, forKey: .op)
        if let valueExpr { try c.encode(valueExpr, forKey: .value) } else { try c.encode(value ?? .null, forKey: .value) }
    }

    /// `filter` キーが単体 or 配列(AND)のどちらでも読む。壊れていれば空(=絞り込み無し)。
    static func decodeOneOrMany<K: CodingKey>(_ c: KeyedDecodingContainer<K>, forKey key: K) -> [Filter] {
        if let one = try? c.decodeIfPresent(Filter.self, forKey: key) { return [one] }
        return (try? c.decodeIfPresent(Lossy<Filter>.self, forKey: key))?.wrappedValue ?? []
    }
}
