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
            return L10n.pick(L10n.tr("利用が集中したため、新しくアプリを作るのを止めています。\(when)に再開します"), review: L10n.tr("利用が集中したため、新しい記録帳の用意を止めています。\(when)に再開します"))
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
    // 無料プランの一時停止(#317・503 free_plan_paused)。混雑・AI の停止とは分ける
    static var freePlanPausedTitle: String { L10n.tr("無料プランを一時停止しています") }
    static func freePlanPausedBody(resumeAt: Date?, timeZone: TimeZone = .current) -> String {
        guard let resumeAt else {
            return L10n.tr("利用が集中したため、無料プランの機能を一時的に止めています。しばらくしてからもう一度お試しください")
        }
        return L10n.tr("利用が集中したため、無料プランの機能を一時的に止めています。\(resetTime(resumeAt, timeZone: timeZone))に再開します")
    }
    // プランの上限(#317・403 plan_limit_reached)。参加者には課金の導線を出さない(U3/U4)
    static var planLimitTitle: String { L10n.tr("上限に達しました") }
    /// 何の上限か(上限に当たった時・近い時のお知らせで同じ言い方にする)
    static func planLimitResourceName(_ resource: PlanLimit.Resource) -> String {
        switch resource {
        case .apps: L10n.pick(L10n.tr("作れるアプリの数"), review: L10n.tr("作れる記録帳の数"))
        case .recordsPerApp: L10n.tr("このアプリに保存できる記録の数")
        case .photosPerApp: L10n.tr("このアプリに保存できる写真の数")
        case .other: L10n.tr("このアプリで使える量")
        }
    }
    static func planLimitBody(_ limit: PlanLimit) -> String {
        let what = planLimitResourceName(limit.resource)
        if limit.byCreator { return L10n.tr("\(what)が上限に達しました。作った人に知らせてください") }
        if showsProPath(limit) { return L10n.tr("\(what)が無料プランの上限に達しました。使わないものを消すか、Pro にすると増やせます") }
        return L10n.tr("\(what)が上限に達しました。使わないものを消すと、続けて使えます")
    }
    /// 「Pro にする」を出すか。作成者本人だけ(参加者は払っても直らない)。審査用の版は課金を持たない
    static func showsProPath(_ limit: PlanLimit) -> Bool {
        #if APP_STORE_REVIEW
        return false
        #else
        return limit.upgrade && !limit.byCreator
        #endif
    }
    // 運営がアプリの自動の動きを止めている(#346・403 app_stopped)。理由は出さない
    static var appStoppedTitle: String { L10n.tr("自動の動きが止まっています") }
    static var appStoppedBody: String { L10n.tr("このアプリの自動の動きは運営が止めています。この変更は保存できませんでした") }
    // 表示できない言葉(#321・422 text_not_allowed)。どの言葉かは出さない
    static var textNotAllowed: String { L10n.tr("表示できない言葉が含まれています") }
    static func textNotAllowed(fieldName: String) -> String { L10n.tr("「\(fieldName)」に表示できない言葉が含まれています") }
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
        case .speechRecognitionDenied: L10n.tr("音声認識の許可がありません")
        case .onDeviceRecognitionUnavailable: L10n.tr("まだ声で入力できません")
        }
    }
    static func transcriberFailure(_ f: TranscriberFailure) -> String {
        switch f {
        case .notReady: L10n.tr("少し待ってから、もう一度お試しください")
        case .microphoneDenied: L10n.tr("設定アプリ > Tailor It でマイクを許可してください")
        case .audioSession, .noInputAvailable, .noAudioCaptured: L10n.tr("ほかのアプリの通話や録音を終えてから、もう一度お試しください")
        case .tooShort: L10n.tr("タップしてから話し、話し終えたらもう一度タップしてください")
        case .silent: L10n.tr("マイクに近づいて、もう一度話してください")
        case .speechRecognitionDenied: L10n.tr("音声モデルの準備ができるまでは、文字で入力してください")
        case .onDeviceRecognitionUnavailable: L10n.tr("この端末では音声モデルの準備ができるまで声で入力できません。文字で入力してください")
        }
    }
    static var notHeardTitle: String { L10n.tr("うまく聞き取れませんでした") }
    static var notHeardBody: String { L10n.tr("もう一度、はっきり話してください") }

    /// 失敗の「次の一手」。圏外なら接続の確認、それ以外は少し待って再試行。
    static func nextStep(for error: Error) -> String {
        if (error as? APIError)?.isTextNotAllowed == true { return textNotAllowed }
        return switch FailureKind.classify(error) {
        case .offline: offlineInterruptedBody
        case .sessionExpired: sessionExpiredBody
        case .appCheckFailed: appCheckFailedBody
        case .aiPaused: aiPausedBody
        case .freePlanPaused(let resumeAt): freePlanPausedBody(resumeAt: resumeAt)
        case .planLimit(let limit): planLimitBody(limit)
        case .appLimit(let limit): AppLimitCopy.toast(limit)
        case .appStopped: appStoppedBody
        default: otherBody
        }
    }
    /// 「〇〇できませんでした。次の一手」。エラーの中身(英語の OS の文・サーバーの文)はログにだけ残す。
    static func failure(_ what: String, _ error: Error, context: String) -> String {
        ErrorLog.record(error, context: context)
        return L10n.tr("\(what)。\(nextStep(for: error))")
    }

    /// 設定のアカウント削除・ログアウトの失敗の本文(題は alert 側)。Firebase の英語の文は出さず、ログにだけ残す。
    static func accountFailureBody(_ error: Error, context: String) -> String {
        ErrorLog.record(error, context: context)
        if case AuthError.requiresRecentLogin = error { return L10n.tr("もう一度サインインしてから削除してください") }
        return nextStep(for: error)
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
    static var signInApple: String { L10n.tr("Apple でサインイン") }
    static var upgradePro: String { L10n.tr("Pro にする") }
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
