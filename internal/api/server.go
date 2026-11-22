// Package api はREST API・WebSocket・静的WebUI配信を提供する。
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/poller"
	"github.com/Mono-Gen/avline-sight/internal/sim"
	"github.com/Mono-Gen/avline-sight/internal/store"
	"github.com/Mono-Gen/avline-sight/internal/trap"
)

// Server はHTTPサーバーの依存関係。
type Server struct {
	Version string
	DataDir string
	Store   *store.Store
	Engine  *engine.Engine
	Poller  *poller.Manager
	Trap    *trap.Receiver
	Hub     *Hub
	Lab     *sim.Lab // nil なら非ラボ
	Static  fs.FS    // web/dist
	WebAddr string
	Ctx     context.Context

	mu      sync.Mutex
	groupMu sync.Mutex
	locked  bool
	pinHash string
	replays map[string]*replay
}

type replay struct {
	st   *store.Store
	man  *store.Manifest
	dir  string
	used time.Time
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("応答書込失敗", "err", err)
	}
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("JSONの形式が不正です: %w", err)
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}

// Handler はルーティングを構築する。
func (s *Server) Handler() http.Handler {
	s.replays = map[string]*replay{}
	m := http.NewServeMux()
	h := func(pattern string, f http.HandlerFunc) { m.HandleFunc(pattern, f) }

	h("GET /api/status", s.status)
	h("GET /api/alerts", s.displayAlerts)
	h("GET /api/phase", s.getPhase)
	h("PUT /api/phase", s.putPhase)
	h("GET /api/checklist/pre-show", s.showChecklist)
	h("GET /api/groups", s.groups)
	h("PUT /api/groups/{name}", s.renameGroup)
	h("PUT /api/groups/{$}", s.renameGroup)
	h("PUT /api/groups/order", s.setGroupOrder)
	h("POST /api/counters/reset", s.resetGroupCounters)
	h("POST /api/mode/lock", s.lock)
	h("GET /api/system/nics", s.nics)
	h("PUT /api/system/network-bind", s.networkBind)
	h("GET /api/settings", s.getSettings)
	h("PUT /api/settings", s.putSettings)

	h("GET /api/profiles", s.profiles)
	h("POST /api/profiles", s.createProfile)
	h("PUT /api/profiles/{id}/activate", s.activateProfile)
	h("GET /api/profiles/{id}/export", s.exportProfile)
	h("POST /api/profiles/import", s.importProfile)

	h("GET /api/switches", s.listSwitches)
	h("POST /api/switches", s.saveSwitch)
	h("PUT /api/switches/{id}", s.saveSwitch)
	h("DELETE /api/switches/{id}", s.deleteSwitch)
	h("GET /api/switches/{id}/state", s.switchState)
	h("PUT /api/switches/{id}/ports/{ifIndex}", s.portSetting)
	h("POST /api/switches/{id}/counters/reset", s.resetCounters)
	h("POST /api/switches/{id}/probe", s.probe)
	h("GET /api/switches/{id}/diagnostic-dump", s.diagDump)
	h("GET /api/switches/{id}/access-info", s.accessInfo)
	h("POST /api/device-aliases", s.saveAlias)
	h("POST /api/device-aliases/import", s.importAliases)
	h("GET /api/devices", s.devices)
	h("GET /api/devices/export.csv", s.devicesCSV)
	h("GET /api/replay/{sid}/devices", s.replayDevices)
	h("GET /api/replay/{sid}/devices/export.csv", s.replayDevicesCSV)

	h("GET /api/models", s.panelModels)
	h("GET /api/overview", s.overview)
	h("GET /api/ports/search", s.searchPorts)
	h("GET /api/metrics/port", s.portMetrics)
	h("GET /api/metrics/switch", s.switchMetrics)
	h("GET /api/topology", s.topology)
	h("PUT /api/topology/layout", s.topologyLayout)
	h("POST /api/topology/background", s.uploadBackground)
	h("GET /api/topology/background", s.getBackground)
	h("GET /api/watchlist", s.getWatchlist)
	h("PUT /api/watchlist", s.putWatchlist)
	h("GET /api/multicast", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.Engine.Multicast()) })
	h("GET /api/dhcp", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.Engine.DHCP()) })
	h("GET /api/audit", s.audit)
	h("POST /api/diagnostics/ping", s.ping)
	h("POST /api/diagnostics/ping-bulk", s.pingBulk)

	h("GET /api/events", s.events)
	h("POST /api/events/{id}/ack", s.ack)
	h("POST /api/events/{id}/unack", s.unack)
	h("POST /api/events/ack-all", s.ackAll)
	h("POST /api/markers", s.marker)

	h("GET /api/baselines", s.baselines)
	h("POST /api/baselines", s.createBaseline)
	h("GET /api/baselines/{id}/diff", s.baselineDiff)
	h("DELETE /api/baselines/{id}", s.deleteBaseline)

	h("POST /api/export/pack", s.exportPack)
	h("GET /api/export/csv", s.exportTable)
	h("GET /api/export/show-report.csv", s.showReport)
	h("GET /api/export/json", s.exportTable)
	h("POST /api/replay/import", s.replayImport)
	h("GET /api/replay/{sid}/timeline", s.replayTimeline)
	h("GET /api/replay/{sid}/snapshot", s.replaySnapshot)
	h("GET /api/replay/{sid}/events", s.replayEvents)
	h("DELETE /api/replay/{sid}", s.replayDelete)

	h("POST /api/discovery/scan", s.discoveryScan)
	h("GET /api/discovery/lldp-candidates", s.lldpCandidates)
	h("GET /api/sim/scenarios", s.simScenarios)
	h("POST /api/sim/fault", s.simFault)
	h("POST /api/sim/reset", s.simReset)

	h("GET /ws/live", s.wsLive)
	h("GET /ws/terminal/{id}", s.wsTerminal)

	m.HandleFunc("/api/", http.NotFound)
	if s.Static != nil {
		fileServer := http.FileServer(http.FS(s.Static))
		m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// SPA: 存在しないパスは index.html を返す
			p := strings.TrimPrefix(r.URL.Path, "/")
			if p == "" {
				p = "index.html"
			}
			if _, err := fs.Stat(s.Static, p); err != nil {
				r.URL.Path = "/"
			}
			fileServer.ServeHTTP(w, r)
		})
	}
	return s.middleware(m)
}

// lockExempt は閲覧専用ロック中も許可する変更系パス。
func lockExempt(path string) bool {
	return path == "/api/mode/lock" || path == "/api/markers" || strings.HasPrefix(path, "/api/replay/") ||
		path == "/api/export/pack"
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w = languageWriter{ResponseWriter: w, english: prefersEnglish(r.Header.Get("Accept-Language"))}
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("ハンドラでpanic", "path", r.URL.Path, "panic", rec)
				writeErr(w, 500, "内部エラーが発生しました", fmt.Sprint(rec), "ログを確認し、再現手順とともに報告してください")
			}
		}()
		// 他オリジンからの変更系要求（CSRF）を拒否する
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" && !sameOrigin(o, r.Host) {
				writeErr(w, 403, "他サイトからの操作要求を拒否しました", "Originが一致しません", "AVLine-Sightの画面から操作してください")
				return
			}
			s.mu.Lock()
			locked := s.locked
			s.mu.Unlock()
			if locked && strings.HasPrefix(r.URL.Path, "/api/") && !lockExempt(r.URL.Path) {
				writeErr(w, http.StatusLocked, "閲覧専用ロック中のため変更できません", "", "ヘッダーのロックを解除してから操作してください")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(origin, host string) bool {
	o := strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "https://")
	return o == host
}

// ---- 状態・システム ----

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	locked, pinSet := s.locked, s.pinHash != ""
	s.mu.Unlock()
	pid := s.Engine.ProfileID()
	name := ""
	if ps, err := s.Store.Profiles(); err == nil {
		for _, p := range ps {
			if p.ID == pid {
				name = p.Name
			}
		}
	}
	writeJSON(w, 200, map[string]any{"version": s.Version, "mode": "live", "lab": s.Lab != nil, "locked": locked, "pin_set": pinSet,
		"profile": map[string]any{"id": pid, "name": name}, "trap": s.Trap.Status(), "web_addr": s.WebAddr, "phase": s.Engine.Phase()})
}

func hashPIN(p string) string {
	h := sha256.Sum256([]byte("avline-sight:" + p))
	return hex.EncodeToString(h[:])
}

func (s *Server) lock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Locked bool   `json:"locked"`
		PIN    string `json:"pin"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "ロック要求が不正です", err.Error(), "")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.Locked {
		// ロック中にPINを差し替えると解除できてしまうため、既存PINがある間は変更を許さない
		if s.locked && s.pinHash != "" {
			if in.PIN != "" && hashPIN(in.PIN) != s.pinHash {
				writeErr(w, 403, "PINを変更できません", "既にPIN付きでロックされています", "一度現在のPINで解除してから設定し直してください")
				return
			}
		} else if in.PIN != "" {
			s.pinHash = hashPIN(in.PIN)
		}
		s.locked = true
	} else {
		if s.pinHash != "" && hashPIN(in.PIN) != s.pinHash {
			writeErr(w, 403, "ロックを解除できません", "PINが一致しません", "ロック時に設定したPINを入力してください")
			return
		}
		s.locked, s.pinHash = false, ""
	}
	writeJSON(w, 200, map[string]bool{"locked": s.locked})
	if s.Hub != nil {
		s.Hub.Broadcast("lock_changed", map[string]bool{"locked": s.locked, "pin_set": s.pinHash != ""})
	}
}

func (s *Server) nics(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	ifs, err := netInterfaces()
	if err != nil {
		writeErr(w, 500, "NIC一覧を取得できません", err.Error(), "")
		return
	}
	for _, i := range ifs {
		out = append(out, i)
	}
	writeJSON(w, 200, out)
}

func (s *Server) networkBind(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TrapAddr string `json:"trap_addr"`
	}
	if err := readJSON(r, &in); err != nil || in.TrapAddr == "" {
		writeErr(w, 400, "待受設定が不正です", "trap_addr が必要です", "例: 0.0.0.0:1162")
		return
	}
	st := s.Trap.Start(in.TrapAddr)
	s.Store.DB.Exec(`INSERT OR REPLACE INTO meta(k,v) VALUES('trap_addr',?)`, in.TrapAddr)
	writeJSON(w, 200, st)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Engine.Settings())
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	in := engine.DefaultSettings()
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "設定を保存できません", err.Error(), "")
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, 400, "設定を保存できません", "Ping間隔は30〜3600秒の整数で指定してください", "")
		return
	}
	if err := s.Engine.SaveSettings(in); err != nil {
		writeErr(w, 500, "設定を保存できません", err.Error(), "")
		return
	}
	writeJSON(w, 200, in)
}

// resync はプロファイル・スイッチ変更をポーラーとエンジンへ反映する。
func (s *Server) resync() {
	s.Engine.Reload()
	if s.Poller == nil {
		return
	}
	sws, err := s.Store.Switches(s.Engine.ProfileID())
	if err != nil {
		slog.Warn("スイッチ一覧の取得に失敗", "err", err)
		return
	}
	s.Poller.Sync(s.Ctx, sws)
}

// ---- 書出用一時ディレクトリ ----

func (s *Server) tmpDir() string {
	d := filepath.Join(s.DataDir, "tmp")
	os.MkdirAll(d, 0o755)
	return d
}

var errNotFound = errors.New("not found")
