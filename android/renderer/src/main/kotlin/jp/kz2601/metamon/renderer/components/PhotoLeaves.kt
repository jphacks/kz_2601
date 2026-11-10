package jp.kz2601.metamon.renderer.components

import android.graphics.BitmapFactory
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil3.compose.AsyncImage
import coil3.compose.AsyncImagePainter
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Height
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.VideoValue
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.symbolSize
import java.util.Base64

// avatar / image。写真の値は photo field と同じ表現("data:image/jpeg;base64,…" か https の URL。iOS の PhotoCodec)。
// https は [RemoteImages](Coil)で読む。読み込み中は枠の地の色だけ(iOS の placeholder)、失敗は「写真なし」の枠。

object PhotoValue {
    /** data URL の中身のバイト列。data URL でなければ null。 */
    fun dataBytes(value: String?): ByteArray? {
        if (value == null || !value.startsWith("data:")) return null
        val comma = value.indexOf(',').takeIf { it >= 0 } ?: return null
        return runCatching { Base64.getMimeDecoder().decode(value.substring(comma + 1)) }.getOrNull()?.takeIf { it.isNotEmpty() }
    }

    /** 読み込んでよい https の URL か([RemoteImages.loadableUrl])。http などは写真なし扱い。 */
    fun isRemote(value: String?): Boolean = RemoteImages.loadableUrl(value) != null

    /** 写真があるか(iOS の PhotoView.hasPhoto)。 */
    fun hasPhoto(value: String?): Boolean = dataBytes(value) != null || isRemote(value)
}

/** avatar のイニシャル。「山田 太郎」→「山太」、英字 1 語は先頭 2 文字を大文字、日本語 1 語は 1 文字(iOS と同じ)。 */
fun avatarInitials(name: String?): String {
    val n = name.orEmpty().trim(' ', '\t', '　')
    if (n.isEmpty()) return ""
    val parts = n.split(' ').filter { it.isNotEmpty() }
    if (parts.size >= 2) return firstChar(parts[0]) + firstChar(parts[1])
    return (if (n[0].code < 0x80) n.take(2) else firstChar(n)).uppercase()
}

private fun firstChar(s: String): String = s.substring(0, s.offsetByCodePoints(0, 1))

/** 写真の値 → 絵。読めなければ null。 */
@Composable
private fun rememberPhoto(value: String?): ImageBitmap? = remember(value) {
    PhotoValue.dataBytes(value)?.let { bytes ->
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap()
    }
}

/**
 * 部品の `field` が指すフォームの下書きの写真(iOS の `ctx.form?.values[field]`)。data URL / https の文字列。
 * video field の値はサムネイル(iOS の PhotoCodec と同じ)。空・写真でない値は null。
 */
internal fun photoFieldValue(c: Component, ctx: RenderContext?): String? {
    val field = c.field ?: return null
    val raw = (ctx?.form?.values?.get(field) as? JSONValue.Str)?.value ?: return null
    val value = if (raw.startsWith(VideoValue.PREFIX)) VideoValue.parse(raw)?.thumbnail else raw
    return value?.takeIf { PhotoValue.hasPhoto(it) }
}

/** `avatar`: 円の画像。優先順: photo field → `image`(data URL / https)→ `name` のイニシャル → 人のアイコン。読み上げは name。 */
@Composable
fun AvatarLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val diameter = c.symbolSize * 1.8f
    val value = photoFieldValue(c, ctx) ?: c.image
    val bitmap = rememberPhoto(value)
    val remote = if (bitmap == null) RemoteImages.loadableUrl(value) else null
    val avatar = c.name ?: stringResource(R.string.inputs_avatar)
    val colors = RenderTheme.colors
    Box(
        modifier.fillMaxWidth().clearAndSetSemantics {
            contentDescription = avatar
            role = Role.Image
        },
        contentAlignment = Alignment.Center,
    ) {
        Box(
            Modifier
                .size(diameter)
                .clip(CircleShape)
                .background(colors.brandSoft)
                .border(1.dp, colors.hairline, CircleShape),
            contentAlignment = Alignment.Center,
        ) {
            val initials = avatarInitials(c.name)
            // https は読み込み中・失敗の間イニシャルを出す(iOS の placeholder)
            val phase = remote?.let { rememberRemotePhase(it) }
            when {
                bitmap != null -> Image(bitmap, null, Modifier.fillMaxSize(), contentScale = ContentScale.Crop)
                phase?.value != RemotePhase.LOADED -> when {
                    initials.isNotEmpty() -> Text(
                        initials,
                        color = colors.brandText,
                        fontSize = (diameter.value * 0.4f).sp,
                        fontWeight = FontWeight.SemiBold,
                    )
                    else -> SymbolIcon("person.fill", diameter * 0.45f, colors.brandA.copy(alpha = 0.7f))
                }
            }
            if (remote != null && phase != null) RemoteImage(remote, phase)
        }
    }
}

/** image の高さ(small 100 / large 260 / それ以外 160)。 */
val Component.imageHeight: Dp
    get() = when (height) {
        Height.SMALL -> 100.dp
        Height.LARGE -> 260.dp
        else -> 160.dp
    }

/** `image`: フォームの photo field の写真。無ければ「写真なし」の枠。 */
@Composable
fun ImageLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    PhotoView(photoFieldValue(c, ctx), c.imageHeight, modifier)
}

/** 写真 1 枚の枠(iOS の PhotoView)。角丸・横いっぱい・高さ固定で切り抜く。 */
@Composable
fun PhotoView(value: String?, height: Dp, modifier: Modifier = Modifier) {
    val bitmap = rememberPhoto(value)
    val remote = if (bitmap == null) RemoteImages.loadableUrl(value) else null
    val phase = remote?.let { rememberRemotePhase(it) }
    // 読めなかった https は「写真なし」と同じ枠・読み上げ
    val has = bitmap != null || (remote != null && phase?.value != RemotePhase.FAILED)
    val photoLabel = stringResource(if (has) R.string.inputs_photo else R.string.inputs_no_photo)
    Box(
        modifier
            .fillMaxWidth()
            .height(height)
            .clip(RoundedCornerShape(RenderTheme.Radius.inner))
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .clearAndSetSemantics {
                contentDescription = photoLabel
                if (has) role = Role.Image
            },
        contentAlignment = Alignment.Center,
    ) {
        if (bitmap != null) {
            Image(bitmap, null, Modifier.fillMaxSize(), contentScale = ContentScale.Crop)
        } else if (remote != null && phase != null) {
            // 読み込み中は地の色だけ(スケルトン)
            RemoteImage(remote, phase)
        }
        if (!has) SymbolIcon("photo", 28.dp, MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

/** https の画像の状態。 */
internal enum class RemotePhase { LOADING, LOADED, FAILED }

@Composable
private fun rememberRemotePhase(url: String): MutableState<RemotePhase> = remember(url) { mutableStateOf(RemotePhase.LOADING) }

/** https の画像を枠いっぱいに切り抜いて描き、状態を [phase] に返す。 */
@Composable
private fun RemoteImage(url: String, phase: MutableState<RemotePhase>) {
    AsyncImage(
        model = url,
        contentDescription = null,
        modifier = Modifier.fillMaxSize(),
        contentScale = ContentScale.Crop,
        onState = { state ->
            phase.value = when (state) {
                is AsyncImagePainter.State.Success -> RemotePhase.LOADED
                is AsyncImagePainter.State.Error -> RemotePhase.FAILED
                else -> RemotePhase.LOADING
            }
        },
    )
}
