package android.content

/** デスクトップでは使わない型の置き換え。デスクトップの保存は desktop/app の `store/JsonKeyValueStore`。 */
interface SharedPreferences {
    fun contains(key: String): Boolean
    fun getInt(key: String, defValue: Int): Int
    fun getLong(key: String, defValue: Long): Long
    fun getBoolean(key: String, defValue: Boolean): Boolean
    fun getString(key: String, defValue: String?): String?
    fun edit(): Editor

    interface Editor {
        fun putInt(key: String, value: Int): Editor
        fun putLong(key: String, value: Long): Editor
        fun putBoolean(key: String, value: Boolean): Editor
        fun putString(key: String, value: String?): Editor
        fun remove(key: String): Editor
        fun apply()
    }
}
