package jp.kz2601.metamon.desktop.automations

import jp.kz2601.metamon.services.automations.AppAutomations
import jp.kz2601.metamon.services.automations.AutomationRule
import jp.kz2601.metamon.services.automations.AutomationToggle
import jp.kz2601.metamon.services.automations.AutomationsClient
import jp.kz2601.metamon.services.http.ApiError
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * 自動の動きの一覧の状態(#3358・Android の `AutomationsModel`・iOS の `AutomationsModel`)。
 * 一覧は `GET /v1/apps/{appId}/automations`、ルールのオン / オフは [AutomationsClient.setRule](`PATCH /dsl`・If-Match)。
 */
class AutomationsModel(val appId: String, private val client: AutomationsClient) {
    sealed interface Phase {
        data object Loading : Phase
        data class Loaded(val automations: AppAutomations) : Phase

        /** 作成者ではない(403)。一覧も切り替えも出さない */
        data object NotCreator : Phase
        data object Failed : Phase
    }

    /** 切り替えに失敗した時の一言 */
    enum class ToggleError(val key: String) {
        /** 他で変わっていた。取り直した */
        CONFLICT("automations_conflict"),
        FAILED("automations_toggle_failed"),
    }

    private val _phase = MutableStateFlow<Phase>(Phase.Loading)
    val phase: StateFlow<Phase> = _phase.asStateFlow()

    /** 切り替えを送っている最中のルール(二重に押させない) */
    private val _pending = MutableStateFlow<Set<String>>(emptySet())
    val pending: StateFlow<Set<String>> = _pending.asStateFlow()

    private val _toggleError = MutableStateFlow<ToggleError?>(null)
    val toggleError: StateFlow<ToggleError?> = _toggleError.asStateFlow()

    fun clearToggleError() {
        _toggleError.value = null
    }

    suspend fun load() {
        try {
            _phase.value = Phase.Loaded(client.automations(appId))
        } catch (e: CancellationException) {
            throw e
        } catch (e: ApiError) {
            _phase.value = if (e.status == 403) Phase.NotCreator else failedKeepingLoaded()
        } catch (e: Exception) {
            _phase.value = failedKeepingLoaded()
        }
    }

    private fun failedKeepingLoaded(): Phase = _phase.value as? Phase.Loaded ?: Phase.Failed

    /** ルールのオン / オフ。先に見た目を変え(楽観的)、失敗したら戻して一言。他で変わっていた(412 など)時は取り直す。 */
    suspend fun setEnabled(enabled: Boolean, ruleId: String) {
        val loaded = _phase.value as? Phase.Loaded ?: return
        val before = loaded.automations.rules.firstOrNull { it.id == ruleId }?.enabled ?: return
        if (before == enabled || ruleId in _pending.value) return
        replaceEnabled(ruleId, enabled)
        _pending.update { it + ruleId }
        try {
            client.setRule(ruleId, enabled, appId)
        } catch (e: CancellationException) {
            replaceEnabled(ruleId, before)
            throw e
        } catch (e: Exception) {
            replaceEnabled(ruleId, before)
            if (AutomationToggle.isConflict(e)) {
                _toggleError.value = ToggleError.CONFLICT
                load()
            } else {
                _toggleError.value = ToggleError.FAILED
            }
        } finally {
            _pending.update { it - ruleId }
        }
    }

    private fun replaceEnabled(ruleId: String, enabled: Boolean) {
        _phase.update { p ->
            if (p !is Phase.Loaded) return@update p
            p.copy(automations = p.automations.copy(rules = p.automations.rules.map { if (it.id == ruleId) it.copy(enabled = enabled) else it }))
        }
    }

    companion object {
        /**
         * 切り替えを出すか(Android と同じ)。作成者として読めた一覧のルールだけ(403 は [Phase.NotCreator] で一覧ごと出さない)。
         * サーバーが enabled を返さない(古い)時は出さない。
         */
        fun canToggle(rule: AutomationRule): Boolean = rule.enabled != null
    }
}
