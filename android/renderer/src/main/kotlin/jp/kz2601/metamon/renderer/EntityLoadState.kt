package jp.kz2601.metamon.renderer

/**
 * entity の記録の読み込み状態(iOS の `EmptyState` の元になる `loading` / `failed`・#1116)。
 * 記録が 0 件の時に、読み込み中・取得失敗・本当に空のどれを出すかを決める。
 */
enum class EntityLoadState {
    /** まだ届いていない、または取得失敗からの読み直し中 */
    LOADING,

    /** 読めた(0 件なら本当に空) */
    LOADED,

    /** サーバーから取れず手元にも無い */
    UNAVAILABLE,
    ;

    companion object {
        /**
         * ランタイムの状態から 1 つの entity の状態を決める。
         * - 取得失敗からの読み直し中は LOADING(押した再試行が効いているのを見せる)
         * - 一度読めた entity の裏での取り直しは LOADED のまま(空の案内と骨組みが行き来しない)
         * - [known] にあるのに記録がまだ無い(起動直後・読みに行く前)は LOADING
         */
        fun of(
            entity: String,
            records: Map<String, *>,
            loading: Set<String>,
            failed: Set<String>,
            known: Set<String>,
        ): EntityLoadState = when {
            entity in failed -> if (entity in loading) LOADING else UNAVAILABLE
            entity !in records && (entity in loading || entity in known) -> LOADING
            else -> LOADED
        }

        /** [known]・読み込み中・失敗・記録のある全 entity の状態。 */
        fun all(records: Map<String, *>, loading: Set<String>, failed: Set<String>, known: Set<String>): Map<String, EntityLoadState> =
            (known + loading + failed + records.keys).associateWith { of(it, records, loading, failed, known) }
    }
}
