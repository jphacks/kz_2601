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
        if transcript.contains("薬") || transcript.contains("服薬") { return "medicine" }
        if transcript.contains("最初") || transcript.contains("戻") { return "initial" }
        return "kakeibo"
    }

    public func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
        try await Task.sleep(for: delay)
        let name = Self.sampleName(for: transcript)
        var doc = try DSLExamples.document(name, version: currentVersion + 1)
        doc.appId = appId
        // 改変らしい指示(作った後 / 「アプリ」も句点も無い短い指示)は指示をそのまま要約に返す(「これでいく」で何が反映されたか見えるように・#186)。
        // モックは mode を知らないので推定(同梱例を version 0 で起動した直後の改変も拾う)
        let looksLikeEdit = currentVersion > 0 || (!transcript.contains("アプリ") && !transcript.contains("。"))
        let summary = looksLikeEdit && name != "initial" ? "「\(transcript)」を反映しました(モック)" : "「\(doc.dsl.app.name)」を用意しました(モック)"
        return MorphResult(document: doc, summary: summary, mode: "full")
    }

    /// 対話のモック(状態は持たず、iOS が毎回送る全履歴だけで決める)。
    /// - 問いかけ(「?」「できる?」「可能?」「WebView」等)→ answer(アプリは変えない・#186)
    /// - 「見やすく」「おすすめ」→ proposal 2 案
    /// - edit: それ以外の指示(「金額を大きくして」)→ ready(transcript = その指示)
    /// - create: 1 発目が短い(≤10 字)なら 1 問だけ聞く。2 発目以降・go-ahead 語・長い 1 発目は ready。
    public func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
        try await Task.sleep(for: delay / 2)
        let users = messages.filter { $0.role == .user }.map(\.text)
        let turn = users.count
        let last = users.last ?? ""
        let goAhead = Self.isGoAhead(last)
        let requests = users.filter { !Self.isGoAhead($0) && !Self.isQuestion($0) }
        let mode = mode ?? .create
        if Self.isQuestion(last) {
            return ChatTurn(kind: .answer, text: Self.answer(for: last), suggestions: mode == .edit ? ["金額を大きくして", "見やすくして"] : nil,
                            turn: turn, locale: "ja", source: "mock", mode: mode.rawValue)
        }
        if Self.wantsProposal(last) {
            return ChatTurn(kind: .proposal, text: "こんな案はどうでしょう。よければ「これでいく」を押してください。", turn: turn,
                            options: Self.proposals(mode: mode), locale: "ja", source: "mock", mode: mode.rawValue)
        }
        if mode == .edit {
            let instruction = goAhead ? requests.joined(separator: "。") : last
            return ChatTurn(kind: .ready, text: "この内容で変えますね。", turn: turn,
                            transcript: instruction.isEmpty ? last : instruction, locale: "ja", source: goAhead ? "shortcut" : "mock")
        }
        if turn == 1, !goAhead, last.count <= 10 {
            return ChatTurn(kind: .question, text: "写真も一緒に残したいですか?", suggestions: ["写真も残す", "文字だけでOK", "お任せ"],
                            turn: turn, locale: "ja", source: "mock")
        }
        let transcript = requests.isEmpty ? "家計簿アプリを作って。" : requests.joined(separator: "。")
        let name = Self.sampleName(for: transcript)
        let title = ["training": "筋トレ記録", "walk": "散歩記録", "kakeibo": "家計簿", "shopping": "買い物リスト", "medicine": "服薬チェック"][name] ?? "記録アプリ"
        return ChatTurn(kind: .ready, text: "では作りますね。", turn: turn,
                        brief: ChatBrief(name: title, purpose: requests.first ?? title, entities: [], features: requests.dropFirst().map { $0 }),
                        transcript: transcript, locale: "ja", source: goAhead ? "shortcut" : "mock")
    }

    public func resetChat(appId: String) async {}

    static func isQuestion(_ text: String) -> Bool {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if t.hasSuffix("?") || t.hasSuffix("？") { return true }
        return ["できる", "可能", "ですか", "WebView", "どうなって"].contains { t.contains($0) }
    }

    static func wantsProposal(_ text: String) -> Bool {
        ["見やすく", "おすすめ", "オススメ", "案を"].contains { text.contains($0) }
    }

    static func answer(for question: String) -> String {
        if question.contains("WebView") {
            return "いいえ、WebView ではありません。AI が作った画面の設計図(JSON)を、iPhone の SwiftUI 部品でそのまま描いています。"
        }
        if question.contains("大き") {
            return "できます。「金額を大きくして」のように言ってもらえれば、その部分の文字を大きくします。"
        }
        return "はい、できます。どこをどう変えたいか教えてもらえれば、案を出します。"
    }

    static func proposals(mode: ChatMode) -> [ChatOption] {
        switch mode {
        case .edit:
            return [
                ChatOption(id: "opt1", title: "数字を大きく", detail: "金額や合計を大きな太字にして、ひと目で分かるようにします。",
                           instruction: "金額と合計の数字を大きな太字にして、ひと目で分かるようにして"),
                ChatOption(id: "opt2", title: "カードで区切る", detail: "記録を 1 件ずつカードにして、余白を広げます。",
                           instruction: "一覧の記録を 1 件ずつカードにして、行の間の余白を広げて"),
            ]
        case .create:
            return [
                ChatOption(id: "opt1", title: "家計簿", detail: "金額とカテゴリを記録して、今月の合計を見る。",
                           instruction: "家計簿アプリを作って。金額とカテゴリを記録して、今月の合計を見たい。"),
                ChatOption(id: "opt2", title: "筋トレ記録", detail: "種目と回数を記録して、今日の合計を見る。",
                           instruction: "筋トレ記録アプリを作って。種目と回数を記録して、今日の合計を見たい。"),
            ]
        }
    }

    static func isGoAhead(_ text: String) -> Bool {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard t.count <= 14 else { return false }
        return ["作って", "お任せ", "おまかせ", "それでいい", "よろしく"].contains { t.contains($0) }
    }
}
