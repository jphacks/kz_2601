// 審査用ビルド(APP_STORE_REVIEW)ではプッシュごと外す(#269)。
#if !APP_STORE_REVIEW
import SwiftUI

/// 設定の「他の人の行動で届く通知」(#339)。楽観的 UI(先に反映し、失敗したら戻して知らせる)。
struct PushConsentSection: View {
    @Environment(AppRuntime.self) private var rt
    @State private var available = false
    @State private var isOn = false
    @State private var failed = false

    var body: some View {
        Group {
            if available {
                Section {
                    Toggle("このアプリの通知を受け取る", isOn: Binding(get: { isOn }, set: change))
                        .accessibilityIdentifier("settings.pushConsent")
                } header: {
                    Text("他の人の行動で届く通知")
                } footer: {
                    if failed {
                        Text("変更できませんでした。元に戻しました")
                    } else {
                        Text("他の人の書き込みなどで届く通知です。オフにすると、このアプリからは届きません。")
                    }
                }
            }
        }
        .task(id: rt.document.appId) {
            isOn = rt.notifyConsentAllowed
            available = await rt.canSetNotifyConsent()
        }
    }

    private func change(_ new: Bool) {
        isOn = new
        failed = false
        let appId = rt.document.appId
        Task {
            guard await !rt.setNotifyConsent(new, appId: appId), rt.document.appId == appId else { return }
            isOn = !new
            failed = true
        }
    }
}
#endif
