// AVLine-Sight — NETGEAR AV Line（M4250）専用監視ソフト。
// 単一実行ファイルで起動し、実行ファイルと同階層の ./data/ にDB等を保存する（完全ポータブル）。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/api"
	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/poller"
	"github.com/Mono-Gen/avline-sight/internal/sim"
	"github.com/Mono-Gen/avline-sight/internal/store"
	"github.com/Mono-Gen/avline-sight/internal/trap"
	"github.com/Mono-Gen/avline-sight/web"
)

// version はビルド時に -ldflags "-X main.version=..." で上書きする。
var version = "0.1.0-dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "起動に失敗しました:", err)
		os.Exit(1)
	}
}

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

func run() error {
	var (
		dataDir  = flag.String("data", filepath.Join(exeDir(), "data"), "データ保存先ディレクトリ")
		listen   = flag.String("listen", "127.0.0.1:8080", "WebUI待受アドレス（LAN公開は 0.0.0.0:8080。閲覧専用ロック併用推奨）")
		trapAddr = flag.String("trap", "", "SNMP Trap待受アドレス（既定 0.0.0.0:162、失敗時は設定画面で変更）")
		lab      = flag.Int("lab", 0, "仮想M4250ラボを N 台起動して監視する（実機なし検証用、1〜20）")
		labOff   = flag.Int("lab-port-offset", 0, "ラボのSNMP/Telnet/SSHポートをまとめてずらす（ラボを2つ同時に動かす検証用）")
		openUI   = flag.Bool("open", false, "起動後に既定ブラウザでWebUIを開く")
		fast     = flag.Duration("fast", poller.DefaultIntervals.Fast, "Fast周期")
		medium   = flag.Duration("medium", poller.DefaultIntervals.Medium, "Medium周期")
		slow     = flag.Duration("slow", poller.DefaultIntervals.Slow, "Slow周期")
		debug    = flag.Bool("debug", false, "デバッグログ")
	)
	flag.Parse()
	lvl := slog.LevelInfo
	if *debug {
		lvl = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))
	if *labOff < 0 || *labOff > 10000 {
		return fmt.Errorf("--lab-port-offset は 0〜10000 で指定してください")
	}
	api.LabSNMPBase += *labOff
	api.LabCLIBase += *labOff
	api.LabSSHBase += *labOff
	if *lab < 0 || *lab > 20 {
		return fmt.Errorf("--lab は 0〜20 で指定してください")
	}
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		return fmt.Errorf("データディレクトリを作成できません %s: %w", *dataDir, err)
	}
	dbName := "avline-sight.db"
	if *lab > 0 {
		dbName = "avline-sight-lab.db" // ラボのデータで本番DBを汚さない
	}
	st, err := store.Open(filepath.Join(*dataDir, dbName), false)
	if err != nil {
		return err
	}
	defer st.Close()
	pid, err := st.EnsureDefaultProfile()
	if err != nil {
		return fmt.Errorf("プロファイル初期化に失敗: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var simLab *sim.Lab
	ta := *trapAddr
	if ta == "" {
		var saved string
		if st.DB.QueryRow(`SELECT v FROM meta WHERE k='trap_addr'`).Scan(&saved) == nil && saved != "" {
			ta = saved
		} else if *lab > 0 {
			ta = "127.0.0.1:1162" // ラボは管理者権限不要のポートを使う
		} else {
			ta = "0.0.0.0:162"
		}
	}

	hub := api.NewHub()
	eng := engine.New(st, pid, hub)
	if *lab > 0 {
		simLab = sim.NewLab(*lab, time.Now().UnixNano())
		trapTarget := ta
		if h, p, err := net.SplitHostPort(ta); err == nil && (h == "0.0.0.0" || h == "") {
			trapTarget = net.JoinHostPort("127.0.0.1", p)
		}
		if err := simLab.Start(ctx, sim.Config{BaseSNMPPort: api.LabSNMPBase, BaseCLIPort: api.LabCLIBase, BaseSSHPort: api.LabSSHBase, TrapTarget: trapTarget}); err != nil {
			return err
		}
		if err := registerLab(st, pid, simLab); err != nil {
			return err
		}
		eng.Reload()
		slog.Info("仮想M4250ラボを起動しました", "台数", *lab, "SNMP", fmt.Sprintf("127.0.0.1:%d〜", api.LabSNMPBase))
	}

	iv := poller.Intervals{Fast: *fast, Medium: *medium, Slow: *slow, Timeout: poller.DefaultIntervals.Timeout, Retries: poller.DefaultIntervals.Retries}
	pm := poller.NewManager(iv, eng, eng.Alias)
	defer pm.StopAll()

	sws, _ := st.Switches(pid)
	tr := trap.New(eng.OnEvent, func(ip string, labPort int) (int64, string, bool) {
		list, _ := st.Switches(eng.ProfileID())
		for _, c := range list {
			if labPort > 0 && c.IP == "127.0.0.1" && c.SNMPPort == labPort {
				return c.ID, c.Name, true
			}
			if labPort == 0 && c.IP == ip {
				return c.ID, c.Name, true
			}
		}
		return 0, "", false
	})
	tr.Start(ta)
	defer tr.Stop()

	ln, addr, err := listenWithFallback(*listen)
	if err != nil {
		return err
	}
	srv := &api.Server{Version: version, DataDir: *dataDir, Store: st, Engine: eng, Poller: pm, Trap: tr, Hub: hub, Lab: simLab,
		Static: web.Dist(), WebAddr: addr, Ctx: ctx}
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	pm.Sync(ctx, sws)
	engineDone := make(chan struct{})
	go func() { defer close(engineDone); eng.Run(ctx, store.DefaultRetention) }()
	// 定期Pingを含むエンジンの終了を待ってからDBを閉じる。
	defer func() { stop(); <-engineDone }()
	go srv.RunTicker(ctx)
	go func() {
		<-ctx.Done()
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		hs.Shutdown(sctx)
	}()
	url := "http://" + addr
	if h, p, _ := net.SplitHostPort(addr); h == "0.0.0.0" || h == "::" {
		url = "http://127.0.0.1:" + p
	}
	slog.Info("AVLine-Sight を起動しました", "version", version, "url", url, "data", *dataDir)
	fmt.Println("AVLine-Sight:", url, "（Ctrl+Cで終了）")
	if *openUI {
		openBrowser(url)
	}
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// listenWithFallback は指定ポートが使用中なら +1〜+20 を試す（仕様8.1）。
func listenWithFallback(addr string) (net.Listener, string, error) {
	host, portS, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, "", fmt.Errorf("待受アドレスが不正です %q: %w", addr, err)
	}
	port, _ := strconv.Atoi(portS)
	var last error
	for i := 0; i <= 20; i++ {
		a := net.JoinHostPort(host, strconv.Itoa(port+i))
		ln, err := net.Listen("tcp", a)
		if err == nil {
			if i > 0 {
				slog.Warn("指定ポートが使用中のため別ポートで待受します", "requested", addr, "actual", a)
			}
			return ln, a, nil
		}
		last = err
	}
	return nil, "", fmt.Errorf("WebUIの待受ポートを確保できません: %w", last)
}

func registerLab(st *store.Store, pid int64, lab *sim.Lab) error {
	existing, err := st.Switches(pid)
	if err != nil {
		return err
	}
	have := map[int]bool{}
	for _, c := range existing {
		if c.IP == "127.0.0.1" {
			have[c.SNMPPort] = true
		}
	}
	for _, s := range lab.Switches {
		port := api.LabSNMPBase + s.Index
		if have[port] {
			continue
		}
		tag := "edge"
		if s.Index == 0 {
			tag = "core"
		}
		c := model.SwitchConfig{ProfileID: pid, Name: s.Name, GroupTag: tag, IP: "127.0.0.1", SNMPPort: port, Community: sim.Community, Enabled: true}
		if err := st.SaveSwitch(&c); err != nil {
			return err
		}
	}
	return nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("ブラウザを開けませんでした", "err", err)
	}
}
