package jp.kz2601.metamon.renderer.components.social

import jp.kz2601.metamon.dsl.records.MarkState

/**
 * 共有の記録の印(いいね・保存 #255・iOS の `MarksHost`)。:app の AppRuntime が実装する。持たない runtime では印を出さない。
 */
interface MarksHost {
    /** 付ける / 外す。確定した状態を返す。失敗(オフライン・未送信の行など)は null で、ストアは元の値に戻っている。 */
    suspend fun toggleMark(entity: String, recordId: String, mark: String): MarkState?
}
