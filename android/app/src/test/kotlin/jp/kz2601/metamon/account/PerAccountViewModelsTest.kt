package jp.kz2601.metamon.account

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotSame
import org.junit.Assert.assertSame
import org.junit.Test

class PerAccountViewModelsTest {
    private class Probe : ViewModel() {
        var cleared = 0
        init { addCloseable { cleared++ } }
    }

    private val factory = viewModelFactory { initializer { Probe() } }

    private fun probe(holder: PerAccountViewModels, generation: Int): Probe =
        ViewModelProvider.create(holder.ownerFor(generation), factory)["voice", Probe::class]

    @Test
    fun sameGenerationKeepsTheViewModel() {
        val holder = PerAccountViewModels()
        val first = probe(holder, 0)
        assertSame(first, probe(holder, 0))
        assertEquals(0, first.cleared)
    }

    @Test
    fun wipeDropsThePreviousPersonsViewModel() {
        val holder = PerAccountViewModels()
        val before = probe(holder, 0)
        val after = probe(holder, 1)
        assertNotSame(before, after)
        assertEquals(1, before.cleared) // 前の人の分は onCleared まで走ってメモリから外れる
        assertEquals(0, after.cleared)
    }

    @Test
    fun clearingTheHolderClearsTheCurrentOne() {
        val holder = PerAccountViewModels()
        val vm = probe(holder, 3)
        val store = ViewModelStore()
        ViewModelProvider.create(store, viewModelFactory { initializer { holder } })["h", PerAccountViewModels::class]
        store.clear()
        assertEquals(1, vm.cleared)
    }
}
