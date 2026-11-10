package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.ActionType

/** AppRuntime が出す文言(iOS の L10n キーと同じ日本語)。 */
object RuntimeCopy {
    const val NO_FORM = "保存するフォームがありません"
    const val READ_ONLY = "このデータは作った人だけが変更できます"
    const val SAVE_FAILED = "保存できませんでした。通信を確かめてもう一度お試しください"
    const val OPEN_URL_REJECTED = "openUrl: url は https か line/slack/discord/tel/mailto/maps のみ"
    const val SAVE_PARENT_FIRST = "先に保存してください"
    const val REJECTED_GENERIC = "保存できませんでした。入力を確認して、もう一度お試しください"
    const val CONFLICT = "他の端末で変更されていたため、サーバーの内容に合わせました"
    const val NOT_MEMBER = "この共有に参加していません。招待リンクから参加してください"
    const val APP_STOPPED = "このアプリの自動の動きは運営が止めています。この変更は保存できませんでした"
    const val TEXT_NOT_ALLOWED = "表示できない言葉が含まれています"
    const val ADMINS_ONLY = "管理者だけができる操作です"
    const val ONE_PER_PERSON = "1 人 1 件までです。もう登録しています"
    const val DUPLICATE = "同じ内容の記録がもうあります"
    const val NOTIFICATION_FAILED = "通知を予約できませんでした"
    const val FAILED = "うまくいきませんでした"
    const val DELETED = "削除しました"
    const val UNDO = "元に戻す"
    const val UNAVAILABLE_ON_ANDROID = "この機能は Android ではまだ使えません"
    const val NAME_SEPARATOR = "・"

    fun missingRequired(names: List<String>) = "${names.joinToString(NAME_SEPARATOR)} を入力してください"

    fun saveParentFirst(name: String) = if (name.isEmpty()) SAVE_PARENT_FIRST else "先に「$name」を保存してください"

    fun rejected(fieldName: String) = "保存できませんでした。「$fieldName」の入力を確認してください"

    fun paused(pending: Int) = "送信に失敗しました(未送信 $pending 件)。接続を確認してください"

    fun notificationScheduled(whenText: String) = "通知を予約しました($whenText)"

    fun unknownAction(rawType: String) = "未対応のアクション: $rawType"

    fun adminsOnly(fieldName: String) = "「$fieldName」は管理者だけが変更できます"

    fun duplicate(names: List<String>) = "「${names.joinToString(NAME_SEPARATOR)}」が同じ記録がもうあります"

    fun textNotAllowed(fieldName: String) = "「$fieldName」に表示できない言葉が含まれています"

    /** 確認ダイアログの主ボタン(iOS の `Confirm.button(for:)`)。 */
    fun confirmButton(type: ActionType): Pair<String, Boolean> = when (type) {
        ActionType.DELETE_RECORD -> "削除" to true
        ActionType.RESET_FORM -> "入力を消す" to true
        ActionType.SAVE_RECORD -> "保存" to false
        ActionType.EXPORT -> "書き出す" to false
        ActionType.NAVIGATE, ActionType.OPEN_URL -> "開く" to false
        else -> "続ける" to false
    }
}
