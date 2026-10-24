import Foundation
import MetamonServices

/// エラー時の文言(#60)。`Greeting` と同じく **ここにだけ** 置く(多言語化 #44 / リネーム #69 ではこのファイルを差し替える)。
/// 原則: 1 文で「何が起きたか + 次に何をすればいいか」。主ボタンは動詞 1 語〜2 語。
enum ErrorCopy {
    // 圏外
    static let offlineQueuedTitle = "オフラインです"
    static let offlineQueuedBody = "接続が戻ったら自動で送ります"
    static let offlineInterruptedTitle = "通信が切れました"
    static let offlineInterruptedBody = "接続を確認して、もう一度お試しください"
    // 混雑・タイムアウト
    static let busyTitle = "混み合っています"
    static let busyBody = "少し時間をおいて、もう一度お試しください"
    static func busyBody(retryAfter seconds: TimeInterval) -> String {
        "\(Int(seconds.rounded(.up))) 秒ほど待ってから、もう一度お試しください"
    }
    static let timeoutTitle = "時間がかかりすぎました"
    static let timeoutBody = "サーバーの応答がありませんでした。もう一度お試しください"
    // セッション
    static let sessionExpiredTitle = "サインインの有効期限が切れました"
    static let sessionExpiredBody = "もう一度サインインしてください"
    /// サインイン画面(`SignInView`)に出す 1 文
    static let sessionExpiredSignIn = "サインインの有効期限が切れました。もう一度サインインしてください"
    // 競合
    static let conflictBanner = "別の端末で更新されていたため、最新の状態に合わせました。もう一度お話しください"
    // 入力不備・その他
    static let rejectedTitle = "うまく伝わりませんでした"
    static let failedTitle = "生成に失敗しました"
    static let cancelledToast = "生成をキャンセルしました"
    // サーバー到達不能(起動時)
    static func offlineRestored(version: Int) -> String { "オフライン: 最後に取得したアプリ(v\(version))を表示しています" }
    static func unreachable(_ detail: String) -> String { "サーバーに接続できません: \(detail)" }
    // レコード取得失敗(#85)。Renderer の placeholder 文言は Renderer 側(`EmptyPlaceholder`)に持つ

    // ボタン
    static let retry = "もう一度"
    static let sendNow = "今すぐ送る"
    static let discard = "やめる"
    static let dismiss = "閉じる"
    static let signIn = "サインイン"
    static let edit = "言い直す"
}
