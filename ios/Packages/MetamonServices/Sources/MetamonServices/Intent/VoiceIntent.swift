import Foundation

/// ai に投げる前に iOS 側で拾う発話の意図(G11 / 新規作成 / 複数アプリ切替 #51)。
/// - `.undo`: 「元に戻して」「戻して」「取り消して」「アンドゥ」→ appconfig の `versions/{n}/restore`(ai を呼ばない)
/// - `.newApp`: 発話頭の「別のアプリ」「新しいアプリ」→ appId を採番し直してから ai へ(前のアプリはサーバーに残る)
/// - `.switchApp(name)`: 「家計簿に切り替えて」「家計簿を開いて」→ `GET /apps` の name と照合して切替。見つからない/曖昧なら変身一覧シート
/// - `.listApps`: 「アプリ一覧」「変身一覧」「別のアプリに切り替えて」(名前なし)→ 変身一覧シートを開く
/// - `.morph`: それ以外は従来どおり ai へ
public enum VoiceIntent: Equatable, Sendable {
    case undo
    case newApp
    case switchApp(name: String)
    case listApps
    case morph

    static let undoPrefixes = ["元に戻", "もとに戻", "取り消", "とりけし", "アンドゥ", "あんどぅ", "undo", "ひとつ前", "一つ前", "さっきの状態", "前の状態"]
    /// 「戻して」単独や「前に戻して」のような短い発話だけ undo 扱い(「ボタンを右に戻して」のような編集指示を拾わない)
    static let shortUndoWords = ["戻して", "戻す", "もどして", "もどす"]
    static let shortUndoMaxLength = 8
    static let newAppPrefixes = ["別のアプリ", "別アプリ", "新しいアプリ", "新規アプリ", "違うアプリ", "次のアプリ", "べつのアプリ", "あたらしいアプリ"]

    /// 「アプリ一覧」「変身の一覧」「アプリリスト」(主語 × 「の」有無 × 一覧語 の組み合わせ)
    static let listAppsWords: [String] = {
        let subjects = ["アプリ", "変身", "へんしん"]
        let lists = ["一覧", "いちらん", "リスト", "list"]
        return subjects.flatMap { s in lists.flatMap { l in [s + l, s + "の" + l] } }
    }()
    /// 「<名前>に切り替えて」「<名前>を開いて」「<名前>に変身して」。名前 = 最初に現れた切替語より前の部分
    static let switchWords = ["に切り替え", "に切替", "にきりかえ", "を切り替え", "を切替", "をきりかえ", "へ切り替え", "へ切替",
                              "を開い", "をひらい", "を開け", "をひらけ", "に変身", "にへんしん"]
    /// 名前の末尾から落とす語(「家計簿アプリに切り替えて」→「家計簿」)
    static let nameSuffixes = ["のアプリ", "アプリ", "の方", "のほう", "の"]
    /// 名前が無い・総称だけ(「別のアプリに切り替えて」)→ 一覧を開く
    static let genericNames = ["", "アプリ", "別", "別の", "べつの", "他", "他の", "ほか", "ほかの", "違う", "ちがう", "次", "次の", "つぎの", "前", "前の", "まえの", "どれか", "何か", "なにか"]
    /// 「新しいアプリに切り替えて」は新規作成
    static let freshNames = ["新しい", "あたらしい", "新規", "新しく"]

    public static func detect(_ transcript: String) -> VoiceIntent {
        let text = normalize(transcript)
        guard !text.isEmpty else { return .morph }
        if undoPrefixes.contains(where: { text.contains($0) }) { return .undo }
        if text.count <= shortUndoMaxLength, shortUndoWords.contains(where: { text.contains($0) }) { return .undo }
        if listAppsWords.contains(where: { text.contains($0) }) { return .listApps }
        if let raw = switchTarget(in: text) {
            let name = cleanAppName(raw)
            if freshNames.contains(name) { return .newApp }
            if genericNames.contains(name) { return .listApps }
            return .switchApp(name: name)
        }
        if newAppPrefixes.contains(where: { text.hasPrefix($0) }) { return .newApp }
        return .morph
    }

    /// 切替語(`switchWords`)のうち最も手前に現れたものより前の部分。切替語が無ければ nil。
    static func switchTarget(in text: String) -> String? {
        var best: Range<String.Index>?
        for word in switchWords {
            guard let r = text.range(of: word) else { continue }
            if best == nil || r.lowerBound < best!.lowerBound { best = r }
        }
        guard let best else { return nil }
        return String(text[text.startIndex..<best.lowerBound])
    }

    /// 「家計簿アプリ」「家計簿の方」→「家計簿」。先頭の「じゃあ」「次は」などの接続語も落とす。
    static func cleanAppName(_ raw: String) -> String {
        var s = raw
        for lead in ["じゃあ", "じゃ", "では", "次は", "つぎは", "今度は", "こんどは", "やっぱり", "やっぱ"] where s.hasPrefix(lead) && s.count > lead.count {
            s = String(s.dropFirst(lead.count))
        }
        var changed = true
        while changed {
            changed = false
            for suf in nameSuffixes where s.hasSuffix(suf) && s.count > suf.count {
                s = String(s.dropLast(suf.count))
                changed = true
            }
        }
        return s
    }

    /// 空白・句読点・記号を落として小文字化(WhisperKit の「元に戻して。」「元に 戻して!」の揺れを吸収)。
    static func normalize(_ s: String) -> String {
        String(s.lowercased().unicodeScalars.filter { !CharacterSet.whitespacesAndNewlines.union(.punctuationCharacters).union(.symbols).contains($0) })
    }
}

/// 発話に含まれたアプリ名を `GET /apps` の name 一覧と照合する(完全一致 → 前方一致 → 部分一致の順。同じ段で複数なら曖昧)。
public enum AppNameMatch: Equatable, Sendable {
    case none
    /// `names` のインデックス
    case one(Int)
    case ambiguous([Int])

    public static func match(_ query: String, in names: [String]) -> AppNameMatch {
        let q = key(query)
        guard !q.isEmpty else { return .none }
        let keys = names.map(key)
        let tiers: [(String) -> Bool] = [
            { $0 == q },
            { $0.hasPrefix(q) || q.hasPrefix($0) },
            { $0.contains(q) || q.contains($0) },
        ]
        for tier in tiers {
            let hits = keys.indices.filter { !keys[$0].isEmpty && tier(keys[$0]) }
            if hits.count == 1 { return .one(hits[0]) }
            if hits.count > 1 { return .ambiguous(hits) }
        }
        return .none
    }

    /// 比較用キー: 正規化 + 末尾の「アプリ」を落とす(「家計簿アプリ」と「家計簿」を同一視)。
    static func key(_ s: String) -> String {
        VoiceIntent.cleanAppName(VoiceIntent.normalize(s))
    }
}
