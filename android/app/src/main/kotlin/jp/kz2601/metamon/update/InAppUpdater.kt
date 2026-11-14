package jp.kz2601.metamon.update

import androidx.activity.result.ActivityResultLauncher
import androidx.activity.result.IntentSenderRequest
import jp.kz2601.metamon.services.update.UpdateRequirement
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/** Play の更新の画面を出す口(Activity の結果を受ける launcher)。 */
typealias UpdateLauncher = ActivityResultLauncher<IntentSenderRequest>

/** Play の更新の出し方(AppUpdateType)。最低の版より古い = IMMEDIATE、新しい版がある = FLEXIBLE。 */
enum class InAppUpdateFlow { IMMEDIATE, FLEXIBLE }

/** FLEXIBLE のダウンロードの進み。DOWNLOADED で「再起動して更新」を出す。 */
enum class FlexibleProgress { IDLE, DOWNLOADING, DOWNLOADED }

/** Play から読んだ更新の様子(Play の型から写す・テストで作れる形)。 */
data class PlayUpdateStatus(
    val availability: Availability,
    val immediateAllowed: Boolean = false,
    val flexibleAllowed: Boolean = false,
    val install: FlexibleProgress = FlexibleProgress.IDLE,
) {
    enum class Availability { NOT_AVAILABLE, AVAILABLE, IN_PROGRESS }
}

/** Play の AppUpdateManager の薄い口([PlayAppUpdates])。テストでは差し替える。 */
interface PlayUpdates {
    suspend fun status(): PlayUpdateStatus

    /** Play の更新の画面を出す。出せなければ false(ストアへのリンクに落とす)。 */
    suspend fun start(flow: InAppUpdateFlow, launcher: UpdateLauncher): Boolean

    /** ダウンロード済みの更新を入れてアプリを再起動する。 */
    fun completeUpdate()

    /** ダウンロードの進みを受ける。戻り値で受けるのをやめる。 */
    fun listen(onChange: (FlexibleProgress) -> Unit): () -> Unit
}

/** 「アップデート」を押した時にすること。 */
sealed interface InAppUpdateAction {
    data class Start(val flow: InAppUpdateFlow) : InAppUpdateAction
    /** FLEXIBLE はもう落としている(終われば再起動のバナー) */
    data object AlreadyDownloading : InAppUpdateAction
    /** 落とし終わっている: 再起動して入れる */
    data object Install : InAppUpdateAction
    /** Play で更新できない(Play 以外から入れた・Play が無い・その種類が許されない): ストアのページを開く */
    data object OpenStore : InAppUpdateAction
}

object InAppUpdatePolicy {
    fun flowFor(requirement: UpdateRequirement): InAppUpdateFlow? = when (requirement) {
        is UpdateRequirement.Required -> InAppUpdateFlow.IMMEDIATE
        is UpdateRequirement.Recommended -> InAppUpdateFlow.FLEXIBLE
        UpdateRequirement.None -> null
    }

    /** [status] が null = Play から読めない。 */
    fun decide(wanted: InAppUpdateFlow, status: PlayUpdateStatus?): InAppUpdateAction {
        status ?: return InAppUpdateAction.OpenStore
        if (status.install == FlexibleProgress.DOWNLOADED) return InAppUpdateAction.Install
        if (wanted == InAppUpdateFlow.FLEXIBLE && status.install == FlexibleProgress.DOWNLOADING) return InAppUpdateAction.AlreadyDownloading
        return when (status.availability) {
            // 前に始めた IMMEDIATE が途中: 続ける
            PlayUpdateStatus.Availability.IN_PROGRESS ->
                if (wanted == InAppUpdateFlow.IMMEDIATE) InAppUpdateAction.Start(wanted) else InAppUpdateAction.OpenStore
            PlayUpdateStatus.Availability.AVAILABLE -> {
                val allowed = if (wanted == InAppUpdateFlow.IMMEDIATE) status.immediateAllowed else status.flexibleAllowed
                if (allowed) InAppUpdateAction.Start(wanted) else InAppUpdateAction.OpenStore
            }
            // サーバーは新しい版と言うが Play にはまだ無い(段階的な公開など)
            PlayUpdateStatus.Availability.NOT_AVAILABLE -> InAppUpdateAction.OpenStore
        }
    }
}

/**
 * 「アップデートしてください」・新しい版のお知らせから Play の更新をその場で始める(Play In-App Updates)。
 * [play] が null(Play 以外から入れた stg・debug、モック)はいつもストアへのリンクに落とす。
 */
class InAppUpdater(private val play: PlayUpdates?) {
    private val _progress = MutableStateFlow(FlexibleProgress.IDLE)
    val progress: StateFlow<FlexibleProgress> = _progress.asStateFlow()

    /** Play の更新の画面を出せる端末か(ボタンの文言を変える)。 */
    val available: Boolean get() = play != null

    /** 「アップデート」を押した。Play で始めた・進めたら true、ストアのページを開くべきなら false。 */
    suspend fun update(requirement: UpdateRequirement, launcher: UpdateLauncher): Boolean {
        val wanted = InAppUpdatePolicy.flowFor(requirement) ?: return false
        val play = play ?: return false
        return when (val action = InAppUpdatePolicy.decide(wanted, readStatus(play))) {
            is InAppUpdateAction.Start -> play.start(action.flow, launcher)
            InAppUpdateAction.AlreadyDownloading -> true.also { _progress.value = FlexibleProgress.DOWNLOADING }
            InAppUpdateAction.Install -> true.also { restart() }
            InAppUpdateAction.OpenStore -> false
        }
    }

    /** 前面に戻った時: 途中の IMMEDIATE を続け、落とし終わった FLEXIBLE は再起動のバナーを出す。 */
    suspend fun resume(launcher: UpdateLauncher) {
        val play = play ?: return
        val status = readStatus(play) ?: return
        if (status.install == FlexibleProgress.DOWNLOADED) _progress.value = FlexibleProgress.DOWNLOADED
        if (status.availability == PlayUpdateStatus.Availability.IN_PROGRESS) play.start(InAppUpdateFlow.IMMEDIATE, launcher)
    }

    /** ダウンロードの進みを [progress] に映す。戻り値で受けるのをやめる。 */
    fun listen(): () -> Unit = play?.listen { _progress.value = it } ?: {}

    /** 「再起動して更新」。 */
    fun restart() {
        play?.completeUpdate()
    }

    private suspend fun readStatus(play: PlayUpdates): PlayUpdateStatus? = try {
        play.status()
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        // Play で入れていない(ERROR_APP_NOT_OWNED)・Play が古い・通信できない
        null
    }
}
