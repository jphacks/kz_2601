package jp.kz2601.metamon.voice

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** 「元に戻す」の戻り先の選び方(#1786・iOS の `UndoWalkTests` と同じケース)。 */
class UndoWalkTest {
    private val history = listOf(
        VersionInfo(4, parentVersion = 3, kind = "restore", restoredFrom = 2),
        VersionInfo(3, parentVersion = 2, kind = "edit"),
        VersionInfo(2, parentVersion = 1, kind = "edit"),
        VersionInfo(1, kind = "create"),
    )

    @Test
    fun targetSkipsRestoreVersions() {
        assertEquals(2, UndoWalk.undoTarget(3, history))
        assertEquals("v4 は v2 の中身なので、その 1 つ前の v1 へ", 1, UndoWalk.undoTarget(4, history))
        assertNull("最初の版より前は無い", UndoWalk.undoTarget(1, history))
    }

    @Test
    fun originReadsRestoreChainsThrough() {
        val byVersion = UndoWalk.byVersion(history + VersionInfo(5, parentVersion = 4, kind = "restore", restoredFrom = 4))
        assertEquals("v5 → v4 → v2", 2, UndoWalk.origin(5, byVersion))
        assertEquals(3, UndoWalk.origin(3, byVersion))
    }

    @Test
    fun originStopsOnACycle() {
        val loop = UndoWalk.byVersion(
            listOf(
                VersionInfo(2, kind = "restore", restoredFrom = 3),
                VersionInfo(3, kind = "restore", restoredFrom = 2),
            ),
        )
        assertEquals("一度見た版に戻ってきたら止まる(無限に回らない)", 2, UndoWalk.origin(2, loop))
    }

    @Test
    fun targetWithoutParentFallsBackToThePreviousListedVersion() {
        val old = listOf(VersionInfo(7), VersionInfo(5))
        assertEquals(5, UndoWalk.undoTarget(7, old))
        assertNull(UndoWalk.undoTarget(5, old))
    }

    @Test
    fun emptyHistoryHasNoTarget() {
        assertNull(UndoWalk.undoTarget(1, emptyList()))
    }
}
