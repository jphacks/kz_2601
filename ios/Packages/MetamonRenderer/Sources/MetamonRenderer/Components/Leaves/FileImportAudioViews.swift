import SwiftUI
import MetamonDSL

/// `fileImport` の accept audio: 文字起こし中の進み具合とキャンセル。前面にある間だけ進むことを出す。
struct FileImportAudioProgress: View {
    let progress: Double
    let cancel: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            ProgressView(value: progress) {
                HStack {
                    Text(L("録音を文字にしています…"))
                    Spacer()
                    Text(verbatim: "\(Int((progress * 100).rounded()))%").monospacedDigit()
                }
                .font(.subheadline)
            }
            .accessibilityIdentifier("fileImport.audioProgress")
            Text(L("端末の中で文字にしています。終わるまでアプリを開いたままにしてください"))
                .font(.footnote)
                .foregroundStyle(.secondary)
            Button(role: .cancel, action: cancel) {
                Text(L("キャンセル")).frame(maxWidth: .infinity)
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary))
            .accessibilityIdentifier("fileImport.audioCancel")
        }
    }
}

/// 録音ファイルを文字にする音声モデルの状態(用意できていない時だけ出す)。
struct FileImportAudioModelRow: View {
    let host: (any AudioFileTranscriptionHost)?

    var body: some View {
        if let host {
            switch host.audioFileReadiness {
            case .ready:
                EmptyView()
            case .preparing:
                HStack(spacing: RenderTheme.Spacing.sm) {
                    ProgressView()
                    Text(L("録音ファイル用の音声モデルを準備しています…")).font(.footnote).foregroundStyle(.secondary)
                }
            case .needsModel, .failed:
                Button { host.prepareAudioFileTranscription() } label: {
                    Label(L("録音ファイル用の音声モデルを用意する"), systemImage: "arrow.down.circle")
                }
                .font(.footnote)
                .accessibilityIdentifier("fileImport.audioPrepare")
            }
        } else {
            Text(L("この画面では録音ファイルを文字にできません"))
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
    }
}
