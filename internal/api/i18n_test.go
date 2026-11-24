package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/coder/websocket"
)

func TestLanguageNegotiation(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   bool
	}{
		{"", false}, {"ja", false}, {"en", true}, {"en-US,en;q=0.8", true},
		{"de, ja;q=0.8, en;q=0.4", false}, {"en;q=0.2,ja;q=0.9", false},
		{"ja;q=0.2,en-GB;q=0.9", true}, {"en;q=0", false},
		{"en;q=invalid,ja", false}, {"en;q=2", false}, {"en;q=NaN", false},
		{"fr", false}, {"*", false},
	} {
		t.Run(tc.header, func(t *testing.T) {
			if got := prefersEnglish(tc.header); got != tc.want {
				t.Fatalf("prefersEnglish(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}

func TestLocalizedErrorsAndLock(t *testing.T) {
	s, h := newDevicesServer(t)
	s.locked = true
	for _, tc := range []struct {
		language, expected string
	}{
		{"", "閲覧専用ロック中のため変更できません 対処: 左ペインのロックを解除してから操作してください"},
		{"ja", "閲覧専用ロック中のため変更できません 対処: 左ペインのロックを解除してから操作してください"},
		{"en-US", "Changes are unavailable while the read-only lock is enabled Action: Unlock using the sidebar control before trying again"},
	} {
		r := httptest.NewRequest(http.MethodPut, "/api/phase", strings.NewReader(`{"phase":"setup"}`))
		r.Header.Set("Accept-Language", tc.language)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var result map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusLocked || result["error"] != tc.expected {
			t.Fatalf("language %q: status=%d result=%v", tc.language, w.Code, result)
		}
	}
	s.locked = false
	r := httptest.NewRequest(http.MethodPut, "/api/phase", strings.NewReader(`{"phase":"invalid"}`))
	r.Header.Set("Accept-Language", "en")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Choose setup, rehearsal or show") {
		t.Fatalf("handler error was not translated: %d %s", w.Code, w.Body)
	}
}

func TestErrorReasonsAndJapanesePreservation(t *testing.T) {
	w := httptest.NewRecorder()
	writeErr(w, 400, "機器名CSVを読み込めません", "JSONの形式が不正です: unexpected EOF", "")
	if !strings.Contains(w.Body.String(), "機器名CSVを読み込めません（原因: JSONの形式が不正です: unexpected EOF）") {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	writeErr(languageWriter{ResponseWriter: w, english: true}, 400, "機器名CSVを読み込めません", "JSONの形式が不正です: unexpected EOF", "")
	if !strings.Contains(w.Body.String(), "Unable to import the device-name CSV (Reason: Invalid JSON: unexpected EOF)") {
		t.Fatal(w.Body.String())
	}
	if got := englishMessage("3行にエラーがあります"); got != "3 rows contain errors" {
		t.Fatal(got)
	}
}

func TestDeviceCSVLanguageHeadersAndRoundTrip(t *testing.T) {
	s, h := newDevicesServer(t)
	for _, lang := range []string{"ja", "en"} {
		w := do(h, "GET", "/api/devices/export.csv?lang="+lang, "")
		if w.Code != 200 || !bytes.HasPrefix(w.Body.Bytes(), []byte("\uFEFF")) {
			t.Fatalf("CSV response: %d %s", w.Code, w.Body)
		}
		reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\uFEFF")))
		header, err := reader.Read()
		if err != nil {
			t.Fatal(err)
		}
		if lang == "en" && (header[1] != "name" || header[2] != "note" || header[3] != "planned_ip") {
			t.Fatal(header)
		}
		if lang == "ja" && (header[1] != "名称" || header[2] != "メモ" || header[3] != "予定IP") {
			t.Fatal(header)
		}
		header = header[:4]
		var input bytes.Buffer
		writer := csv.NewWriter(&input)
		if err := writer.Write(header); err != nil {
			t.Fatal(err)
		}
		if err := writer.Write([]string{"00:11:22:33:44:55", "Desk", "Memo", "10.0.0.20"}); err != nil {
			t.Fatal(err)
		}
		writer.Flush()
		body, err := json.Marshal(map[string]any{"csv": input.String(), "commit": true})
		if err != nil {
			t.Fatal(err)
		}
		w = do(h, "POST", "/api/device-aliases/import", string(body))
		if w.Code != 200 {
			t.Fatalf("%s headers could not be imported: %d %s", lang, w.Code, w.Body)
		}
		aliases, err := s.Store.Aliases(s.Engine.ProfileID())
		if err != nil || len(aliases) != 1 || aliases[0].PlannedIP != "10.0.0.20" {
			t.Fatalf("%s: aliases=%v err=%v", lang, aliases, err)
		}
	}
}

type failingCSVWriter struct{}

func (failingCSVWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestCSVHeaderWriterFailureAndFragments(t *testing.T) {
	var output bytes.Buffer
	w := &inventoryHeaderWriter{Writer: &output, expected: []byte("old\r\n"), replacement: []byte("new\r\n")}
	for _, part := range []string{"o", "ld\r", "\nrow\r\n"} {
		if n, err := w.Write([]byte(part)); n != len(part) || err != nil {
			t.Fatalf("fragment write=%d,%v", n, err)
		}
	}
	if output.String() != "new\r\nrow\r\n" {
		t.Fatal(output.String())
	}
	if err := writeLocalizedInventoryCSV(failingCSVWriter{}, nil, time.UTC, true); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	var plain, english bytes.Buffer
	rows := []engine.InventoryRow{{MAC: "00:11:22:33:44:55", Label: "=Unsafe", Note: "日本語"}}
	if err := writeLocalizedInventoryCSV(&plain, rows, time.UTC, false); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalizedInventoryCSV(&english, rows, time.UTC, true); err != nil {
		t.Fatal(err)
	}
	_, plainRows, _ := strings.Cut(plain.String(), "\r\n")
	_, englishRows, _ := strings.Cut(english.String(), "\r\n")
	if plainRows != englishRows || !strings.Contains(englishRows, "'=Unsafe") {
		t.Fatalf("CSV data or formula protection changed: %q / %q", plainRows, englishRows)
	}
}

func TestExplicitEmptyInventoryGroup(t *testing.T) {
	rows := []engine.InventoryRow{{MAC: "00:11:22:33:44:55", GroupTag: "audio"}, {MAC: "00:11:22:33:44:66", GroupTag: ""}}
	for _, tc := range []struct {
		query string
		count int
	}{
		{"", 2}, {"?group=", 1}, {"?group=audio", 2},
	} {
		r := httptest.NewRequest("GET", "/api/devices"+tc.query, nil)
		got := inventoryGroupRows(r, rows)
		if len(got) != tc.count {
			t.Fatalf("%q: %v", tc.query, got)
		}
		if tc.query == "?group=" && got[0].GroupTag != "" {
			t.Fatal(got)
		}
	}
}

func TestLanguageWriterSupportsWebSocketUpgrade(t *testing.T) {
	s := &Server{}
	server := httptest.NewServer(s.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := conn.Write(ctx, websocket.MessageText, []byte("connected")); err != nil {
			t.Error(err)
		}
	})))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, server.URL, &websocket.DialOptions{HTTPHeader: http.Header{"Accept-Language": []string{"en"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	_, data, err := conn.Read(ctx)
	if err != nil || string(data) != "connected" {
		t.Fatalf("WebSocket upgrade or message failed: %q %v", data, err)
	}
}
