package jp.kz2601.metamon.renderer.components.actions

import android.annotation.SuppressLint
import android.graphics.Bitmap
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.logic.ExternalUrlPolicy
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors

/** webView のページ内の移動の決まり(iOS の `decidePolicyFor`)。 */
object WebViewNavigation {
    enum class Decision { ALLOW, OPEN_OUTSIDE, BLOCK }

    /** https / http / about はそのまま表示。他のスキームは allowlist(tel・line など)なら OS に渡し、それ以外は止める。 */
    fun decide(url: String): Decision {
        val scheme = ExternalUrlPolicy.scheme(url) ?: return Decision.BLOCK
        return when {
            scheme == "https" || scheme == "http" || scheme == "about" -> Decision.ALLOW
            ExternalUrlPolicy.isAllowed(url) -> Decision.OPEN_OUTSIDE
            else -> Decision.BLOCK
        }
    }
}

@Stable
private class WebState {
    var isLoading by mutableStateOf(false)
    var progress by mutableFloatStateOf(0f)
    var canGoBack by mutableStateOf(false)
    var canGoForward by mutableStateOf(false)
    var pageTitle by mutableStateOf("")
    var web: WebView? = null

    fun sync(view: WebView) {
        canGoBack = view.canGoBack()
        canGoForward = view.canGoForward()
    }
}

/** 高さが決まらない所(縦に並ぶ中)に置かれた時の高さ。 */
private val FALLBACK_HEIGHT = 420.dp

/**
 * `webView`: https の URL をアプリ内の WebView で見せる。上に 戻る / 進む / 題名 / 再読み込み の小さな帯と読み込み中の線。
 * 開けるのは [ExternalUrlPolicy.webViewUrl] を通った https だけ(iOS と同じ)。ファイル・content の読み込みは切る。
 */
@Composable
fun WebViewLeaf(c: Component, modifier: Modifier = Modifier) {
    val url = ExternalUrlPolicy.webViewUrl(c.url)
    if (url == null) {
        ActionNotice(stringResource(R.string.renderer_web_invalid), modifier)
        return
    }
    val state = remember { WebState() }
    val shape = RoundedCornerShape(12.dp)
    BoxWithConstraints(modifier.fillMaxWidth()) {
        val sized = if (constraints.hasBoundedHeight) Modifier.fillMaxSize() else Modifier.fillMaxWidth().height(FALLBACK_HEIGHT)
        Column(
            sized
                .clip(shape)
                .background(MaterialTheme.colorScheme.surface)
                .border(0.5.dp, RenderTheme.colors.hairline, shape),
        ) {
            WebToolbar(c.title?.takeIf { it.isNotEmpty() } ?: state.pageTitle, state)
            HorizontalDivider(color = RenderTheme.colors.hairline)
            Box(Modifier.fillMaxWidth().weight(1f)) {
                WebContent(url, state)
                if (state.isLoading) {
                    LinearProgressIndicator(progress = { state.progress }, modifier = Modifier.fillMaxWidth().align(Alignment.TopCenter))
                }
            }
        }
    }
}

@Composable
private fun WebToolbar(title: String, state: WebState) {
    val tint = RenderTheme.colors.brandText
    Row(
        Modifier.fillMaxWidth().padding(horizontal = RenderTheme.Spacing.xs),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs),
    ) {
        IconButton(onClick = { state.web?.goBack() }, enabled = state.canGoBack) {
            Icon(Icons.AutoMirrored.Filled.ArrowBack, stringResource(R.string.renderer_web_back), tint = tint.copy(alpha = if (state.canGoBack) 1f else 0.35f))
        }
        IconButton(onClick = { state.web?.goForward() }, enabled = state.canGoForward) {
            Icon(Icons.AutoMirrored.Filled.ArrowForward, stringResource(R.string.renderer_web_forward), tint = tint.copy(alpha = if (state.canGoForward) 1f else 0.35f))
        }
        Text(
            title,
            Modifier.weight(1f),
            style = MaterialTheme.typography.bodyMedium,
            fontWeight = FontWeight.Medium,
            textAlign = TextAlign.Center,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        IconButton(onClick = { state.web?.let { if (state.isLoading) it.stopLoading() else it.reload() } }) {
            if (state.isLoading) {
                Icon(Icons.Filled.Close, stringResource(R.string.renderer_web_stop), tint = tint)
            } else {
                Icon(Icons.Filled.Refresh, stringResource(R.string.renderer_web_reload), tint = tint)
            }
        }
    }
}

@SuppressLint("SetJavaScriptEnabled")
@Composable
private fun WebContent(url: String, state: WebState) {
    val context = LocalContext.current
    var loadedUrl by remember { mutableStateOf<String?>(null) }
    AndroidView(
        modifier = Modifier.fillMaxSize(),
        factory = { ctx ->
            WebView(ctx).apply {
                settings.javaScriptEnabled = true
                settings.domStorageEnabled = true
                settings.allowFileAccess = false
                settings.allowContentAccess = false
                webViewClient = object : WebViewClient() {
                    override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                        val target = request.url.toString()
                        return when (WebViewNavigation.decide(target)) {
                            WebViewNavigation.Decision.ALLOW -> false
                            WebViewNavigation.Decision.OPEN_OUTSIDE -> {
                                openExternal(context, listOf(target), preferApp = true)
                                true
                            }
                            WebViewNavigation.Decision.BLOCK -> true
                        }
                    }

                    override fun onPageStarted(view: WebView, url: String?, favicon: Bitmap?) {
                        state.isLoading = true
                        state.sync(view)
                    }

                    override fun onPageFinished(view: WebView, url: String?) {
                        state.isLoading = false
                        state.sync(view)
                    }

                    override fun doUpdateVisitedHistory(view: WebView, url: String?, isReload: Boolean) {
                        state.sync(view)
                    }
                }
                webChromeClient = object : WebChromeClient() {
                    override fun onProgressChanged(view: WebView, newProgress: Int) {
                        state.progress = newProgress / 100f
                    }

                    override fun onReceivedTitle(view: WebView, title: String?) {
                        state.pageTitle = title.orEmpty()
                    }
                }
                state.web = this
            }
        },
        // DSL の url が変わった時だけ読み直す(同じ URL での再描画でリロードしない)
        update = { web ->
            if (loadedUrl != url) {
                loadedUrl = url
                web.loadUrl(url)
            }
        },
        onRelease = { web ->
            web.stopLoading()
            web.destroy()
            if (state.web === web) state.web = null
        },
    )
}
