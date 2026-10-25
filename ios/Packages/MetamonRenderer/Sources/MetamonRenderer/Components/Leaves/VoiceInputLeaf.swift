import SwiftUI
import MetamonDSL

/// `voiceInput` のプレースホルダ。
/// 本物の音声画面は WhisperKit(Services)に依存するので Renderer には置かず、アプリ本体が
/// `ComponentRegistry.standard.registering(.voiceInput) { c, _ in VoiceInputView(prompt: c.prompt) }` で差し替える。
struct VoiceInputLeaf: View {
    let c: Component

    var body: some View {
        UnsupportedComponent(label: L("voiceInput: アプリ本体が ComponentRegistry に登録してください"))
    }
}
