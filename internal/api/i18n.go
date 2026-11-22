package api

import (
	"net/http"
	"strconv"
	"strings"
	"unicode"
)

// 言語は要求ごとに保持し、並行する別ブラウザーの選択に影響させない。
type languageWriter struct {
	http.ResponseWriter
	english bool
}

// WebSocketとResponseControllerが元のライターの機能を利用できるようにする。
func (w languageWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func prefersEnglish(value string) bool {
	best, english := -1.0, false
	for _, part := range strings.Split(value, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		tag := strings.ToLower(strings.TrimSpace(fields[0]))
		if tag != "ja" && !strings.HasPrefix(tag, "ja-") && tag != "en" && !strings.HasPrefix(tag, "en-") {
			continue
		}
		q := 1.0
		for _, field := range fields[1:] {
			if key, number, ok := strings.Cut(strings.TrimSpace(field), "="); ok && key == "q" {
				parsed, err := strconv.ParseFloat(number, 64)
				if err != nil || parsed < 0 || parsed > 1 {
					q = 0
				} else {
					q = parsed
				}
			}
		}
		if q > 0 && q > best {
			best, english = q, strings.HasPrefix(tag, "en")
		}
	}
	return english
}

// 日本語は既存の保存文言を正とし、翻訳はHTTP境界でのみ適用する。
func writeErr(w http.ResponseWriter, code int, what, why, todo string) {
	english := false
	if localized, ok := w.(languageWriter); ok {
		english = localized.english
	}
	msg := what
	if english {
		msg = englishMessage(what)
		if why != "" {
			msg += " (Reason: " + englishMessage(why) + ")"
		}
		if todo != "" {
			msg += " Action: " + englishMessage(todo)
		}
		w.Header().Set("Content-Language", "en")
	} else {
		if why != "" {
			msg += "（原因: " + why + "）"
		}
		if todo != "" {
			msg += " 対処: " + todo
		}
		w.Header().Set("Content-Language", "ja")
	}
	w.Header().Add("Vary", "Accept-Language")
	writeJSON(w, code, map[string]string{"error": msg})
}

func englishMessage(value string) string {
	if translated, ok := errorEnglish[value]; ok {
		return translated
	}
	for _, pair := range errorFragments {
		value = strings.ReplaceAll(value, pair[0], pair[1])
	}
	for _, r := range value {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			// 未知の下位層エラーを推測翻訳せず、診断先を示す。
			return "The server reported an error; check the input and retry."
		}
	}
	return value
}

var errorFragments = [][2]string{
	{"Ping間隔は30〜3600秒の整数で指定してください", "Enter an integer ping interval between 30 and 3600 seconds"},
	{"JSONの形式が不正です: ", "Invalid JSON: "},
	{"行にエラーがあります", " rows contain errors"},
	{"1回の上限は", "The limit per request is "},
	{"件です", " targets"},
	{" は監視中に見つかったIPではありません", " is not a monitored IP address"},
	{"画像ファイルではありません（", "This is not an image ("},
	{"）", ")"},
}

var errorEnglish = map[string]string{
	"プロファイルIDが不正です":                                               "Invalid profile ID",
	"表示中のプロファイルが変更されました":                                          "The active profile has changed",
	"画面を更新してから再試行してください":                                          "Refresh the page and try again",
	"グループ名が不正です":                                                  "Invalid group name",
	"作業中のグループは名前を変更できません":                                         "Cannot rename a group while its work mode is active",
	"作業中を終了してから名前を変更してください":                                       "End work mode before renaming the group",
	"グループ名を保存できません":                                               "Could not save the group name",
	"グループ順が不正です":                                                  "Invalid group order",
	"グループ順を保存できません":                                               "Could not save the group order",
	"内部エラーが発生しました":                                                "An internal error occurred",
	"ログを確認し、再現手順とともに報告してください":                                     "Check the logs and report the steps to reproduce the issue",
	"他サイトからの操作要求を拒否しました":                                          "The request from another website was rejected",
	"Originが一致しません":                                               "The request origin does not match",
	"AVLine-Sightの画面から操作してください":                                   "Perform this action from AVLine-Sight",
	"閲覧専用ロック中のため変更できません":                                          "Changes are unavailable while the read-only lock is enabled",
	"ヘッダーのロックを解除してから操作してください":                                     "Unlock the header control before trying again",
	"接続機器一覧を作れません":                                                "Unable to load the device inventory",
	"時刻の指定が不正です":                                                  "The specified time is invalid",
	"ts はRFC3339形式で指定します":                                         "Specify ts in RFC3339 format",
	"スナップショットがありません":                                              "No snapshot is available",
	"機器名CSVを読み込めません":                                              "Unable to import the device-name CSV",
	"機器名CSVが大きすぎます":                                               "The device-name CSV is too large",
	"上限は2MBです":                                                    "The maximum size is 2 MB",
	"ファイルを分けて読み込んでください":                                           "Split the file and import each part separately",
	"読込方法の指定が不正です":                                                "The import mode is invalid",
	"mode は merge か replace です":                                   "mode must be merge or replace",
	"エラーのある行があるため反映していません":                                        "Changes were not applied because some rows contain errors",
	"CSVを修正してから読み込み直してください":                                       "Correct the CSV and import it again",
	"反映できる行がありません":                                                "There are no rows to apply",
	"名称が入力された行がありません":                                             "No rows have a device name",
	"名称の列を記入してから読み込んでください":                                        "Fill in the name column and import the file again",
	"機器名を保存できません":                                                 "Unable to save device names",
	"スイッチ一覧を取得できません":                                              "Unable to load the switch list",
	"スイッチ設定が不正です":                                                 "The switch settings are invalid",
	"スイッチを保存できません":                                                "Unable to save the switch",
	"名前またはIPアドレスが不正です":                                            "The name or IP address is invalid",
	"名前とIPv4アドレス（例 192.168.0.239）を入力してください":                       "Enter a name and an IPv4 address (for example 192.168.0.239)",
	"SNMPポート番号が範囲外です":                                             "The SNMP port is out of range",
	"通常は161です":                                                    "The usual port is 161",
	"不正なID":                                                       "Invalid ID",
	"スイッチを削除できません":                                                "Unable to delete the switch",
	"スイッチが見つかりません":                                                "The switch was not found",
	"一覧を再読込してください":                                                "Reload the list",
	"不正なパラメータ":                                                    "Invalid parameter",
	"ポート設定が不正です":                                                  "The port settings are invalid",
	"ポート設定を保存できません":                                               "Unable to save the port settings",
	"診断ダンプを取得できません":                                               "Unable to retrieve the diagnostic dump",
	"ラベルを保存できません":                                                 "Unable to save the label",
	"match_type は mac か port、match_key が必要です":                     "match_type must be mac or port, and match_key is required",
	"予定IPはIPv4アドレスか空欄にしてください":                                     "The planned IP must be an IPv4 address or blank",
	"時系列を取得できません":                                                 "Unable to load historical metrics",
	"レイアウトを保存できません":                                               "Unable to save the layout",
	"背景画像を読み込めません":                                                "Unable to load the background image",
	"10MB以下のPNG/JPEG/SVGを指定してください":                                "Choose a PNG, JPEG or SVG file no larger than 10 MB",
	"PNG/JPEG/SVGを指定してください":                                       "Choose a PNG, JPEG or SVG file",
	"背景画像を保存できません":                                                "Unable to save the background image",
	"Watch Listを保存できません":                                          "Unable to save the watch list",
	"Pingできません":                                                   "Unable to ping",
	"IPv4アドレスが不正です":                                               "The IPv4 address is invalid",
	"登録スイッチ・観測済み機器・登録機器の最終IPではありません":                              "The IP does not belong to a registered switch, observed device or last known device address",
	"任意のIPは接続機器画面の一括Pingで手入力してください":                               "Enter other IP addresses manually using bulk ping on the Devices screen",
	"一括Pingできません":                                                 "Unable to run bulk ping",
	"任意のIPは手入力欄に入れてください":                                          "Enter other IP addresses in the manual input field",
	"手入力のIPを解釈できません":                                              "Unable to parse the manually entered IP addresses",
	"例: 192.168.1.10、192.168.1.10-50、192.168.1.0/24":              "Examples: 192.168.1.10, 192.168.1.10-50, 192.168.1.0/24",
	"宛先が多すぎます":                                                    "Too many targets",
	"範囲を分けて実行してください":                                              "Split the range and run each part separately",
	"Pingの宛先がありません":                                               "No ping targets were specified",
	"機器を選ぶか、IPを手入力してください":                                         "Select devices or enter IP addresses manually",
	"絞り込んでから実行してください":                                             "Narrow the selection before trying again",
	"イベントを取得できません":                                                "Unable to load events",
	"確認済みにできません":                                                  "Unable to acknowledge the events",
	"確認を取り消せません":                                                  "Unable to undo acknowledgement",
	"イベントIDが不正です":                                                 "Invalid event ID",
	"アラートが見つかりません":                                                "The alert was not found",
	"ベースライン一覧を取得できません":                                            "Unable to load baselines",
	"ベースラインを保存できません":                                              "Unable to save the baseline",
	"ベースラインが見つかりません":                                              "The baseline was not found",
	"削除できません":                                                     "Unable to delete",
	"書出条件が不正です":                                                   "The export options are invalid",
	"マーカーが見つかりません":                                                "The marker was not found",
	"書出期間が不正です":                                                   "The export period is invalid",
	"開始が終了より後です":                                                  "The start time is after the end time",
	"書出に失敗しました":                                                   "The export failed",
	"ディスク空き容量を確認してください":                                           "Check the available disk space",
	"未対応の種別です":                                                    "The requested type is unsupported",
	"events / connections / ports のいずれか":                          "Choose events, connections or ports",
	"ファイルを受け取れません":                                                "Unable to receive the file",
	".avsight ファイルを指定してください":                                      "Choose an .avsight file",
	"作業領域を作成できません":                                                "Unable to create the workspace",
	"ファイルを保存できません":                                                "Unable to save the file",
	"ログパックを読み込めません":                                               "Unable to import the log pack",
	"再生セッションが見つかりません":                                             "The replay session was not found",
	"期限切れまたは閉じられています":                                             "The session has expired or was closed",
	"もう一度ファイルを読み込んでください":                                          "Import the file again",
	"探索条件が不正です":                                                   "The discovery options are invalid",
	"探索範囲が不正です":                                                   "The discovery range is invalid",
	"例: 192.168.0.0/24 または 192.168.0.10-192.168.0.50（最大1024アドレス）": "Examples: 192.168.0.0/24 or 192.168.0.10-192.168.0.50 (up to 1024 addresses)",
	"障害注入の指定が不正です":                                                "The fault injection options are invalid",
	"障害を注入できません":                                                  "Unable to inject the fault",
	"運用フェーズを変更できません":                                              "Unable to change the operating phase",
	"運用フェーズが不正です":                                                 "The operating phase is invalid",
	"setup/rehearsal/show から指定してください":                             "Choose setup, rehearsal or show",
	"運用フェーズを保存できません":                                              "Unable to save the operating phase",
	"グループ一覧を取得できません":                                              "Unable to load groups",
	"作業中を開始できません":                                                 "Unable to start work mode",
	"group と minutes が必要です":                                       "group and minutes are required",
	"作業時間が不正です":                                                   "The work duration is invalid",
	"15/30/60/120 分から指定してください":                                    "Choose 15, 30, 60 or 120 minutes",
	"グループを確認できません":                                                "Unable to check the group",
	"グループが見つかりません":                                                "The group was not found",
	"作業中を保存できません":                                                 "Unable to save work mode",
	"作業中を解除できません":                                                 "Unable to end work mode",
	"カウンタをリセットできません":                                              "Unable to reset the counters",
	"group が必要です":                                                 "group is required",
	"ロック要求が不正です":                                                  "The lock request is invalid",
	"PINを変更できません":                                                 "Unable to change the PIN",
	"既にPIN付きでロックされています":                                           "The read-only lock already has a PIN",
	"一度現在のPINで解除してから設定し直してください":                                   "Unlock with the current PIN before setting a new one",
	"ロックを解除できません":                                                 "Unable to unlock",
	"PINが一致しません":                                                  "The PIN does not match",
	"ロック時に設定したPINを入力してください":                                       "Enter the PIN used when locking",
	"NIC一覧を取得できません":                                               "Unable to load network interfaces",
	"待受設定が不正です":                                                   "The listener settings are invalid",
	"trap_addr が必要です":                                             "trap_addr is required",
	"例: 0.0.0.0:1162":                                             "Example: 0.0.0.0:1162",
	"設定を保存できません":                                                  "Unable to save settings",
	"プロファイル一覧を取得できません":                                            "Unable to load profiles",
	"プロファイルを作成できません":                                              "Unable to create the profile",
	"名前が空です":                                                      "The name is empty",
	"現場名を入力してください":                                                "Enter the site name",
	"プロファイルを切替できません":                                              "Unable to switch profiles",
	"プロファイルが見つかりません":                                              "The profile was not found",
	"プロファイルを読み込めません":                                              "Unable to import the profile",
	"JSON形式が不正です":                                                 "The JSON format is invalid",
	"エクスポートしたファイルを指定してください":                                       "Choose an exported profile file",
	"閲覧専用ロック中はターミナルを使用できません":                                      "The terminal is unavailable while the read-only lock is enabled",
	"ロックを解除してください":                                                "Disable the read-only lock",
	"接続を拒否しました":                                                   "The connection was rejected",
	"登録されていないスイッチです":                                              "The switch is not registered",
	"スイッチを登録してから接続してください":                                         "Register the switch before connecting",
}
