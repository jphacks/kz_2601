package jp.kz2601.metamon.widget

import android.content.Context
import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

/** 中身を書いた・消した後に、置かれているウィジェットを描き直す(Glance の `updateAll`)。 */
object WidgetRefresh {
    fun requester(context: Context): () -> Unit {
        val app = context.applicationContext
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        return {
            scope.launch {
                try {
                    MetamonWidget.refresh(app)
                } catch (e: Exception) {
                    Log.w("MetamonWidget", "widget refresh failed", e)
                }
            }
        }
    }
}
