import Foundation

enum PresentStyle: String, Decodable, Sendable {
    case sheet, fullScreen
    init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .sheet) }
}

/// ユーザー操作・ライフサイクルから実行される副作用。文字列値はすべて `{{...}}` 展開される。
indirect enum Action: Sendable {
    case navigate(screen: String, params: [String: JSONValue])
    case back
    case present(screen: String, params: [String: JSONValue], style: PresentStyle)
    case dismiss
    case setState(path: String, value: JSONValue)
    case entityList(entity: String, into: String, query: JSONValue?)
    case entityCreate(entity: String, data: JSONValue)
    case entityUpdate(entity: String, id: String, data: JSONValue)
    case entityDelete(entity: String, id: String)
    case native(feature: String, args: JSONValue, into: String?)
    case openURL(String)
    case aiPrompt(text: String)
    case sequence([Action])
    case unknown(type: String)
}

extension Action: Decodable {
    private enum Keys: String, CodingKey {
        case type, screen, params, style, path, value, entity, into, query, data, id, feature, args, url, text, actions
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        let type = try c.decode(String.self, forKey: .type)
        let params = try c.decodeIfPresent([String: JSONValue].self, forKey: .params) ?? [:]
        switch type {
        case "navigate":
            self = .navigate(screen: try c.decode(String.self, forKey: .screen), params: params)
        case "back":
            self = .back
        case "present":
            self = .present(screen: try c.decode(String.self, forKey: .screen), params: params,
                            style: try c.decodeIfPresent(PresentStyle.self, forKey: .style) ?? .sheet)
        case "dismiss":
            self = .dismiss
        case "setState":
            self = .setState(path: try c.decode(String.self, forKey: .path),
                             value: try c.decodeIfPresent(JSONValue.self, forKey: .value) ?? .null)
        case "entity.list":
            self = .entityList(entity: try c.decode(String.self, forKey: .entity),
                               into: try c.decode(String.self, forKey: .into),
                               query: try c.decodeIfPresent(JSONValue.self, forKey: .query))
        case "entity.create":
            self = .entityCreate(entity: try c.decode(String.self, forKey: .entity),
                                 data: try c.decodeIfPresent(JSONValue.self, forKey: .data) ?? .object([:]))
        case "entity.update":
            self = .entityUpdate(entity: try c.decode(String.self, forKey: .entity),
                                 id: try c.decode(String.self, forKey: .id),
                                 data: try c.decodeIfPresent(JSONValue.self, forKey: .data) ?? .object([:]))
        case "entity.delete":
            self = .entityDelete(entity: try c.decode(String.self, forKey: .entity),
                                 id: try c.decode(String.self, forKey: .id))
        case "native":
            self = .native(feature: try c.decode(String.self, forKey: .feature),
                           args: try c.decodeIfPresent(JSONValue.self, forKey: .args) ?? .object([:]),
                           into: try c.decodeIfPresent(String.self, forKey: .into))
        case "openURL":
            self = .openURL(try c.decode(String.self, forKey: .url))
        case "ai.prompt":
            self = .aiPrompt(text: try c.decode(String.self, forKey: .text))
        case "sequence":
            self = .sequence(try c.decode(Lossy<Action>.self, forKey: .actions).wrappedValue)
        default:
            self = .unknown(type: type)
        }
    }
}
