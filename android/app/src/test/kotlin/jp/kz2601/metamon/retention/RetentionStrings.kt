package jp.kz2601.metamon.retention

import jp.kz2601.metamon.R
import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/** テスト用: strings_retention.xml を読んで `Context.getString` の代わりにする(日 = values・英 = values-en)。 */
object RetentionStrings {
    private val androidRoot: File = File(System.getProperty("user.dir")).absoluteFile.parentFile

    fun resolver(english: Boolean): (Int, Array<out Any>) -> String {
        val file = File(androidRoot, "app/src/main/res/${if (english) "values-en" else "values"}/strings_retention.xml")
        val nodes = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file).getElementsByTagName("string")
        val texts = (0 until nodes.length).associate { i ->
            val e = nodes.item(i) as Element
            e.getAttribute("name") to e.textContent.removeSurrounding("\"").replace("\\'", "'")
        }
        return { id, args ->
            val name = R.string::class.java.fields.first { it.getInt(null) == id }.name
            String.format(texts.getValue(name), *args)
        }
    }

    fun text(id: Int, english: Boolean, vararg args: Any): String = resolver(english)(id, args)
}
