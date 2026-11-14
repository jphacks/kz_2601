import Foundation
import MetamonRenderer

/// 「破棄」で進める操作(#943)。
struct DiscardRequest: Identifiable {
    let id = UUID()
    let perform: () -> Void
}

/// 保存していない入力を確認なしで失わない(#943・iOS の M10 と同じ考え方)。
/// フォームの閉じるボタン・戻るボタンは `ScreenView` が確認する。ここは Esc・⌘[・⌘W・アプリの切り替えのように外から閉じる操作。
extension MacAppRuntime {
    /// 今も出ているフォームのうち `leaving`(閉じる画面)に当たるものに、保存していない入力があるか。
    func hasUnsavedInput(leaving: (Route) -> Bool = { _ in true }) -> Bool {
        let state = navigationState
        return !openForms.unsavedRoutes { route in
            leaving(route) && NavigationResolver.isPresented(route, in: state, dsl: dsl)
        }.isEmpty
    }

    /// 入力を失うなら確認を出し、「破棄」で `action`。失わなければすぐ `action`。
    func confirmDiscard(leaving: (Route) -> Bool = { _ in true }, then action: @escaping () -> Void) {
        if hasUnsavedInput(leaving: leaving) {
            discardRequest = DiscardRequest(perform: action)
        } else {
            action()
        }
    }

    /// Esc: いちばん上のモーダルを閉じる。中のフォームに入力があれば先に確認する。
    func requestDismissTopModal() {
        let stack = fullScreen != nil ? "full" : "sheet"
        confirmDiscard(leaving: { $0.stack == stack }) { [weak self] in self?.dismissTopModal() }
    }

    /// ⌘[: 1 つ戻る。戻って消える画面(積まれた最上段・無ければモーダルごと)のフォームに入力があれば先に確認する。
    func requestGoBack() {
        guard canGoBack else { return }
        let stack = topStack
        let last = paths[stack]?.last
        confirmDiscard(leaving: { route in last.map { route == $0 } ?? (route.stack == stack) }) { [weak self] in
            self?.goBack()
        }
    }
}
