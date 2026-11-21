# AVLine-Sight API 契約（v1）

バックエンド（Go）とWebUI（React）の間の契約。型の正は `internal/model/model.go` のJSONタグ。
時刻はすべて RFC3339（UTC）。エラー時は HTTP 4xx/5xx と `{"error": "日本語メッセージ"}`。
閲覧専用ロック中は変更系（POST/PUT/DELETE、ただし `/api/mode/lock` と `/api/markers` を除く）が `423 Locked`。

## 状態・システム
| メソッド | パス | 応答 |
|---|---|---|
| GET | `/api/status` | `{version, mode:"live", lab:bool, locked:bool, pin_set:bool, profile:{id,name}, trap:{addr, ok, error}, web_addr}` |
| POST | `/api/mode/lock` | 入力 `{locked:bool, pin?:string}` → `{locked}`（解除時、PIN設定済みならPIN必須） |
| GET | `/api/system/nics` | `[{name, addrs:[string]}]` |
| GET/PUT | `/api/settings` | `Settings`（下記） |

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
| GET | `/api/switches` | `[SwitchConfig]`（communityは `"****"` でマスク） |
| POST | `/api/switches` | `SwitchConfig`（id無し）→ 作成結果 |
| PUT/DELETE | `/api/switches/:id` | 更新/削除 |
| GET | `/api/switches/:id/state` | `SwitchState`（全詳細） |
| PUT | `/api/switches/:id/ports/:ifIndex` | `{memo?, excluded?, expected_speed_mbps?, av_role?}`。av_role: `dante`/`aes67`/`ndi`/`ndi4`/`sdvoe`/`st2110`/`qsys`/`lighting`/`avb`/`control`/`uplink`/`""` |
| POST | `/api/switches/:id/counters/reset` | `{if_index:0}`（0=全ポート）ソフト・カウンタゼロリセット |
| POST | `/api/switches/:id/probe` | OID互換性プローブ再実行 |
| GET | `/api/switches/:id/diagnostic-dump` | JSONダウンロード |
| GET | `/api/switches/:id/access-info` | `{web_url, https, ssh:{enabled,port}, telnet:{port}}` |
| POST | `/api/device-aliases` | `{match_type:"mac"|"port", match_key, label, note}` |

## 監視ビュー
- `GET /api/overview` → `{switches:[SwitchSummary], alerts:{critical,warning,info}, recent_events:[Event]}`
  - `SwitchSummary`: `{id,name,ip,group_tag,reachable,last_error,model,fw,uptime_sec,cpu_5s,mem_free_pct,mbuf_used_pct,temp_max_c,temp_state,fans_ok,psu_ok,poe_budget_w,poe_used_w,ports_up,ports_total,rx_bps,tx_bps,alerts:{critical,warning}}`
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
- `POST /api/diagnostics/ping` 入力 `{ip}` → `{ok, rtt_ms, output}`（登録スイッチまたは観測済み機器IPのみ）

## イベント・マーカー
- `GET /api/events?category=&severity=&switch_id=&q=&from=&to=&limit=500` → `[Event]`（新しい順）
- `POST /api/events/:id/ack`、`POST /api/events/ack-all`
- `POST /api/markers` 入力 `{note}` → `Event`（category=MARKER）

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
- `{"type":"switch_state","data":SwitchState}`（購読中スイッチの更新時）
クライアント→サーバー: `{"subscribe": <switch_id>}`（0で解除）

### `/ws/terminal/:switchId?proto=telnet|ssh`
1. 接続直後、クライアントが `{"type":"auth","user":"..","password":"..","cols":120,"rows":32}` を送る（認証情報はサーバーで保存しない）
2. 以降 クライアント→サーバー `{"type":"data","data":"文字列"}` / `{"type":"resize","cols":N,"rows":M}`
3. サーバー→クライアント: 端末出力をテキストフレームでそのまま送る。終了時はクローズ理由に日本語メッセージ
- 未登録スイッチ・閲覧専用ロック中は接続拒否（HTTP 403）。10分無操作で切断。
