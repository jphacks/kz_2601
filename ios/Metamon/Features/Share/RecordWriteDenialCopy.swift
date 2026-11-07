import MetamonDSL
import MetamonServices

/// アプリの決まりで書き込みが断られた時の文言(#272 #273)。審査用の版でも起きうるので外さない。
extension AppRuntime {
    func denialMessage(_ denial: RecordWriteDenial, entity: String) -> String {
        let ent = dsl.entity(entity)
        switch denial {
        case .adminsOnly(let field):
            guard let name = field.flatMap({ ent?.field($0)?.name }) else { return L10n.tr("管理者だけができる操作です") }
            return L10n.tr("「\(name)」は管理者だけが変更できます")
        case .duplicate(let fields):
            if fields == ["$author"] { return L10n.tr("1 人 1 件までです。もう登録しています") }
            let names = fields.filter { $0 != "$author" }.compactMap { ent?.field($0)?.name }
            guard !names.isEmpty else { return L10n.tr("同じ内容の記録がもうあります") }
            return L10n.tr("「\(names.joined(separator: "・"))」が同じ記録がもうあります")
        }
    }
}
