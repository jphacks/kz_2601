package jp.kz2601.metamon.desktop.i18n

import org.w3c.dom.Element
import java.io.InputStream
import java.util.Locale
import javax.xml.parsers.DocumentBuilderFactory

/** UI の言語。iOS / Android と同じく日本語と英語。 */
enum class Lang(val locale: Locale) {
    JA(Locale.JAPAN),
    EN(Locale.US),
    ;

    companion object {
        /**
         * 言語を決める。`override`(`-Dtailorit.lang`・`TAILORIT_LANG`)が先、無ければ OS の言語。
         * `ja*` だけ日本語、それ以外は英語(conformance の `device.locale` と同じ規則)。
         */
        fun resolve(override: String?, system: Locale = Locale.getDefault()): Lang {
            val tag = override?.trim()?.takeIf { it.isNotEmpty() } ?: system.toLanguageTag()
            return if (tag.lowercase().replace('_', '-').startsWith("ja")) JA else EN
        }
    }
}

/**
 * Android の strings.xml の形(`<string>`・`<plurals>`)を読んだ表。
 * キーは Android と同じ。引けない時は 英語 → 日本語 → キーそのもの の順に落ちる。
 */
class StringTable(
    private val strings: Map<Lang, Map<String, String>>,
    private val plurals: Map<Lang, Map<String, Map<String, String>>> = emptyMap(),
) {
    fun has(lang: Lang, key: String): Boolean = strings[lang]?.containsKey(key) == true

    fun get(lang: Lang, key: String): String =
        order(lang).firstNotNullOfOrNull { strings[it]?.get(key) } ?: key

    fun format(lang: Lang, key: String, vararg args: Any?): String =
        if (args.isEmpty()) get(lang, key) else String.format(lang.locale, get(lang, key), *args)

    /** `<plurals>`。英語は 1 だけ one、日本語は other。無い時は `<string>` を引く。 */
    fun plural(lang: Lang, key: String, count: Int, vararg args: Any?): String {
        for (l in order(lang)) {
            val q = plurals[l]?.get(key) ?: continue
            val pick = (if (l == Lang.EN && count == 1) q["one"] else null) ?: q["other"] ?: q.values.first()
            return String.format(l.locale, pick, *args)
        }
        return format(lang, key, *args)
    }

    private fun order(lang: Lang) = if (lang == Lang.EN) listOf(Lang.EN, Lang.JA) else listOf(lang, Lang.EN, Lang.JA)

    companion object {
        /** クラスパスの i18n/(Android から同梱した物 + デスクトップだけの物)を読む。 */
        fun fromClasspath(loader: ClassLoader = StringTable::class.java.classLoader): StringTable {
            val paths = loader.getResourceAsStream("i18n/index.txt")?.bufferedReader()?.readLines().orEmpty()
                .map { it.trim() }.filter { it.isNotEmpty() } + DESKTOP_FILES
            val strings = Lang.entries.associateWith { mutableMapOf<String, String>() }
            val plurals = Lang.entries.associateWith { mutableMapOf<String, Map<String, String>>() }
            for (path in paths) {
                val lang = if (path.contains("/values-en/")) Lang.EN else if (path.contains("/values/")) Lang.JA else continue
                val parsed = loader.getResourceAsStream("i18n/$path")?.use(AndroidStringsXml::parse) ?: continue
                strings.getValue(lang).putAll(parsed.strings)
                plurals.getValue(lang).putAll(parsed.plurals)
            }
            return StringTable(strings, plurals)
        }

        private val DESKTOP_FILES = listOf("desktop/values/strings_desktop.xml", "desktop/values-en/strings_desktop.xml")
    }
}

/** Android の res/values*\/strings*.xml を読む(エスケープは Android と同じ規則のうち使っている物)。 */
object AndroidStringsXml {
    data class Parsed(val strings: Map<String, String>, val plurals: Map<String, Map<String, String>>)

    fun parse(input: InputStream): Parsed {
        val factory = DocumentBuilderFactory.newInstance().apply {
            isNamespaceAware = false
            setFeature("http://apache.org/xml/features/disallow-doctype-decl", true)
        }
        val root = factory.newDocumentBuilder().parse(input).documentElement
        val strings = linkedMapOf<String, String>()
        val plurals = linkedMapOf<String, Map<String, String>>()
        val nodes = root.childNodes
        for (i in 0 until nodes.length) {
            val e = nodes.item(i) as? Element ?: continue
            val name = e.getAttribute("name").takeIf { it.isNotEmpty() } ?: continue
            when (e.tagName) {
                "string" -> strings[name] = unescape(e.textContent)
                "plurals" -> {
                    val items = e.getElementsByTagName("item")
                    plurals[name] = (0 until items.length).map { items.item(it) as Element }
                        .associate { it.getAttribute("quantity") to unescape(it.textContent) }
                }
            }
        }
        return Parsed(strings, plurals)
    }

    /** `"…"` で囲んだ値・`\'` `\"` `\n` `\t` `\\` `\@` `\?` `\uXXXX`。 */
    fun unescape(raw: String): String {
        var s = raw.trim()
        if (s.length >= 2 && s.startsWith('"') && s.endsWith('"')) s = s.substring(1, s.length - 1)
        val out = StringBuilder(s.length)
        var i = 0
        while (i < s.length) {
            val ch = s[i]
            if (ch == '\\' && i + 1 < s.length) {
                val n = s[i + 1]
                when (n) {
                    'n' -> out.append('\n')
                    't' -> out.append('\t')
                    'u' -> if (i + 5 < s.length) {
                        s.substring(i + 2, i + 6).toIntOrNull(16)?.let { out.append(it.toChar()); i += 6; continue }
                        out.append(n)
                    } else {
                        out.append(n)
                    }
                    else -> out.append(n)
                }
                i += 2
            } else {
                out.append(ch)
                i++
            }
        }
        return out.toString()
    }
}

/**
 * アプリの文言。`Strings["renderer_back"]`・`Strings.format("runtime_missing_required", name)`。
 * 言語は起動時に OS(または `-Dtailorit.lang` / `TAILORIT_LANG`)から決める。テストは [lang] を差し替える。
 */
object Strings {
    val table: StringTable by lazy { StringTable.fromClasspath() }

    @Volatile
    var lang: Lang = Lang.resolve(System.getProperty("tailorit.lang") ?: System.getenv("TAILORIT_LANG"))

    val locale: Locale get() = lang.locale

    operator fun get(key: String): String = table.get(lang, key)

    fun format(key: String, vararg args: Any?): String = table.format(lang, key, *args)

    fun plural(key: String, count: Int, vararg args: Any?): String = table.plural(lang, key, count, *args)

    /** 言語を一時的に替えて [block] を動かす(テスト・conformance の `device.locale`)。 */
    fun <T> withLang(l: Lang, block: () -> T): T {
        val prev = lang
        lang = l
        return try { block() } finally { lang = prev }
    }
}
