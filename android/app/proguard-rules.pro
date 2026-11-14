# release の R8(#1134)。ライブラリの keep は各 AAR / JAR の consumer rules に任せ、ここには足りない分だけ書く。
# 確かめた consumer rules: kotlinx.serialization(@Serializable の Companion.serializer())・Firebase・Play Billing・
# ML Kit・MapLibre・PdfBox-Android(SecurityHandler の反射)・Coil 3・OkHttp・Media3・CameraX。
# whisper の JNI は native メソッドだけでコールバックが無いので、AGP 既定の keep(native メソッドを持つクラス名)で足りる。

# クラッシュのスタックトレースの行番号を残す(Crashlytics)
-keepattributes SourceFile,LineNumberTable
-renamesourcefileattribute SourceFile

# PdfBox-Android の JPEG2000(JPX)は別ライブラリ(jp2-android)の任意の依存で、入れていない。
# JPX の画像を含む PDF はその画像だけ読めない(文字の取り出しは動く)。PdfBox-Android の README の指示どおり
-dontwarn com.gemalto.jp2.JP2Decoder
-dontwarn com.gemalto.jp2.JP2Encoder
