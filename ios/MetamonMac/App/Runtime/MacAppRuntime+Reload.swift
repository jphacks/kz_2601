import Foundation
import MetamonDSL
import MetamonRenderer

/// 表示 > 再読み込み ⌘R(#944)。iOS の「下に引いて更新」(`ScreenView` の refreshable)の代わりに、
/// いちばん上の画面が読む記録を読み直す。
extension MacAppRuntime {
    /// いちばん上の画面が読む entity(`ScreenView` の読み込みと同じ)
    var menuReloadEntities: Set<String> {
        let top = NavigationResolver.top(navigationState, dsl: dsl)
        guard let screen = dsl.screen(top.screen) else { return [] }
        return dsl.entitiesNeeded(by: screen)
    }

    /// 記録を読む画面だけ押せる。確認ダイアログの間は押せない
    var canReloadFromMenu: Bool {
        !isShowingDialog && !menuReloadEntities.isEmpty
    }

    func reloadFromMenu() async {
        guard canReloadFromMenu else { return }
        await load(entities: menuReloadEntities, force: true)
    }
}
