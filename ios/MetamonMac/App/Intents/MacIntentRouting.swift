import SwiftUI

extension View {
    /// ショートカット・Siri・Spotlight の「開く / 話しかける」(#1034)を受け取る。
    /// 開く → 一覧の選択(押された通知と同じ経路: 入力中なら確認・シートの間は閉じるまで待つ)、話しかける → ⇧⌘M と同じ会話。
    func macIntentRouting(selection: Binding<String?>, appIds: [String], talk: @escaping () -> Void) -> some View {
        modifier(MacIntentRouting(selection: selection, appIds: appIds, talk: talk))
    }
}

private struct MacIntentRouting: ViewModifier {
    @Binding var selection: String?
    let appIds: [String]
    let talk: () -> Void
    private var host: IntentHost { .shared }

    func body(content: Content) -> some View {
        content
            .onAppear(perform: route)
            .onChange(of: host.pending) { route() }
    }

    private func route() {
        guard host.pending != nil else { return }
        Task { @MainActor in
            // 起動時の同意・チュートリアル・シェアリンクが閉じてから(シートを同時に出さない・#1885)
            guard let action = await MacIntentRoute.takeAfterLaunch(host.take) else { return }
            MacIntentRoute.apply(action, appIds: appIds, select: { selection = $0 }, talk: talk)
        }
    }
}

/// 受け取った Intent を画面の操作に振り分ける(単体テストで確かめる)
enum MacIntentRoute {
    /// 起動時の段(同意 → チュートリアル → シェアリンク)が全部終わるまで待ってから受け取る(iOS の `IntentHost.perform` と同じ順・#1885)。
    /// 待っている間に別の Intent が来ても、受け取るのは最後の 1 つ(先に受け取った方は nil)
    @MainActor
    static func takeAfterLaunch(_ take: () -> IntentHost.Pending?, hold: LaunchConsentHold = .shared,
                                splash: SplashPresence = .shared) async -> IntentHost.Pending? {
        await splash.waitUntilHidden()
        await hold.waitUntilReleased()
        return take()
    }

    @MainActor
    static func apply(_ action: IntentHost.Pending, appIds: [String], select: (String) -> Void, talk: () -> Void) {
        switch action {
        case .open(let appId):
            // 一覧に無い(消した・サーバーの一覧をまだ取っていない)時は前に出すだけ
            if appIds.contains(appId) { select(appId) }
        case .talk:
            talk()
        }
    }
}
