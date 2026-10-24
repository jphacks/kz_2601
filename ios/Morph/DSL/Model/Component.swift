import Foundation

/// DSL ツリーの1ノード = 共通メタ(id/visible/padding) + 種別ごとの props。
struct Node: Decodable, Identifiable, Sendable {
    let id: String
    /// `true/false` または式 `"{{state.flag}}"`。nil は常に表示。
    let visible: JSONValue?
    let padding: Double?
    let component: Component

    private enum Keys: String, CodingKey { case id, visible, padding }

    init(id: String = UUID().uuidString, visible: JSONValue? = nil, padding: Double? = nil, component: Component) {
        self.id = id
        self.visible = visible
        self.padding = padding
        self.component = component
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? UUID().uuidString
        visible = try c.decodeIfPresent(JSONValue.self, forKey: .visible)
        padding = try c.decodeIfPresent(Double.self, forKey: .padding)
        component = try Component(from: decoder)
    }
}

/// `type` で分岐するポリモーフィック表現。
/// 未知 type は `.unknown`、既知 type の props 不正は `.invalid` として描画側で安全に無視する。
indirect enum Component: Sendable {
    case text(TextProps)
    case image(ImageProps)
    case button(ButtonProps)
    case textField(TextFieldProps)
    case toggle(ToggleProps)
    case picker(PickerProps)
    case stack(StackProps)
    case list(ListProps)
    case card(CardProps)
    case section(SectionProps)
    case spacer(SpacerProps)
    case divider
    case progress(ProgressProps)
    case map(MapProps)
    case camera(CameraProps)
    case aiVoice(AIVoiceProps)
    case unknown(type: String)
    case invalid(type: String, reason: String)
}

extension Component: Decodable {
    private enum Keys: String, CodingKey { case type }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        let type = try c.decode(String.self, forKey: .type)
        do {
            switch type {
            case "text": self = .text(try TextProps(from: decoder))
            case "image": self = .image(try ImageProps(from: decoder))
            case "button": self = .button(try ButtonProps(from: decoder))
            case "textField": self = .textField(try TextFieldProps(from: decoder))
            case "toggle": self = .toggle(try ToggleProps(from: decoder))
            case "picker": self = .picker(try PickerProps(from: decoder))
            case "vstack", "hstack", "zstack": self = .stack(try StackProps(from: decoder))
            case "list": self = .list(try ListProps(from: decoder))
            case "card": self = .card(try CardProps(from: decoder))
            case "section": self = .section(try SectionProps(from: decoder))
            case "spacer": self = .spacer(try SpacerProps(from: decoder))
            case "divider": self = .divider
            case "progress": self = .progress(try ProgressProps(from: decoder))
            case "map": self = .map(try MapProps(from: decoder))
            case "camera": self = .camera(try CameraProps(from: decoder))
            case "aiVoice": self = .aiVoice(try AIVoiceProps(from: decoder))
            default: self = .unknown(type: type)
            }
        } catch {
            self = .invalid(type: type, reason: String(describing: error))
        }
    }
}

// MARK: - Props

enum TextStyle: String, Decodable, Sendable {
    case largeTitle, title, title2, headline, body, caption, footnote
    init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .body) }
}

enum TextAlign: String, Decodable, Sendable {
    case leading, center, trailing
    init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .leading) }
}

struct TextProps: Decodable, Sendable {
    var text: String
    var style: TextStyle?
    var color: String?
    var align: TextAlign?
    var bold: Bool?
}

struct ImageProps: Decodable, Sendable {
    var url: String?
    var systemName: String?
    var height: Double?
    var cornerRadius: Double?
}

enum ButtonKind: String, Decodable, Sendable {
    case primary, secondary, destructive, plain
    init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .primary) }
}

struct ButtonProps: Decodable, Sendable {
    var label: String?
    var icon: String?
    var style: ButtonKind?
    var action: Action?
    var disabled: JSONValue?
}

struct TextFieldProps: Decodable, Sendable {
    var bind: String
    var placeholder: String?
    var keyboard: String?
    var secure: Bool?
    var multiline: Bool?
}

struct ToggleProps: Decodable, Sendable {
    var bind: String
    var label: String
}

struct PickerOption: Decodable, Hashable, Sendable {
    var label: String
    var value: JSONValue
}

struct PickerProps: Decodable, Sendable {
    var bind: String
    var label: String?
    var options: [PickerOption]
}

enum StackAxis: String, Decodable, Sendable {
    case vstack, hstack, zstack
    init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .vstack) }
}

struct StackProps: Decodable, Sendable {
    var axis: StackAxis
    var spacing: Double?
    var alignment: String?
    @Lossy var children: [Node]

    private enum CodingKeys: String, CodingKey {
        case axis = "type", spacing, alignment, children
    }
}

struct ListProps: Decodable, Sendable {
    /// 配列を返す式。例 `"{{state.todos}}"`
    var source: String
    /// 行テンプレート。中では `{{item.*}}` / `{{index}}` が使える。
    var item: Node
    var emptyText: String?
    var onTap: Action?
    var onDelete: Action?
}

struct CardProps: Decodable, Sendable {
    var title: String?
    var action: Action?
    @Lossy var children: [Node]
}

struct SectionProps: Decodable, Sendable {
    var title: String?
    var footer: String?
    @Lossy var children: [Node]
}

struct SpacerProps: Decodable, Sendable {
    var minLength: Double?
}

struct ProgressProps: Decodable, Sendable {
    /// 0...1 の数値または式。nil は不定(スピナー)。
    var value: JSONValue?
    var label: String?
}

struct MapPin: Decodable, Identifiable, Sendable {
    var id: String { "\(lat),\(lng)" }
    var lat: Double
    var lng: Double
    var title: String?
}

struct MapProps: Decodable, Sendable {
    var lat: Double
    var lng: Double
    var span: Double?
    @Lossy var pins: [MapPin]
}

struct CameraProps: Decodable, Sendable {
    /// 撮影/選択した画像(base64 JPEG)を書き込む state パス
    var bind: String
    var label: String?
}

struct AIVoiceProps: Decodable, Sendable {
    var prompt: String?
}
