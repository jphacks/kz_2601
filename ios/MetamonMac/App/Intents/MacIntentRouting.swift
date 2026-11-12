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
        guard let resolution = host.resolve(among: appIds, listSettled: listSettled) else { return }
        MacIntentRoute.apply(resolution, select: { selection = $0 }, talk: talk, missing: missing)
    }
}

/// 受け取った Intent を画面の操作に振り分ける(単体テストで確かめる)
enum MacIntentRoute {
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
