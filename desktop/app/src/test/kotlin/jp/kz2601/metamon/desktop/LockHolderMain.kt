package jp.kz2601.metamon.desktop

import java.io.File

/** 2 つのプロセスのロックの確かめ(RobustnessTest)。子のプロセスでロックを取り、標準入力が閉じるまで持つ。 */
object LockHolderMain {
    @JvmStatic
    fun main(args: Array<String>) {
        if (!SingleInstance.claim(File(args[0]))) {
            println("busy")
            return
        }
        SingleInstance.onHandoff {
            println("got " + it.joinToString(","))
            System.out.flush()
        }
        println("claimed")
        System.out.flush()
        while (System.`in`.read() >= 0) Unit
    }
}
