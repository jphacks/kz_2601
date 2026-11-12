package jp.kz2601.metamon.desktop.i18n

import jp.kz2601.metamon.desktop.DataText
import jp.kz2601.metamon.desktop.PlayRules
import jp.kz2601.metamon.desktop.PlayTexts
import jp.kz2601.metamon.desktop.TableCells
import jp.kz2601.metamon.dsl.TableColumn
import jp.kz2601.metamon.dsl.TableColumnAggregate
import org.junit.Assert.assertEquals
import org.junit.Test

/** 遊び・データの部品の文言(精算・トーナメント・カード・表の集計)を Android と同じキーで引く(#1759)。 */
class LeafStringsTest {
    private fun header(aggregate: TableColumnAggregate) = TableCells.header(TableColumn(field = "金額", aggregate = aggregate), null)

    @Test
    fun englishShowsEnglishTexts() = Strings.withLang(Lang.EN) {
        assertEquals("Settle up", Strings["play_settlement_title"])
        assertEquals("All settled up", Strings["play_settlement_none"])
        assertEquals("Champion: 青木", Strings.format("play_bracket_champion", "青木"))
        assertEquals("Add entrants", PlayTexts.bracketEmpty(0))
        assertEquals("The bracket appears once there are 2 or more entrants", PlayTexts.bracketEmpty(1))
        assertEquals("Bye", PlayTexts.emptySlot(1))
        assertEquals("TBD", PlayTexts.emptySlot(2))
        assertEquals("Final", PlayTexts.roundTitle(3, 3))
        assertEquals("Semifinal", PlayTexts.roundTitle(2, 3))
        assertEquals("Round 1", PlayTexts.roundTitle(1, 4))
        assertEquals("1 entrant added after the start is not in this bracket", Strings.plural("play_bracket_late", 1, 1))
        assertEquals("2 entrants added after the start are not in this bracket", Strings.plural("play_bracket_late", 2, 2))
        assertEquals("No cards yet", Strings["play_flashcards_empty"])
        assertEquals("Front (click for the back)", Strings["desktop_flashcards_front_hint"])
        assertEquals("Count", header(TableColumnAggregate.COUNT))
        assertEquals("Total 金額", header(TableColumnAggregate.SUM))
        assertEquals("Avg 金額", header(TableColumnAggregate.AVG))
        assertEquals("Everyone", DataText.EVERYONE)
        assertEquals("Member", DataText.MEMBER)
    }

    @Test
    fun japaneseKeepsTheTextsTheConformanceCasesCompare() = Strings.withLang(Lang.JA) {
        // 表の見出し・名簿の文字は描画の木にも出る。日本語は今までと同じ
        assertEquals("件数", header(TableColumnAggregate.COUNT))
        assertEquals("金額の合計", header(TableColumnAggregate.SUM))
        assertEquals("金額の平均", header(TableColumnAggregate.AVG))
        assertEquals("金額の最小", header(TableColumnAggregate.MIN))
        assertEquals("金額の最大", header(TableColumnAggregate.MAX))
        assertEquals("全員", DataText.EVERYONE)
        assertEquals("参加者", DataText.MEMBER)
        assertEquals("決勝", PlayTexts.roundTitle(3, 3))
        assertEquals("2 回戦", PlayTexts.roundTitle(2, 5))
        assertEquals("不戦勝", PlayTexts.emptySlot(1))
        assertEquals("精算は不要です", Strings["play_settlement_none"])
    }

    @Test
    fun treeRoundNamesStayJapaneseInEnglish() = Strings.withLang(Lang.EN) {
        // 描画の木の回戦の名前は iOS の日本語と突き合わせる(Android の BracketRules と同じ)
        assertEquals("決勝", PlayRules.roundTitle(3, 3))
        assertEquals("準決勝", PlayRules.roundTitle(2, 3))
        assertEquals("1 回戦", PlayRules.roundTitle(1, 4))
    }
}
