package ai

import "strings"

// sensorRules (#419): iPhone sensors (tilt / shake / rotation / pressure /
// altitude) map to sensorValue; temperature and humidity have no sensor on
// iPhone, so the model must say so instead of faking it with another sensor.
// Lines routing to the part use 「…」は sensorValue so a review build
// (restrict.go) drops them; the decline lines stay.
var sensorRules = strings.TrimSpace(`
## 端末のセンサー(#419)
- 「水平器」「スマホの傾き・角度を測る」は sensorValue(sensor tilt・style level。前後だけ / 左右だけなら pitch / roll)。entity も入力フォームも要らない。
- 「揺れ・振動・衝撃・急ブレーキを測る」は sensorValue(sensor acceleration・単位 G・style gauge で min 0 max 2)。回転の速さは sensor rotation(°/s)。
- 「気圧を見る・記録する」「高度・何階分登った・坂の上り下り」は sensorValue(sensor pressure(hPa)/ altitude(m・画面を開いた時を 0 とした変化で標高ではない))。
- 「〜を記録」「1 分ごとに記録」は sensorValue に entity + field + dateField(+ 一定間隔なら seconds・件数の上限 limit)。entity は reading{value: number(unit は °・G・hPa・m), at: datetime(required, defaultValue now)} のように値と時刻だけ、**フォーム画面は作らない**(押した時・一定間隔で部品が保存する)。同じ画面に list(新しい順)か chart を置いて記録を見せる。記録は画面を開いている間だけ(閉じると止まる)。
- 「〜度を超えたら知らせて・警告して」は sensorValue.threshold{above(下回ったらなら below), message}。そのとき記録も残すなら save: true(entity が要る)。通知も出すなら action scheduleNotification。
- **温度・気温・室温・体温・湿度は iPhone にセンサーが無いので測れない**。気圧や他のセンサーで代用・推定しない。代わりに手で入力する記録(number field・unit ℃ / %)を作り、app.description(編集は summary)に「iPhone には温度・湿度のセンサーが無いため自動では測れません。温度計の値を手で記録する形にしました。」と書く。明るさ・騒音(周りの音の大きさ)も測れない。
`)

// sensorChatRule goes into both chat prompts (answers to 「温度測れる?」).
var sensorChatRule = strings.TrimSpace(`
## 端末のセンサー(#419)
- 「傾き・揺れ・回転・気圧・高度の変化を測る」は sensorValue(画面を開いている間だけ測って表示し、押した時・一定間隔で記録でき、「〇度を超えたら知らせる」もできる)。
- **温度・湿度は iPhone にセンサーが無いので測れない**。聞かれたら「iPhone には温度・湿度のセンサーが無いので測れません」とはっきり言い、代わりに温度計の値を手で記録するアプリを添える。外付けのセンサーとつなぐことも今はできない。明るさ・騒音も測れない。
`)

func init() {
	systemPrompt += "\n\n" + sensorRules
	// right after the hand-written capabilities (the create prompt must keep
	// ending with the capability index, the edit prompt with the catalog)
	withSensor := chatCapabilities + "\n\n" + sensorChatRule
	chatEditSystemPrompt = strings.Replace(chatEditSystemPrompt, chatCapabilities, withSensor, 1)
	chatCreateSystemPrompt = strings.Replace(chatCreateSystemPrompt, chatCapabilities, withSensor, 1)
	t := dslTerm{"sensorValue", "センサーの数字", "sensor card"}
	dslTerms = append(dslTerms, t)
	termIndex[strings.ToLower(t.term)] = t
}
