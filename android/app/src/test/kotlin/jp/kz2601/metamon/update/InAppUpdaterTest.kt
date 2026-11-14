package jp.kz2601.metamon.update

import androidx.activity.result.ActivityResultLauncher
import androidx.activity.result.IntentSenderRequest
import androidx.activity.result.contract.ActivityResultContract
import androidx.core.app.ActivityOptionsCompat
import com.google.android.play.core.install.model.InstallStatus
import jp.kz2601.metamon.services.update.UpdateRequirement
import jp.kz2601.metamon.update.PlayUpdateStatus.Availability
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class InAppUpdaterTest {
    private val required = UpdateRequirement.Required(minimum = 10)
    private val recommended = UpdateRequirement.Recommended(latest = 12)
    private val available = PlayUpdateStatus(Availability.AVAILABLE, immediateAllowed = true, flexibleAllowed = true)

    @Test
    fun requiredIsImmediateAndNewerIsFlexible() {
        assertEquals(InAppUpdateFlow.IMMEDIATE, InAppUpdatePolicy.flowFor(required))
        assertEquals(InAppUpdateFlow.FLEXIBLE, InAppUpdatePolicy.flowFor(recommended))
        assertNull(InAppUpdatePolicy.flowFor(UpdateRequirement.None))
    }

    @Test
    fun decideFallsBackToTheStoreWhenPlayCannotUpdate() {
        val store = InAppUpdateAction.OpenStore
        // Play から読めない(Play 以外から入れた)
        assertEquals(store, InAppUpdatePolicy.decide(InAppUpdateFlow.IMMEDIATE, null))
        // サーバーは新しいと言うが Play にはまだ無い
        assertEquals(store, InAppUpdatePolicy.decide(InAppUpdateFlow.FLEXIBLE, PlayUpdateStatus(Availability.NOT_AVAILABLE)))
        // その種類が許されていない
        assertEquals(store, InAppUpdatePolicy.decide(InAppUpdateFlow.IMMEDIATE, available.copy(immediateAllowed = false)))
        assertEquals(store, InAppUpdatePolicy.decide(InAppUpdateFlow.FLEXIBLE, available.copy(flexibleAllowed = false)))
    }

    @Test
    fun decideStartsResumesAndInstalls() {
        assertEquals(InAppUpdateAction.Start(InAppUpdateFlow.IMMEDIATE), InAppUpdatePolicy.decide(InAppUpdateFlow.IMMEDIATE, available))
        assertEquals(InAppUpdateAction.Start(InAppUpdateFlow.FLEXIBLE), InAppUpdatePolicy.decide(InAppUpdateFlow.FLEXIBLE, available))
        // 途中の IMMEDIATE は続ける
        assertEquals(
            InAppUpdateAction.Start(InAppUpdateFlow.IMMEDIATE),
            InAppUpdatePolicy.decide(InAppUpdateFlow.IMMEDIATE, PlayUpdateStatus(Availability.IN_PROGRESS)),
        )
        assertEquals(
            InAppUpdateAction.AlreadyDownloading,
            InAppUpdatePolicy.decide(InAppUpdateFlow.FLEXIBLE, available.copy(install = FlexibleProgress.DOWNLOADING)),
        )
        // 落とし終わっていれば、強制でも入れて再起動するだけ
        val downloaded = available.copy(install = FlexibleProgress.DOWNLOADED)
        assertEquals(InAppUpdateAction.Install, InAppUpdatePolicy.decide(InAppUpdateFlow.IMMEDIATE, downloaded))
        assertEquals(InAppUpdateAction.Install, InAppUpdatePolicy.decide(InAppUpdateFlow.FLEXIBLE, downloaded))
    }

    @Test
    fun withoutPlayEveryUpdateOpensTheStore() = runBlocking {
        val updater = InAppUpdater(null)
        assertFalse(updater.available)
        assertFalse(updater.update(required, NoLauncher))
        assertFalse(updater.update(recommended, NoLauncher))
        updater.resume(NoLauncher)
        updater.restart()
        assertEquals(FlexibleProgress.IDLE, updater.progress.value)
    }

    @Test
    fun startsTheWantedFlowOnPlay() = runBlocking {
        val play = FakePlay(available)
        val updater = InAppUpdater(play)
        assertTrue(updater.update(required, NoLauncher))
        assertTrue(updater.update(recommended, NoLauncher))
        assertEquals(listOf(InAppUpdateFlow.IMMEDIATE, InAppUpdateFlow.FLEXIBLE), play.started)
        // Play が画面を出せなければストアへ
        play.startResult = false
        assertFalse(updater.update(required, NoLauncher))
        // 版の要求が無ければ何もしない
        assertFalse(updater.update(UpdateRequirement.None, NoLauncher))
        assertEquals(3, play.started.size)
    }

    @Test
    fun playErrorsFallBackToTheStore() = runBlocking {
        val play = FakePlay(available).apply { failStatus = true }
        assertFalse(InAppUpdater(play).update(required, NoLauncher))
        assertTrue(play.started.isEmpty())
    }

    @Test
    fun flexibleDownloadShowsTheRestartBannerAndRestartInstalls() = runBlocking {
        val play = FakePlay(available)
        val updater = InAppUpdater(play)
        val stop = updater.listen()
        assertTrue(updater.update(recommended, NoLauncher))
        play.emit(FlexibleProgress.DOWNLOADING)
        assertEquals(FlexibleProgress.DOWNLOADING, updater.progress.value)
        play.emit(FlexibleProgress.DOWNLOADED)
        assertEquals(FlexibleProgress.DOWNLOADED, updater.progress.value)
        updater.restart()
        assertEquals(1, play.completed)
        stop()
        assertNull(play.listener)
    }

    @Test
    fun pressingUpdateAfterTheDownloadInstalls() = runBlocking {
        val play = FakePlay(available.copy(install = FlexibleProgress.DOWNLOADED))
        assertTrue(InAppUpdater(play).update(required, NoLauncher))
        assertEquals(1, play.completed)
        assertTrue(play.started.isEmpty())
    }

    @Test
    fun resumeContinuesImmediateAndShowsDownloadedBanner() = runBlocking {
        val play = FakePlay(PlayUpdateStatus(Availability.IN_PROGRESS))
        val updater = InAppUpdater(play)
        updater.resume(NoLauncher)
        assertEquals(listOf(InAppUpdateFlow.IMMEDIATE), play.started)

        play.status = available.copy(install = FlexibleProgress.DOWNLOADED)
        updater.resume(NoLauncher)
        assertEquals(FlexibleProgress.DOWNLOADED, updater.progress.value)
        assertEquals(1, play.started.size)
    }

    @Test
    fun playInstallStatusMapsToTheBannerProgress() {
        assertEquals(FlexibleProgress.DOWNLOADING, progressOf(InstallStatus.PENDING))
        assertEquals(FlexibleProgress.DOWNLOADING, progressOf(InstallStatus.DOWNLOADING))
        assertEquals(FlexibleProgress.DOWNLOADED, progressOf(InstallStatus.DOWNLOADED))
        listOf(InstallStatus.INSTALLING, InstallStatus.INSTALLED, InstallStatus.FAILED, InstallStatus.CANCELED, InstallStatus.UNKNOWN)
            .forEach { assertEquals(FlexibleProgress.IDLE, progressOf(it)) }
    }

    private class FakePlay(var status: PlayUpdateStatus) : PlayUpdates {
        val started = mutableListOf<InAppUpdateFlow>()
        var startResult = true
        var failStatus = false
        var completed = 0
        var listener: ((FlexibleProgress) -> Unit)? = null

        override suspend fun status(): PlayUpdateStatus = if (failStatus) error("not owned") else status

        override suspend fun start(flow: InAppUpdateFlow, launcher: UpdateLauncher): Boolean {
            started += flow
            return startResult
        }

        override fun completeUpdate() {
            completed++
        }

        override fun listen(onChange: (FlexibleProgress) -> Unit): () -> Unit {
            listener = onChange
            return { listener = null }
        }

        fun emit(progress: FlexibleProgress) = listener?.invoke(progress)
    }

    private object NoLauncher : ActivityResultLauncher<IntentSenderRequest>() {
        override val contract: ActivityResultContract<IntentSenderRequest, *> get() = error("unused")
        override fun launch(input: IntentSenderRequest, options: ActivityOptionsCompat?) = Unit
        override fun unregister() = Unit
    }
}
