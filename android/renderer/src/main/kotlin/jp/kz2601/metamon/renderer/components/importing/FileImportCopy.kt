package jp.kz2601.metamon.renderer.components.importing

import android.content.res.Resources
import jp.kz2601.metamon.dsl.logic.ImportRowFailure
import jp.kz2601.metamon.dsl.logic.RecordImport
import jp.kz2601.metamon.renderer.R

/** fileImport の確認・結果・失敗の文(iOS の FileImportLeaf の summary / failureText / problem)。 */
internal object FileImportCopy {
    fun message(res: Resources, m: ImportMessage): String = when (m) {
        is ImportMessage.Csv -> csv(res, m)
        is ImportMessage.Body -> buildList {
            add(res.getString(R.string.renderer_import_body, m.fileName, m.characters, m.entityName))
            if (m.truncated) add(res.getString(R.string.renderer_import_body_truncated, m.characters))
        }.joinToString("\n")
    }

    /** 足す件数・使う列・使わない列(8 つまで)・読めない行(最初の 3 つ)・上限で入れない行。 */
    private fun csv(res: Resources, m: ImportMessage.Csv): String {
        val sep = res.getString(R.string.renderer_import_separator)
        val lines = mutableListOf(res.getString(R.string.renderer_import_csv_count, m.entityName, m.count))
        val used = m.used.joinToString(sep) { (column, field) -> if (column == field) column else "$column→$field" }
        lines += res.getString(R.string.renderer_import_csv_used, used)
        val unused = m.unused.take(8).joinToString(sep)
        if (unused.isNotEmpty()) lines += res.getString(R.string.renderer_import_csv_unused, unused)
        if (m.failures.isNotEmpty()) {
            lines += res.getString(R.string.renderer_import_csv_failures, m.failures.size)
            m.failures.take(3).forEach { lines += "・" + failure(res, it) }
        }
        if (m.overLimit > 0) {
            lines += res.getString(R.string.renderer_import_csv_over, RecordImport.MAX_ROWS, RecordImport.MAX_RECORDS_PER_ENTITY, m.overLimit)
        }
        return lines.joinToString("\n")
    }

    private fun failure(res: Resources, f: ImportRowFailure): String = when (f.kind) {
        ImportRowFailure.Kind.INVALID -> res.getString(R.string.renderer_import_failure_invalid, f.line, f.fieldName, f.cell)
        ImportRowFailure.Kind.REQUIRED -> res.getString(R.string.renderer_import_failure_required, f.line, f.fieldName)
        ImportRowFailure.Kind.TOO_LONG -> res.getString(R.string.renderer_import_failure_too_long, f.line, f.fieldName)
    }

    fun problem(res: Resources, p: ImportProblem): String = when (p) {
        ImportProblem.Unsupported -> res.getString(R.string.renderer_import_problem_unsupported)
        ImportProblem.CannotOpen -> res.getString(R.string.renderer_import_problem_open)
        is ImportProblem.TooLarge -> res.getString(R.string.renderer_import_problem_too_large, p.megabytes)
        ImportProblem.Encoding -> res.getString(R.string.renderer_import_problem_encoding)
        is ImportProblem.NoMatchingColumns ->
            res.getString(R.string.renderer_import_problem_columns, p.entityName, p.fieldNames.joinToString(res.getString(R.string.renderer_import_separator)))
        is ImportProblem.NothingToAdd -> csv(res, p.summary)
        ImportProblem.EmptyText -> res.getString(R.string.renderer_import_problem_empty_text)
        ImportProblem.PdfUnreadable -> res.getString(R.string.renderer_import_problem_pdf_unreadable)
        ImportProblem.Full -> res.getString(R.string.renderer_import_problem_full)
        ImportProblem.KindUnavailable -> res.getString(R.string.renderer_import_problem_kind_unavailable)
    }

    fun result(res: Resources, added: Int, total: Int): String =
        if (added == total) res.getString(R.string.renderer_import_added, added)
        else res.getString(R.string.renderer_import_added_partly, added, total - added)
}
