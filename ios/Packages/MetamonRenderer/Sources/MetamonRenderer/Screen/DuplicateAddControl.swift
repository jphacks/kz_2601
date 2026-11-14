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

    /// 空の一覧の「最初の 1 件を追加しましょう」を押した時の動き(M12)。同じ画面にある、その entity のフォーム画面を開く
    /// button(先)/ toolbar(出している分)と同じ navigate を返す。無ければ nil(案内はただの文字のまま)。
    static func emptyListAdd(entity: String, on screen: Screen, in dsl: AppDSL) -> Action? {
        let opens: (Action?) -> Bool = { a in
            guard let a, a.type == .navigate, let target = a.screen else { return false }
            return dsl.screen(target)?.entity == entity
        }
        if let button = screen.allComponents.first(where: { $0.type == .button && opens($0.action) }) { return button.action }
        return toolbar(of: screen, in: dsl).first { opens($0.action) }?.action
    }
}
