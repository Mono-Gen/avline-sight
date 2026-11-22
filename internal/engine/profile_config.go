package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

const MaxProfileConfigBytes = 4 << 20

func ReadProfileConfig(r io.Reader) (store.ProfileConfig, error) {
	var c store.ProfileConfig
	data, err := io.ReadAll(io.LimitReader(r, MaxProfileConfigBytes+1))
	if err != nil {
		return c, fmt.Errorf("file: %w", err)
	}
	if len(data) > MaxProfileConfigBytes {
		return c, fmt.Errorf("file: must be at most %d bytes", MaxProfileConfigBytes)
	}
	var fields struct {
		Version  json.RawMessage
		TrapAddr json.RawMessage `json:"trap_addr"`
		Switches []struct{ Enabled json.RawMessage }
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return c, fmt.Errorf("JSON: %w", err)
	}
	if len(fields.Version) != 0 && string(fields.Version) != strconv.Itoa(store.ProfileConfigVersion) {
		return c, fmt.Errorf("version: expected %d or omitted for legacy files", store.ProfileConfigVersion)
	}
	if string(fields.TrapAddr) == "null" {
		return c, fmt.Errorf("trap_addr: expected a string")
	}
	for i, sw := range fields.Switches {
		if string(sw.Enabled) == "null" {
			return c, fmt.Errorf("switches[%d].enabled: expected true or false", i)
		}
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("JSON: %w", err)
	}
	legacy := c.Version == 0
	if strings.TrimSpace(c.Name) == "" {
		return c, fmt.Errorf("name: must not be empty")
	}
	if !legacy && string(c.Settings) == "null" {
		return c, fmt.Errorf("settings: expected an object")
	}
	if err := normalizeConfigSettings(&c, true); err != nil {
		return c, err
	}
	if err := normalizeConfigSwitches(&c, legacy); err != nil {
		return c, err
	}
	c.Version = store.ProfileConfigVersion
	return c, nil
}

// ExportProfileConfig は保存済みの設定から、現在の型で表現できる項目だけを書出用に残す。
func ExportProfileConfig(c store.ProfileConfig) (store.ProfileConfig, error) {
	if err := normalizeConfigSettings(&c, false); err != nil {
		return c, err
	}
	for i, sw := range c.Switches {
		if sw.WebURL != "" {
			link, err := url.Parse(sw.WebURL)
			if err != nil || !validConfigWebURL(link) {
				c.Switches[i].WebURL = ""
				continue
			}
			c.Switches[i].WebURL = configWebURL(link)
		}
	}
	if c.GroupOrder == nil {
		c.GroupOrder = []string{}
	}
	return c, nil
}

func normalizeConfigSettings(c *store.ProfileConfig, validate bool) error {
	settings := DefaultSettings()
	if len(c.Settings) != 0 && string(c.Settings) != "null" {
		if validate {
			if err := validateSettingTypes(c.Settings); err != nil {
				return err
			}
		}
		if err := json.Unmarshal(c.Settings, &settings); err != nil {
			return fmt.Errorf("settings: %w", err)
		}
	}
	if validate {
		if err := settings.Validate(); err != nil {
			return fmt.Errorf("settings.%w", err)
		}
	} else {
		settings.normalizePingIntervals()
	}
	data, err := json.Marshal(settings)
	c.Settings = data
	return err
}

func validateSettingTypes(data json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	for key, raw := range fields {
		switch strings.ToLower(key) {
		case "setup_ping_interval_sec", "rehearsal_ping_interval_sec", "expert_display_default", "thresholds":
			if string(raw) == "null" {
				return fmt.Errorf("settings.%s: null is not allowed", key)
			}
		}
		if strings.EqualFold(key, "thresholds") {
			var thresholds map[string]json.RawMessage
			if err := json.Unmarshal(raw, &thresholds); err != nil {
				return fmt.Errorf("settings.thresholds: %w", err)
			}
			for field, value := range thresholds {
				switch strings.ToLower(field) {
				case "cpu_pct", "mem_free_pct", "temp_c", "poe_pct", "util_pct", "sfp_rx_dbm", "sfp_drop_db", "err_delta":
					if string(value) == "null" {
						return fmt.Errorf("settings.thresholds.%s: expected a number", field)
					}
				}
			}
		}
	}
	return nil
}

func (s Settings) validateMonitoringSettings() error {
	th := s.Thresholds
	for _, pct := range []struct {
		field string
		value float64
	}{{"cpu_pct", th.CPUPct}, {"mem_free_pct", th.MemFreePct}, {"poe_pct", th.PoEPct}, {"util_pct", th.UtilPct}} {
		if pct.value < 0 || pct.value > 100 {
			return fmt.Errorf("thresholds.%s: must be between 0 and 100", pct.field)
		}
	}
	if th.TempC < 0 {
		return fmt.Errorf("thresholds.temp_c: must be nonnegative")
	}
	if th.SFPDropDB < 0 {
		return fmt.Errorf("thresholds.sfp_drop_db: must be nonnegative")
	}
	seen := map[string]bool{}
	for i, view := range s.Views {
		if strings.TrimSpace(view.ID) == "" || seen[view.ID] {
			return fmt.Errorf("views[%d].id: must be nonempty and unique", i)
		}
		seen[view.ID] = true
		if strings.TrimSpace(view.Name) == "" {
			return fmt.Errorf("views[%d].name: must not be empty", i)
		}
		for j, vlan := range view.VLANs {
			if vlan < 1 || vlan > 4094 {
				return fmt.Errorf("views[%d].vlans[%d]: must be between 1 and 4094", i, j)
			}
		}
	}
	return nil
}

func validConfigWebURL(link *url.URL) bool {
	if (link.Scheme != "http" && link.Scheme != "https") || !model.ValidSwitchHost(link.Hostname()) {
		return false
	}
	if port := link.Port(); port != "" {
		n, err := strconv.Atoi(port)
		return err == nil && n >= 1 && n <= 65535
	}
	return true
}

func configWebURL(link *url.URL) string {
	// 管理画面URLの認証情報やクエリのセッショントークンを現場間で持ち運ばない。
	link.User, link.RawQuery, link.Fragment, link.RawFragment, link.ForceQuery = nil, "", "", "", false
	return link.String()
}

func normalizeConfigSwitches(c *store.ProfileConfig, legacy bool) error {
	groups := map[string]bool{}
	for i, name := range c.GroupOrder {
		if name != strings.TrimSpace(name) || !model.ValidGroupName(name) || groups[name] {
			return fmt.Errorf("group_order[%d]: must be a valid, unique group name", i)
		}
		groups[name] = true
	}
	if c.TrapAddr != "" {
		host, port, err := net.SplitHostPort(c.TrapAddr)
		p, portErr := strconv.Atoi(port)
		if err != nil || portErr != nil || p < 1 || p > 65535 || (host != "" && !model.ValidSwitchHost(host)) {
			return fmt.Errorf("trap_addr: expected a valid host and UDP port (1 to 65535)")
		}
	}
	ids := map[int64]int64{}
	portKeys := map[string]bool{}
	ports := map[string]map[int]store.PortSetting{}
	for i, sw := range c.Switches {
		sw.IP = strings.TrimSpace(sw.IP)
		sw.GroupTag = strings.TrimSpace(sw.GroupTag)
		switch {
		case strings.TrimSpace(sw.Name) == "":
			return fmt.Errorf("switches[%d].name: must not be empty", i)
		case !model.ValidSwitchHost(sw.IP):
			return fmt.Errorf("switches[%d].ip: expected a valid IP address or host name", i)
		case sw.GroupTag != "" && !model.ValidGroupName(sw.GroupTag):
			return fmt.Errorf("switches[%d].group_tag: invalid group name", i)
		case sw.SNMPPort < 0 || sw.SNMPPort > 65535 || (!legacy && sw.SNMPPort == 0):
			return fmt.Errorf("switches[%d].snmp_port: must be between 1 and 65535", i)
		case (!legacy && sw.ID <= 0) || sw.ID < 0 || ids[sw.ID] != 0:
			return fmt.Errorf("switches[%d].id: must be positive and unique", i)
		}
		if sw.WebURL != "" {
			link, err := url.Parse(sw.WebURL)
			if err != nil || !validConfigWebURL(link) {
				return fmt.Errorf("switches[%d].web_url: expected an HTTP(S) URL with a valid host and port", i)
			}
			sw.WebURL = configWebURL(link)
		}
		if sw.SNMPPort == 0 {
			sw.SNMPPort = 161
		}
		portKey := strconv.FormatInt(sw.ID, 10)
		if legacy {
			portKey = sw.Name
		}
		portKeys[portKey] = true
		fileID := int64(i + 1)
		if sw.ID > 0 {
			ids[sw.ID] = fileID
		}
		sw.ID = fileID
		c.Switches[i] = sw
		for index, port := range c.Ports[portKey] {
			if index <= 0 {
				return fmt.Errorf("ports[%q][%d]: interface index must be positive", portKey, index)
			}
			if port.ExpectedSpeed < 0 {
				return fmt.Errorf("ports[%q][%d].expected_speed_mbps: must be nonnegative", portKey, index)
			}
		}
		if len(c.Ports[portKey]) > 0 {
			ports[strconv.FormatInt(fileID, 10)] = c.Ports[portKey]
		}
	}
	for key := range c.Ports {
		if !portKeys[key] {
			return fmt.Errorf("ports: contains an unknown switch reference")
		}
	}
	c.Ports = ports
	if err := validateConfigReferences(c, ids, legacy); err != nil {
		return err
	}
	c.RemapReferences(ids)
	if c.GroupOrder == nil {
		c.GroupOrder = []string{}
	}
	if c.Switches == nil {
		c.Switches = []store.ConfigSwitch{}
	}
	return nil
}

func validateConfigReferences(c *store.ProfileConfig, ids map[int64]int64, legacy bool) error {
	for key := range c.Layout {
		if suffix, ok := strings.CutPrefix(key, "sw-"); ok {
			id, err := strconv.ParseInt(suffix, 10, 64)
			if err != nil || id <= 0 || (!legacy && ids[id] == 0) {
				return fmt.Errorf("topology_layout: invalid switch reference")
			}
		} else if !strings.HasPrefix(key, "ext-") {
			return fmt.Errorf("topology_layout: invalid node reference")
		}
	}
	for i, item := range c.Watchlist {
		if item.SwitchID <= 0 || (!legacy && ids[item.SwitchID] == 0) {
			return fmt.Errorf("watchlist[%d].switch_id: invalid switch reference", i)
		}
		if item.IfIndex <= 0 {
			return fmt.Errorf("watchlist[%d].if_index: must be positive", i)
		}
	}
	seen := map[[2]string]bool{}
	for i, a := range c.Aliases {
		if a.MatchType != "mac" && a.MatchType != "port" {
			return fmt.Errorf("aliases[%d].match_type: expected mac or port", i)
		}
		a.ID = 0
		a.MatchKey = strings.ToUpper(strings.TrimSpace(a.MatchKey))
		if a.MatchType == "port" {
			source, index, _ := strings.Cut(a.MatchKey, ":")
			id, _ := strconv.ParseInt(source, 10, 64)
			port, _ := strconv.Atoi(index)
			if id <= 0 || port <= 0 || (!legacy && ids[id] == 0) {
				return fmt.Errorf("aliases[%d].match_key: invalid switch or port reference", i)
			}
		} else if _, err := net.ParseMAC(a.MatchKey); err != nil {
			return fmt.Errorf("aliases[%d].match_key: expected a MAC address", i)
		}
		key := [2]string{a.MatchType, a.MatchKey}
		if seen[key] {
			return fmt.Errorf("aliases[%d].match_key: duplicate alias", i)
		}
		seen[key] = true
		if strings.TrimSpace(a.Label) == "" {
			return fmt.Errorf("aliases[%d].label: must not be empty", i)
		}
		if !ValidPlannedIP(a.PlannedIP) {
			return fmt.Errorf("aliases[%d].planned_ip: expected an IPv4 address or an empty value", i)
		}
		c.Aliases[i] = a
	}
	return nil
}
