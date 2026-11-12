import CoreSpotlight
import SwiftUI
import MetamonDSL
import MetamonServices

extension AppRuntime {
    /// 生成で新しいアプリができた時: 一覧を取り直さずにその 1 件を入れる。
    /// 一覧全体は `appsList()`(変身一覧を開いた時など)の結果で合わせる(起動時に Spotlight のためだけに取らない)。
    func indexCreatedApp(_ doc: AppDocument) async {
        guard !doc.dsl.isVoiceOnly else { return }
        let app = AppSummary(id: doc.appId, name: doc.dsl.app.name, currentVersion: doc.version)
        await spotlight.upsert(app, icon: doc.dsl.app.icon)
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
