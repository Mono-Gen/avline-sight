# AVLine-Sight API 契約（v1）

バックエンド（Go）とWebUI（React）の間の契約。型の正は `internal/model/model.go` のJSONタグ。
時刻はすべて RFC3339（UTC）。エラー時は HTTP 4xx/5xx と `{"error": "日本語メッセージ"}`。
閲覧専用ロック中は変更系（POST/PUT/DELETE）が `423 Locked`。例外は `/api/mode/lock`、`/api/markers`、イベント確認（個別・一括）、再生関連、`/api/export/pack`。手動Ping・フェーズ変更・作業中変更・カウンタリセットはロック中に実行不可。

## 状態・システム
| メソッド | パス | 応答 |
|---|---|---|
| GET | `/api/status` | `{version, mode:"live", lab:bool, locked:bool, pin_set:bool, phase, profile:{id,name}, trap:{addr, ok, error}, web_addr}` |
| POST | `/api/mode/lock` | 入力 `{locked:bool, pin?:string}` → `{locked}`（解除時、PIN設定済みならPIN必須） |
| GET | `/api/system/nics` | `[{name, addrs:[string]}]` |
| GET/PUT | `/api/settings` | `Settings`（下記） |
| GET | `/api/phase` | `{phase:"setup"|"rehearsal"|"show"|"teardown"}`。既定 `show`、metaの `op_phase` に保存 |
| PUT | `/api/phase` | 入力 `{phase}` → `{phase}`。実際の変更時にINFO `PHASE_CHANGED` とWS `phase_changed` |

設営（`setup`）は `if_index>0` のポートアラートと `DEVICE_MISSING` / `PING_FAIL` / `IP_MISMATCH` を抑止。撤収（`teardown`）は全件抑止。リハ（`rehearsal`）と本番（`show`）は抑止なし。抑止中もイベントは保存するが、件数に含めず、抑止中の復旧はラッチしない。フェーズによる抑止を作業中より優先して `suppressed_by` に示す。

設営・撤収ではFastとMediumの間隔を設定値の3倍、Slowを2倍にする。リハ・本番は設定値どおり。ポーラーを再起動せずフェーズ変更を反映する。

## グループ・作業中

グループは既存の `switches.group_tag`（1台1グループ、空文字は未所属）。

| メソッド | パス | 入出力 |
|---|---|---|
| GET | `/api/groups` | `[{name, switch_ids:[id], work_until:RFC3339|null}]`。設定順、未指定のグループは名前順、未所属は最後。全体とグループ両方の作業中があれば遅い期限を表示 |
| PUT | `/api/groups/{name}` | `{new_name}` → グループ一覧。対象プロファイルの全スイッチを一括変更し、同名のグループがあれば統合。旧名はURLエスケープし、未所属は `/api/groups/`。空の新名と予約名 `*`、`.`、`..` は400、旧グループ独自の作業中は409 |
| PUT | `/api/groups/order` | `{names:[...]}` → グループ一覧。順序はプロファイルごとにmetaに保存。重複名・空名は400。`{new_name}` を送ると名前が `order` のグループを改名 |
| POST | `/api/work` | `{group, minutes:15|30|60|120}` → グループ一覧。`group:"*"` は全スイッチ。存在しないグループは404 |
| DELETE | `/api/work/{group}` | グループ名をURLエスケープして指定、`*` も可。未所属は `/api/work/`。解除済みの再解除も成功 → グループ一覧 |
| POST | `/api/counters/reset` | `{group}`（`"*"` は全台）→ `{results:[{switch_id,ok,reset_at?,error?}]}`。既存ソフトリセットを呼び、停止中・無効なスイッチは `ok:false` |

作業中はmetaの `work_mode` にグループ名→期限のJSONとして保存し、再起動で復元。プロファイルを切り替えても同名グループは期限を共有する。対象スイッチのアラートを抑止する。横断アラート（switch_id=0）は対象外。開始時INFO `WORK_STARTED`、手動解除・期限到達時INFO `WORK_ENDED`。期限切れは5秒周期と起動時に削除し、`work_changed`・`alerts_changed` を配信する。`*` の解除は全体指定のみを解除し、別途指定したグループの作業中は残す。

改名・順序保存・作業中の開始と解除・一括リセットで `X-AVLine-Profile` を指定すると、表示していたプロファイルIDが現在のIDと異なる場合に409を返す。旧データに `*` という名前のグループがある場合、改名だけが可能。旧名が `.` または `..` の場合は `/api/groups/?legacy_name=.` または `?legacy_name=..` で指定する。

`Settings`:
```json
{"thresholds":{"cpu_pct":80,"mem_free_pct":15,"temp_c":65,"poe_pct":80,"util_pct":80,"sfp_rx_dbm":-10,"sfp_drop_db":3,"err_delta":1},
 "alert_sound":true,
 "views":[{"id":"audio","name":"音響","vlans":[10],"tags":[],"categories":[]}]}
```

## プロファイル
`GET /api/profiles` → `[{id,name,active}]` / `POST /api/profiles {name}` / `PUT /api/profiles/:id/activate` /
`GET /api/profiles/:id/export`（JSONダウンロード）/ `POST /api/profiles/import`（JSON本文）

## スイッチ
| メソッド | パス | 説明 |
|---|---|---|
| GET | `/api/switches` | `[SwitchConfig]`（communityは `"****"` でマスク、`last_trap_ts:RFC3339|null` は最終Trap受信時刻） |
| POST | `/api/switches` | `SwitchConfig`（id無し）→ 作成結果 |
| PUT/DELETE | `/api/switches/:id` | 更新/削除 |
| GET | `/api/switches/:id/state` | `SwitchState`（全詳細） |
| PUT | `/api/switches/:id/ports/:ifIndex` | `{memo?, excluded?, expected_speed_mbps?, av_role?}`。av_role: `dante`/`aes67`/`ndi`/`ndi4`/`sdvoe`/`st2110`/`qsys`/`lighting`/`avb`/`control`/`uplink`/`""` |
| POST | `/api/switches/:id/counters/reset` | `{if_index:0}`（0=全ポート）ソフト・カウンタゼロリセット |
| POST | `/api/switches/:id/probe` | OID互換性プローブ再実行 |
| GET | `/api/switches/:id/diagnostic-dump` | JSONダウンロード |
| GET | `/api/switches/:id/access-info` | `{web_url, https, ssh:{enabled,port}, telnet:{port}}` |
| POST | `/api/device-aliases` | `{match_type:"mac"|"port", match_key, label, note, planned_ip?}`。予定IPはIPv4または空文字。不正値は400 |
| POST | `/api/device-aliases/import` | `{csv, mode:"merge"|"replace", commit:bool}` → `{committed, result:{added,updated,unchanged,deleted}, rows:[{line,mac,label,note,planned_ip,action:"add"|"update"|"same",old_label?}], skipped_empty, errors:[{line,reason}]}`。commit=false は試行のみ（書込なし）。エラー行があると commit は 422。上限2MB/5000行 |
| GET | `/api/devices?scope=all|unnamed&group=&vlans=10,20&q=` | 接続機器一覧 `[InventoryRow]`（MAC単位で所在1か所。スイッチ間リンクのみで見える機器は `via_uplink:true`。登録済みで見えない機器は `seen:false`）。`group=*` は全グループ・未観測の登録機器も含む |
| GET | `/api/devices/export.csv?…` | 同条件のCSV（BOM付きUTF-8、`group=*` 対応）。列: MAC,名称,メモ,予定IP,IP,メーカー,LLDP名,スイッチ,ポート,VLAN,PoE(W),リンク速度,最終確認,検出元,Ping |
| GET | `/api/replay/:sid/devices?ts=RFC3339&…` / `/api/replay/:sid/devices/export.csv?ts=…` | 再生時刻のスナップショット基準の一覧/CSV |

`InventoryRow`: `{mac,label,note,planned_ip,ip_mismatch,ip,ip_stale?,vendor,lldp_name,switch_id?,switch_name,group_tag,port,if_index?,vlan,poe_w?,speed_mbps,seen,last_seen?,source,via_uplink,ping?,ping_ok:bool|null,ping_rtt_ms:number|null,ping_ts:RFC3339|null}`。`ping` は既存互換の `{status,rtt_ms,at}`。Ping未実施は追加3フィールドがnull。結果はMAC単位で直近1件をメモリに保持し、再起動・プロファイル切替で消える。再生にはライブPing結果を付けない。

`GET /api/devices?include=registration` は `registered:bool` を追加し、MAC単位で名称登録された機器をポート名だけが設定された機器と区別する。ラック表示はこの情報で登録機器がつながるポートのリンク断を重大として表示する。

機器名CSVは第4列に `予定IP`（英語見出し `planned_ip`）を追加。旧3列CSV・旧書出CSV（第4列が観測IP）も読込可能。見出しがあれば列名で判定し、なければ先頭4列を使う。不正IPv4・IPv6は行エラーとなり、確定は422。予定IPも変更件数・プレビューの比較対象。`device_aliases.planned_ip` は起動時に既存DBへ自動追加し、旧再生DBの列欠落は空文字として読む。

登録機器は最終観測したスイッチが到達可能なとき、FDBの取得で2回連続不在ならWARNING `DEVICE_MISSING`。最初の不在から10秒後にFDBだけを再取得する（同じスイッチへの要求は集約し、その前にSlowでFDBを取得した場合は兼用）。登録機器の最終ポートでポーリングが `LINK_DOWN` を検知した場合はFDBの即時取得も要求する。未観測・到達不能・FDB取得失敗では発報せず、連続回数をリセットする。詳細は `mac,name,port,last_ip`。復帰で解消。予定IPが空でなく最終IPが判明し異なる場合は `ip_mismatch:true`、リハ・本番でWARNING `IP_MISMATCH`。

設営・リハのみ60秒ごとに登録MACを最大16並列でPingする。宛先は最後に観測したIP、なければ予定IP。定期Pingの2回連続失敗はMACごとにWARNING `PING_FAIL`、成功で解消。フェーズ変更時に進行中の処理を中断し失敗回数をリセットする。本番・撤収では定期送信なし。手動の1台・一括PingもIPからMACへ照合して表示結果を更新するが、定期失敗回数は変更しない。

## 監視ビュー
- `GET /api/overview` → `{switches:[SwitchSummary], alerts:{CRITICAL,WARNING,INFO}, recent_events:[Event]}`。件数は抑止されていない「発生中または未確認」のエントリ数（スイッチ別件数も同条件）
  - `SwitchSummary`: `{id,name,ip,group_tag,reachable,last_error,last_trap_ts:RFC3339|null,model,fw,uptime_sec,cpu_5s,mem_free_pct,mbuf_used_pct,temp_max_c,temp_state,fans_ok,psu_ok,poe_budget_w,poe_used_w,ports_up,ports_total,rx_bps,tx_bps,alerts:{CRITICAL,WARNING,INFO?}}`
- `GET /api/ports/search?q=&switch_id=&sort=err|poe|bw` → `[{switch_id, switch_name, port: Port}]`
- `GET /api/metrics/port?switch_id=&if_index=&from=&to=` → `{resolution:"raw"|"1m", points:[{ts, rx_bps, tx_bps, rx_mc_pps, tx_mc_pps, err_delta, fcs_delta, discard_delta, cos_drop_delta, pause_delta, poe_w, sfp_rx_dbm}]}`（from/to省略時は直近30分）
- `GET /api/metrics/switch?switch_id=&from=&to=` → `{resolution, points:[{ts, cpu_5s, mem_free_pct, mbuf_used_pct, temp_max_c, poe_used_w}]}`
- `GET /api/topology` → `{nodes:[{id, label, kind:"switch"|"external"|"device", switch_id?, reachable?, is_root?, x?, y?}], edges:[{id, source, target, local_port, remote_port, blocking, speed_mbps, util_pct}], background:string|null}`
- `PUT /api/topology/layout` → 入力 `{positions:{"<nodeId>":{x,y}}}`
- `POST /api/topology/background`（multipart `file`）→ `{url}`、`GET /api/topology/background` で画像
- `GET/PUT /api/watchlist` → `[{switch_id, if_index, label}]`
- `GET /api/multicast` → `{vlans:[VlanIGMP+{switch_id,switch_name}], groups:[McastGroup+{switch_id,switch_name}], mrouters:[Mrouter+{switch_id,switch_name}], gptp:[{switch_id,switch_name,...GPTP}], findings:[Finding]}`
- `GET /api/dhcp` → `{servers:[{switch_id,switch_name,server_on,pools,lease_count,decline_rx,snooping_on,invalid_server_msgs}], leases:[Lease+{switch_id,switch_name}], conflicts:[{switch_id,switch_name,ip}], findings:[Finding]}`
- `GET /api/audit` → `{findings:[Finding], fw_versions:{"13.0.7.6":["AV-SW-01",...]}}`
  - `Finding`: `{severity, code, title, hint, switch_id, switch_name, port?}`
- `POST /api/diagnostics/ping` 入力 `{ip}` → `{ok, status, rtt_ms, output, elapsed_ms}`（登録スイッチ・観測済み機器・登録機器の最終IP・予定IPのみ）
- `POST /api/diagnostics/ping-bulk` 入力 `{ips?:[...], manual?:"192.168.1.10, 192.168.1.20-40, 10.0.0.0/28"}` → `{results:[{ip,status:"ok"|"timeout"|"unreachable"|"error",rtt_ms,detail,source:"known"|"manual"}], ok, total, elapsed_ms, at}`。`ips` は上記の既知IPのみ（未知は403）。閲覧専用ロック中は（1台版も含め）423。1回256件まで、同時16並列・1宛先最大2回・1秒待ち

## イベント・マーカー
- `GET /api/events?category=&severity=&switch_id=&q=&from=&to=&limit=500` → `[Event]`（新しい順）
- `POST /api/events/:id/ack`、`POST /api/events/ack-all`
- `GET /api/alerts` → `[{event_id,switch_id,switch_name,if_index,severity,code,title,ts,active,acked,cleared_at:RFC3339|null,port,mac?,device_name?,state:"active"|"latched",suppressed,suppressed_by:"phase"|"work"|""}]`。抑止中の発生条件も含む。CRITICAL→WARNING→INFO、同重要度はts降順（同時刻はevent_id降順）
  - REST応答は保存イベント由来の翻訳引数 `detail`、対処文 `hint`、確認時刻 `acked_at` を取得できる場合に補う。WebSocket通知では省略する。
- `POST /api/markers` 入力 `{note}` → `Event`（category=MARKER）

発生時にイベントIDを持つエントリを作成し、未確認の復旧は `active:false,state:"latched"` と復旧時刻を保持する。復旧時刻はDBのnullable `cleared_at` 列（Unixミリ秒）へ永続化し、再発生時はnullへ戻す。旧データや再評価前など復旧時刻が不明な場合は `cleared_at:null` とし、画面には不明と表示する。同じキーが未確認の間に再発生したら同じイベントIDを再使用する。発生中の確認は `acked:true` として残り、確認済みで復旧すると削除。復旧後の確認は直ちに削除。DBの `acked_at` を更新するが確認者は記録しない。確認後は `alerts_changed` を全ブラウザへ配信する。

ポーラー由来の一過性CRITICAL/WARNING（SWITCH_REBOOT、POE_FAULT、RELINK_SLOWER、速度低下など）は発生ごとに独立した未確認ラッチとして保持する。継続条件がないため `active:false`、`cleared_at` は検知時刻とする。抑止中は履歴だけに保存する。通常のTrapとSYSLOGはラッチを作らない。重大イベントの配信には履歴・ラッチのどちらにも `detail._suppressed` を付け、音声通知もこの抑止に従う。手動Ping成功は該当MACの連続失敗回数とPING_FAILの発生条件を解除し、未確認分だけラッチとして残す。

起動・プロファイル切替で、対象現場の過去36時間の未確認CRITICAL/WARNINGイベント（通常のTrapを除く）を復旧済みとして復元し、再評価で発生中に戻す。復旧時刻は保存値を使い、不明な場合はnullのまま保持する。通常はスイッチID・コード・ポートごとに新しいものを優先するが、一過性ラッチは個々の発生を保持する。機器の3コード（DEVICE_MISSING/PING_FAIL/IP_MISMATCH）は所在が変わってもコード・MACごとに最新1件を復元し、最終観測した所在のグループで抑止を判定する（履歴イベントの発生場所は保持）。重要度が変化した場合は別の発生イベントとして記録し、古い未確認ラッチも確認まで残す。抑止中の復旧は再起動してもラッチしない。対象現場を特定できない旧横断イベントは復元対象外。

## SNMP Trap

- `/api/status` の `trap:{addr,ok,error}` で受信待受を確認する。登録スイッチの `last_trap_ts` は受信時刻を保持し、未受信は `null`。再起動後も保持する。
- 登録スイッチのTrapはFast取得を要求し、リンク・PoE・STP・ストーム・ErrDisable・再起動・LLDPではSlow取得も要求する。同じスイッチのTrapが続く場合は5秒に1回までに集約する。到達不能時は既存のバックオフに従う。未登録送信元はイベント履歴に記録する。
- Trapは状態を直接変更しない。`linkDown` Trap後の最初のFast取得でリンクが既に復帰していた場合、または連続するFast取得の間に両方でリンクがupのまま `ifLastChange` が進んだ場合、WARNING `LINK_FLAP` を履歴に残す。新規発生は未確認の `state:"latched"` で、抑止中は履歴にだけ記録する。ポートごとに最初の発生から60秒未満のフラップは同じイベントの `detail.count` に加える。ただし現在の抑止状態が直近の履歴と異なる場合は新規記録とし、抑止解除後の発生を通知対象としてラッチする。1回のFast取得までに複数の断復帰通知が届いた場合も、`linkUp` を挟む断は個別に数え、先に `ifLastChange` で検知した断のTrapは送信元の `sysUpTime` があれば二重計上を防ぐ。通常のTrapイベントは再起動時のラッチ復元対象外。

M4250でSNMPv2c Trapの送信先を設定するCLI例（要実機確認。リポジトリ内のCLI資料では構文を確認できない）:

```text
snmp-server host <PC IP> traps version snmpv2 <community>
```

M4250（FW 13.0.4.7）の工場出荷時・各AVプロファイル適用後の設定（`show running-config` の全表示）で確認できた事実:
- `snmp-server enable traps` と `linkmode`・`stpmode`・`multiusers` のTrapは既定で有効。各ポートも `snmp trap link-status` が既定で有効。
- Trapの送信先は既定で未設定。必要なのは送信先（このPCのIPとコミュニティ）の追加だけ。送信先を設定する行の書式は、設定済みの例が無いため未確認。
- 既定の設定にはSNMPv2cのコミュニティの行が無く、SNMPv3ユーザー `admin` のみが表示される。v2cで監視する場合は、コミュニティの設定が要るかを実機で確認する。

待受・ファイアウォールでTrap用UDPポート（既定162）を確認し、設定後は `last_trap_ts` の更新で受信を確認する。

## ベースライン
`GET /api/baselines` → `[{id,name,captured_ts}]` / `POST /api/baselines {name}` / `GET /api/baselines/:id/diff` → `{diffs:[{switch_name, item, before, after, severity}]}` / `DELETE /api/baselines/:id`

## 書出・読込・オフライン再生
- `POST /api/export/pack` 入力 `{from,to}` または `{marker_id, minutes}` → `.avsight`（application/zip）ダウンロード
- `GET /api/export/csv?type=events|connections|ports&from=&to=&bom=1` / `GET /api/export/json?type=...`
- `POST /api/replay/import`（multipart `file`）→ `{session_id, from, to, switches:[name]}`
- `GET /api/replay/:sid/timeline` → `{from, to, markers:[{ts, severity, title, category}]}`
- `GET /api/replay/:sid/snapshot?ts=` → `{ts, switches:[SwitchState]}`（ts以前で最新のスナップショット）
- `GET /api/replay/:sid/events?from=&to=` → `[Event]`
- `DELETE /api/replay/:sid`

## 探索・ラボ
- `POST /api/discovery/scan` 入力 `{cidr, community, port?}` → `{found:[{ip, port, sys_name, sys_descr, sys_object_id, registered}]}`
- `GET /api/discovery/lldp-candidates` → `[{ip, sys_name, via_switch, via_port}]`
- `GET /api/sim/scenarios` → `[{id,title}]`（ラボモード時のみ。非ラボは404）
- `POST /api/sim/fault` 入力 `{switch_index, scenario}` / `POST /api/sim/reset`

## WebSocket
### `/ws/live`
サーバー→クライアント（JSONテキスト）:
- `{"type":"overview_tick","data":Overview}`（2秒毎）
- `{"type":"event_pushed","data":Event}`
- `{"type":"alerts_changed","data":[Alert]}`（発生・復旧ラッチ・削除・確認・抑止変更時。GET `/api/alerts` と同形式）
- `{"type":"phase_changed","data":{"phase":"setup"|"rehearsal"|"show"|"teardown"}}`
- `{"type":"work_changed","data":[{name,switch_ids,work_until}]}`（GET `/api/groups` と同形式）
- `{"type":"groups_changed","data":null}`（名前・表示順を更新、またはプロファイルを切替。GET `/api/groups` で再取得）
- `{"type":"lock_changed","data":{"locked":bool,"pin_set":bool}}`
- `{"type":"switch_state","data":SwitchState}`（購読中スイッチの更新時）
クライアント→サーバー: `{"subscribe": <switch_id>}`（0で解除）

### `/ws/terminal/:switchId?proto=telnet|ssh`
1. 接続直後、クライアントが `{"type":"auth","user":"..","password":"..","cols":120,"rows":32}` を送る（認証情報はサーバーで保存しない）
2. 以降 クライアント→サーバー `{"type":"data","data":"文字列"}` / `{"type":"resize","cols":N,"rows":M}`
3. サーバー→クライアント: 端末出力をテキストフレームでそのまま送る。終了時はクローズ理由に日本語メッセージ
- 未登録スイッチ・閲覧専用ロック中は接続拒否（HTTP 403）。10分無操作で切断。

## フロントパネル定義
- `GET /api/models` → `{"<モデル名>": {model, rows:[[ifName...],[ifName...]], note}}`。`rows` は上段・下段のifName並び、`""` は隙間。`data/models/*.json` に同形式を置くと内蔵定義を上書き。SwitchState.model に一致する定義が無い場合、UIはポート種別から自動生成する。
