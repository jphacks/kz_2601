import Foundation
import MetamonDSL

/// `transcribeField` の文字起こし。声を文字にする音声モデルはアプリの音声入力(マイク FAB)と同じものをホストが持つ。
/// 声は端末の外に出さない。持たないホスト(プレビュー・テスト)では部品が「使えません」を出す。
@MainActor
public protocol DictationHost: AnyObject {
    /// 音声モデルの状態。
    var dictationReadiness: DictationReadiness { get }
    /// マイク FAB の音声入力を出している間 true。部品は録音中ならやめる(マイクを取り合わない)。
    var isVoiceInputActive: Bool { get }
    /// 音声モデルが無い・失敗した時: 用意を始める(ダウンロードの確認はホストが出す)。
    func prepareDictation()
    /// 録音を始める。読み上げ中なら止めてから。失敗は `DictationFailure`(表示用の文言)。
    func startDictation() async throws
    /// 録音を止めて文字にする。何も聞き取れなければ `DictationFailure`。
    func finishDictation() async throws -> String
    /// 録音を捨てる(画面を離れた・音声入力が始まった)。録音していなければ何もしない。
    func cancelDictation()
}

public enum DictationReadiness: Equatable, Sendable {
    case ready
    /// ダウンロード中・読み込み中・文字起こし中
    case preparing
    /// 端末にモデルが無い(まだ用意していない)
    case needsModel
    case failed
}

/// 録音・文字起こしの失敗。`message` はホストが表示言語で作った文言。
/// `settingsURL`: 許可が無い時に「設定を開く」で開く先(Mac はシステム設定のマイク・音声認識の欄)。nil ならボタンを出さない。
public struct DictationFailure: LocalizedError, Equatable, Sendable {
    public let message: String
    public let settingsURL: URL?
    public init(_ message: String, settingsURL: URL? = nil) {
        self.message = message
        self.settingsURL = settingsURL
    }
    public var errorDescription: String? { message }
}

public extension DSLRuntime {
    var dictation: (any DictationHost)? { self as? any DictationHost }
}
