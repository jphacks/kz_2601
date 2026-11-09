import Foundation

/// コンポーネント種別(schema `type`)。未知は `.unknown` に丸めてプレースホルダ描画する。
/// 新しい種別を足す時はここに case を追加し、Renderer 側で `ComponentRegistry` に View を1つ登録する。
public enum ComponentType: String, Decodable, Sendable, CaseIterable {
    // container(children は Leaf のみ・1段)
    case vstack, hstack, section
    // leaf
    case text, button, textField, numberField, datePicker, picker, toggle
    case photoField, image, list, stat, chart, voiceInput, spacer, divider
    // leaf(動画・#206)
    case videoField, videoPlayer
    // leaf(録音・#420)
    case audioRecorder
    // leaf(文字起こし・読み上げ。端末の中だけ)。transcribeField: field / label / placeholder、speakButton: text / field / value / label は共用
    case transcribeField, speakButton
    // leaf(手書き・お絵かき。PencilKit → photo field)
    case drawingField
    // container(構造系: card/disclosure/carousel/grid)
    case card, disclosure, carousel, grid
    // leaf(ナビ・ネイティブ系)
    case searchBar, calendar, shareButton, openUrlButton, mapView, locationButton, locationPicker, qrScanner, timerDisplay
    case webView
    // leaf(QR を作って表示・#422)。text / field / value / label / title は共用
    case qrCode
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
    // leaf(オンデバイス AI・#78)
    case aiButton
    // leaf(近距離共有・#36)
    case nearbyShare
    // leaf(位置の記録・#168)
    case locationTracker
    // leaf(チャット・#260)
    case chatThread
    // leaf(クラウドの AI と何往復も話す)。entity / textField / length / placeholder / emptyText は共用
    case aiChat
    // leaf(ヘルスデータ・#167)
    case healthStat
    // leaf(時間割のマス目・#234)
    case gridTable
    // leaf(ランダムに 1 件選ぶ・#263)。entity / title / titleField / subtitleField / filter / label / onItemTap は共用
    case randomPicker
    // leaf(トーナメント表・BracketDraw)。entity / title / titleField は共用
    case bracket
    // leaf(ファイルの取り込み: CSV の行 / テキスト・PDF の本文を記録に。端末内だけ)。entity / field / label は共用
    case fileImport
    // leaf(目的地までの道案内・#411)。title / label / entity / locationField / titleField / mode は共用
    case routeGuide
    // leaf(近くの場所を Apple のマップで探してフォームに入れる)。label / nameField / locationField は共用
    case placeSearch
    // leaf(ドラレコ・#410)。title は共用
    case dashcam
    // leaf(端末のセンサー・#419)。label / style / min / max / entity / field / dateField / seconds / limit は共用
    case sensorValue
    // leaf(Apple Music の再生と友だちからの曲リクエスト・#412)。title は共用
    case musicPlayer
    // leaf(仲間探しの届いた人・送った人・つながった人・#507)。entity / title / emptyText / item / onItemTap は共用
    case matchList
    // leaf(単語カード・#264)。entity / filter は共用
    case flashcards
    // leaf(外部の公開データ・#348)。title / limit / emptyText は共用。appconfig が許可リストの出どころだけから取る
    case feed
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
    /// 読む・描く入れ子の段数の上限(スキーマは 2 段)。これより深い `children` は読まない(空の枠)。
    /// 1 段のデコードが大きなスタックを使い、非同期処理のスレッド(512KB)では約 20 段(Debug は約 10 段)で落ちるため。
    public static let maxNestingDepth = 8

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
    public var source: String?         // photoField / videoField: camera|library|both
    public var maxDuration: Int?       // videoField: 録画の上限秒(既定 60)
    public var height: String?         // image: small|medium|large
    /// button: "admins" = 管理者にだけ出す(#272)。表示だけで、守るのはサーバー(editableBy / deleteBy)。
    public var visibleTo: String?
    /// text: 文中の語に印をつける(TextHighlight.swift)。
    public var highlight: TextHighlight?

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
    public var aggregate: String?      // count|sum|avg|min|max(stat は latest|earliest も)
    public var by: String?             // stat latest/earliest: 並べる日付 field(#133)
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
    public var nameField: String?      // settlement / locationPicker(場所の名前の string field・#212)
    public var balanceField: String?
    // aiButton(#78)。label / prompt / entity / limit は共用
    public var task: String?           // summarize|classify|rewrite|extract|suggest|generate
    public var input: String?          // AI に渡す field(nil = フォーム全体)
    public var output: String?         // 結果を入れる field
    public var options: [String]?      // classify の候補(nil = enum field の options)
    public var shape: String?          // generate(#417): line|list|fields
    public var count: Int?             // generate list の個数
    public var items: [String]?        // generate fields の項目名
    public var length: String?         // short(既定)| long = 長い文・オンライン AI だけ
    public var inputPhoto: String?     // 一緒に AI に見せる photo field(オンライン AI だけ)
    public var saveTo: AISaveTo?       // generate list: 1 項目 = 1 件の記録(AISaveTo.swift)
    public var chatSaveTo: AIChatSaveTo? // aiChat の saveTo {entity, fields}: 話しかけて記録する(AIChatSaveTo.swift)
    public var inputFeeds: [String]?   // generate: 同じ画面の feed の id(行を材料に足す・AIFeedInput.swift)
    public var outputs: [String: String]? // generate fields: 項目名 → フォームの field(AIOutputs.swift)
    public var target: Double?         // progressBar entity モードの分母
    public var targetExpr: Expr?       // progressBar: target が式 {"setting": …} の時(変えられる目標)。数値なら `target`
    public var tone: String?           // progressBar: positive(目標)| negative(予算)・#140
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
    public var locationField: String?  // mapView / locationTracker
    public var zoom: String?           // mapView: near|city|wide(mapView の kind: pins|route・#168)
    public var mode: String?           // timerDisplay: stopwatch|countdown / routeGuide: automobile|walking
    public var address: String?        // routeGuide: 固定の目的地(住所・施設名)
    public var seconds: Int?           // timerDisplay countdown 初期秒
    public var liveActivity: Bool?     // timerDisplay: ロック画面 / Dynamic Island にも出す(#423)
    public var shareTo: String?        // shareButton: any|line|slack|discord(`target` は progressBar の数値)
    // 入力系バッチ(slider / stepper / rating / textEditor)
    public var min: Double?            // slider / stepper / gauge
    public var max: Double?            // rating: 星の数(既定 5) / gauge
    public var step: Double?           // slider: 省略で連続 / stepper: 既定 1
    public var minLines: Int?          // textEditor: 最低表示行数(既定 4)
    // healthStat(#167)。label / target は共用
    public var metric: String?         // steps|weight
    public var period: String?         // today(既定)|week
    // gridTable(#234)。entity / titleField / subtitleField / onItemTap は共用
    public var columnField: String?    // 横軸(例: 曜日 enum)
    public var rowField: String?       // 縦軸(例: 時限 integer)
    public var columnValues: [String]? // 列見出しの明示(nil = enum の options)
    public var rowValues: [String]?    // enum / string の行見出しの明示
    public var rowRange: GridRange?    // integer の行範囲(nil = 1…6)
    // chatThread(#260)。entity / placeholder / emptyText は共用
    public var textField: String?      // 本文の string / text field
    public var photoField: String?     // 任意の photo field
    // aiChat。entity / textField / length / placeholder / emptyText は共用
    public var roleField: String?      // user / assistant を書く field(nil = 自分と AI が交互)
    public var persona: String?        // AI の役と話し方(500 字まで・サーバーへは instruction)
    public var speak: Bool?            // aiChat: 新しい AI の返事を端末の声で自動で読み上げる
    public var inputEntity: String?    // aiChat: 自分の記録を参考として AI に添える entity(limit 件・AIChatMaterial.swift)
    // dashcam(#410)。title は共用。nil = 既定値(DashcamConfig)
    public var keepMinutes: Int?       // ループで残す分数
    public var maxStorageMB: Int?      // 録画の容量上限
    public var impactG: Double?        // 衝撃の閾値(重力を除いた G)
    public var autoStart: [String]?    // landscape|still|charging(全部が続いたら開始)
    public var audio: Bool?            // マイクも録る
    // sensorValue(#419)
    public var sensor: String?         // tilt|pitch|roll|acceleration|rotation|pressure|altitude
    public var threshold: SensorThreshold?
    // photoField(#418): 写真から読み取って他の field に入れる。壊れた項目だけ落とす
    public var analyze: [PhotoAnalyze] = []
    // fileImport
    public var accept: [String] = []       // csv|text|pdf
    public var map: [ImportColumn] = []    // csv: 見出し → field id
    // nearbyShare passby(#425)。entity / mode / limit は共用
    public var fields: [String]?       // 交換する field id(明示したものだけ送る)
    public var distance: Bool?         // UWB で距離・方向を出す
    // musicPlayer(#412)
    public var allowRequests: Bool?    // 近くの端末から曲のリクエストを受ける
    public var autoAccept: Bool?       // リクエストを確認せずに再生待ちへ
    // matchList(#507)
    public var show: String?           // incoming|outgoing|matches
    // flashcards(#264)
    public var front: String?          // 表の field
    public var back: String?           // 裏の field
    public var toggle: String?         // 覚えた / まだ を保存する boolean field
    // bracket。entity / title / titleField は共用
    public var winsField: String?      // 勝った回戦の数を保存する integer field
    public var seedField: String?      // シード順の integer / number field(nil = 登録順)
    // feed(#348)。URL は持たない
    public var provider: String?       // weather|warnings|pressure|holidays|rss|arxiv|diet
    public var region: String?         // weather / warnings / pressure: 気象庁の府県予報区のコード
    public var feedId: String?         // rss: 許可したフィードの id / arxiv: 分野
    // placeSearch。label / nameField / locationField は共用
    public var query: String?          // 決まった検索語(placeSearch・feed の diet)
    public var category: String?       // cafe|restaurant|…(PlaceSearch.categories)
    public var near: String?           // current(既定)
    public var addressField: String?   // placeSearch / locationButton: 住所の string / text field
    public var pins: Bool?             // placeSearch: 結果を小さな地図のピンでも出す

    /// デコードで読むキー全部。スキーマとのずれ検出テスト(#537)が使う。
    static let codingKeyNames = Set(Keys.allCases.map(\.rawValue))

    private enum Keys: String, CodingKey, CaseIterable {
        case id, type, area, title, spacing, children, text, style, align, label, icon, action, field, placeholder, multiline, unit
        case includeTime, source, maxDuration, height, entity, item, sort, filter, emptyText, onItemTap, swipeToDelete, aggregate, format
        case kind, groupBy, bucket, prompt, size
        case expanded, itemWidth, columnCount
        case searchField, onDateTap, locationField, zoom, mode, address, seconds, shareTo
        case liveActivity
        case name, color, value, target, url, image, message, columns, limit, dateField, titleField, subtitleField
        case min, max, step, minLines
        case by
        case nameField, balanceField
        case task, input, output, options, shape, count, items, length, inputPhoto, saveTo, inputFeeds, outputs
        case tone
        case metric, period
        case columnField, rowField, columnValues, rowValues, rowRange
        case textField, photoField, roleField, persona, speak, inputEntity
        case keepMinutes, maxStorageMB, impactG, autoStart, audio, sensor, threshold, analyze, fields, distance, allowRequests, autoAccept
        case accept, map
        case show
        case front, back, toggle
        case winsField, seedField
        case visibleTo, highlight
        case provider, region, feedId
        case query, category, near, addressField, pins
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        rawType = try c.decodeIfPresent(String.self, forKey: .type) ?? ""
        type = ComponentType(rawValue: rawType) ?? .unknown
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? UUID().uuidString
        area = try? c.decodeIfPresent(String.self, forKey: .area)
        title = try? c.decodeIfPresent(String.self, forKey: .title)
        spacing = try? c.decodeIfPresent(String.self, forKey: .spacing)
        let depth = decoder.codingPath.lazy.filter { $0.stringValue == Keys.children.stringValue }.count
        children = depth + 1 < Self.maxNestingDepth ? (try? c.decode(Lossy<Component>.self, forKey: .children).wrappedValue) ?? [] : []
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
        maxDuration = try? c.decodeIfPresent(Int.self, forKey: .maxDuration)
        height = try? c.decodeIfPresent(String.self, forKey: .height)
        visibleTo = try? c.decodeIfPresent(String.self, forKey: .visibleTo)
        highlight = try? c.decodeIfPresent(TextHighlight.self, forKey: .highlight)
        entity = try? c.decodeIfPresent(String.self, forKey: .entity)
        item = try? c.decodeIfPresent(ListItem.self, forKey: .item)
        sort = try? c.decodeIfPresent(Sort.self, forKey: .sort)
        filters = Filter.decodeOneOrMany(c, forKey: .filter)
        filter = filters.first
        emptyText = try? c.decodeIfPresent(String.self, forKey: .emptyText)
        onItemTap = try? c.decodeIfPresent(Action.self, forKey: .onItemTap)
        swipeToDelete = try? c.decodeIfPresent(Bool.self, forKey: .swipeToDelete)
        aggregate = try? c.decodeIfPresent(String.self, forKey: .aggregate)
        by = try? c.decodeIfPresent(String.self, forKey: .by)
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
        address = try? c.decodeIfPresent(String.self, forKey: .address)
        seconds = try? c.decodeIfPresent(Int.self, forKey: .seconds)
        liveActivity = try? c.decodeIfPresent(Bool.self, forKey: .liveActivity)
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
        task = try? c.decodeIfPresent(String.self, forKey: .task)
        input = try? c.decodeIfPresent(String.self, forKey: .input)
        output = try? c.decodeIfPresent(String.self, forKey: .output)
        options = try? c.decodeIfPresent([String].self, forKey: .options)
        shape = try? c.decodeIfPresent(String.self, forKey: .shape)
        count = try? c.decodeIfPresent(Int.self, forKey: .count)
        items = try? c.decodeIfPresent([String].self, forKey: .items)
        length = try? c.decodeIfPresent(String.self, forKey: .length)
        inputPhoto = try? c.decodeIfPresent(String.self, forKey: .inputPhoto)
        saveTo = try? c.decodeIfPresent(AISaveTo.self, forKey: .saveTo)
        chatSaveTo = try? c.decodeIfPresent(AIChatSaveTo.self, forKey: .saveTo)
        inputFeeds = try? c.decodeIfPresent([String].self, forKey: .inputFeeds)
        outputs = try? c.decodeIfPresent([String: String].self, forKey: .outputs)
        target = try? c.decodeIfPresent(Double.self, forKey: .target)
        if target == nil { targetExpr = try? c.decodeIfPresent(Expr.self, forKey: .target) }
        tone = try? c.decodeIfPresent(String.self, forKey: .tone)
        url = try? c.decodeIfPresent(String.self, forKey: .url)
        image = try? c.decodeIfPresent(String.self, forKey: .image)
        message = try? c.decodeIfPresent(String.self, forKey: .message)
        columns = try? c.decode(Lossy<TableColumnSpec>.self, forKey: .columns).wrappedValue
        limit = try? c.decodeIfPresent(Int.self, forKey: .limit)
        dateField = try? c.decodeIfPresent(String.self, forKey: .dateField)
        titleField = try? c.decodeIfPresent(String.self, forKey: .titleField)
        subtitleField = try? c.decodeIfPresent(String.self, forKey: .subtitleField)
        metric = try? c.decodeIfPresent(String.self, forKey: .metric)
        period = try? c.decodeIfPresent(String.self, forKey: .period)
        columnField = try? c.decodeIfPresent(String.self, forKey: .columnField)
        rowField = try? c.decodeIfPresent(String.self, forKey: .rowField)
        columnValues = try? c.decodeIfPresent([String].self, forKey: .columnValues)
        rowValues = try? c.decodeIfPresent([String].self, forKey: .rowValues)
        rowRange = try? c.decodeIfPresent(GridRange.self, forKey: .rowRange)
        textField = try? c.decodeIfPresent(String.self, forKey: .textField)
        photoField = try? c.decodeIfPresent(String.self, forKey: .photoField)
        roleField = try? c.decodeIfPresent(String.self, forKey: .roleField)
        persona = try? c.decodeIfPresent(String.self, forKey: .persona)
        speak = try? c.decodeIfPresent(Bool.self, forKey: .speak)
        inputEntity = try? c.decodeIfPresent(String.self, forKey: .inputEntity)
        keepMinutes = try? c.decodeIfPresent(Int.self, forKey: .keepMinutes)
        maxStorageMB = try? c.decodeIfPresent(Int.self, forKey: .maxStorageMB)
        impactG = try? c.decodeIfPresent(Double.self, forKey: .impactG)
        autoStart = try? c.decodeIfPresent([String].self, forKey: .autoStart)
        audio = try? c.decodeIfPresent(Bool.self, forKey: .audio)
        sensor = try? c.decodeIfPresent(String.self, forKey: .sensor)
        threshold = try? c.decodeIfPresent(SensorThreshold.self, forKey: .threshold)
        analyze = ((try? c.decodeIfPresent(Lossy<PhotoAnalyze>.self, forKey: .analyze))?.wrappedValue ?? []).filter { $0.kind != .unknown }
        accept = (try? c.decodeIfPresent(Lossy<String>.self, forKey: .accept))?.wrappedValue ?? []
        map = (try? c.decodeIfPresent(Lossy<ImportColumn>.self, forKey: .map))?.wrappedValue ?? []
        fields = try? c.decodeIfPresent([String].self, forKey: .fields)
        distance = try? c.decodeIfPresent(Bool.self, forKey: .distance)
        allowRequests = try? c.decodeIfPresent(Bool.self, forKey: .allowRequests)
        autoAccept = try? c.decodeIfPresent(Bool.self, forKey: .autoAccept)
        show = try? c.decodeIfPresent(String.self, forKey: .show)
        front = try? c.decodeIfPresent(String.self, forKey: .front)
        back = try? c.decodeIfPresent(String.self, forKey: .back)
        toggle = try? c.decodeIfPresent(String.self, forKey: .toggle)
        winsField = try? c.decodeIfPresent(String.self, forKey: .winsField)
        seedField = try? c.decodeIfPresent(String.self, forKey: .seedField)
        provider = try? c.decodeIfPresent(String.self, forKey: .provider)
        region = try? c.decodeIfPresent(String.self, forKey: .region)
        feedId = try? c.decodeIfPresent(String.self, forKey: .feedId)
        query = try? c.decodeIfPresent(String.self, forKey: .query)
        category = try? c.decodeIfPresent(String.self, forKey: .category)
        near = try? c.decodeIfPresent(String.self, forKey: .near)
        addressField = try? c.decodeIfPresent(String.self, forKey: .addressField)
        pins = try? c.decodeIfPresent(Bool.self, forKey: .pins)
    }
}

/// `sensorValue.threshold`(#419): above / below を超えた瞬間に 1 回だけ発火。
public struct SensorThreshold: Decodable, Sendable {
    public var above: Double?
    public var below: Double?
    public var message: String?
    public var save: Bool?
    public var action: Action?

    public init(above: Double? = nil, below: Double? = nil, message: String? = nil, save: Bool? = nil, action: Action? = nil) {
        self.above = above
        self.below = below
        self.message = message
        self.save = save
        self.action = action
    }
}

/// `gridTable.rowRange`: integer の行範囲(両端を含む)。
public struct GridRange: Decodable, Sendable, Hashable {
    public var from: Int
    public var to: Int

    public init(from: Int, to: Int) {
        self.from = from
        self.to = to
    }
}

/// `table` の1列。`title` 省略時は Field.name を見出しにする。
public struct TableColumnSpec: Decodable, Sendable {
    public var field: String
    public var title: String?
    /// `table.groupBy` 時の集計(count|sum|avg|min|max・#261)。nil = グループ名の列
    public var aggregate: String?

    public init(field: String, title: String? = nil, aggregate: String? = nil) {
        self.field = field
        self.title = title
        self.aggregate = aggregate
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
    /// "daily" = toggle は date field。押すと今日の日付を保存し、値が今日の間だけ済み(#132)
    public var resets: String?
    /// entity.marks の id → 行の右端にいいね・保存のトグルと件数(#255・1 人 1 回)
    public var mark: String?
    /// true = match "profile" の一覧の他の人の行に「つながりたい」ボタン(#507)
    public var interest: Bool?

    public init(title: String, subtitle: String? = nil, trailing: String? = nil, icon: String? = nil, image: String? = nil, toggle: String? = nil, resets: String? = nil, mark: String? = nil, interest: Bool? = nil) {
        self.title = title
        self.subtitle = subtitle
        self.trailing = trailing
        self.icon = icon
        self.image = image
        self.toggle = toggle
        self.resets = resets
        self.mark = mark
        self.interest = interest
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
        if (try? c.decodeIfPresent([String: Bool].self, forKey: .value)) == ["me": true] {
            value = Self.meValue // 自分(#251)。Expr ではない
        } else if (try? c.decodeIfPresent([String: Bool].self, forKey: .value)) == ["myFollows": true] {
            value = Self.myFollowsValue // フォローしている人(#270)。Expr ではない
        } else if let e = try c.decodeIfPresent(Expr.self, forKey: .value) {
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
