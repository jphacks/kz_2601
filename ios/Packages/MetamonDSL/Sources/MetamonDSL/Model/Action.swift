import Foundation

public enum ActionType: String, Decodable, Sendable {
    case navigate, back, saveRecord, deleteRecord, openVoice, openUrl, resetForm
    case scheduleNotification, cancelNotification
    case startActivity, endActivity // #423 ライブアクティビティ
    case unknown

    public init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .unknown) }
}

public enum Presentation: String, Decodable, Sendable {
    case push, sheet, fullScreen
    public init(from decoder: Decoder) throws { self = Lenient.decode(Self.self, from: decoder, fallback: .push) }
}

/// ボタン / ツールバー / list タップから実行される副作用(ネストしない・Go の `dsl.Action` と 1:1)。
public struct Action: Decodable, Sendable {
    public var type: ActionType
    public var rawType: String
    /// navigate: 遷移先 / saveRecord: 保存後の遷移先(省略時は back)
    public var screen: String?
    public var presentation: Presentation?
    /// navigate: 親から子を作る(#256)。遷移先フォームの ref field id。今のレコードの id を入れて新規で開く
    public var prefillRef: String?
    /// saveRecord / deleteRecord の対象 entity
    public var entity: String?
    public var url: String?
    /// 指定時は確認ダイアログを先に出す
    public var confirmMessage: String?
    /// scheduleNotification
    public var notification: NotificationSpec?
    /// cancelNotification
    public var notificationId: String?
    /// startActivity(#423)
    public var activity: ActivitySpec?

    private enum Keys: String, CodingKey {
        case type, screen, presentation, prefillRef, entity, url, confirmMessage, notification, notificationId
        case activity
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        rawType = try c.decodeIfPresent(String.self, forKey: .type) ?? ""
        type = ActionType(rawValue: rawType) ?? .unknown
        screen = try? c.decodeIfPresent(String.self, forKey: .screen)
        presentation = try? c.decodeIfPresent(Presentation.self, forKey: .presentation)
        prefillRef = try? c.decodeIfPresent(String.self, forKey: .prefillRef)
        entity = try? c.decodeIfPresent(String.self, forKey: .entity)
        url = try? c.decodeIfPresent(String.self, forKey: .url)
        confirmMessage = try? c.decodeIfPresent(String.self, forKey: .confirmMessage)
        notification = try? c.decodeIfPresent(NotificationSpec.self, forKey: .notification)
        notificationId = try? c.decodeIfPresent(String.self, forKey: .notificationId)
        activity = try? c.decodeIfPresent(ActivitySpec.self, forKey: .activity)
    }

    /// DSL 外でクライアントが合成するアクション(例: list の長押し削除)。
    public init(type: ActionType, entity: String? = nil, screen: String? = nil,
                presentation: Presentation? = nil, prefillRef: String? = nil) {
        self.type = type
        self.rawType = type.rawValue
        self.entity = entity
        self.screen = screen
        self.presentation = presentation
        self.prefillRef = prefillRef
    }
}

/// `startActivity` の中身(#423)。ロック画面と Dynamic Island に出す「いま◯◯中」。
public struct ActivitySpec: Decodable, Equatable, Sendable {
    public var title: String
    public var subtitle: String?
    /// SF Symbols 名(省略時 timer)
    public var icon: String?
    /// 今から N 分後を終わりに残り時間を数える。nil は経過時間
    public var minutes: Int?
    /// 一緒に出す stat / progressBar の部品 id
    public var stat: String?

    public init(title: String, subtitle: String? = nil, icon: String? = nil, minutes: Int? = nil, stat: String? = nil) {
        self.title = title
        self.subtitle = subtitle
        self.icon = icon
        self.minutes = minutes
        self.stat = stat
    }
}
