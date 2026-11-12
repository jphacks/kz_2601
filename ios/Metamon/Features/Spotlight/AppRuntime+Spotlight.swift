import CoreSpotlight
import SwiftUI

extension AppRuntime {
    /// 起動時: 一覧を黙って取り、索引を合わせる(失敗してもトーストは出さない)
    func refreshSpotlight() async {
        guard let list = try? await services.appConfig.listApps() else { return }
        await spotlight.sync(list)
    }
}

/// WindowGroup の根に付ける(#1996): Spotlight の結果のタップでその生成アプリを開く。
/// Siri の「開く」と同じ `IntentHost` の道(サインイン・bootstrap の前に届いたら準備の後に開く)。
struct SpotlightOpenReceiver: ViewModifier {
    func body(content: Content) -> some View {
        content.onContinueUserActivity(CSSearchableItemActionType) { activity in
            guard let appId = SpotlightID.appId(from: activity) else { return }
            Task { await IntentHost.shared.request(.open(appId: appId)) }
        }
    }
}
