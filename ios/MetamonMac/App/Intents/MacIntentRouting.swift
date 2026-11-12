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
        guard let action = host.take() else { return }
        MacIntentRoute.apply(action, appIds: appIds, select: { selection = $0 }, talk: talk)
    }
}

/// 受け取った Intent を画面の操作に振り分ける(単体テストで確かめる)
enum MacIntentRoute {
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
