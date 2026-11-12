import SwiftUI

extension View {
    /// ショートカット・Siri・Spotlight・ウィジェットの「開く / 話しかける」(#1034)を受け取る。
    /// 開く → 一覧の選択(押された通知と同じ経路: 入力中なら確認・シートの間は閉じるまで待つ)、話しかける → ⇧⌘M と同じ会話。
    /// 一覧を読み終える前に来た「開く」は、読み終えて(`listSettled`)から開く。それでも無ければ `missing`(#1970)
    func macIntentRouting(selection: Binding<String?>, appIds: [String], listSettled: Bool,
                          talk: @escaping () -> Void, missing: @escaping (String) -> Void) -> some View {
        modifier(MacIntentRouting(selection: selection, appIds: appIds, listSettled: listSettled, talk: talk, missing: missing))
    }
}

private struct MacIntentRouting: ViewModifier {
    @Binding var selection: String?
    let appIds: [String]
    let listSettled: Bool
    let talk: () -> Void
    let missing: (String) -> Void
    private var host: IntentHost { .shared }

    func body(content: Content) -> some View {
        content
            .onAppear(perform: route)
            .onChange(of: host.pending) { route() }
            .onChange(of: appIds) { route() } // 起動直後: 一覧が載ってから開く(#1970)
            .onChange(of: listSettled) { route() }
            .task(id: host.requestSerial) { // 一覧が読み終わらないまま待つ長さを過ぎたら諦める(同じ「開く」が来ても測り直す)
                while let remaining = host.openWaitRemaining {
                    try? await Task.sleep(for: .seconds(remaining))
                    guard !Task.isCancelled else { return }
                    route()
                }
            }
    }

    private func route() {
        guard host.pending != nil else { return }
        let appIds = appIds, listSettled = listSettled
        Task { @MainActor in
            // 起動時の同意・チュートリアル・シェアリンクが閉じてから(シートを同時に出さない・#1885)
            // 一覧を読み終える前の「開く」は残る(読み終えたら route() がまた呼ばれる・#1970)
            guard let resolution = await MacIntentRoute.takeAfterLaunch({
                host.resolve(among: appIds, listSettled: listSettled)
            }) else { return }
            MacIntentRoute.apply(resolution, select: { selection = $0 }, talk: talk, missing: missing)
        }
    }
}

/// 受け取った Intent を画面の操作に振り分ける(単体テストで確かめる)
enum MacIntentRoute {
    /// 起動時の段(同意 → チュートリアル → シェアリンク)が全部終わるまで待ってから受け取る(iOS の `IntentHost.perform` と同じ順・#1885)。
    /// 待っている間に別の Intent が来ても、受け取るのは最後の 1 つ(先に受け取った方は nil)
    @MainActor
    static func takeAfterLaunch<T>(_ take: () -> T?, hold: LaunchConsentHold = .shared,
                                   splash: SplashPresence = .shared) async -> T? {
        await splash.waitUntilHidden()
        await hold.waitUntilReleased()
        return take()
    }

    @MainActor
    static func apply(_ resolution: IntentHost.Resolution, select: (String) -> Void, talk: () -> Void,
                      missing: (String) -> Void) {
        switch resolution {
        case .select(let appId): select(appId)
        case .talk: talk()
        case .missing(let appId): missing(appId)
        }
    }
}
