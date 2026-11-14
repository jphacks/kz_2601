package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.CurrentAppStore

/**
 * アプリの持ち物(どのアプリを開いているか・一覧)を [AppRuntime] に渡す口。
 * iOS の `DeviceAppID` + `appConfig.listApps` + `AppListCache` + 初期の DSL をまとめたもの。
 */
class AppDirectory(
    /** 今のアプリの appId(端末に保存・次の起動で開き直す) */
    val store: CurrentAppStore,
    /** `GET /v1/apps` */
    val list: suspend () -> List<AppSummary>,
    /** 最後に取得した一覧(オフライン) */
    val cache: AppListCache = AppListCache(null),
    /** 新しいアプリの初期状態(話しかける前の画面・version 0) */
    val fresh: (appId: String) -> AppDocument,
)
