import Foundation
import MetamonRenderer

/// 端末のデータ削除のうち UserDefaults の分。iOS と Mac で共有する(#972)。
enum LocalWipe {
    /// 消さずに残すキー: 個人のデータではない端末の設定(表示の言語)と、本人が選んだ送信のオプトアウト
    /// (クラッシュ報告 #296・AI ボタンの利用状況 #446。消すと、画面はオフのまま次の起動で黙ってオンに戻る)と、端末の移行の印。
    static let preservedKeys: Set<String> = [
        "metamon.appLanguage",
        "metamon.crashReporting.enabled",
        "metamon.aiButtonMetrics.enabled",
        "metamon.push.installationId", // 端末ごとに安定させる(#269)。個人のデータではない
        TimerKeyMigration.doneKey, // タイマーのキーの移行(#623)の印。消すと次の起動で移行がもう一度走る(データではない)
    ]

    /// removePersistentDomain は直後の書き込み(新 appId)が永続化されないことがあったので、キーを 1 つずつ消す。
    static func clearDefaults(_ defaults: UserDefaults, domain: String?) {
        guard let domain, let keys = defaults.persistentDomain(forName: domain)?.keys else { return }
        for key in keys where !preservedKeys.contains(key) {
            defaults.removeObject(forKey: key)
        }
    }
}
