import SwiftUI
import MetamonDSL

/// `speakButton`: text / field / value の文を端末の声で読み上げる(もう一度押すと止まる)。声は UI の言語。
/// 中身の作り方は qrCode と同じ(`QRCodeContent.resolve`: `{{fieldId}}`・保存値・computed・式)。画面を離れたら止める。
struct SpeakButtonLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext
    @State private var owner = UUID().uuidString
    private var speech: LeafSpeech { .shared }

    var body: some View {
        let text = QRCodeContent.resolve(c, form: ctx.form) { rt.evaluator() }
        let speaking = speech.speakingOwner == owner
        let empty = text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        Button {
            if speaking { speech.stop(owner: owner) } else { speech.speak(text, owner: owner) }
        } label: {
            Label(speaking ? L("止める") : title, systemImage: speaking ? "stop.fill" : "speaker.wave.2.fill")
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(RenderButtonStyle(kind: .secondary))
        .disabled(empty && !speaking)
        .accessibilityHint(empty ? L("読み上げる内容がまだありません") : "")
        .accessibilityIdentifier("speakButton")
        .onDisappear { speech.stop(owner: owner) }
    }

    private var title: String { c.label.flatMap { $0.isEmpty ? nil : $0 } ?? L("読み上げる") }
}
