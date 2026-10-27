import Foundation
import MetamonServices

/// 新着(#268): アプリを開いた時(起動・前面復帰・切替)に、前回より後の他人の共有レコードを数える。
/// 表示は `NewItemsHost`。モックは stamp を取る口が無いので出さない(DEBUG の `METAMON_MOCK_NEW_ITEMS` だけ例外)。
extension AppRuntime {
    var newItemsSource: (any SharedRecordStampSource)? {
        #if DEBUG
        if let mock = DebugMockNewItems.source { return mock }
        #endif
        return services.appConfig as? SharedRecordStampSource
    }

    /// 出す件数(0 / 初回 / 取れない時は nil)。
    func checkNewItems(store: LastOpenedStore = LastOpenedStore()) async -> Int? {
        guard let source = newItemsSource, let uid = services.auth.currentUser?.uid else { return nil }
        let appId = document.appId
        #if DEBUG
        DebugMockNewItems.prepare(store: store, appId: appId)
        #endif
        let count = await NewItemsCheck(source: source, store: store)
            .run(appId: appId, entities: dsl.entities.map(\.id), uid: uid)
        guard document.appId == appId, let count, count > 0 else { return nil }
        print("[Metamon] newItems: app=\(appId) count=\(count)")
        return count
    }
}
