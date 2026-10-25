import Foundation
import MetamonDSL

/// `POST /v1/generate` の結果。`document` は適用済みの最新 DSL 封筒、`summary` は Claude が付けた変更要約(UI に表示)。
public struct MorphResult: Sendable {
    public var document: AppDocument
    public var summary: String?
    /// full | patch(サーバーがどう変えたか。診断用)
    public var mode: String?

    public init(document: AppDocument, summary: String? = nil, mode: String? = nil) {
        self.document = document
        self.summary = summary
        self.mode = mode
    }
}

/// ai サービス: 発話テキスト → (Claude が DSL / JSON Patch 生成 → appconfig に適用) → 最新 DSL 封筒 + 要約を返す。
/// 生成前の対話(`chat` / `resetChat`)も同じ ai サービスが持つ。
public protocol AIClient: Sendable {
    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult
    /// `locale`("ja" | "en" | nil=発話から自動判定)付きの生成(#45)。UI 言語を明示した時に生成アプリの文言言語を揃える。
    func morph(transcript: String, appId: String, currentVersion: Int, locale: String?) async throws -> MorphResult
    /// `POST /v1/chat`(#74 / #75)。iOS は手元の全履歴を毎回送る(2 件以上はサーバー側の会話状態を置き換える契約なので、
    /// ai の再起動や 30 分の期限切れでも会話が途切れない)。
    /// `mode`(#184): create = 新規作成の相談 / edit = 作ったアプリの改変・質問。nil は backend が appId の DSL 有無で判定。
    func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn
    /// `DELETE /v1/chat/{appId}`。失敗しても UI は止めない(サーバー側は 30 分で自然に消える)。
    func resetChat(appId: String) async
}

public extension AIClient {
    /// locale を扱わない実装(モック等)は従来の morph に委ねる。
    func morph(transcript: String, appId: String, currentVersion: Int, locale: String?) async throws -> MorphResult {
        try await morph(transcript: transcript, appId: appId, currentVersion: currentVersion)
    }

    /// mode を指定しない対話(backend が appId の DSL 有無で create / edit を判定)。
    func chat(appId: String, messages: [ChatMessage], locale: String?) async throws -> ChatTurn {
        try await chat(appId: appId, messages: messages, locale: locale, mode: nil)
    }
}

/// 対話の種類(#184 会話の取り決め `mode`)。
public enum ChatMode: String, Codable, Sendable {
    /// 初期画面からの新規作成
    case create
    /// 作ったアプリの上で開く音声シート(改変・質問)
    case edit
}

/// iOS が表示できる応答 kind(#184)。リクエストの `capabilities` に載せ、backend は載っていない kind を返さない(古いアプリ互換)。
public enum ChatCapability {
    public static let all = ["answer", "proposal"]
}

// MARK: - 生成前の対話(clarify)

/// `POST /v1/chat` に送る 1 メッセージ(openapi `ChatMessage`)。
public struct ChatMessage: Codable, Equatable, Sendable {
    public enum Role: String, Codable, Sendable { case user, assistant }
    public var role: Role
    public var text: String

    public init(role: Role, text: String) {
        self.role = role
        self.text = text
    }
}

/// 現時点の要件理解(openapi `ChatBrief`)。「この内容で作る」カードの表示に使う(生成には `ChatTurn.transcript`)。
public struct ChatBrief: Codable, Equatable, Sendable {
    public var name: String
    public var purpose: String
    public var entities: [String]
    public var features: [String]
    public var native: [String]

    public init(name: String, purpose: String, entities: [String] = [], features: [String] = [], native: [String] = []) {
        self.name = name
        self.purpose = purpose
        self.entities = entities
        self.features = features
        self.native = native
    }
}

/// proposal の案 1 つ(#184 `options[]`)。「これでいく」で `instruction` を `/v1/generate` に渡す。
public struct ChatOption: Codable, Equatable, Sendable, Identifiable {
    public var id: String
    public var title: String
    public var detail: String
    /// edit なら変更内容、create なら作るアプリの要約
    public var instruction: String
    /// #77: この案が有料機能(Tailor It Pro)を使う。旧 backend は送らない(nil = 無料)
    public var requiresPro: Bool?
    public var paidFeatures: [PaidFeature]?
    /// #77: 有料機能を抜いた指示文(「この機能なしで作る」)
    public var freeInstruction: String?

    public init(id: String, title: String, detail: String = "", instruction: String,
                requiresPro: Bool? = nil, paidFeatures: [PaidFeature]? = nil, freeInstruction: String? = nil) {
        self.id = id
        self.title = title
        self.detail = detail
        self.instruction = instruction
        self.requiresPro = requiresPro
        self.paidFeatures = paidFeatures
        self.freeInstruction = freeInstruction
    }

    enum CodingKeys: String, CodingKey { case id, title, detail, instruction, requiresPro, paidFeatures, freeInstruction }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        title = try c.decode(String.self, forKey: .title)
        detail = try c.decodeIfPresent(String.self, forKey: .detail) ?? ""
        instruction = try c.decodeIfPresent(String.self, forKey: .instruction) ?? title
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? title
        requiresPro = try? c.decodeIfPresent(Bool.self, forKey: .requiresPro)
        paidFeatures = try? c.decodeIfPresent([PaidFeature].self, forKey: .paidFeatures)
        freeInstruction = try? c.decodeIfPresent(String.self, forKey: .freeInstruction)
    }
}

/// 有料機能 1 つ(#77・openapi `PaidFeature`)。一覧はサーバーの `paid_features.json`。
public struct PaidFeature: Codable, Equatable, Sendable, Identifiable {
    /// sharedData など
    public var id: String
    /// カードに出す名前(例: みんなで共有するデータ)
    public var title: String
    /// なぜ有料か 1 文
    public var reason: String

    public init(id: String, title: String, reason: String = "") {
        self.id = id
        self.title = title
        self.reason = reason
    }

    enum CodingKeys: String, CodingKey { case id, title, reason }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? id
        reason = try c.decodeIfPresent(String.self, forKey: .reason) ?? ""
    }
}

/// `POST /v1/chat` の応答(openapi `ChatResponse`・#184 で answer / proposal を追加)。
/// - answer: 質問への答え(アプリは変えない) / question: 聞き返し / proposal: 案 1〜3 / ready: すぐ実行できる(`transcript` を generate へ)
public struct ChatTurn: Codable, Equatable, Sendable {
    public enum Kind: String, Codable, Sendable {
        case answer, question, proposal, ready

        /// 知らない kind(backend が先に増えた等)は answer 扱い = 吹き出しに出すだけ(アプリは変えない・落ちない)。
        public init(from decoder: Decoder) throws {
            let raw = try decoder.singleValueContainer().decode(String.self)
            self = Kind(rawValue: raw) ?? .answer
        }
    }
    public var kind: Kind
    /// answer: 答え / question: 質問文 / proposal: 案の前置き / ready: 「では作りますね。」
    public var text: String
    /// question: チップに出す短い選択肢(≤3) / answer(任意・#185): そのまま言える次の指示
    public var suggestions: [String]?
    /// これまでのユーザー発話数(1 始まり)
    public var turn: Int
    /// ready の時だけ
    public var brief: ChatBrief?
    /// ready の時だけ。`POST /v1/generate` の text にそのまま渡す依頼文
    public var transcript: String?
    /// proposal の時だけ。案 1〜3
    public var options: [ChatOption]?
    /// #220: proposal の案 / question のチップを複数選べる(組み合わせられる時だけ true)。旧 backend は送らない(nil = 1 つ選択)
    public var multiSelect: Bool?
    public var isMultiSelect: Bool { multiSelect == true }
    public var locale: String?
    /// claude | shortcut | forced
    public var source: String?
    /// 実際に使った mode(#185・chat-first のリクエストの時だけ)。edit を送っても未生成なら create。知らない値は nil 扱い
    public var mode: String?
    public var resolvedMode: ChatMode? { mode.flatMap(ChatMode.init(rawValue:)) }
    /// #77: ready の transcript が有料機能(Tailor It Pro)を使う。旧 backend は送らない(nil = 無料)
    public var requiresPro: Bool?
    public var paidFeatures: [PaidFeature]?
    /// #77: 有料機能を抜いた依頼文(「この機能なしで作る」で transcript の代わりに generate へ)
    public var freeTranscript: String?

    public init(kind: Kind, text: String, suggestions: [String]? = nil, turn: Int, brief: ChatBrief? = nil,
                transcript: String? = nil, options: [ChatOption]? = nil, locale: String? = nil, source: String? = nil, mode: String? = nil,
                requiresPro: Bool? = nil, paidFeatures: [PaidFeature]? = nil, freeTranscript: String? = nil, multiSelect: Bool? = nil) {
        self.multiSelect = multiSelect
        self.requiresPro = requiresPro
        self.paidFeatures = paidFeatures
        self.freeTranscript = freeTranscript
        self.kind = kind
        self.text = text
        self.suggestions = suggestions
        self.turn = turn
        self.brief = brief
        self.transcript = transcript
        self.options = options
        self.locale = locale
        self.source = source
        self.mode = mode
    }
}
