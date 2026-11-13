// 審査用ビルド(APP_STORE_REVIEW)は共有リンクの入口を外す(iOS の AppRuntime+Sharing.swift と同じ)
#if !APP_STORE_REVIEW
import Foundation
import MetamonServices

/// 外から届いた URL のうち、アプリが開くもの(#975)。読み方は iOS と同じ `ShareURL.token`。
/// - `tailorit://import/<token>`: URL スキーム(QR・共有ページの「アプリで開く」)
/// - `tailorit://share-inbox`: 共有シートの拡張が受け取った物を置いた(#2224。定数は ios/MetamonShare/Shared/ShareInbox.swift)
/// - Web 閲覧の NSUserActivity: 中の URL を同じ規則で読む。`ShareURL` は Universal Links の `https://tailor-it.app/s/<token>` も受ける(#331)
///   (associated domains は iOS だけ。Mac には無いので、Mac に届くのは Handoff などで Web 閲覧の activity が渡った時)
/// それ以外(知らない URL)は nil。iOS と同じく何もしない。
enum IncomingLink: Equatable {
    case share(token: String)
    case shareInbox

    init?(url: URL) {
        if let token = ShareURL.token(from: url) {
            self = .share(token: token)
            return
        }
        guard url.scheme?.lowercased() == ShareURL.scheme, url.host?.lowercased() == "share-inbox" else { return nil }
        self = .shareInbox
    }

    init?(activity: NSUserActivity) {
        guard activity.activityType == NSUserActivityTypeBrowsingWeb, let url = activity.webpageURL else { return nil }
        self.init(url: url)
    }
}
#endif
