import Foundation
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
    static var failedTitle: String { L10n.pick(L10n.tr("生成に失敗しました"), review: L10n.tr("記録帳を用意できませんでした")) }
    static var cancelledToast: String { L10n.pick(L10n.tr("生成をキャンセルしました"), review: L10n.tr("用意を取りやめました")) }
    // 生成前の対話(#75)
    static var chatFailedTitle: String { L10n.tr("質問をうまく用意できませんでした") }
    // サーバー到達不能(起動時)
    static func offlineRestored(version: Int) -> String {
        L10n.pick(L10n.tr("オフライン: 最後に取得したアプリ(v\(version))を表示しています"),
                  review: L10n.tr("オフライン: 最後に取得した記録帳(v\(version))を表示しています"))
    }
    static func unreachable(_ detail: String) -> String { L10n.tr("サーバーに接続できません: \(detail)") }
    // レコード取得失敗(#85)。Renderer の placeholder 文言は Renderer 側(`EmptyPlaceholder`)に持つ

    // 録音/認識の失敗(#44)。`TranscriberFailure.errorDescription`(Services・日本語固定)の表示言語対応版
    static func transcriberFailure(_ f: TranscriberFailure) -> String {
        switch f {
        case .notReady(let status): L10n.tr("音声モデルを準備中です(\(status))。少し待ってからお試しください")
        case .microphoneDenied: L10n.tr("マイクの権限がありません。設定アプリ > Tailor It でマイクを許可してください")
        case .audioSession(let reason): L10n.tr("録音を開始できませんでした(オーディオセッション: \(reason))")
        case .noInputAvailable: L10n.tr("使えるマイクが見つかりません(他のアプリが使用中か、入力ルートがありません)")
        case .noAudioCaptured(let s): L10n.tr("マイクから音声が届きませんでした(録音\(seconds(s))秒・0サンプル)。他のアプリの通話/録音を終了して再試行してください")
        case .tooShort(let s): L10n.tr("録音が短すぎました(\(seconds(s))秒)。タップしてから話し、話し終えてからもう一度タップしてください")
        case .silent(let s, let db): L10n.tr("音声が小さすぎました(録音\(seconds(s))秒・音量\(Int(db.rounded()))dB)。マイクに向かって話してください")
        }
    }
    static func message(for error: Error) -> String {
        (error as? TranscriberFailure).map(transcriberFailure) ?? error.localizedDescription
    }
    /// toast に添える診断の 1 行要約(`TranscriberDiagnostics.summary` の表示言語対応版)
    static func diagnosticsSummary(_ d: TranscriberDiagnostics) -> String {
        var parts = [L10n.tr("録音\(seconds(d.recordedSeconds))秒")]
        parts.append(d.peakAmplitude > 0 ? L10n.tr("音量\(Int(d.peakDecibels.rounded()))dB") : L10n.tr("無音"))
        if !d.inputRoute.isEmpty { parts.append(L10n.tr("入力:\(d.inputRoute)")) }
        return parts.joined(separator: L10n.tr("・"))
    }
    private static func seconds(_ s: Double) -> String { String(format: "%.1f", s) }

    // ボタン
    static var retry: String { L10n.tr("もう一度") }
    static var sendNow: String { L10n.tr("今すぐ送る") }
    static var discard: String { L10n.tr("やめる") }
    static var dismiss: String { L10n.tr("閉じる") }
    static var signIn: String { L10n.tr("サインイン") }
    static var edit: String { L10n.tr("言い直す") }
}
