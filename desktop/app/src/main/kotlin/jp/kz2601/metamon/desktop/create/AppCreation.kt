package jp.kz2601.metamon.desktop.create

import jp.kz2601.metamon.desktop.chat.ChatBuild
import jp.kz2601.metamon.desktop.chat.GenerateEnd
import jp.kz2601.metamon.desktop.chat.GenerateStream
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.http.FailureKind
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import java.io.IOException

/** 作る(`/v1/generate?stream=1`)の依頼 1 つ。 */
data class GenerateRequest(val appId: String, val transcript: String, val locale: String? = null)

/**
 * 作るストリームの行の出どころ(HTTP は [HttpGenerateSource]・テストは作り物の行)。
 * 行を 1 本ずつ [onLine] に渡し、[onLine] が true を返したら読むのをやめる。
 * HTTP の失敗は ApiError、通信が切れたら IOException を投げる。
 */
fun interface GenerateSource {
    suspend fun stream(request: GenerateRequest, onLine: (String) -> Boolean)
}

/** 作っている間の段階。 */
enum class CreatePhase {
    BUILDING,

    /** 完成した(アプリの一覧に足してウィンドウを開く) */
    DONE,

    /** サーバーが失敗を返した・結果が読めない */
    FAILED,

    /** 終わりの印が来ないまま切れた */
    CUT,
}

/** 作っている画面が見る状態。[preview] は最後に描けた途中の DSL(壊れた途中は前のまま)。 */
data class CreateState(
    val phase: CreatePhase = CreatePhase.BUILDING,
    val preview: AppDSL? = null,
    val result: MorphResult? = null,
    val failure: FailureKind? = null,
) {
    val isBuilding: Boolean get() = phase == CreatePhase.BUILDING

    /** 「もう一度」を出すか。切れた時と、やり直す意味がある失敗(圏外・混雑など)の時だけ */
    val canRetry: Boolean get() = phase == CreatePhase.CUT || (phase == CreatePhase.FAILED && failure?.isRetryable == true)
}

/**
 * 会話の ready・案から 1 つのアプリを作る(#2163)。画面は持たない(描くのは CreateWindow)。
 * 届いた行を [GenerateStream] に流し、描ける途中の DSL が変わるたびに [state] の preview を替える。
 * 「もう一度」([run] をもう一度)は前に描けた途中の画面を、次に描けるまで残す。
 */
class AppCreation(
    private val source: GenerateSource,
    val build: ChatBuild,
    val appId: String,
    private val locale: () -> String? = { null },
) {
    private val _state = MutableStateFlow(CreateState())
    val state: StateFlow<CreateState> = _state.asStateFlow()

    /** 作る。終わった時の状態を返す。止められた(cancel)時は投げ直す。 */
    suspend fun run(): CreateState {
        _state.update { CreateState(preview = it.preview) }
        val stream = GenerateStream(appId)
        var lines = 0
        val end = try {
            source.stream(GenerateRequest(appId, build.instruction, locale())) { line ->
                lines++
                val e = stream.feed(line)
                stream.preview.value?.let { p -> if (p != _state.value.preview) _state.update { it.copy(preview = p) } }
                e != null
            }
            stream.finish()
        } catch (c: CancellationException) {
            throw c
        } catch (io: IOException) {
            // 何か届いた後に切れたら「途中で切れた」、何も届かなければ通信の失敗
            if (lines > 0) stream.finish() else GenerateEnd.Failed(FailureKind.classify(io))
        } catch (t: Throwable) {
            GenerateEnd.Failed(FailureKind.classify(t))
        }
        _state.update {
            when (end) {
                is GenerateEnd.Completed -> it.copy(phase = CreatePhase.DONE, result = end.result, preview = end.result.document.dsl)
                is GenerateEnd.Failed -> it.copy(phase = CreatePhase.FAILED, failure = end.kind)
                is GenerateEnd.Cut -> it.copy(phase = CreatePhase.CUT, preview = end.last ?: it.preview)
            }
        }
        return _state.value
    }
}
