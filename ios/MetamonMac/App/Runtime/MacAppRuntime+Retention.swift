import Foundation
import MetamonDSL
import MetamonServices

/// 記録の保存期間(#482)を Mac で消す(iOS の `AppRuntime+Retention` の `purgeExpired`)。
/// 確認(`RetentionConsent`)は端末ごと・アプリごとなので、Mac は iPhone と別に確認するまで消さない(#979)。
/// 消す時機: アプリを開いた時・確認した時(`didConfirm`)。消すのは `RecordRetention.expired`
/// (private は全部、shared / public は自分が作った記録だけ)。削除は通常の削除と同じ同期キューで送る
extension MacAppRuntime {
    /// 確認した(MacRootView)。開いているランタイムが消し直す。object は appId
    static let retentionConfirmed = Notification.Name("MacAppRuntime.retentionConfirmed")

    /// 確認済みの期間がある entity の期限切れを消す
    func purgeExpiredRecords() async {
        await purgeExpiredRecords(uid: MacAccount.shared.session.user?.uid, now: DSLClock.now())
    }

    func purgeExpiredRecords(uid: String?, now: Date) async {
        let consented = RetentionConsent.days(appId: appId)
        for entity in dsl.entities {
            guard let days = RecordRetention.activeDays(entity, consented: consented[entity.id]) else { continue }
            let rows = await recordSync.cached(appId: appId, entity: entity.id)
            let gone = RecordRetention.expired(rows, in: entity, days: days, now: now, uid: uid)
            guard !gone.isEmpty else { continue }
            for r in gone { _ = await recordSync.delete(appId: appId, entity: entity.id, recordId: r.id) }
            print("[Metamon] Mac retention: deleted \(gone.count) record(s) older than \(days)d entity=\(entity.id)")
            if records[entity.id] != nil { await reloadLocal(entity.id) } // 未表示の entity は開いた時に読む
        }
    }
}
