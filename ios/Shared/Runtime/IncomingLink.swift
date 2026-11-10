// 審査用ビルド(APP_STORE_REVIEW)は共有リンクの入口を外す(iOS の AppRuntime+Sharing.swift と同じ)
#if !APP_STORE_REVIEW
import Foundation
import MetamonServices

/// 外から届いた URL のうち、アプリが開くもの(#975)。読み方は iOS と同じ `ShareURL.token`。
/// - `tailorit://import/<token>`: URL スキーム(QR・共有ページの「アプリで開く」)
/// - Web 閲覧の NSUserActivity: 中の URL を同じ規則で読む。`ShareURL` は https を受けないので、今は常に nil
///   (iOS・Mac とも associated domains が無く Universal Links は未対応。https の共有ページは「アプリで開く」で tailorit:// に渡す)
/// それ以外(知らない URL)は nil。iOS と同じく何もしない。
enum IncomingLink: Equatable {
    case share(token: String)

    init?(url: URL) {
        guard let token = ShareURL.token(from: url) else { return nil }
        self = .share(token: token)
    }

    init?(activity: NSUserActivity) {
        guard activity.activityType == NSUserActivityTypeBrowsingWeb, let url = activity.webpageURL else { return nil }
        self.init(url: url)
    }
}
#endif
