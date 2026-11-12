package android.app

/**
 * デスクトップでは使わない型の置き換え。共有する `AuthService` の引数(Apple のサインイン)にだけ出てくる。
 * デスクトップのサインイン(#1749)では呼ばない。
 */
open class Activity
