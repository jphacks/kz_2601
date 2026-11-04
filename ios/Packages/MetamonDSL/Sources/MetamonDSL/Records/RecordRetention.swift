import Foundation

/// 記録の保存期間(#482 `Entity.retentionDays`)。期限を過ぎたレコードの判定と、設定の確認状態だけを持つ(削除は呼び出し側)。
///
/// - 期間は `Record.createdAt`(作った日時)から数える。createdAt が無い・読めないレコードは消さない
/// - 消すのは自分が消せるレコードだけ: private は全部、shared / public は `author` が自分の uid と一致するものだけ
///   (author の無い旧データ・uid が分からない時は消さない。手で消す時の `RecordPermission` より厳しい)
/// - 消したレコードは戻せないので、ユーザーが確認した期間(`consented`)より短い設定は確認が済むまで消さない
public enum RecordRetention {
    public static let secondsPerDay: TimeInterval = 86_400

    /// この entity で今消してよい期間(日数)。設定が無い・未確認・確認した期間より短くなった時は nil。
    /// 確認した期間以上に延ばした設定は、消す量が減るだけなので確認し直さずに使う。
    public static func activeDays(_ entity: Entity, consented: Int?) -> Int? {
        guard let days = entity.retentionDays, let consented, consented <= days else { return nil }
        return days
    }

    /// 確認が要る entity(設定があり、未確認か確認した期間より短い)。DSL の順。
    public static func pending(_ dsl: AppDSL, consented: [String: Int]) -> [Entity] {
        dsl.entities.filter { e in e.retentionDays != nil && activeDays(e, consented: consented[e.id]) == nil }
    }

    /// 期限切れの境目。これより前に作ったレコードが消える。
    public static func cutoff(days: Int, now: Date) -> Date {
        now.addingTimeInterval(-Double(days) * secondsPerDay)
    }

    /// 期限を過ぎていて、自分が消せるレコード。
    public static func expired(_ records: [Record], in entity: Entity, days: Int, now: Date, uid: String?) -> [Record] {
        let limit = cutoff(days: days, now: now)
        return records.filter { r in
            guard let created = ChatThread.date(r.createdAt), created < limit else { return false }
            return isOwn(r, in: entity, uid: uid)
        }
    }

    /// 自動で消してよい持ち主か。private は全部自分のもの。共有は author が自分の uid の時だけ。
    public static func isOwn(_ record: Record, in entity: Entity, uid: String?) -> Bool {
        guard entity.isShared else { return true }
        guard let uid, !uid.isEmpty, let author = record.author else { return false }
        return author == uid
    }

    /// 画面に出す期間の言い方(「1週間」「30日」)。7 の倍数は週、365 の倍数は年。
    public static func periodLabel(days: Int, english: Bool = false) -> String {
        if days % 365 == 0 {
            let n = days / 365
            return english ? (n == 1 ? "1 year" : "\(n) years") : "\(n)年"
        }
        if days % 7 == 0 && days <= 28 {
            let n = days / 7
            return english ? (n == 1 ? "1 week" : "\(n) weeks") : "\(n)週間"
        }
        return english ? (days == 1 ? "1 day" : "\(days) days") : "\(days)日"
    }
}
