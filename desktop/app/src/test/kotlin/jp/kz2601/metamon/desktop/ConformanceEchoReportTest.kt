package jp.kz2601.metamon.desktop

import org.junit.Test

/**
 * 適合テストで「DSL の値を写すだけ」(DSL_ECHO)として比べていない props の数を部品ごとに表にする(#2639)。
 * どれを比べ始められるかの手がかり。数を見るだけで落とさない。表は Gradle の出力に出す(app/build.gradle.kts)。
 */
class ConformanceEchoReportTest {
    @Test
    fun printEchoPropsTable() {
        val rows = ConformanceAllowlist.propsNotCompared
            .mapValues { (_, props) -> props.filterValues { it == ConformanceAllowlist.DSL_ECHO }.keys.sorted() }
            .filterValues { it.isNotEmpty() }
            .toSortedMap()
        print(table(rows))
    }

    private fun table(rows: Map<String, List<String>>): String = buildString {
        appendLine("DSL_ECHO で比べていない props: ${rows.values.sumOf { it.size }} 個 / ${rows.size} 部品")
        appendLine("| 部品 | 数 | props |")
        appendLine("| --- | --- | --- |")
        rows.forEach { (type, keys) -> appendLine("| $type | ${keys.size} | ${keys.joinToString(", ")} |") }
    }
}
