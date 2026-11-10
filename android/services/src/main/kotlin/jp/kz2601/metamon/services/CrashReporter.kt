package jp.kz2601.metamon.services

/**
 * クラッシュ報告の送り先(iOS の `CrashReporter`・docs/ios/crash-reporting.md)。
 * 送るのは SDK が自動で集めるクラッシュの情報と uid だけ。話した内容・入力した文・記録・DSL を送らないため、
 * `log` やカスタムキーの口はわざと持たせない。足す時はプライバシーポリシーを先に直す。
 */
interface CrashReporter {
    /** オン / オフ(設定の切り替えは android-053)。オフにすると未送信の報告も消す */
    fun setCollectionEnabled(enabled: Boolean)

    fun setUserId(uid: String?)
}

/** Firebase が無い時(google-services.json 無し・テスト)。何もしない。 */
object NoopCrashReporter : CrashReporter {
    override fun setCollectionEnabled(enabled: Boolean) = Unit

    override fun setUserId(uid: String?) = Unit
}
