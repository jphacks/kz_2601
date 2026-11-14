package jp.kz2601.metamon.switcher

import jp.kz2601.metamon.R
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.AppSummary
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * 変身一覧の「…」の名前の変更・複製・並べ替え・削除(iOS の `AppRuntime+AppList` / `AppRuntime+Delete`・#896)。
 * どれも先に一覧へ反映し(楽観的 UI)、サーバーが失敗したら戻してトーストで知らせる。
 * 削除は先に隠し([hidden])、成功したら端末のデータ(記録・DSL・サムネ・会話の履歴・通知)を消す。
 * 使用中のアプリを消す時は先に切り替える(消した後の `GET /dsl` で同じ appId が空のアプリとして作り直されないよう)。
 */
class AppListEditor(
    private val client: AppConfigClient,
    /** ランタイムが持つ一覧(`AppRuntime.apps`) */
    private val apps: () -> List<AppSummary>,
    private val setApps: (List<AppSummary>) -> Unit,
    private val currentAppId: () -> String,
    /** 別のアプリへ切り替える(読めなければ false) */
    private val switchTo: suspend (String) -> Boolean,
    /** 話しかける前の画面へ(新しい appId) */
    private val goHome: () -> Unit,
    /** 消したアプリの端末のデータを消す */
    private val forgetLocal: suspend (String) -> Unit,
    private val toast: (UiText) -> Unit,
    /** 生成中は削除しない(生成の結果が消したアプリに書かれるのを防ぐ) */
    val isBusy: () -> Boolean = { false },
) {
    private val _hidden = MutableStateFlow<Set<String>>(emptySet())

    /** 削除中で一覧から隠しているアプリ。一覧そのものは書き換えないので、取り直しても消したカードが戻らない。 */
    val hidden: StateFlow<Set<String>> = _hidden.asStateFlow()

    private val visible: List<AppSummary> get() = AppSummary.visible(apps(), currentAppId()).filter { it.id !in _hidden.value }

    /** 名前を変える。空・長すぎはトーストで断る。 */
    suspend fun rename(app: AppSummary, raw: String) {
        val name = AppListEdit.normalizedName(raw)
        if (name == null) {
            toast(UiText.Res(R.string.app_menu_rename_invalid, AppListEdit.MAX_NAME_LENGTH))
            return
        }
        if (name == app.name || apps().none { it.id == app.id }) return
        val before = app.name
        setApps(apps().map { if (it.id == app.id) it.copy(name = name) else it })
        try {
            val saved = client.renameApp(app.id, name)
            setApps(apps().map { if (it.id == app.id) saved.copy(sortIndex = saved.sortIndex ?: it.sortIndex) else it })
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            setApps(apps().map { if (it.id == app.id) it.copy(name = before) else it })
            toast(UiText.Res(R.string.app_menu_rename_failed))
        }
    }

    /** 複製する(画面の定義だけ・記録は空)。自分で並べている時は元のすぐ後ろに置く。切り替えはしない。 */
    suspend fun duplicate(app: AppSummary, copyName: (String) -> String) {
        val name = AppListEdit.copyName(app.name, copyName)
        val order = visible.map { it.id }
        val copy = try {
            client.duplicateApp(app.id, name).summary
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            toast(UiText.Res(R.string.app_menu_duplicate_failed, displayName(app)))
            return
        }
        val before = apps()
        setApps(before + copy)
        toast(UiText.Res(R.string.app_menu_duplicated, displayName(copy)))
        val at = order.indexOf(app.id)
        if (before.any { it.sortIndex != null } && at >= 0) {
            saveOrder(order.toMutableList().apply { add(at + 1, copy.id) }, rollbackTo = apps())
        }
    }

    /** 一覧の [index] 番目に動かす(「左へ / 右へ」)。 */
    suspend fun move(id: String, index: Int) {
        val order = visible.map { it.id }
        val next = AppListEdit.moved(order, id, index)
        if (next == order) return
        saveOrder(next, rollbackTo = apps())
    }

    /** 削除する。true = 消えた。 */
    suspend fun delete(app: AppSummary): Boolean {
        if (isBusy()) {
            toast(UiText.Res(R.string.app_menu_delete_busy))
            return false
        }
        if (app.id in _hidden.value) return false // 二度押し
        val fallback = AppListEdit.fallback(app.id, currentAppId(), apps())
        _hidden.update { it + app.id }
        try {
            if (fallback != null) leave(app.id, fallback)
            client.deleteApp(app.id)
            setApps(apps().filter { it.id != app.id })
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            toast(UiText.Res(R.string.app_menu_delete_failed, displayName(app)))
            return false
        } finally {
            _hidden.update { it - app.id }
        }
        forgetLocal(app.id)
        toast(UiText.Res(R.string.app_menu_deleted, displayName(app)))
        return true
    }

    private suspend fun leave(appId: String, fallback: AppListEdit.Fallback) {
        if (fallback is AppListEdit.Fallback.App) switchTo(fallback.id)
        if (currentAppId() == appId) goHome()
    }

    private suspend fun saveOrder(ids: List<String>, rollbackTo: List<AppSummary>) {
        setApps(AppListEdit.applyingOrder(ids, apps()))
        try {
            client.reorderApps(ids)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            setApps(rollbackTo)
            toast(UiText.Res(R.string.app_menu_reorder_failed))
        }
    }

    private fun displayName(app: AppSummary): Any = app.name.ifEmpty { null } ?: UiText.Res(R.string.switcher_untitled)
}
