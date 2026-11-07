import Foundation
import MetamonDSL

/// aiButton の `entity` 入力で AI に渡すレコードを選ぶ。
/// クラウドの AI(オンライン版)に送る時は、shared / public の entity なら自分が書いた記録だけに絞り、
/// 最近の `AITextLimits.maxRecords` 件までにする(`POST /v1/text` と同じ上限)。
/// 端末内 AI はデータが端末を出ないので絞らない。author の無いレコード(private・未送信の自分の分)は自分のもの。
enum AIInputRecords {
    enum SelectError: LocalizedError, Equatable {
        /// 絞った結果、渡せる記録が残らなかった
        case noOwnRecords

        var errorDescription: String? {
            switch self {
            case .noOwnRecords: "AI に渡せるのは自分が書いた記録だけです"
            }
        }
    }

    /// - Parameters:
    ///   - records: 表示用に hidden を落とした後のレコード
    ///   - uid: サインイン中の uid。分からない時は author の付いた記録を渡さない
    static func select(_ records: [Record], in entity: Entity, route: AIGenerateRoute, uid: String?) throws -> [Record] {
        guard route == .online else { return records }
        let own = entity.isShared ? records.filter { isOwn($0, uid: uid) } : records
        if own.isEmpty, !records.isEmpty { throw SelectError.noOwnRecords }
        return Array(own.sorted { ($0.updatedAt ?? $0.createdAt ?? "") > ($1.updatedAt ?? $1.createdAt ?? "") }
            .prefix(AITextLimits.maxRecords))
    }

    static func isOwn(_ record: Record, uid: String?) -> Bool {
        guard let author = record.author, !author.isEmpty else { return true }
        guard let uid, !uid.isEmpty else { return false }
        return author == uid
    }
}
