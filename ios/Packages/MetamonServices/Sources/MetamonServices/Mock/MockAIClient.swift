import Foundation
import MetamonDSL

/// 発話のキーワードで同梱サンプル DSL(docs/dsl/examples)を返す ai モック。
public struct MockAIClient: AIClient {
    public var delay: Duration

    public init(delay: Duration = .seconds(1.2)) { self.delay = delay }

    public static func sampleName(for transcript: String) -> String {
        if transcript.contains("トレ") || transcript.contains("筋") || transcript.contains("運動") { return "training" }
        if transcript.contains("散歩") || transcript.contains("写真") || transcript.contains("場所") { return "walk" }
        if transcript.contains("買い物") || transcript.contains("チェックリスト") { return "shopping" }
        if transcript.contains("最初") || transcript.contains("戻") { return "initial" }
        return "kakeibo"
    }

    public func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
        try await Task.sleep(for: delay)
        let name = Self.sampleName(for: transcript)
        var doc = try DSLExamples.document(name, version: currentVersion + 1)
        doc.appId = appId
        return MorphResult(document: doc, summary: "「\(doc.dsl.app.name)」を用意しました(モック)", mode: "full")
    }

    /// 対話のモック(状態は持たず、iOS が毎回送る全履歴だけで決める)。
    /// 1 発目が短い(≤10 字)なら 1 問だけ聞く。2 発目以降・go-ahead 語・長い 1 発目は ready。
    public func chat(appId: String, messages: [ChatMessage], locale: String?) async throws -> ChatTurn {
        try await Task.sleep(for: delay / 2)
        let users = messages.filter { $0.role == .user }.map(\.text)
        let turn = users.count
        let last = users.last ?? ""
        let goAhead = Self.isGoAhead(last)
        let requests = users.filter { !Self.isGoAhead($0) }
        if turn == 1, !goAhead, last.count <= 10 {
            return ChatTurn(kind: .question, text: "写真も一緒に残したいですか?", suggestions: ["写真も残す", "文字だけでOK", "お任せ"],
                            turn: turn, locale: "ja", source: "mock")
        }
        let transcript = requests.isEmpty ? "家計簿アプリを作って。" : requests.joined(separator: "。")
        let name = Self.sampleName(for: transcript)
        let title = ["training": "筋トレ記録", "walk": "散歩記録", "kakeibo": "家計簿", "shopping": "買い物リスト"][name] ?? "記録アプリ"
        return ChatTurn(kind: .ready, text: "では作りますね。", turn: turn,
                        brief: ChatBrief(name: title, purpose: requests.first ?? title, entities: [], features: requests.dropFirst().map { $0 }),
                        transcript: transcript, locale: "ja", source: goAhead ? "shortcut" : "mock")
    }

    public func resetChat(appId: String) async {}

    static func isGoAhead(_ text: String) -> Bool {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard t.count <= 14 else { return false }
        return ["作って", "お任せ", "おまかせ", "それでいい", "よろしく"].contains { t.contains($0) }
    }
}
