import AppIntents
import SwiftUI
import WidgetKit

// コントロールセンター・ロック画面・アクションボタンのコントロール(iOS 18・#1993)。配布の最低は iOS 17 なので版で守る。
// どちらも押すと本体が開く(`OpenAppFromControlIntent` / `OpenRecordFromControlIntent` は本体のプロセスで動く)。
// 生成アプリのボタンの動作を直接動かすコントロールは DSL の exposeAsControl の決定(#1775)の後にここへ足す。

/// 設定: どの生成アプリを開くか
@available(iOS 18, *)
struct SelectControlAppIntent: ControlConfigurationIntent {
    #if APP_STORE_REVIEW // 審査用の版は記録帳と言う(#247)
    static var title: LocalizedStringResource = "記録帳を選ぶ"
    @Parameter(title: "記録帳")
    var app: ControlAppEntity?
    #else
    static var title: LocalizedStringResource = "アプリを選ぶ"
    @Parameter(title: "アプリ")
    var app: ControlAppEntity?
    #endif
}

/// 設定: どの記録先の入力画面を開くか
@available(iOS 18, *)
struct SelectControlRecordTargetIntent: ControlConfigurationIntent {
    static var title: LocalizedStringResource = "記録先を選ぶ"

    @Parameter(title: "記録先")
    var target: ControlRecordTargetEntity?
}

/// 文言と記号
enum ControlCopy {
    static let openAppSymbol = "square.grid.2x2" // ショートカットの「アプリを開く」と同じ
    static let recordSymbol = "plus.circle" // ショートカットの「記録を追加」と同じ

    #if APP_STORE_REVIEW // 審査用の版は記録帳と言う(#247・docs/store/wording.md)
    static let openAppName: LocalizedStringResource = "記録帳を開く"
    static let openAppDescription: LocalizedStringResource = "選んだ記録帳を開きます。"
    static let openAppUnselected: LocalizedStringResource = "記録帳を選ぶ"
    static let recordDescription: LocalizedStringResource = "選んだ記録帳の入力画面を開きます。"
    #else
    static let openAppName: LocalizedStringResource = "アプリを開く"
    static let openAppDescription: LocalizedStringResource = "Tailor It で作ったアプリを開きます。"
    static let openAppUnselected: LocalizedStringResource = "アプリを選ぶ"
    static let recordDescription: LocalizedStringResource = "選んだアプリの記録の入力画面を開きます。"
    #endif
    static let recordName: LocalizedStringResource = "記録する"
    static let recordUnselected: LocalizedStringResource = "記録先を選ぶ"
}

@available(iOS 18, *)
struct OpenAppControl: ControlWidget {
    static let kind = "jp.kz2601.metamon.control.openApp"

    var body: some ControlWidgetConfiguration {
        AppIntentControlConfiguration(kind: Self.kind, intent: SelectControlAppIntent.self) { config in
            ControlWidgetButton(action: OpenAppFromControlIntent(appId: config.app?.id)) {
                if let name = ControlCatalog.load().controlTitle(appId: config.app?.id) {
                    Label(name, systemImage: ControlCopy.openAppSymbol) // アプリ名はデータなので訳さない
                } else {
                    Label { Text(ControlCopy.openAppUnselected) } icon: { Image(systemName: ControlCopy.openAppSymbol) }
                }
            }
        }
        .displayName(ControlCopy.openAppName)
        .description(ControlCopy.openAppDescription)
        .promptsForUserConfiguration()
    }
}

@available(iOS 18, *)
struct RecordControl: ControlWidget {
    static let kind = "jp.kz2601.metamon.control.record"

    var body: some ControlWidgetConfiguration {
        AppIntentControlConfiguration(kind: Self.kind, intent: SelectControlRecordTargetIntent.self) { config in
            ControlWidgetButton(action: OpenRecordFromControlIntent(appId: config.target?.appId, entityId: config.target?.entityId)) {
                if let title = ControlCatalog.load().controlTitle(recordTargetId: config.target?.id) {
                    Label(title, systemImage: ControlCopy.recordSymbol)
                } else {
                    Label { Text(ControlCopy.recordUnselected) } icon: { Image(systemName: ControlCopy.recordSymbol) }
                }
            }
        }
        .displayName(ControlCopy.recordName)
        .description(ControlCopy.recordDescription)
        .promptsForUserConfiguration()
    }
}
