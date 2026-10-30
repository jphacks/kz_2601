import MetamonDSL

/// 同じ画面に「同じフォームを開く追加」を 2 つ出さない(#416)。
/// toolbar(右上の＋)と button が同じフォーム画面(`screen.entity`)を同じ `prefillRef` で開くなら、toolbar 側を出さない。
/// 押しやすい右下の button を残す。保存済みの DSL にも効く(Go の `dsl.App.DuplicateAddToolbar` と同じ判定)。
enum DuplicateAddControl {
    private struct Opener: Hashable {
        let screen: String
        let prefillRef: String?
    }

    /// ナビバーに出してよい toolbar 項目。
    static func toolbar(of screen: Screen, in dsl: AppDSL) -> [ToolbarButton] {
        let buttons = Set(screen.allComponents.compactMap { c -> Opener? in
            guard c.type == .button, let a = c.action, a.type == .navigate, let target = a.screen else { return nil }
            return Opener(screen: target, prefillRef: a.prefillRef)
        })
        guard !buttons.isEmpty else { return screen.toolbar }
        return screen.toolbar.filter { tb in
            let a = tb.action
            guard a.type == .navigate, let target = a.screen, dsl.screen(target)?.entity != nil else { return true }
            return !buttons.contains(Opener(screen: target, prefillRef: a.prefillRef))
        }
    }
}
