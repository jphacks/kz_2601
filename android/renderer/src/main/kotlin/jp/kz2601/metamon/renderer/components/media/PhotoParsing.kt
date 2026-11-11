package jp.kz2601.metamon.renderer.components.media

import java.time.YearMonth
import kotlin.math.abs
import kotlin.math.max
import kotlin.math.min

/** レシートなどから拾った日付(iOS の `DateComponents` のうち使う分)。時刻が無ければ hour / minute は null。 */
data class ReadDate(val year: Int, val month: Int, val day: Int, val hour: Int? = null, val minute: Int? = null)

/** OCR の行からレシートの合計金額と日付を拾う(iOS の `ReceiptParser`・純関数)。 */
object ReceiptParser {
    /** 合計を表す語。小計・点数は含めない。 */
    internal val totalWords = listOf("総合計", "合計", "お会計", "お買上計", "お買上げ計", "ご請求額", "お支払金額", "お支払い金額", "税込", "total")
    internal val excludedWords = listOf("小計", "点数", "お預", "預り", "お釣", "釣銭", "おつり")

    /** 全角の英数字・記号を半角へ。OCR が円記号を `\` と読むことがあるので ¥ に寄せる。 */
    fun normalize(s: String): String = buildString(s.length) {
        for (ch in s) {
            append(
                when (ch) {
                    in '！'..'～' -> ch - 0xFEE0
                    '　' -> ' '
                    '￥', '\\' -> '¥'
                    else -> ch
                },
            )
        }
    }

    private val numberRe = Regex("""(¥)?\s*(\d{1,3}(?:[,.]\d{3})+|\d+)\s*(円)?""")
    private val quantityMarks = setOf('点', '個', '%', '％')

    /** 行の中の金額候補(値・¥/円が付いているか)。8 桁以上(電話番号・会員番号)は外す。 */
    internal fun amounts(line: String): List<Pair<Double, Boolean>> = numberRe.findAll(line).mapNotNull { m ->
        val digits = m.groupValues[2].replace(",", "").replace(".", "")
        val v = digits.takeIf { it.length <= 7 }?.toDoubleOrNull() ?: return@mapNotNull null
        // 「3点」「2個」は数量
        val after = m.range.last + 1
        if (after < line.length && line[after] in quantityMarks) return@mapNotNull null
        v to (m.groups[1] != null || m.groups[3] != null)
    }.toList()

    /** 合計の行(無ければ次の行)の金額。合計の行が無ければ ¥/円 付きの最大額。 */
    fun amount(lines: List<String>): Double? {
        val ls = lines.map(::normalize)
        val totals = mutableListOf<Double>()
        ls.forEachIndexed { i, line ->
            val lower = line.lowercase()
            if (totalWords.none { lower.contains(it) } || excludedWords.any { line.contains(it) }) return@forEachIndexed
            var found = amounts(line)
            // 「合計」と金額が別の行に分かれて読まれることがある
            if (found.isEmpty() && i + 1 < ls.size) found = amounts(ls[i + 1])
            val marked = found.filter { it.second }
            val best = (marked.ifEmpty { found }).maxOfOrNull { it.first }
            if (best != null && best > 0) totals += best
        }
        totals.maxOrNull()?.let { return it }
        return ls.filter { line -> excludedWords.none { line.contains(it) } }
            .flatMap(::amounts).filter { it.second }.map { it.first }.filter { it > 0 }.maxOrNull()
    }

    private val dateRe = Regex("""((?:19|20)\d{2})\s*[年/\-.]\s*(\d{1,2})\s*[月/\-.]\s*(\d{1,2})""")
    private val reiwaRe = Regex("""令和\s*(\d{1,2}|元)\s*年\s*(\d{1,2})\s*月\s*(\d{1,2})\s*日""")
    private val timeRe = Regex("""(?<!\d)([01]?\d|2[0-3])\s*[:時]\s*([0-5]\d)(?!\d)""")

    /** 最初に見つかった日付(西暦・令和)。同じ行に時刻があれば時・分も入れる。 */
    fun date(lines: List<String>): ReadDate? {
        for (line in lines.map(::normalize)) {
            val found = dateRe.find(line)?.let { m ->
                ReadDate(m.groupValues[1].toInt(), m.groupValues[2].toInt(), m.groupValues[3].toInt())
            } ?: reiwaRe.find(line)?.let { m ->
                val y = m.groupValues[1]
                ReadDate(2018 + (if (y == "元") 1 else y.toInt()), m.groupValues[2].toInt(), m.groupValues[3].toInt())
            }
            if (found == null || !isValid(found)) continue
            val t = timeRe.find(line) ?: return found
            return found.copy(hour = t.groupValues[1].toInt(), minute = t.groupValues[2].toInt())
        }
        return null
    }

    internal fun isValid(d: ReadDate): Boolean =
        d.year in 1900..2100 && d.month in 1..12 && d.day in 1..YearMonth.of(d.year, d.month).lengthOfMonth()
}

/** 画像分類のラベル(英語)と OCR の文字から候補を選ぶ・日本語にする(iOS の `PhotoClassifier`・純関数)。 */
object PhotoClassifier {
    /** 候補名に含まれる日本語 → それを示すラベル(部分一致)。 */
    internal val synonyms: List<Pair<String, List<String>>> = listOf(
        "食" to listOf("food", "meal", "dish", "fruit", "vegetable", "bread", "dessert", "cake", "sushi", "noodle", "pizza", "salad", "sandwich", "rice", "meat"),
        "ごはん" to listOf("food", "meal", "dish", "rice"),
        "料理" to listOf("food", "meal", "dish"),
        "飲" to listOf("drink", "beverage", "coffee", "tea", "beer", "wine", "juice", "bottle"),
        "カフェ" to listOf("coffee", "cafe", "latte"),
        "犬" to listOf("dog", "canine", "puppy"),
        "猫" to listOf("cat", "feline", "kitten"),
        "鳥" to listOf("bird"),
        "魚" to listOf("fish"),
        "花" to listOf("flower", "blossom", "rose", "tulip", "petal"),
        "観葉" to listOf("houseplant", "foliage", "potted", "plant"),
        "多肉" to listOf("succulent", "cactus", "aloe"),
        "植物" to listOf("plant", "foliage", "leaf", "tree"),
        "木" to listOf("tree"),
        "本" to listOf("book", "magazine", "publication"),
        "書類" to listOf("document", "paper", "text", "receipt"),
        "レシート" to listOf("receipt", "document", "paper"),
        "服" to listOf("clothing", "shirt", "dress", "jacket", "coat", "apparel", "jeans"),
        "靴" to listOf("shoe", "footwear", "sneaker", "boot"),
        "交通" to listOf("car", "vehicle", "train", "bus", "bicycle", "taxi", "railway", "airplane"),
        "車" to listOf("car", "vehicle", "automobile"),
        "電車" to listOf("train", "railway", "subway"),
        "日用品" to listOf("toiletry", "detergent", "household", "tissue", "soap", "toothbrush"),
        "薬" to listOf("medicine", "pill", "drug", "capsule"),
        "景色" to listOf("landscape", "scenery", "sky", "mountain", "sea", "beach", "outdoor"),
        "風景" to listOf("landscape", "scenery", "sky", "mountain", "sea", "beach", "outdoor"),
        "海" to listOf("sea", "ocean", "beach", "water"),
        "山" to listOf("mountain", "hill"),
        "空" to listOf("sky", "cloud"),
        "人" to listOf("people", "person", "adult", "child", "portrait", "face"),
        "家電" to listOf("appliance", "electronics", "computer", "television", "phone"),
    )

    /** 最も当てはまる候補(どれにも当てはまらなければ null)。labels は信頼度順。 */
    fun classify(options: List<String>, labels: List<String>, text: String): String? {
        val ls = labels.map { it.lowercase() }
        val t = ReceiptParser.normalize(text).lowercase()
        var best: String? = null
        var bestScore = 0.0
        for (option in options) {
            val o = option.lowercase()
            var score = 0.0
            if (o.isNotEmpty() && t.contains(o)) score += 2
            val keys = listOf(o) + synonyms.filter { o.contains(it.first) }.flatMap { it.second }
            ls.forEachIndexed { rank, label -> if (keys.any { label.contains(it) }) score += 1.0 / (rank + 1) }
            if (score > bestScore) {
                best = option
                bestScore = score
            }
        }
        return best
    }

    /** ラベルの日本語。知らないラベルは英語のまま(`_` は空白に)。 */
    fun japanese(label: String): String = words[label.lowercase()] ?: label.replace("_", " ")

    /** 主なラベル → 日本語。物・動植物・料理を中心に、よく出るものだけ(iOS と同じ表)。 */
    internal val words: Map<String, String> = mapOf(
        // 大まかな語
        "food" to "食べ物", "meal" to "食事", "dish" to "料理", "fruit" to "果物", "vegetable" to "野菜", "drink" to "飲み物", "beverage" to "飲み物",
        "plant" to "植物", "flower" to "花", "tree" to "木", "animal" to "動物", "mammal" to "哺乳類", "people" to "人", "person" to "人",
        "adult" to "大人", "child" to "子ども", "baby" to "赤ちゃん", "document" to "書類", "paper" to "紙", "text" to "文字", "receipt" to "レシート",
        "outdoor" to "屋外", "indoor" to "屋内", "structure" to "建造物", "building" to "建物",
        // 料理・食材
        "sushi" to "寿司", "ramen" to "ラーメン", "noodles" to "麺", "rice" to "ご飯", "curry" to "カレー", "pizza" to "ピザ", "hamburger" to "ハンバーガー",
        "sandwich" to "サンドイッチ", "salad" to "サラダ", "soup" to "スープ", "pasta" to "パスタ", "spaghetti" to "スパゲッティ", "steak" to "ステーキ",
        "meat" to "肉", "fried_chicken" to "唐揚げ", "egg" to "卵", "tempura" to "天ぷら", "dumpling" to "餃子",
        "bread" to "パン", "cake" to "ケーキ", "cookie" to "クッキー", "chocolate" to "チョコレート", "ice_cream" to "アイスクリーム", "dessert" to "デザート",
        "donut" to "ドーナツ", "pancake" to "パンケーキ", "cheese" to "チーズ", "milk" to "牛乳", "yogurt" to "ヨーグルト", "tofu" to "豆腐",
        "apple" to "りんご", "banana" to "バナナ", "orange" to "オレンジ", "strawberry" to "いちご", "grape" to "ぶどう", "lemon" to "レモン",
        "watermelon" to "すいか", "peach" to "もも", "cherry" to "さくらんぼ", "tomato" to "トマト", "carrot" to "にんじん", "cabbage" to "キャベツ",
        "lettuce" to "レタス", "onion" to "たまねぎ", "potato" to "じゃがいも", "broccoli" to "ブロッコリー", "cucumber" to "きゅうり",
        "mushroom" to "きのこ", "corn" to "とうもろこし", "eggplant" to "なす", "pumpkin" to "かぼちゃ", "pepper" to "ピーマン",
        "coffee" to "コーヒー", "tea" to "お茶", "beer" to "ビール", "wine" to "ワイン", "juice" to "ジュース", "water" to "水",
        // 植物・花
        "rose" to "バラ", "tulip" to "チューリップ", "sunflower" to "ひまわり", "cherry_blossom" to "桜", "daisy" to "デイジー", "lily" to "ユリ",
        "orchid" to "ラン", "hydrangea" to "アジサイ", "lotus" to "ハス", "dandelion" to "タンポポ", "cactus" to "サボテン", "succulent" to "多肉植物",
        "houseplant" to "観葉植物", "bonsai" to "盆栽", "fern" to "シダ", "moss" to "コケ", "grass" to "草", "leaf" to "葉", "maple" to "もみじ",
        "bamboo" to "竹", "pine" to "松",
        // 動物
        "dog" to "犬", "cat" to "猫", "bird" to "鳥", "fish" to "魚", "rabbit" to "うさぎ", "hamster" to "ハムスター", "horse" to "馬", "cow" to "牛",
        "pig" to "豚", "sheep" to "羊", "duck" to "アヒル", "insect" to "虫", "butterfly" to "チョウ", "bee" to "ハチ",
        "turtle" to "カメ", "frog" to "カエル", "deer" to "シカ",
        // 物
        "book" to "本", "notebook" to "ノート", "pen" to "ペン", "bottle" to "ボトル", "cup" to "カップ", "can" to "缶", "plastic_bag" to "ビニール袋",
        "cardboard" to "段ボール", "box" to "箱", "bag" to "かばん", "shoe" to "靴", "clothing" to "服", "shirt" to "シャツ", "jacket" to "ジャケット",
        "computer" to "パソコン", "laptop" to "ノートパソコン", "phone" to "電話", "television" to "テレビ", "camera" to "カメラ", "watch" to "腕時計",
        "chair" to "いす", "table" to "テーブル", "bed" to "ベッド", "car" to "車", "bicycle" to "自転車", "train" to "電車", "bus" to "バス",
        "airplane" to "飛行機", "boat" to "船",
        // 風景・場所
        "sky" to "空", "cloud" to "雲", "mountain" to "山", "sea" to "海", "beach" to "浜辺", "river" to "川", "lake" to "湖", "forest" to "森",
        "park" to "公園", "temple" to "寺", "shrine" to "神社", "castle" to "城", "bridge" to "橋", "tower" to "塔", "street" to "通り",
    )

    /** label / labels で飛ばす大まかな語。具体的なラベルが無ければ使う。 */
    internal val generic: Set<String> = setOf(
        "food", "meal", "dish", "drink", "beverage", "plant", "animal", "mammal", "people", "adult", "structure", "material",
        "outdoor", "indoor", "consumables", "container", "conveyance", "vegetation", "liquid", "furniture", "machine", "tool",
        "produce", "document", "text", "clothing", "art", "land", "water_body", "sky",
    )

    /** 写っている物の名前(大まかな語を除き、日本語に直して重複を除く)。最大 [max] 個。 */
    fun names(labels: List<String>, max: Int): List<String> {
        val specific = labels.filter { it.lowercase() !in generic }
        return specific.ifEmpty { labels }.map(::japanese).distinct().take(max)
    }

    /** ML Kit のラベル("Ice cream" など)を表の形(小文字・空白は `_`)に寄せる。 */
    fun key(mlKitLabel: String): String = mlKitLabel.trim().lowercase().replace(' ', '_')
}

/** 画素の色を 12 色の名前に振り分ける(iOS の `PhotoColor`・純関数)。 */
object PhotoColor {
    /** 0〜1 の RGB → 色の名前。 */
    fun name(r: Double, g: Double, b: Double): String {
        val maxC = max(r, max(g, b))
        val minC = min(r, min(g, b))
        val v = maxC
        val d = maxC - minC
        val s = if (maxC > 0) d / maxC else 0.0
        if (v < 0.2) return "黒"
        if (s < 0.18) return if (v > 0.85) "白" else "灰色"
        var h = when (maxC) {
            r -> 60 * ((g - b) / d)
            g -> 60 * ((b - r) / d + 2)
            else -> 60 * ((r - g) / d + 4)
        }
        if (h < 0) h += 360
        return when {
            h < 15 || h >= 330 -> if (v < 0.55 && s > 0.4) "茶色" else if (s < 0.5 && v > 0.75) "ピンク" else "赤"
            h < 45 -> if (v < 0.6) "茶色" else "オレンジ"
            h < 70 -> if (v < 0.5) "茶色" else "黄色"
            h < 165 -> "緑"
            h < 200 -> "水色"
            h < 255 -> "青"
            h < 290 -> "紫"
            else -> "ピンク"
        }
    }

    /** 0xRRGGBB(上位の alpha は無視)→ 色の名前。 */
    fun name(rgb: Int): String =
        name(((rgb shr 16) and 0xFF) / 255.0, ((rgb shr 8) and 0xFF) / 255.0, (rgb and 0xFF) / 255.0)

    /** 画素の中でいちばん多い色の名前(画素が無ければ null)。同数は名前順で決める(毎回同じに)。 */
    fun dominant(pixels: List<Int>): String? = pixels.groupingBy(::name).eachCount().entries
        .maxWithOrNull(compareBy<Map.Entry<String, Int>> { it.value }.thenByDescending { it.key })?.key
}

/** OCR の 1 断片。座標は画像のピクセル(左上原点)。 */
data class OcrPiece(val text: String, val left: Float, val top: Float, val width: Float, val height: Float) {
    private val midY: Float get() = top + height / 2

    companion object {
        /** 縦の中心が近い断片(高さの半分以内)を 1 行にし、左から並べる。行は上から(iOS の `OCRPiece.lines`)。 */
        fun lines(pieces: List<OcrPiece>): List<String> {
            val rows = mutableListOf<MutableList<OcrPiece>>()
            for (p in pieces.sortedBy { it.midY }) {
                val first = rows.lastOrNull()?.first()
                if (first != null && abs(first.midY - p.midY) < max(first.height, p.height) / 2) rows.last() += p else rows += mutableListOf(p)
            }
            return rows.map { row -> row.sortedBy { it.left }.joinToString(" ") { it.text } }
        }
    }
}
