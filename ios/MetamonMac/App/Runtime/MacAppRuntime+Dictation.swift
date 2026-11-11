import Foundation
import MetamonRenderer

/// 生成したアプリの話して入力(aiChat のマイク・transcribeField・#1328)。声で頼む(#966)と同じ認識(WhisperKit・準備中は Apple の端末内の認識)。
/// 声は Mac の外に出さない。録音を始めたら読み上げを止める・画面を離れたら録音を捨てるのは Renderer の部品がやる(iOS と同じ)。
/// 許可が無い時は「システム設定 > プライバシーとセキュリティ」のマイク / 音声認識の欄を開けるようにする(`MacVoiceCopy.dictationFailure`)。
/// 認識を持たない時(`dictationSource` が nil)は使えない(失敗として出す)。
extension MacAppRuntime: DictationHost {
    var dictationReadiness: DictationReadiness { dictationSource?.readiness ?? .failed }

    var isVoiceInputActive: Bool { dictationSource?.isVoiceInputActive ?? false }

    func prepareDictation() { dictationSource?.prepare() }

    func startDictation() async throws {
        guard let dictationSource else { throw Self.noDictation }
        try await dictationSource.start()
    }

    func finishDictation() async throws -> String {
        guard let dictationSource else { throw Self.noDictation }
        return try await dictationSource.finish()
    }

    func cancelDictation() { dictationSource?.cancel() }

    private static var noDictation: DictationFailure { DictationFailure(L10n.tr("録音できませんでした。もう一度お試しください")) }
}
