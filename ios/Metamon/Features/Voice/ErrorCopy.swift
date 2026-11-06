import Foundation
import OSLog
import MetamonServices
import MetamonRenderer

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
    // 1 日の上限(#280・429 rate_limited)
    static var rateLimitedTitle: String { L10n.tr("今日はここまでです") }
    static var rateLimitedGlobalTitle: String { L10n.tr("本日の受付を終了しました") }
    static func rateLimitedBody(_ info: RateLimitInfo, timeZone: TimeZone = .current) -> String {
        let when = info.resetAt.map { resetTime($0, timeZone: timeZone) } ?? L10n.tr("明日")
        switch info.scope {
        case .global:
            return L10n.pick(L10n.tr("利用が集中したため、新しい生成を止めています。\(when)に再開します"), review: L10n.tr("利用が集中したため、新しい記録帳の用意を止めています。\(when)に再開します"))
        case .ip:
            return L10n.tr("このネットワークからの今日の上限に達しました。サインインすると続けて使えます")
        case .user:
            var body = info.limit.map { L10n.tr("今日の上限(\($0)回分)に達しました。\(when)からまた使えます") }
                ?? L10n.tr("今日の上限に達しました。\(when)からまた使えます")
            if info.isAnonymous { body += L10n.tr("。サインインすると上限が増えます") }
            return body
        }
    }
    /// 「10月26日 0:00」(英語表示なら "Oct 26, 0:00")
    static func resetTime(_ date: Date, timeZone: TimeZone = .current) -> String {
        let english = LanguagePreference.shared.code == "en"
        let f = DateFormatter()
        f.locale = Locale(identifier: english ? "en_US_POSIX" : "ja_JP")
        f.timeZone = timeZone
        f.dateFormat = english ? "MMM d, H:mm" : "M月d日 H:mm"
        return f.string(from: date)
    }
    // 運営による AI の一時停止(#511・503 ai_paused)。上限の文言とは分ける
    static var aiPausedTitle: String { L10n.tr("AI を一時停止しています") }
    static var aiPausedBody: String { L10n.tr("AI を一時的に止めています。しばらくしてからもう一度お試しください") }
    static var timeoutTitle: String { L10n.tr("時間がかかりすぎました") }
    static var timeoutBody: String { L10n.tr("サーバーの応答がありませんでした。もう一度お試しください") }
    // セッション
    static var sessionExpiredTitle: String { L10n.tr("サインインの有効期限が切れました") }
    static var sessionExpiredBody: String { L10n.tr("もう一度サインインしてください") }
    /// サインイン画面(`SignInView`)に出す 1 文
    static var sessionExpiredSignIn: String { L10n.tr("サインインの有効期限が切れました。もう一度サインインしてください") }
    // App Check(#318・401 app_check_failed)。サインインでは直らない
    static var appCheckFailedBody: String { L10n.tr("このアプリからの接続を確認できませんでした。少し待ってもう一度お試しください。続く時は App Store から最新版を入れてください") }
    // 競合
    static var conflictBanner: String { L10n.tr("別の端末で更新されていたため、最新の状態に合わせました。もう一度お話しください") }
    // 入力不備・その他
    static var rejectedTitle: String { L10n.tr("うまく伝わりませんでした") }
    static var failedTitle: String { L10n.pick(L10n.tr("うまく作れませんでした"), review: L10n.tr("記録帳を用意できませんでした")) }
    static var cancelledToast: String { L10n.pick(L10n.tr("作るのをやめました"), review: L10n.tr("用意を取りやめました")) }
    // 生成前の対話(#75)
    static var chatFailedTitle: String { L10n.tr("質問をうまく用意できませんでした") }
    // サーバー到達不能(起動時)。版の番号・OS のエラー文は出さない(M15・H9)
    static var offlineRestored: String {
        L10n.pick(L10n.tr("オフラインです。最後に保存したアプリを表示しています"),
                  review: L10n.tr("オフラインです。最後に保存した記録帳を表示しています"))
    }
    static var unreachableTitle: String { L10n.tr("サーバーに接続できません") }
    static var unreachableBody: String { L10n.tr("接続を確認してください。つながると自動で読み込み直します") }
    // レコード取得失敗(#85)。Renderer の placeholder 文言は Renderer 側(`EmptyPlaceholder`)に持つ

    // 生成の失敗(H9)。サーバーの文はそのまま出さず、ログにだけ残す
    static var rejectedBody: String { L10n.tr("言い方を変えて、もう一度お話しください") }
    static var otherBody: String { L10n.tr("少し待ってから、もう一度お試しください") }

    // 録音/認識の失敗(#44・H9)。秒数・dB・サンプル数・オーディオセッションの理由は出さない(診断は `[Voice]` のログ)
    static func transcriberTitle(_ f: TranscriberFailure) -> String {
        switch f {
        case .notReady: L10n.tr("音声モデルを準備中です")
        case .microphoneDenied: L10n.tr("マイクを使えません")
        case .audioSession, .noInputAvailable, .noAudioCaptured: L10n.tr("録音できませんでした")
        case .tooShort, .silent: L10n.tr("うまく聞き取れませんでした")
        }
    }
    static func transcriberFailure(_ f: TranscriberFailure) -> String {
        switch f {
        case .notReady: L10n.tr("少し待ってから、もう一度お試しください")
        case .microphoneDenied: L10n.tr("設定アプリ > Tailor It でマイクを許可してください")
        case .audioSession, .noInputAvailable, .noAudioCaptured: L10n.tr("ほかのアプリの通話や録音を終えてから、もう一度お試しください")
        case .tooShort: L10n.tr("タップしてから話し、話し終えたらもう一度タップしてください")
        case .silent: L10n.tr("マイクに近づいて、もう一度話してください")
        }
    }
    static var notHeardTitle: String { L10n.tr("うまく聞き取れませんでした") }
    static var notHeardBody: String { L10n.tr("もう一度、はっきり話してください") }

    /// 失敗の「次の一手」。圏外なら接続の確認、それ以外は少し待って再試行。
    static func nextStep(for error: Error) -> String {
        switch FailureKind.classify(error) {
        case .offline: offlineInterruptedBody
        case .sessionExpired: sessionExpiredBody
        case .appCheckFailed: appCheckFailedBody
        case .aiPaused: aiPausedBody
        default: otherBody
        }
    }
    /// 「〇〇できませんでした。次の一手」。エラーの中身(英語の OS の文・サーバーの文)はログにだけ残す。
    static func failure(_ what: String, _ error: Error, context: String) -> String {
        ErrorLog.record(error, context: context)
        return L10n.tr("\(what)。\(nextStep(for: error))")
    }

    // 完成の読み上げ(H8)。要約が無い時だけ使う
    static func completedAnnouncement(appName: String, created: Bool) -> String {
        guard created, !appName.isEmpty else { return L10n.tr("変更しました") }
        return L10n.tr("「\(appName)」ができました")
    }

    // ボタン
    static var retry: String { L10n.tr("もう一度") }
    static var sendNow: String { L10n.tr("今すぐ送る") }
    static var discard: String { L10n.tr("やめる") }
    static var dismiss: String { L10n.tr("閉じる") }
    static var signIn: String { L10n.tr("サインイン") }
    static var edit: String { L10n.tr("言い直す") }
}

/// 失敗の詳しい中身(OS のエラー文・サーバーの文・診断の値)の置き場(H9)。画面には出さず、stdout と os_log に残す。
/// 実機は `xcrun devicectl device process launch --console` か Console.app(subsystem `jp.kz2601.metamon` category `error`)で見る。
enum ErrorLog {
    private static let logger = Logger(subsystem: "jp.kz2601.metamon", category: "error")

    static func record(_ error: Error, context: String) {
        record("\(error)", context: context)
    }

    static func record(_ detail: String, context: String) {
        print("[Metamon] \(context) failed: \(detail)")
        logger.error("\(context, privacy: .public) failed: \(detail)")
    }
}
