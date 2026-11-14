import AVFoundation
import SwiftUI
import MetamonRenderer

/// 読み上げの声と速さの設定行(#2229)。iOS の設定の「読み上げ」に出す。Mac の設定も同じ行を使えるよう Shared に置く。
/// 選んだ値は `SpeechVoice.preference` に保存し、本体の読み上げ・speakButton・道案内の 3 か所に効く。
struct SpeechVoiceSettingsRows: View {
    @State private var options: [SpeechVoiceOption] = []
    /// 言語 → 声の id(空文字 = 自動)
    @State private var chosen: [String: String] = [:]
    @State private var rate: SpeechRateChoice = SpeechVoice.preference.rate
    @State private var personal = SpeechVoice.personalVoiceStatus

    var body: some View {
        Group {
            ForEach(SpeechVoice.languages, id: \.self) { language in
                Picker(SpeechVoiceCopy.voiceTitle(language), selection: binding(language)) {
                    Text("自動").tag("")
                    ForEach(SpeechVoiceChoice.options(for: language, in: options)) { option in
                        Text(SpeechVoiceCopy.label(option)).tag(option.id)
                    }
                }
                .accessibilityIdentifier("settings.speechVoice.\(language)")
            }
            Picker("読み上げの速さ", selection: $rate) {
                ForEach(SpeechRateChoice.allCases) { r in
                    Text(SpeechVoiceCopy.rateTitle(r)).tag(r)
                }
            }
            .accessibilityIdentifier("settings.speechRate")
            if personal == .notDetermined {
                Button("パーソナルボイスを使う") {
                    Task {
                        personal = await SpeechVoice.requestPersonalVoice()
                        load()
                    }
                }
                .accessibilityIdentifier("settings.speechPersonalVoice")
            }
        }
        .onAppear {
            SpeechVoice.reload() // 声の追加・削除(設定 App でのダウンロードなど)を拾う
            load()
        }
        .onChange(of: rate) { _, value in SpeechVoice.preference.rate = value }
    }

    private func load() {
        options = SpeechVoice.options
        var map: [String: String] = [:]
        for language in SpeechVoice.languages {
            // 消えた声は自動に戻す(保存も消える)
            let resolution = SpeechVoiceChoice.resolve(language: language, chosenId: SpeechVoice.preference.voiceId(for: language), options: options)
            if resolution.chosenIsGone { SpeechVoice.preference.setVoiceId(nil, for: language) }
            map[language] = SpeechVoice.preference.voiceId(for: language) ?? ""
        }
        chosen = map
    }

    private func binding(_ language: String) -> Binding<String> {
        Binding {
            chosen[language] ?? ""
        } set: { id in
            chosen[language] = id
            SpeechVoice.preference.setVoiceId(id.isEmpty ? nil : id, for: language)
        }
    }
}

/// 声の設定の文言。
enum SpeechVoiceCopy {
    static func voiceTitle(_ language: String) -> LocalizedStringKey {
        language.hasPrefix("en") ? "英語の声" : "日本語の声"
    }

    static func rateTitle(_ rate: SpeechRateChoice) -> String {
        switch rate {
        case .slow: L10n.tr("ゆっくり")
        case .normal: L10n.tr("ふつう")
        case .fast: L10n.tr("速め")
        }
    }

    /// 「Kyoko(拡張)」のように品質を添える。
    static func label(_ option: SpeechVoiceOption) -> String {
        let tag: String? = if option.isPersonal {
            L10n.tr("パーソナルボイス")
        } else {
            switch option.quality {
            case 2: L10n.tr("プレミアム")
            case 1: L10n.tr("拡張")
            default: nil
            }
        }
        guard let tag else { return option.name }
        return L10n.tr("\(option.name)(\(tag))")
    }

    static var footer: String {
        L10n.pick(L10n.tr("声と速さは、生成したアプリの読み上げボタンと道案内にも効きます。選んだ声が端末から消えた時は自動に戻ります。"),
                  review: L10n.tr("声と速さは、記録帳の読み上げボタンと道案内にも効きます。選んだ声が端末から消えた時は自動に戻ります。"))
    }
}
