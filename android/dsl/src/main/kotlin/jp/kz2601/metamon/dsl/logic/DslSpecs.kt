package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.ActionBookLookup
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.CalendarEvent
import jp.kz2601.metamon.dsl.ImportColumn

// 生成した DSL の型(Screen・Action・Component)から logic の型へ写す入口(#1018)。呼び出し側で詰め替えない。

/** フォーム画面(`screen.entity`)を持つ entity id(iOS の `dsl.screens.compactMap(\.entity)`)。 */
val AppDSL.formEntityIds: Set<String> get() = screens.mapNotNullTo(linkedSetOf()) { it.entity }

/** `addToCalendar.calendarEvent` → [CalendarEventRequest]。 */
val CalendarEvent.request: CalendarEventRequest
    get() = CalendarEventRequest(
        startField = startField,
        endField = endField,
        durationMinutes = durationMinutes,
        titleField = titleField,
        locationField = locationField,
        notesField = notesField,
        alertMinutes = alertMinutes,
    )

/** `lookupBook.bookLookup` → [BookLookupFields]。 */
val ActionBookLookup.fields: BookLookupFields
    get() = BookLookupFields(isbnField, titleField, authorsField, publisherField, publishedDateField)

/** `fileImport.map` → [CsvColumnMapping] の並び。 */
val List<ImportColumn>.csvMappings: List<CsvColumnMapping> get() = map { CsvColumnMapping(it.column, it.field) }
