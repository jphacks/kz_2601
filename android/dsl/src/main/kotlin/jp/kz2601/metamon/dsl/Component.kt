package jp.kz2601.metamon.dsl

import kotlinx.serialization.Serializable

// 骨格(#356)の仮の型。#351 の生成器(backend/cmd/dslgen)ができたら、その出力(generated/)に置き換える。
// 形は #351・#354 §6 の「平らな 1 クラス + UNKNOWN に丸める enum」に合わせてある。

/** 部品の種類。docs/dsl/dsl.schema.json の Container の enum と Leaf の const。知らない値は [UNKNOWN]。 */
@Serializable(with = ComponentTypeSerializer::class)
enum class ComponentType(val wire: String, val isContainer: Boolean = false) {
    VSTACK("vstack", true),
    HSTACK("hstack", true),
    SECTION("section", true),
    CARD("card", true),
    DISCLOSURE("disclosure", true),
    CAROUSEL("carousel", true),
    GRID("grid", true),
    TEXT("text"),
    BUTTON("button"),
    TEXT_FIELD("textField"),
    NUMBER_FIELD("numberField"),
    DATE_PICKER("datePicker"),
    PICKER("picker"),
    TOGGLE("toggle"),
    PHOTO_FIELD("photoField"),
    IMAGE("image"),
    VIDEO_FIELD("videoField"),
    VIDEO_PLAYER("videoPlayer"),
    AUDIO_RECORDER("audioRecorder"),
    TRANSCRIBE_FIELD("transcribeField"),
    SPEAK_BUTTON("speakButton"),
    LIST("list"),
    STAT("stat"),
    CHART("chart"),
    VOICE_INPUT("voiceInput"),
    SPACER("spacer"),
    DIVIDER("divider"),
    DRAWING_FIELD("drawingField"),
    SLIDER("slider"),
    STEPPER("stepper"),
    TEXT_EDITOR("textEditor"),
    SECURE_FIELD("secureField"),
    TIME_PICKER("timePicker"),
    COLOR_PICKER("colorPicker"),
    RATING("rating"),
    LABEL("label"),
    ICON("icon"),
    PROGRESS_BAR("progressBar"),
    GAUGE("gauge"),
    BADGE("badge"),
    LINK("link"),
    MARKDOWN("markdown"),
    AVATAR("avatar"),
    TABLE("table"),
    TIMELINE("timeline"),
    EMPTY_STATE("emptyState"),
    SEARCH_BAR("searchBar"),
    CALENDAR("calendar"),
    GRID_TABLE("gridTable"),
    SHARE_BUTTON("shareButton"),
    OPEN_URL_BUTTON("openUrlButton"),
    MAP_VIEW("mapView"),
    LOCATION_BUTTON("locationButton"),
    LOCATION_PICKER("locationPicker"),
    ROUTE_GUIDE("routeGuide"),
    PLACE_SEARCH("placeSearch"),
    QR_SCANNER("qrScanner"),
    QR_CODE("qrCode"),
    TIMER_DISPLAY("timerDisplay"),
    WEB_VIEW("webView"),
    SETTLEMENT("settlement"),
    RANDOM_PICKER("randomPicker"),
    BRACKET("bracket"),
    AI_BUTTON("aiButton"),
    NEARBY_SHARE("nearbyShare"),
    LOCATION_TRACKER("locationTracker"),
    CHAT_THREAD("chatThread"),
    AI_CHAT("aiChat"),
    HEALTH_STAT("healthStat"),
    DASHCAM("dashcam"),
    FILE_IMPORT("fileImport"),
    SENSOR_VALUE("sensorValue"),
    MUSIC_PLAYER("musicPlayer"),
    MATCH_LIST("matchList"),
    FLASHCARDS("flashcards"),
    FEED("feed"),

    /** 知らない type(新しい版の部品)。描画器は「更新すると表示」の枠を出す(#351 §5)。 */
    UNKNOWN(""),
}

internal object ComponentTypeSerializer : LenientEnumSerializer<ComponentType>(
    "jp.kz2601.metamon.dsl.ComponentType",
    ComponentType.entries,
    ComponentType::wire,
    ComponentType.UNKNOWN,
)

/** 部品 1 つ。全部品の props を平らに持つ(骨格では描画に要るものだけ。残りは生成器で足す)。 */
@Serializable
data class Component(
    val type: ComponentType = ComponentType.UNKNOWN,
    val id: String? = null,
    /** グリッドの位置("A1:F12")。screen 直下の部品だけ。 */
    val area: String? = null,
    val title: String? = null,
    val text: String? = null,
    val label: String? = null,
    /** voiceInput の呼びかけ。 */
    val prompt: String? = null,
    /** small | medium | large */
    val size: String? = null,
    /** text の書体(title | headline | body | caption)・button の種類など。 */
    val style: String? = null,
    /** text の寄せ(leading | center | trailing)。 */
    val align: String? = null,
    val action: Action? = null,
    /** container の間隔: none | small | medium | large */
    val spacing: String? = null,
    /** disclosure の初期の開閉。 */
    val expanded: Boolean? = null,
    /** carousel の 1 枚の幅: small | medium | large */
    val itemWidth: String? = null,
    /** grid の列数(2〜4)。 */
    val columnCount: Int? = null,
    @Serializable(with = LossyListSerializer::class)
    val children: List<Component> = emptyList(),
)
