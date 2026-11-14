package jp.kz2601.metamon.account

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.ViewModelStoreOwner

/**
 * 人ごとの ViewModel の置き場(#1167)。後片付け([AccountGuard.generation])で番号が変わったら、
 * 前の人の ViewModel を捨てる(onCleared まで走らせてメモリから外す)。
 * Activity の ViewModel なので画面の回転では残る。
 */
class PerAccountViewModels : ViewModel() {
    private var generation: Int? = null
    private var store = ViewModelStore()

    /** [generation] の人の置き場。番号が変わったら前の置き場を空にして新しくする。 */
    fun ownerFor(generation: Int): ViewModelStoreOwner {
        if (this.generation != null && this.generation != generation) {
            store.clear()
            store = ViewModelStore()
        }
        this.generation = generation
        val current = store
        return object : ViewModelStoreOwner {
            override val viewModelStore: ViewModelStore = current
        }
    }

    override fun onCleared() {
        store.clear()
    }
}
