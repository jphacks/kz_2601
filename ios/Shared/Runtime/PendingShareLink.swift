// 審査用ビルド(APP_STORE_REVIEW)は共有リンクの入口を外す(iOS の AppRuntime+Sharing.swift と同じ)
#if !APP_STORE_REVIEW
import Foundation
import MetamonServices

/// 受け取った共有のリンクの保留(#278・#1287)。iOS の `SharingUI.pendingLink` / `takeLink` と同じ決まりで、UIKit に依存しない。
/// サインインの前・準備の前に届いたリンクも捨てずに 1 つ持ち、準備ができてから 1 回だけ渡す。共有のリンクでない URL は持たない。
/// 今は Mac(`MacShareFlow`)が使う。iOS の `SharingUI` も同じ決まり(置き換えは別の PR)
struct PendingShareLink: Equatable {
    private(set) var url: URL?

    /// 共有のリンクなら保留して true。それ以外の URL は false(触らない)
    @discardableResult
    mutating func receive(_ url: URL) -> Bool {
        guard ShareURL.token(from: url) != nil else { return false }
        self.url = url
        return true
    }

    /// Universal Links(#331)。Web 閲覧の activity の URL が共有のリンクなら保留して true
    @discardableResult
    mutating func receive(activity: NSUserActivity) -> Bool {
        guard activity.activityType == NSUserActivityTypeBrowsingWeb, let url = activity.webpageURL else { return false }
        return receive(url)
    }

    /// 準備ができていれば保留を渡して空にする。まだなら nil(保留は残す)
    mutating func take(isReady: Bool) -> URL? {
        guard isReady, let url else { return nil }
        self.url = nil
        return url
    }
}
#endif
