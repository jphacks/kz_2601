package jp.kz2601.metamon.renderer.components.social

import jp.kz2601.metamon.dsl.records.Record

/**
 * 投稿に出す名前を聞く口(iOS の `RecordPosting` と `AuthorNamePrompt`・#250)。:app の AppRuntime が実装する。
 * 持たない runtime(プレビュー・テスト)は聞かずにそのまま [jp.kz2601.metamon.renderer.DSLRuntime.addRecord] で送る。
 */
interface AuthorNameHost {
    /** 送る前に名前を聞くか(共有の entity への初めての投稿で、まだ聞いていない)。 */
    fun needsAuthorName(entity: String): Boolean

    /** 聞いた答え。null は「匿名で投稿」。どちらでも聞いたことにする。 */
    fun answerAuthorName(name: String?)
}

/**
 * 他人の共有レコードの通報・ブロック(iOS の `ModerationHost`・#321)。持たない runtime ではメニューを出さない。
 * 理由の選択・確認・結果の一言はホストが出す(長押しメニューは項目を並べるだけ)。
 */
interface ModerationHost {
    /** 通報・ブロックできるか(シェアの無い版・自分の投稿・private の entity は false)。 */
    fun canReport(entity: String, record: Record): Boolean

    /** 通報の理由を選ぶ画面をホストが出す。 */
    fun reportRecord(entity: String, record: Record)

    /** 作者のブロックの確認をホストが出す。 */
    fun blockAuthor(entity: String, record: Record)
}
