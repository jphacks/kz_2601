import Foundation

/// ai に投げる前に iOS 側で拾う発話の意図(G11 / 新規作成)。
/// - `.undo`: 「元に戻して」「戻して」「取り消して」「アンドゥ」→ appconfig の `versions/{n}/restore`(ai を呼ばない)
/// - `.newApp`: 発話頭の「別のアプリ」「新しいアプリ」→ appId を採番し直してから ai へ(前のアプリはサーバーに残る)
/// - `.morph`: それ以外は従来どおり ai へ
public enum VoiceIntent: Equatable, Sendable {
    case undo
    case newApp
    case morph

    static let undoPrefixes = ["元に戻", "もとに戻", "取り消", "とりけし", "アンドゥ", "あんどぅ", "undo", "ひとつ前", "一つ前", "さっきの状態", "前の状態"]
    /// 「戻して」単独や「前に戻して」のような短い発話だけ undo 扱い(「ボタンを右に戻して」のような編集指示を拾わない)
    static let shortUndoWords = ["戻して", "戻す", "もどして", "もどす"]
    static let shortUndoMaxLength = 8
    static let newAppPrefixes = ["別のアプリ", "別アプリ", "新しいアプリ", "新規アプリ", "違うアプリ", "次のアプリ", "べつのアプリ", "あたらしいアプリ"]

    public static func detect(_ transcript: String) -> VoiceIntent {
        let text = normalize(transcript)
        guard !text.isEmpty else { return .morph }
        if newAppPrefixes.contains(where: { text.hasPrefix($0) }) { return .newApp }
        if undoPrefixes.contains(where: { text.contains($0) }) { return .undo }
        if text.count <= shortUndoMaxLength, shortUndoWords.contains(where: { text.contains($0) }) { return .undo }
        return .morph
    }

    /// 空白・句読点・記号を落として小文字化(WhisperKit の「元に戻して。」「元に 戻して!」の揺れを吸収)。
    static func normalize(_ s: String) -> String {
        String(s.lowercased().unicodeScalars.filter { !CharacterSet.whitespacesAndNewlines.union(.punctuationCharacters).union(.symbols).contains($0) })
    }
}
