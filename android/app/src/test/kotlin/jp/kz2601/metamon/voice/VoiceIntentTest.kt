package jp.kz2601.metamon.voice

import org.junit.Assert.assertEquals
import org.junit.Test

/** 発話の意図と名前の照合(#884・iOS の AppSwitcherTests / MockServicesTests の VoiceIntent を移植)。 */
class VoiceIntentTest {
    private fun detect(t: String) = VoiceIntent.detect(t)

    @Test
    fun switchAppIntent() {
        assertEquals(VoiceIntent.SwitchApp("家計簿"), detect("家計簿に切り替えて"))
        assertEquals(VoiceIntent.SwitchApp("家計簿"), detect("家計簿アプリに切り替えて。"))
        assertEquals(VoiceIntent.SwitchApp("筋トレ"), detect("筋トレ を 開いて"))
        assertEquals(VoiceIntent.SwitchApp("散歩メモ"), detect("じゃあ散歩メモに変身して"))
        assertEquals(VoiceIntent.SwitchApp("家計簿"), detect("家計簿の方に切替"))
    }

    @Test
    fun listAppsIntent() {
        assertEquals(VoiceIntent.ListApps, detect("アプリ一覧"))
        assertEquals(VoiceIntent.ListApps, detect("変身一覧を見せて"))
        assertEquals(VoiceIntent.ListApps, detect("アプリのリストを出して"))
        // 名前の無い切替は一覧
        assertEquals(VoiceIntent.ListApps, detect("別のアプリに切り替えて"))
        assertEquals(VoiceIntent.ListApps, detect("他のアプリを開いて"))
        assertEquals(VoiceIntent.ListApps, detect("アプリを切り替えて"))
    }

    @Test
    fun existingIntentsUnchanged() {
        assertEquals(VoiceIntent.Undo, detect("元に戻して"))
        assertEquals(VoiceIntent.Undo, detect("前の状態に戻して"))
        assertEquals(VoiceIntent.NewApp, detect("別のアプリ。家計簿が欲しい"))
        assertEquals(VoiceIntent.NewApp, detect("新しいアプリに切り替えて"))
        assertEquals(VoiceIntent.Morph, detect("ダークモードにして"))
        assertEquals(VoiceIntent.Morph, detect("ボタンを右に戻して"))
        assertEquals(VoiceIntent.Morph, detect("一覧画面にグラフを足して"))
    }

    @Test
    fun undo() {
        for (t in listOf("元に戻して", "元に戻して。", "もとに戻して!", "取り消して", "アンドゥ", "戻して", "前に戻して", "ひとつ前の状態にして", "さっきの状態に戻して")) {
            assertEquals(t, VoiceIntent.Undo, detect(t))
        }
    }

    @Test
    fun newApp() {
        for (t in listOf("別のアプリ。家計簿が欲しい", "新しいアプリを作って、散歩メモ", "別アプリで筋トレ記録")) {
            assertEquals(t, VoiceIntent.NewApp, detect(t))
        }
    }

    @Test
    fun morphIsDefault() {
        for (t in listOf("家計簿が欲しい", "追加ボタンを左端まで広げて", "ボタンを右に戻して配置して", "", "   ")) {
            assertEquals(t, VoiceIntent.Morph, detect(t))
        }
        // 「別のアプリ」が文中にあるだけでは新規扱いにしない(発話頭だけ)
        assertEquals(VoiceIntent.Morph, detect("家計簿に別のアプリへのリンクを足して"))
    }

    @Test
    fun english() {
        assertEquals(VoiceIntent.Undo, detect("Undo"))
        assertEquals(VoiceIntent.Undo, detect("undo that, please."))
        assertEquals(VoiceIntent.ListApps, detect("Show my apps"))
        assertEquals(VoiceIntent.ListApps, detect("List apps"))
        assertEquals(VoiceIntent.ListApps, detect("Switch to another app"))
        assertEquals(VoiceIntent.SwitchApp("budget"), detect("Switch to budget app"))
        assertEquals(VoiceIntent.NewApp, detect("Switch to a new app"))
        assertEquals(VoiceIntent.NewApp, detect("New app: a walking log"))
        assertEquals(VoiceIntent.Morph, detect("Make the buttons blue"))
        assertEquals(VoiceIntent.Morph, detect("make a budget app"))
    }

    @Test
    fun nameMatchTiers() {
        val names = listOf("家計簿", "筋トレ記録", "散歩の写真メモ", "家計簿(旅行用)")
        assertEquals(AppNameMatch.One(0), AppNameMatch.match("家計簿", names)) // 完全一致が前方一致より優先
        assertEquals(AppNameMatch.One(1), AppNameMatch.match("筋トレ", names)) // 前方一致
        assertEquals(AppNameMatch.One(2), AppNameMatch.match("写真メモ", names)) // 部分一致
        assertEquals(AppNameMatch.One(0), AppNameMatch.match("家計簿アプリ", names)) // 「アプリ」は無視
        assertEquals(AppNameMatch.Ambiguous(listOf(0, 3)), AppNameMatch.match("家計", names))
        assertEquals(AppNameMatch.None, AppNameMatch.match("日記", names))
        assertEquals(AppNameMatch.None, AppNameMatch.match("", names))
        assertEquals(AppNameMatch.One(0), AppNameMatch.match("thebudget", listOf("Budget", "Workout log")))
    }
}
