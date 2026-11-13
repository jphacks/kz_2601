import AppKit
import SwiftUI

/// 「ショートカットで自動にする」の Mac の案内の中身(#3080・iOS は `AutomationGuide`・#1995)。今ある Intent(記録を追加・話しかける)だけを使う。
/// - オートメーションは利用者が Mac のショートカット App で作る(アプリからは作れない)。ここは例と手順を見せるだけ
/// - 例は Mac で起きるきっかけ(App・集中モード・Wi-Fi)。iPhone の NFC タグは Mac に無いので出さない
/// - 文はここにだけ置く。キーは日本語(英語は `Localizable.xcstrings`)
/// - 審査用の版でも出すので「アプリを作る」系の言い方をしない(docs/store/wording.md)
enum MacAutomationGuide {
    typealias Example = AutomationGuideExample

    /// どれも Mac を使っている(ロックを解除している)時に起きるきっかけ。「記録を追加」はロック解除が要るため。
    static let examples: [Example] = [
        Example(id: "appOpen", symbol: "macwindow",
                triggerKey: "ゲームアプリを開いた時",
                actionKey: "「記録を追加」で、記録先「ゲーム時間」に「開始」を入れる"),
        Example(id: "appClose", symbol: "xmark.app",
                triggerKey: "ゲームアプリを閉じた時",
                actionKey: "「記録を追加」で、記録先「ゲーム時間」に「終了」を入れる"),
        Example(id: "focusOn", symbol: "moon",
                triggerKey: "「仕事」の集中モードをオンにした時",
                actionKey: "「記録を追加」で、記録先「作業時間」に「開始」を入れる"),
        Example(id: "focusOff", symbol: "moon.zzz",
                triggerKey: "「仕事」の集中モードをオフにした時",
                actionKey: "「記録を追加」で、記録先「作業時間」に「終了」を入れる"),
        Example(id: "wifi", symbol: "wifi",
                triggerKey: "職場の Wi-Fi につないだ時",
                actionKey: "「記録を追加」で、記録先「出社」に「出社」を入れる"),
    ]

    /// 作り方の手順(ショートカット App の中での操作)。
    static let stepKeys = [
        "ショートカット App を開き、左の「オートメーション」を選ぶ",
        "「+」を押し、きっかけ(App・集中モード・Wi-Fi など)を選ぶ",
        "「すぐに実行」を選ぶ(動かす前に確かめたい時は「確認後に実行」)",
        "アクションの検索で「Tailor It」と入れ、「記録を追加」を選ぶ",
        "記録先と内容を決めて「完了」を押す",
    ]

    static let titleKey = "ショートカットで自動にする"
    static let introKey = "App を開いた時や集中モードを切り替えた時など、Mac を使っている時に Tailor It の「記録を追加」を自動で動かせます。オートメーションはショートカット App で作ります。"
    static let examplesHeaderKey = "きっかけの例"
    static let stepsHeaderKey = "作り方"
    static let otherActionsKey = "「記録を追加」のほかに「話しかける」(音声入力を開く)も、きっかけに合わせて動かせます。"
    static let openButtonKey = "ショートカット App を開く"
    static let openFailedKey = "ショートカット App を開けませんでした。"
    static let noteKeys = [
        "「記録を追加」で記録するには、Mac のロックを解除している必要があります。ロック中に起きるきっかけではなく、Mac を使っている時に起きるきっかけを選んでください。",
        "先に Tailor It を一度開いておいてください。開いたことが無いと記録できません。",
        "お使いの macOS のショートカット App に「オートメーション」が無い時は、macOS を新しくすると使えるようになります。",
    ]

    /// ショートカット App を開く URL。
    static let shortcutsURL = URL(string: "shortcuts://")!

    static var title: String { L10n.tr(String.LocalizationValue(titleKey)) }
    static var intro: String { L10n.tr(String.LocalizationValue(introKey)) }
    static var steps: [String] { stepKeys.map { L10n.tr(String.LocalizationValue($0)) } }
    static var notes: [String] { noteKeys.map { L10n.tr(String.LocalizationValue($0)) } }

    /// 文言のキーすべて(英訳・言い回しのテスト用)。
    static var allKeys: [String] {
        [titleKey, introKey, examplesHeaderKey, stepsHeaderKey, otherActionsKey, openButtonKey, openFailedKey]
            + examples.flatMap { [$0.triggerKey, $0.actionKey] } + stepKeys + noteKeys
    }
}

/// 設定の「詳細」から開く案内のシート(#3080)。例・手順・注意を見せ、ショートカット App を開くボタンを置く。
struct MacAutomationGuideView: View {
    @Environment(\.dismiss) private var dismiss
    @State private var openFailed = false

    var body: some View {
        VStack(spacing: 0) {
            Form {
                Section {
                    Text(MacAutomationGuide.intro)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                Section {
                    ForEach(MacAutomationGuide.examples) { example in
                        Label {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(example.trigger).fontWeight(.semibold)
                                Text(example.action)
                                    .font(.callout)
                                    .foregroundStyle(.secondary)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                        } icon: {
                            Image(systemName: example.symbol).frame(width: 20)
                        }
                        .accessibilityElement(children: .combine)
                        .accessibilityIdentifier("automationGuide.example.\(example.id)")
                    }
                } header: {
                    Text(L10n.tr(String.LocalizationValue(MacAutomationGuide.examplesHeaderKey)))
                } footer: {
                    Text(L10n.tr(String.LocalizationValue(MacAutomationGuide.otherActionsKey)))
                }
                Section {
                    ForEach(Array(MacAutomationGuide.steps.enumerated()), id: \.offset) { index, step in
                        HStack(alignment: .firstTextBaseline, spacing: 8) {
                            Text(verbatim: "\(index + 1).").monospacedDigit().fontWeight(.semibold)
                            Text(step).fixedSize(horizontal: false, vertical: true)
                        }
                        .accessibilityElement(children: .combine)
                    }
                    Button {
                        openFailed = !NSWorkspace.shared.open(MacAutomationGuide.shortcutsURL)
                    } label: {
                        Label(L10n.tr(String.LocalizationValue(MacAutomationGuide.openButtonKey)), systemImage: "arrow.up.forward.app")
                    }
                    .accessibilityIdentifier("automationGuide.openShortcuts")
                } header: {
                    Text(L10n.tr(String.LocalizationValue(MacAutomationGuide.stepsHeaderKey)))
                } footer: {
                    VStack(alignment: .leading, spacing: 4) {
                        if openFailed {
                            Text(L10n.tr(String.LocalizationValue(MacAutomationGuide.openFailedKey)))
                                .foregroundStyle(Theme.Colors.warning)
                                .accessibilityIdentifier("automationGuide.openFailed")
                        }
                        ForEach(MacAutomationGuide.notes, id: \.self) { Text($0) }
                    }
                }
            }
            .formStyle(.grouped)
            Divider()
            HStack {
                Spacer()
                Button("閉じる") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                    .accessibilityIdentifier("automationGuide.close")
            }
            .padding(MacTheme.Spacing.group)
        }
        .frame(width: 520, height: 600)
        .accessibilityIdentifier("automationGuide.list")
    }
}
