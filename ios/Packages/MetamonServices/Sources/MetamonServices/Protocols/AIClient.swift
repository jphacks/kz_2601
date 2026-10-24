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
    func chat(appId: String, messages: [ChatMessage], locale: String?) async throws -> ChatTurn
    /// `DELETE /v1/chat/{appId}`。失敗しても UI は止めない(サーバー側は 30 分で自然に消える)。
    func resetChat(appId: String) async
}

public extension AIClient {
    /// locale を扱わない実装(モック等)は従来の morph に委ねる。
    func morph(transcript: String, appId: String, currentVersion: Int, locale: String?) async throws -> MorphResult {
        try await morph(transcript: transcript, appId: appId, currentVersion: currentVersion)
    }
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

/// `POST /v1/chat` の応答(openapi `ChatResponse`)。question = 次に聞くこと / ready = 要件が固まった(`transcript` を `/v1/generate` へ)。
public struct ChatTurn: Codable, Equatable, Sendable {
    public enum Kind: String, Codable, Sendable { case question, ready }
    public var kind: Kind
    /// question: 質問文 / ready: 「では作りますね。」
    public var text: String
    /// question の時だけ。チップに出す短い選択肢(≤3)
    public var suggestions: [String]?
    /// これまでのユーザー発話数(1 始まり)
    public var turn: Int
    /// ready の時だけ
    public var brief: ChatBrief?
    /// ready の時だけ。`POST /v1/generate` の text にそのまま渡す依頼文
    public var transcript: String?
    public var locale: String?
    /// claude | shortcut | forced
    public var source: String?

    public init(kind: Kind, text: String, suggestions: [String]? = nil, turn: Int, brief: ChatBrief? = nil,
                transcript: String? = nil, locale: String? = nil, source: String? = nil) {
        self.kind = kind
        self.text = text
        self.suggestions = suggestions
        self.turn = turn
        self.brief = brief
        self.transcript = transcript
        self.locale = locale
        self.source = source
    }
}
