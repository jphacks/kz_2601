import SwiftUI
import MetamonRenderer
import MetamonServices

/// 生成直後の「個人の情報も公開されます」の確認(#253)。
/// ai が `meta.publicPersonalData` を返した時だけ出す(旧サーバーは送らない = 出さない)。summary の読み上げはそのまま。
/// - 公開のままにする: 何もしない
/// - この欄を消す / 自分だけにする: 既存の修正(generate・mode edit)に指示文を送る
extension AppRuntime {
    func notePublicPersonalData(_ result: MorphResult) {
        publicDataPrompt = PublicDataPrompt(fields: result.publicPersonalData, version: result.document.version)
        if let prompt = publicDataPrompt {
            print("[Metamon] public personal data: \(prompt.fields.map { "\($0.entity).\($0.field)(\($0.kind))" })")
        }
    }

    func keepPublic() {
        publicDataPrompt = nil
    }

    func removePublicPersonalFields() async {
        guard let prompt = takePublicDataPrompt() else { return }
        await generate(prompt.removeInstruction(english: Self.instructionsInEnglish))
    }

    func makePublicDataPrivate() async {
        guard let prompt = takePublicDataPrompt() else { return }
        await generate(prompt.privateInstruction(english: Self.instructionsInEnglish))
    }

    /// 出した時の版のままなら取り出して閉じる(別の版に変わっていたら何も送らない)。
    private func takePublicDataPrompt() -> PublicDataPrompt? {
        guard let prompt = publicDataPrompt else { return nil }
        publicDataPrompt = nil
        return prompt.version == document.version ? prompt : nil
    }

    private static var instructionsInEnglish: Bool { LanguagePreference.shared.code == "en" }
}

/// 確認ダイアログ。生成直後は音声シートが閉じる途中なので、少し待ってから出す。
struct PublicDataPromptHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt
    @State private var shown: PublicDataPrompt?

    func body(content: Content) -> some View {
        content
            .task(id: rt.publicDataPrompt?.id) {
                shown = nil
                guard rt.publicDataPrompt != nil else { return }
                try? await Task.sleep(for: .milliseconds(700))
                guard !Task.isCancelled else { return }
                shown = rt.publicDataPrompt
            }
            .alert(L10n.tr("個人の情報も公開されます"), isPresented: Binding(get: { shown != nil }, set: { if !$0 { shown = nil } }),
                   presenting: shown) { _ in
                Button(L10n.tr("この欄を消す"), role: .destructive) { Task { await rt.removePublicPersonalFields() } }
                Button(L10n.tr("自分だけにする")) { Task { await rt.makePublicDataPrivate() } }
                Button(L10n.tr("公開のままにする"), role: .cancel) { rt.keepPublic() }
            } message: { prompt in
                Text(L10n.tr("\(prompt.fieldNames(english: LanguagePreference.shared.code == "en"))は、アプリを開いた人なら誰でも見られます。"))
            }
    }
}

/// チャットの吹き出しに付ける公開の印(#253 `confirmPublic` の question)。
struct PublicConfirmTag: View {
    var body: some View {
        Label(L10n.tr("公開の記録の確認"), systemImage: "globe")
            .font(.caption.weight(.semibold))
            .foregroundStyle(Theme.Colors.warning)
            .accessibilityIdentifier("chat.publicConfirm")
    }
}
