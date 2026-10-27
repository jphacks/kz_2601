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
        if transcript.contains("時間割") || transcript.contains("時限") { return "timetable" }
        if transcript.contains("ナビ") || transcript.contains("道案内") || transcript.contains("ドライブ") { return "drive" }
        if transcript.contains("ランダム") || transcript.contains("くじ") || transcript.contains("晩ごはん") { return "dinner" }
        if transcript.contains("最初") || transcript.contains("戻") { return "initial" }
        return "kakeibo"
    }

    public func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
        try await Task.sleep(for: delay)
        if MockPublicData.matches(transcript) { // #253 公開の記録に個人の情報
            return try MockPublicData.morph(transcript: transcript, appId: appId, version: currentVersion + 1)
        }
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
    /// - 「足したい」→ 組み合わせられる proposal 3 案(multiSelect・#220)
    /// - create の 1 発目に「項目」→ 複数当てはまる question(multiSelect・#220)
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
        if Self.wantsMultiProposal(last) {
            return ChatTurn(kind: .proposal, text: "組み合わせて選べます。", turn: turn, options: Self.multiProposals,
                            locale: "ja", source: "mock", mode: mode.rawValue, multiSelect: true)
        }
        if mode == .create, turn == 1, last.contains("項目") {
            return ChatTurn(kind: .question, text: "どの項目を一覧に出しますか?", suggestions: ["金額", "日付", "メモ"],
                            turn: turn, locale: "ja", source: "mock", mode: mode.rawValue, multiSelect: true)
        }
        if let confirm = MockPublicData.confirmTurn(messages: messages, turn: turn, mode: mode) { return confirm } // #253
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
            return Self.markingPaid(ChatTurn(kind: .ready, text: "この内容で変えますね。", turn: turn,
                                             transcript: instruction.isEmpty ? last : instruction, locale: "ja", source: goAhead ? "shortcut" : "mock"))
        }
        if turn == 1, !goAhead, last.count <= 10 {
            return ChatTurn(kind: .question, text: "写真も一緒に残したいですか?", suggestions: ["写真も残す", "文字だけでOK", "お任せ"],
                            turn: turn, locale: "ja", source: "mock")
        }
        let transcript = requests.isEmpty ? "家計簿アプリを作って。" : requests.joined(separator: "。")
        let name = Self.sampleName(for: transcript)
        let title = ["training": "筋トレ記録", "walk": "散歩記録", "kakeibo": "家計簿", "shopping": "買い物リスト", "medicine": "服薬チェック"][name] ?? "記録アプリ"
        return Self.markingPaid(ChatTurn(kind: .ready, text: "では作りますね。", turn: turn,
                                         brief: ChatBrief(name: title, purpose: requests.first ?? title, entities: [], features: requests.dropFirst().map { $0 }),
                                         transcript: transcript, locale: "ja", source: goAhead ? "shortcut" : "mock"))
    }

    /// 有料機能(#77)のモック。backend `paid_features.json` の sharedData の代表的な語だけ見る。
    static let paidKeywords = ["みんなで", "共有", "掲示板", "投稿"]
    static let freeSuffix = "(無料版で作る: 他のユーザーとデータを共有する機能は入れない)"
    public static let sharedDataFeature = PaidFeature(id: "sharedData", title: "みんなで共有するデータ",
                                                      reason: "みんなで書き込むデータはクラウドに保存して全員の端末に同期するため、サーバーの費用がかかります。")

    static func markingPaid(_ turn: ChatTurn) -> ChatTurn {
        guard turn.kind == .ready, let t = turn.transcript, paidKeywords.contains(where: t.contains) else { return turn }
        var out = turn
        out.requiresPro = true
        out.paidFeatures = [sharedDataFeature]
        out.freeTranscript = t + "\n" + freeSuffix
        return out
    }

    public func resetChat(appId: String) async {}

    static func isQuestion(_ text: String) -> Bool {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if t.hasSuffix("?") || t.hasSuffix("？") { return true }
        return ["できる", "可能", "ですか", "WebView", "どうなって"].contains { t.contains($0) }
    }

    static func wantsMultiProposal(_ text: String) -> Bool { text.contains("足したい") }

    /// #220: 足す系で互いに邪魔しない案(複数選んで 1 回で反映できる)
    public static let multiProposals = [
        ChatOption(id: "opt1", title: "グラフを足す", detail: "ホームにカテゴリ別の円グラフを置きます。", instruction: "ホームにカテゴリ別の円グラフを追加して"),
        ChatOption(id: "opt2", title: "予算バーを足す", detail: "今月の予算の進み具合をバーで見せます。", instruction: "ホームに今月の予算の進捗バーを追加して"),
        ChatOption(id: "opt3", title: "並べ替えを足す", detail: "一覧を金額の大きい順に並べ替えられます。", instruction: "一覧を金額で並べ替えられるようにして"),
    ]

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
