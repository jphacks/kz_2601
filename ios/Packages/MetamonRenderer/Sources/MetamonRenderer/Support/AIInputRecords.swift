import Foundation
import MetamonDSL

/// aiButton の `entity` 入力で AI に渡すレコードを選ぶ。
/// クラウドの AI(オンライン版)に送る時は、shared / public の entity なら自分が書いた記録だけに絞り、
/// 最近の `AITextLimits.maxRecords` 件までにする(`POST /v1/text` と同じ上限)。
/// 端末内 AI はデータが端末を出ないので絞らない。author(空文字は無いのと同じ)の無いレコードは、サーバーの規則が作った分や古い分もあるので、
/// 端末で作ってまだ送っていないもの(`Record.unsent`)だけを自分の分とみなす。
enum AIInputRecords {
    enum SelectError: LocalizedError, Equatable {
        /// 絞った結果、渡せる記録が残らなかった
        case noOwnRecords

        var errorDescription: String? {
            switch self {
            case .noOwnRecords: L("AI に渡せるのは自分が書いた記録だけです")
            }
        }
    }

    /// - Parameters:
    ///   - records: 表示用に hidden を落とした後のレコード
    ///   - uid: サインイン中の uid。分からない時は author の付いた記録を渡さない
    static func select(_ records: [Record], in entity: Entity, route: AIGenerateRoute, uid: String?) throws -> [Record] {
        guard route == .online else { return records }
        let own = entity.isShared ? records.filter { isOwn($0, uid: uid, shared: true) } : records
        if own.isEmpty, !records.isEmpty { throw SelectError.noOwnRecords }
        return Array(own.sorted { ($0.updatedAt ?? $0.createdAt ?? "") > ($1.updatedAt ?? $1.createdAt ?? "") }
            .prefix(AITextLimits.maxRecords))
    }

    /// author がサインイン中の uid と同じなら自分。author が無い(空も同じ)時は private なら自分、shared / public は未送信の分だけ。
    static func isOwn(_ record: Record, uid: String?, shared: Bool) -> Bool {
        guard let author = record.author, !author.isEmpty else { return !shared || record.unsent }
        guard let uid, !uid.isEmpty else { return false }
        return author == uid
    }
}
