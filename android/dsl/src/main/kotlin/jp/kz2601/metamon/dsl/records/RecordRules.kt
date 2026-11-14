package jp.kz2601.metamon.dsl.records

import java.time.Duration
import java.time.Instant

/**
 * single の entity(設定値の置き場・#267)の 1 件。ふつうは 0〜1 件。2 件以上ある時は createdAt が最も新しい 1 件(同じなら後の方)。
 * iOS の SingleRecord.swift・backend の `expreval.SingleRecord` と同じ。
 */
object SingleRecord {
    fun pick(records: List<Record>): Record? {
        var best: Record? = null
        for (r in records) if (best == null || (r.createdAt ?: "") >= (best.createdAt ?: "")) best = r
        return best
    }

    /** 保存前の 1 件の data: 保存する field の defaultValue(date / datetime の "now" は当てない)。 */
    fun defaults(entity: Entity): Map<String, JSONValue> {
        val out = LinkedHashMap<String, JSONValue>()
        for (f in entity.storedFields) defaultValue(f)?.let { out[f.id] = it }
        return out
    }

    /** field の defaultValue(式で読む値)。null・date の "now" は null。 */
    fun defaultValue(f: Field): JSONValue? {
        val v = f.defaultValue ?: return null
        if (v.isNull) return null
        if (f.type.isDate && v.stringValue == "now") return null
        return v
    }
}

/**
 * 共有レコードを自分が変えられるか(#249 / #252)。appconfig の権限と同じ規則:
 * - private の entity は全部自分のもの
 * - shared / public は作った人だけが更新・削除できる。`edit:"members"` なら更新は誰でも、削除は作った人だけ
 * uid が分からない時と author が無い時は止めない(サーバーが最後に判定する)。
 */
object RecordPermission {
    fun canEdit(record: Record, entity: Entity?, uid: String?): Boolean =
        isMine(record, entity, uid) || (entity?.membersCanEdit ?: false)

    /** `entity.deleteBy`(#272): "admins" は管理者だけ、"authorOrAdmins" は作った人か管理者。isAdmin が分からない時は作った人だけ。 */
    fun canDelete(record: Record, entity: Entity?, uid: String?, isAdmin: Boolean? = null): Boolean {
        val mine = isMine(record, entity, uid)
        if (entity == null || !entity.isShared) return mine
        return when (entity.deleteBy) {
            "admins" -> isAdmin ?: mine
            "authorOrAdmins" -> mine || isAdmin == true
            else -> mine
        }
    }

    private fun isMine(record: Record, entity: Entity?, uid: String?): Boolean {
        if (entity == null || !entity.isShared || uid == null) return true
        val author = record.author ?: return true
        return author == uid
    }
}

/**
 * 記録の保存期間(#482 `Entity.retentionDays`)。期限を過ぎたレコードの判定と確認状態だけを持つ(削除は呼び出し側)。
 * - 期間は `Record.createdAt` から数える。無い・読めないレコードは消さない
 * - 消すのは自分が消せるレコードだけ: private は全部、shared / public は author が自分の uid のものだけ
 * - ユーザーが確認した期間(`consented`)より短い設定は確認が済むまで消さない
 */
object RecordRetention {
    const val SECONDS_PER_DAY = 86_400L

    /** この entity で今消してよい期間(日数)。設定が無い・未確認・確認した期間より短くなった時は null。 */
    fun activeDays(entity: Entity, consented: Int?): Int? {
        val days = entity.retentionDays ?: return null
        if (consented == null || consented > days) return null
        return days
    }

    /** 確認が要る entity(設定があり、未確認か確認した期間より短い)。DSL の順。 */
    fun pending(entities: List<Entity>, consented: Map<String, Int>): List<Entity> =
        entities.filter { it.retentionDays != null && activeDays(it, consented[it.id]) == null }

    /** 期限切れの境目。これより前に作ったレコードが消える。 */
    fun cutoff(days: Int, now: Instant): Instant = now.minus(Duration.ofSeconds(days * SECONDS_PER_DAY))

    /** 期限を過ぎていて、自分が消せるレコード。 */
    fun expired(records: List<Record>, entity: Entity, days: Int, now: Instant, uid: String?): List<Record> {
        val limit = cutoff(days, now)
        return records.filter { r ->
            val created = r.createdAt?.let { FieldCoding.strictDatetime(it) } ?: return@filter false
            created < limit && isOwn(r, entity, uid)
        }
    }

    /** 自動で消してよい持ち主か。private は全部自分のもの。共有は author が自分の uid の時だけ。 */
    fun isOwn(record: Record, entity: Entity, uid: String?): Boolean {
        if (!entity.isShared) return true
        if (uid.isNullOrEmpty()) return false
        val author = record.author ?: return false
        return author == uid
    }

    /** 画面に出す期間の言い方(「1週間」「30日」)。7 の倍数(28 日まで)は週、365 の倍数は年。 */
    fun periodLabel(days: Int, english: Boolean = false): String {
        if (days % 365 == 0) {
            val n = days / 365
            return if (english) (if (n == 1) "1 year" else "$n years") else "${n}年"
        }
        if (days % 7 == 0 && days <= 28) {
            val n = days / 7
            return if (english) (if (n == 1) "1 week" else "$n weeks") else "${n}週間"
        }
        return if (english) (if (days == 1) "1 day" else "$days days") else "${days}日"
    }
}

/**
 * 精算(割り勘の「誰が誰にいくら払うか」・#101 `settlement`)。balance は 正 = 受け取る / 負 = 払う。
 * 貪欲法: 最大債務者 → 最大債権者へ min(|債務|, 債権) を移す、を繰り返す。`unit`(既定 1 = 円)未満は 0 とみなす。
 */
object Settlement {
    data class Transfer(val from: String, val to: String, val amount: Double)

    fun transfers(balances: List<Pair<String, Double>>, unit: Double = 1.0): List<Transfer> {
        val eps = unit / 2
        val debtors = balances.filter { it.second < -eps }.map { it.first to -it.second }.toMutableList()
        val creditors = balances.filter { it.second > eps }.map { it.first to it.second }.toMutableList()
        val out = mutableListOf<Transfer>()
        while (true) {
            debtors.sortByDescending { it.second }
            creditors.sortByDescending { it.second }
            val d = debtors.firstOrNull() ?: break
            val c = creditors.firstOrNull() ?: break
            if (d.second <= eps || c.second <= eps) break
            val x = minOf(d.second, c.second)
            val rounded = jp.kz2601.metamon.dsl.expr.SwiftNumbers.rounded(x / unit) * unit
            if (rounded > 0) out.add(Transfer(d.first, c.first, rounded))
            debtors[0] = d.first to d.second - x
            creditors[0] = c.first to c.second - x
            debtors.removeAll { it.second <= eps }
            creditors.removeAll { it.second <= eps }
        }
        return out
    }
}
