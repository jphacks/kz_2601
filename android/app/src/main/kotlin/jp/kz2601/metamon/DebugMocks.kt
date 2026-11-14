package jp.kz2601.metamon

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.onboarding.FirstRunTutorialStore
import jp.kz2601.metamon.services.PlanUsageSource
import jp.kz2601.metamon.services.Record
import jp.kz2601.metamon.services.consent.AiConsentStore
import jp.kz2601.metamon.services.mock.DummyRecords
import jp.kz2601.metamon.services.mock.MockMembersClient
import jp.kz2601.metamon.services.mock.MockPlanUsageSource
import jp.kz2601.metamon.services.mock.MockSeed
import jp.kz2601.metamon.services.sharing.MembersClient
import java.time.Instant

/**
 * 機能ごとのモックのつまみ(#1023・#1024)を構成に当てる。release の [DebugKnobs] は常に [DebugKnobs.NONE] なので、
 * ここは何も差し替えない(本物の口・既定のまま)。
 */
internal object DebugMocks {
    /** モックの appconfig の初期の記録。`METAMON_SEED_DUMMY` があれば、最初に描く DSL の entity 分をダミーで上書きする */
    fun seed(dsl: AppDSL, knobs: DebugKnobs, now: Instant = Instant.now()): Map<String, List<Record>> {
        val demo = MockSeed.demoRecords(now)
        val count = knobs.seedDummy ?: return demo
        val dummy = DummyRecords.make(dsl, count, now, knobs.seedStepHours)
        return demo + if (knobs.seedAuthorsMixed) DummyRecords.alternatingAuthors(dummy) else dummy
    }

    /** 使用量の口。`METAMON_MOCK_USAGE` があればモック(接続先に関係なく)、無ければ [live](モックの構成は null = 出さない) */
    fun usage(knobs: DebugKnobs, live: PlanUsageSource?): PlanUsageSource? =
        knobs.mockUsage?.let(::MockPlanUsageSource) ?: live.takeUnless { knobs.mock }

    /** 参加者の口(モックの構成)。`METAMON_MOCK_MEMBERS` が無ければ null(参加者の欄を出さない) */
    fun members(knobs: DebugKnobs): MembersClient? =
        knobs.mockMembers?.let { MockMembersClient(it, failsWrites = knobs.mockMembersFail) }

    /**
     * `METAMON_AI_CONSENT`(iOS の `AIConsentDebug`)。granted は同意済み + 説明を出した扱い、reset は初回の状態。
     * granted の時はチュートリアルも見た扱いにして画面を塞がない(iOS の `FirstRunTutorial.applyLaunchOverride`)。
     */
    fun applyAiConsent(knobs: DebugKnobs, consent: AiConsentStore, tutorial: FirstRunTutorialStore) {
        when (knobs.aiConsent) {
            DebugKnobs.AiConsentOverride.GRANTED -> {
                if (!consent.isGranted) consent.grant()
                consent.markPrompted()
                tutorial.markTutorialSeen()
            }
            DebugKnobs.AiConsentOverride.RESET -> consent.forget()
            null -> Unit
        }
    }
}
