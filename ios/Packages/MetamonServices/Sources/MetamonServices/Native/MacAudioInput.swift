#if os(macOS)
import AVFoundation

/// Mac の音声入力の診断(mac-005)。iOS の AVAudioSession の代わりに、既定の入力装置を見て `TranscriberDiagnostics` に残す。
/// 装置の選び方(外付けマイクの切り替えなど)の作り込みは mac-044。
enum MacAudioInput {
    static func fill(_ diag: inout TranscriberDiagnostics) {
        let device = AVCaptureDevice.default(for: .audio)
        diag.sessionCategory = "macOS"
        diag.sessionMode = "default"
        diag.inputAvailable = device != nil
        diag.inputRoute = device?.localizedName ?? ""
        diag.inputSampleRate = AVAudioEngine().inputNode.outputFormat(forBus: 0).sampleRate
    }
}
#endif
