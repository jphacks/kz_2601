package jp.kz2601.metamon.voice

import jp.kz2601.metamon.dsl.AppDocument

/** 音声の画面から頼むアプリの切り替え(#884・[VoiceIntent] の行き先)。実体はランタイムと変身一覧を持つ画面。 */
interface VoiceAppRequests {
    /** 変身一覧を開く */
    fun openAppList()

    /** 名前の近いアプリへ切り替える。見つからない・曖昧なら変身一覧を開く */
    fun switchApp(name: String)

    /** appId を作り直して話しかける前の画面へ。新しい文書を返す(null = できなかった) */
    fun startNewApp(): AppDocument?
}
