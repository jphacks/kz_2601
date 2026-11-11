package jp.kz2601.metamon.renderer.components.music

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.saveable.listSaver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.actions.openExternal
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard

private val recentSaver = listSaver<List<MusicSong>, String>(
    save = { list -> list.flatMap { listOf(it.title, it.artist) } },
    restore = { flat -> flat.chunked(2).filter { it.size == 2 }.map { MusicSong(it[0], it[1]) } },
)

/**
 * `musicPlayer`(#879)の Android の代わり。iOS は Apple Music(MusicKit)で再生するが、Android には同じものが無い(C 区分・#351)。
 * 再生はせず「この端末では再生できません」+ 曲名・アーティストで外部の音楽アプリ(無ければ Web)の検索を開く。
 * 探した曲はこの画面の間だけ一覧に残す(押すともう一度開く)。`allowRequests`・`autoAccept`(近くの友だちからのリクエスト)は
 * Android に近距離共有が無いので使わない。
 */
@Composable
fun MusicPlayerLeaf(c: Component, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    var title by rememberSaveable { mutableStateOf("") }
    var artist by rememberSaveable { mutableStateOf("") }
    var recent by rememberSaveable(stateSaver = recentSaver) { mutableStateOf(emptyList<MusicSong>()) }
    val query = MusicSearch.sanitize(MusicSong(title, artist))

    fun open(song: MusicSong) {
        val url = MusicSearch.searchUrl(song) ?: return
        if (openExternal(context, listOf(url), preferApp = true)) recent = MusicSearch.remember(recent, song)
    }

    Column(
        modifier.fillMaxWidth().renderCard().semantics { testTag = "music.${c.id ?: "player"}" },
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            Box(
                Modifier.size(18.dp).background(RenderTheme.colors.brandSoft, RoundedCornerShape(6.dp)),
                contentAlignment = Alignment.Center,
            ) {
                // アイコンの表に音符が無いので文字で描く(読み上げない)
                Text("♪", style = RenderTheme.Fonts.sectionTitle, color = RenderTheme.colors.brandA, modifier = Modifier.clearAndSetSemantics {})
            }
            Text(
                MusicSearch.heading(c, stringResource(R.string.music_title_default)),
                style = RenderTheme.Fonts.statLabel,
                color = RenderTheme.colors.secondaryLabel,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.semantics { heading() },
            )
        }
        Row(verticalAlignment = Alignment.Top, horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
            SymbolIcon("exclamationmark.triangle", 16.dp, RenderTheme.colors.secondaryLabel)
            Text(stringResource(R.string.music_unavailable), style = RenderTheme.Fonts.rowSubtitle, color = RenderTheme.colors.secondaryLabel)
        }
        OutlinedTextField(
            value = title,
            onValueChange = { title = it },
            modifier = Modifier.fillMaxWidth(),
            label = { Text(stringResource(R.string.music_song_title)) },
            singleLine = true,
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next),
        )
        OutlinedTextField(
            value = artist,
            onValueChange = { artist = it },
            modifier = Modifier.fillMaxWidth(),
            label = { Text(stringResource(R.string.music_artist)) },
            singleLine = true,
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
            keyboardActions = KeyboardActions(onSearch = { query?.let(::open) }),
        )
        ActionButton(
            label = stringResource(R.string.music_search),
            kind = ActionButtonKind.SECONDARY,
            icon = "magnifyingglass",
            enabled = query != null,
            onClick = { query?.let(::open) },
        )
        Text(stringResource(R.string.music_search_note), style = RenderTheme.Fonts.rowSubtitle, color = RenderTheme.colors.tertiaryLabel)
        if (recent.isNotEmpty()) {
            Text(stringResource(R.string.music_recent), style = RenderTheme.Fonts.sectionTitle, color = RenderTheme.colors.secondaryLabel)
            Column {
                recent.forEachIndexed { i, song ->
                    if (i > 0) HorizontalDivider(color = RenderTheme.colors.hairline)
                    RecentRow(song, onOpen = { open(song) })
                }
            }
        }
    }
}

@Composable
private fun RecentRow(song: MusicSong, onOpen: () -> Unit) {
    val openLabel = stringResource(R.string.music_open)
    Row(
        Modifier
            .fillMaxWidth()
            .defaultMinSize(minHeight = 44.dp)
            .clickable(role = Role.Button, onClickLabel = openLabel, onClick = onOpen),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        Column(Modifier.weight(1f)) {
            Text(song.title, style = RenderTheme.Fonts.rowTitle, color = RenderTheme.colors.label, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (song.artist.isNotEmpty()) {
                Text(song.artist, style = RenderTheme.Fonts.rowSubtitle, color = RenderTheme.colors.secondaryLabel, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
        SymbolIcon("arrow.up.right.square", 18.dp, RenderTheme.colors.secondaryLabel)
    }
}
