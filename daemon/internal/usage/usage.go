// Package usage: medidor de uso (claude.json + opencode sqlite) — port de usage.ts.
package usage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"colmeia/daemon/internal/auth"

	"github.com/fsnotify/fsnotify"
	_ "modernc.org/sqlite"
)

type Window struct {
	Label    string   `json:"label"`
	Pct      *float64 `json:"pct"`
	Used     *int     `json:"used,omitempty"`
	Limit    *int     `json:"limit,omitempty"`
	ResetsAt *float64 `json:"resetsAt,omitempty"`
}

type Provider struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Status    string   `json:"status"`
	Windows   []Window `json:"windows"`
	Detail    string   `json:"detail,omitempty"`
	UpdatedAt *float64 `json:"updatedAt,omitempty"`
}

type Report struct {
	Providers []Provider `json:"providers"`
}

type Settings struct {
	OpencodeDailyLimit int `json:"opencodeDailyLimit,omitempty"`
}

var (
	mu    sync.Mutex
	cache *struct {
		at time.Time
		r  Report
	}
)

func claudeFile() string   { return filepath.Join(auth.ConfigDir(), "usage", "claude.json") }
func settingsFile() string { return filepath.Join(auth.ConfigDir(), "usage-settings.json") }
func opencodeDB() string {
	b := os.Getenv("XDG_DATA_HOME")
	if b == "" {
		h, _ := os.UserHomeDir()
		b = filepath.Join(h, ".local", "share")
	}
	return filepath.Join(b, "opencode", "opencode.db")
}

func toLimit(v any) int {
	f, ok := v.(float64)
	if !ok || !(f > 0) {
		return 0
	}
	return int(math.Floor(f))
}

func loadSettings() Settings {
	var raw map[string]any
	if b, err := os.ReadFile(settingsFile()); err == nil && json.Unmarshal(b, &raw) == nil {
		return Settings{toLimit(raw["opencodeDailyLimit"])}
	}
	return Settings{}
}

// SetSettings grava e devolve o relatório recalculado.
func SetSettings(raw map[string]any) Report {
	s := Settings{toLimit(raw["opencodeDailyLimit"])}
	_ = os.MkdirAll(auth.ConfigDir(), 0o755)
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := settingsFile() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, settingsFile())
	}
	mu.Lock()
	cache = nil
	mu.Unlock()
	return GetReport()
}

func statusOf(pcts []*float64) string {
	max := -1.0
	for _, p := range pcts {
		if p != nil && *p > max {
			max = *p
		}
	}
	switch {
	case max >= 100:
		return "exhausted"
	case max >= 70:
		return "warn"
	}
	return "ok"
}

func fp(f float64) *float64 { return &f }

func claudeProvider() Provider {
	base := Provider{ID: "claude", Label: "Claude Code", Status: "unknown", Windows: []Window{}}
	miss := func() Provider { b := base; b.Detail = "abra um agente Claude para medir"; return b }
	b, err := os.ReadFile(claudeFile())
	if err != nil {
		return miss()
	}
	var raw struct {
		RateLimits map[string]struct {
			Used   *float64 `json:"used_percentage"`
			Resets *float64 `json:"resets_at"`
		} `json:"rate_limits"`
		UpdatedAt *float64 `json:"updatedAt"`
	}
	if json.Unmarshal(b, &raw) != nil || raw.RateLimits == nil {
		return miss()
	}
	now := float64(time.Now().UnixMilli())
	var ws []Window
	for _, it := range [][2]string{{"5 horas", "five_hour"}, {"Semana", "seven_day"}} {
		w, ok := raw.RateLimits[it[1]]
		if !ok {
			continue
		}
		win := Window{Label: it[0], Pct: w.Used}
		if w.Resets != nil {
			r := *w.Resets * 1000
			win.ResetsAt = &r
			if r <= now {
				win.Pct = fp(0)
			}
		}
		ws = append(ws, win)
	}
	if ws == nil {
		ws = []Window{}
	}
	out := base
	out.Windows = ws
	if len(ws) > 0 {
		var ps []*float64
		for _, w := range ws {
			ps = append(ps, w.Pct)
		}
		out.Status = statusOf(ps)
	}
	out.Detail = "limite da conta · vale para todos os modelos (Opus, Sonnet, Haiku)"
	out.UpdatedAt = raw.UpdatedAt
	return out
}

func opencodeProvider(s Settings) Provider {
	base := Provider{ID: "opencode", Label: "opencode", Status: "unknown", Windows: []Window{}}
	nodata := func() Provider { b := base; b.Detail = "sem dados ainda"; return b }
	if _, err := os.Stat(opencodeDB()); err != nil {
		return nodata()
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: opencodeDB(), RawQuery: "mode=ro"}).String())
	if err != nil {
		return nodata()
	}
	defer db.Close()
	n := time.Now()
	midnight := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location()).UnixMilli()
	var used int
	if db.QueryRow(`SELECT COUNT(*) FROM session_message WHERE time_created >= ? AND type = 'assistant'`, midnight).Scan(&used) != nil {
		return nodata()
	}
	win := Window{Label: "Hoje", Used: &used}
	if s.OpencodeDailyLimit > 0 {
		l := s.OpencodeDailyLimit
		win.Limit = &l
		win.Pct = fp(math.Min(100, math.Round(float64(used)/float64(l)*100)))
	}
	status := "ok"
	if win.Pct != nil {
		status = statusOf([]*float64{win.Pct})
	}
	detail := fmt.Sprintf("%d respostas hoje · todos os modelos somados", used)
	var t int64
	var data string
	if db.QueryRow(`SELECT time_created, data FROM session_message WHERE data LIKE '%provider.free-limit%' ORDER BY time_created DESC LIMIT 1`).Scan(&t, &data) == nil {
		created, retry := float64(t), 0.0
		var d struct {
			Time  struct{ Created *float64 } `json:"time"`
			Error struct {
				RetryAfterMs *float64 `json:"retryAfterMs"`
				Data         struct {
					RetryAfterMs *float64 `json:"retryAfterMs"`
				} `json:"data"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &d) == nil {
			if d.Time.Created != nil {
				created = *d.Time.Created
			}
			if d.Error.RetryAfterMs != nil {
				retry = *d.Error.RetryAfterMs
			} else if d.Error.Data.RetryAfterMs != nil {
				retry = *d.Error.Data.RetryAfterMs
			}
		}
		resetAt := created + retry
		if retry > 0 && resetAt > float64(time.Now().UnixMilli()) {
			status = "exhausted"
			win.ResetsAt = &resetAt
			detail = "limite atingido até " + time.UnixMilli(int64(resetAt)).Format("15:04")
		}
	}
	out := base
	out.Status, out.Windows, out.Detail = status, []Window{win}, detail
	out.UpdatedAt = fp(float64(time.Now().UnixMilli()))
	return out
}

func GetReport() Report {
	mu.Lock()
	if cache != nil && time.Since(cache.at) < 15*time.Second {
		r := cache.r
		mu.Unlock()
		return r
	}
	mu.Unlock()
	return compute()
}

func compute() Report {
	r := Report{Providers: []Provider{claudeProvider(), opencodeProvider(loadSettings())}}
	mu.Lock()
	cache = &struct {
		at time.Time
		r  Report
	}{time.Now(), r}
	mu.Unlock()
	return r
}

// Watch avisa quando os dados mudam (inotify, debounce 1,5s + fallback 5min). Retorna o cancelador.
func Watch(onChange func(Report)) func() {
	var tmu sync.Mutex
	var timer *time.Timer
	mu.Lock()
	var last any
	if cache != nil {
		last = cache.r
	}
	mu.Unlock()
	fire := func() {
		defer func() { // M9
			if r := recover(); r != nil {
				log.Printf("panic no usage: %v", r)
			}
		}()
		tmu.Lock()
		timer = nil
		tmu.Unlock()
		r := compute()
		tmu.Lock()
		changed := !reflect.DeepEqual(last, r)
		last = r
		tmu.Unlock()
		if changed {
			onChange(r)
		}
	}
	kick := func() {
		tmu.Lock()
		defer tmu.Unlock()
		if timer == nil {
			timer = time.AfterFunc(1500*time.Millisecond, fire)
		}
	}
	w, err := fsnotify.NewWatcher()
	stop := make(chan struct{})
	if err == nil {
		for _, d := range []string{filepath.Dir(claudeFile()), filepath.Dir(opencodeDB())} {
			_ = os.MkdirAll(d, 0o755)
			_ = w.Add(d)
		}
		go func() {
			for {
				select {
				case ev, ok := <-w.Events:
					if !ok {
						return
					}
					b := filepath.Base(ev.Name)
					if b == "claude.json" || strings.HasPrefix(b, "opencode.db") {
						kick()
					}
				case <-w.Errors:
				case <-stop:
					return
				}
			}
		}()
	}
	tick := time.NewTicker(5 * time.Minute)
	go func() {
		for {
			select {
			case <-tick.C:
				kick()
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		tick.Stop()
		if w != nil {
			w.Close()
		}
	}
}
