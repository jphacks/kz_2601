import Foundation
import MetamonServices

/// エラー時の文言(#60)。`Greeting` と同じく **ここにだけ** 置く(多言語化 #44 / リネーム #69 ではこのファイルを差し替える)。
/// 原則: 1 文で「何が起きたか + 次に何をすればいいか」。主ボタンは動詞 1 語〜2 語。
enum ErrorCopy {
    // 圏外
    static var offlineQueuedTitle: String { L10n.tr("オフラインです") }
    static var offlineQueuedBody: String { L10n.tr("接続が戻ったら自動で送ります") }
    static var offlineInterruptedTitle: String { L10n.tr("通信が切れました") }
    static var offlineInterruptedBody: String { L10n.tr("接続を確認して、もう一度お試しください") }
    // 混雑・タイムアウト
    static var busyTitle: String { L10n.tr("混み合っています") }
    static var busyBody: String { L10n.tr("少し時間をおいて、もう一度お試しください") }
    static func busyBody(retryAfter seconds: TimeInterval) -> String {
        L10n.tr("\(Int(seconds.rounded(.up))) 秒ほど待ってから、もう一度お試しください")
    }
    static var timeoutTitle: String { L10n.tr("時間がかかりすぎました") }
    static var timeoutBody: String { L10n.tr("サーバーの応答がありませんでした。もう一度お試しください") }
    // セッション
    static var sessionExpiredTitle: String { L10n.tr("サインインの有効期限が切れました") }
    static var sessionExpiredBody: String { L10n.tr("もう一度サインインしてください") }
    /// サインイン画面(`SignInView`)に出す 1 文
    static var sessionExpiredSignIn: String { L10n.tr("サインインの有効期限が切れました。もう一度サインインしてください") }
    // 競合
    static var conflictBanner: String { L10n.tr("別の端末で更新されていたため、最新の状態に合わせました。もう一度お話しください") }
    // 入力不備・その他
    static var rejectedTitle: String { L10n.tr("うまく伝わりませんでした") }
    static var failedTitle: String { L10n.tr("生成に失敗しました") }
    static var cancelledToast: String { L10n.tr("生成をキャンセルしました") }
    // 生成前の対話(#75)
    static var chatFailedTitle: String { L10n.tr("質問をうまく用意できませんでした") }
    // サーバー到達不能(起動時)
    static func offlineRestored(version: Int) -> String { L10n.tr("オフライン: 最後に取得したアプリ(v\(version))を表示しています") }
    static func unreachable(_ detail: String) -> String { L10n.tr("サーバーに接続できません: \(detail)") }
    // レコード取得失敗(#85)。Renderer の placeholder 文言は Renderer 側(`EmptyPlaceholder`)に持つ

    // ボタン
    static var retry: String { L10n.tr("もう一度") }
    static var sendNow: String { L10n.tr("今すぐ送る") }
    static var discard: String { L10n.tr("やめる") }
    static var dismiss: String { L10n.tr("閉じる") }
    static var signIn: String { L10n.tr("サインイン") }
    static var edit: String { L10n.tr("言い直す") }
}
