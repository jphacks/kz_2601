import Foundation
import MetamonDSL
import MetamonRenderer

/// `Field.health`(#167): フォームで新しく保存した値をヘルスケアにも書き込む。
/// レコードの保存(ローカル DB + 同期キュー)とは独立。失敗してもレコードは保存済みで、理由だけトーストで出す。
/// 編集保存では書かない(同じ計測がヘルスケアに二重に入るため)。
extension AppRuntime {
    func writeHealthFields(entity: String, data: [String: JSONValue], isNew: Bool) {
        guard isNew, let e = dsl.entity(entity) else { return }
        let weights = e.fields.filter { $0.health == "weight" }.compactMap { data[$0.id]?.doubleValue }.filter { $0 > 0 }
        guard !weights.isEmpty else { return }
        Task { @MainActor in
            do {
                for kg in weights { try await HealthData.source.saveWeight(kg: kg, at: Date()) }
                toast = L10n.tr("ヘルスケアにも保存しました")
            } catch {
                toast = L10n.tr("ヘルスケアに保存できませんでした(\(error.localizedDescription))")
            }
        }
    }
}
