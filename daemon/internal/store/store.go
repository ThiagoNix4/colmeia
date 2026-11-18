// Package store: workspaces, presets, state.json e leitura de disco (port de store.ts/wsDisk.ts).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"colmeia/daemon/internal/auth"
)

// Preset espelha types.ts.
type Preset struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Command   string `json:"command"`
	Color     string `json:"color"`
	KeepShell bool   `json:"keepShell,omitempty"`
	Builtin   bool   `json:"builtin,omitempty"`
	Config    bool   `json:"configurable,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Model     string `json:"model,omitempty"`
	Args      string `json:"args,omitempty"`   // args extras anexados ao comando (built-ins editáveis)
	Edited    bool   `json:"edited,omitempty"` // built-in com ajustes do usuário
}

// Workspace é mantido como mapa para não perder campos que só o renderer conhece.
type Workspace = map[string]any

type Edge struct {
	Source, Target, Mode string
}

var mu sync.Mutex

func configDir() string   { return auth.ConfigDir() }
func wsDir() string       { return filepath.Join(configDir(), "workspaces") }
func presetsFile() string { return filepath.Join(configDir(), "presets.json") }
func stateFile() string   { return filepath.Join(configDir(), "state.json") }

func home() string { h, _ := os.UserHomeDir(); return h }

func DefaultProjectsDir() string { return filepath.Join(home(), "ai-projects") }
func PresetsPath() string        { return presetsFile() }

func ensureDirs() {
	_ = os.MkdirAll(wsDir(), 0o755)
	_ = os.MkdirAll(DefaultProjectsDir(), 0o755)
}

func readJSON(file string, v any) bool {
	b, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	return json.Unmarshal(b, v) == nil
}

func writeJSON(file string, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	tmp := file + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, file)
	}
}

// writeDurable: fsync + rename atômico.
func writeDurable(file string, data []byte) error {
	tmp := file + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	_ = f.Sync()
	f.Close()
	return os.Rename(tmp, file)
}

func setLast(n string) {
	var cur struct {
		Last string `json:"last"`
	}
	readJSON(stateFile(), &cur)
	if cur.Last != n {
		writeJSON(stateFile(), map[string]string{"last": n})
	}
}

var safeRe = regexp.MustCompile(`[^\w.-]+`)

// SafeName: mesmo formato do store.ts (64 chars, "default" se vazio).
func SafeName(n string) string {
	s := safeRe.ReplaceAllString(n, "_")
	if r := []rune(s); len(r) > 64 {
		s = string(r[:64])
	}
	if s == "" {
		return "default"
	}
	return ReserveDevice(s)
}

func parseWs(file string) Workspace {
	var ws Workspace
	if !readJSON(file, &ws) {
		return nil
	}
	if _, ok := ws["cards"].([]any); !ok {
		return nil
	}
	return ws
}

// readWorkspaceFile: JSON corrompido vira .corrupt-<ts> e tenta o .bak.
func readWorkspaceFile(file string) Workspace {
	if _, err := os.Stat(file); err != nil {
		return parseWs(file + ".bak")
	}
	if ws := parseWs(file); ws != nil {
		return ws
	}
	_ = os.Rename(file, fmt.Sprintf("%s.corrupt-%d", file, time.Now().UnixMilli()))
	return parseWs(file + ".bak")
}

// ---- presets ----

const managerID = "claude-manager"

// ManagerID: id do preset do Gerente (o spawn escolhe as instruções do card por ele).
const ManagerID = managerID

var managerTools = []string{"claude", "opencode", "codex", "gemini"}
var agentTools = []string{"claude", "opencode", "codex", "gemini", "custom"}
var removedPresets = []string{"opencode-deepseek", "claude-haiku", "claude-sonnet", "claude-opus"}
var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func has(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// ManagerCommand: comando do Gerente para a ferramenta/modelo.
func ManagerCommand(tool, model string) string { return managerCommand(tool, model, "") }

func managerCommand(tool, model, extra string) string {
	if e := strings.TrimSpace(extra); e != "" {
		return managerBase(tool, model) + " " + e
	}
	return managerBase(tool, model)
}

func managerBase(tool, model string) string {
	o := `"$(cat '{docs}/ORCHESTRATOR.md')"`
	if runtime.GOOS == "windows" {
		bin := tool
		if bin != "opencode" && bin != "codex" && bin != "gemini" {
			bin = "claude"
		}
		o = winPromptArg(bin)
	}
	m := strings.TrimSpace(model)
	mf := func(flag string) string {
		if m == "" {
			return ""
		}
		return " " + flag + " " + m
	}
	switch tool {
	case "opencode":
		// sem --prompt: ORCHESTRATOR.md entra como instrução de sistema no spawn (api/opencode.go)
		return "opencode" + mf("-m")
	case "codex":
		return "codex" + mf("-m") + " " + o
	case "gemini":
		return "gemini" + mf("-m") + " -i " + o
	}
	// sem subagents nativos: todo trabalho delegado roda em cards (o preset é regerado a cada load → migra o salvo)
	return "claude --disallowedTools Agent" + mf("--model") + " --effort medium --append-system-prompt " + o
}

const reviewerID = "claude-reviewer"

// ReviewerID: id do preset embutido Revisor.
const ReviewerID = reviewerID

// legacyWorkers: presets embutidos removidos; cards antigos ainda abrem (LegacyPreset).
var legacyWorkers = map[string]string{"claude-haiku": "haiku", "claude-sonnet": "sonnet", "claude-opus": "opus"}

// LegacyPreset: preset sintético para cards criados com os antigos claude-haiku/sonnet/opus.
func LegacyPreset(id string) (Preset, bool) {
	m, ok := legacyWorkers[id]
	if !ok {
		return Preset{}, false
	}
	return Preset{ID: id, Label: "Claude " + m, Color: "#98c379", KeepShell: true, Tool: "claude", Model: m, Command: WorkerCommand(m)}, true
}

// WorkerCommand: comando de uma operária Claude (modelo + regras do WORKER.md).
func WorkerCommand(model string) string {
	o := `"$(cat '{docs}/WORKER.md')"`
	if runtime.GOOS == "windows" {
		o = winPromptFile("claude", "WORKER.md")
	}
	return "claude --model " + model + " --append-system-prompt " + o
}

// reviewerCommand: Revisor (claude = regras do WORKER.md; outras ferramentas: comando simples).
func reviewerCommand(tool, model, extra string) string {
	var cmd string
	if tool == "claude" {
		m := model
		if m == "" {
			m = "opus"
		}
		cmd = WorkerCommand(m)
	} else {
		cmd = tool
		if model != "" {
			cmd += " -m " + model
		}
	}
	if e := strings.TrimSpace(extra); e != "" {
		cmd += " " + e
	}
	return cmd
}

func builtinShell() Preset {
	return Preset{ID: "shell", Label: "Terminal", Color: "#8b93a7", Builtin: true}
}

func isBuiltinAgent(id string) bool { return id == managerID || id == reviewerID }

// withBuiltins: shell + Gerente + Revisor; over = entradas salvas (só `edited` vale, exceto tool/model legados do Gerente).
func withBuiltins(over map[string]map[string]any, userLabels []string) []Preset {
	pick := func(id, label, color, tool, model string) Preset {
		o := over[id]
		edited, _ := o["edited"].(bool)
		if id == managerID && o != nil { // legado: tool/model do Gerente sempre salvos
			tool, model = asStr(o["tool"]), strings.TrimSpace(asStr(o["model"]))
		}
		p := Preset{ID: id, Builtin: true, KeepShell: true, Config: true}
		if edited {
			if t := asStr(o["tool"]); has(managerTools, t) {
				tool = t
			}
			model = strings.TrimSpace(asStr(o["model"]))
			if l := strings.TrimSpace(asStr(o["label"])); l != "" {
				label = l
			}
			if c := asStr(o["color"]); colorRe.MatchString(c) {
				color = c
			}
			p.Args = strings.TrimSpace(asStr(o["args"]))
			p.Edited = true
		}
		if !has(managerTools, tool) {
			tool = "claude"
		}
		p.Label, p.Color, p.Tool, p.Model = label, color, tool, model
		if id == managerID {
			p.Command = managerCommand(tool, model, p.Args)
		} else {
			p.Command = reviewerCommand(tool, model, p.Args)
		}
		return p
	}
	revLabel := "Revisor"
	for _, l := range userLabels {
		if strings.EqualFold(l, "Revisor") {
			revLabel = "Revisor (padrão)" // não duplica o rótulo de um agente do usuário
		}
	}
	return []Preset{builtinShell(),
		pick(managerID, "Gerente", "#e8b04a", "claude", ""),
		pick(reviewerID, revLabel, "#c678dd", "claude", "opus")}
}

// reservedIDs: ids dos presets embutidos (o usuário não os sobrescreve).
func reservedIDs() map[string]bool {
	return map[string]bool{"shell": true, managerID: true, reviewerID: true}
}

func asStr(v any) string { s, _ := v.(string); return s }

// LoadPresets: builtins na versão do código primeiro; agentes do usuário depois, intactos.
func LoadPresets() []Preset {
	mu.Lock()
	defer mu.Unlock()
	ensureDirs()
	var raw []map[string]any
	readJSON(presetsFile(), &raw)
	seen := reservedIDs()
	var users []Preset
	over := map[string]map[string]any{}
	var labels []string
	for _, p := range raw {
		id := asStr(p["id"])
		if isBuiltinAgent(id) {
			over[id] = p
			continue
		}
		if id == "" || seen[id] || has(removedPresets, id) {
			continue
		}
		seen[id] = true
		users = append(users, presetFromMap(p))
		labels = append(labels, asStr(p["label"]))
	}
	merged := append(withBuiltins(over, labels), users...)
	// grava só se mudou
	cur, _ := json.Marshal(merged)
	rawB, _ := json.Marshal(raw)
	if string(cur) != string(rawB) {
		writeJSON(presetsFile(), merged)
	}
	return merged
}

func presetFromMap(m map[string]any) Preset {
	b, _ := json.Marshal(m)
	var p Preset
	_ = json.Unmarshal(b, &p)
	return p
}

// SavePresets grava só os agentes do usuário; valida id, nome e cor.
func SavePresets(input []map[string]any) ([]Preset, error) {
	mu.Lock()
	defer mu.Unlock()
	ensureDirs()
	if input == nil {
		return nil, errors.New("lista de agentes inválida")
	}
	seen := reservedIDs()
	var users []Preset
	over := map[string]map[string]any{}
	var labels []string
	for _, raw := range input {
		if raw == nil {
			continue
		}
		if bid := asStr(raw["id"]); isBuiltinAgent(bid) {
			if r, _ := raw["reset"].(bool); r {
				continue // restaurar padrão
			}
			o := map[string]any{}
			for k, v := range raw {
				o[k] = v
			}
			delete(o, "edited")
			over[bid] = o
			continue
		}
		if b, _ := raw["builtin"].(bool); b {
			continue
		}
		id := strings.TrimSpace(asStr(raw["id"]))
		if id == "" {
			return nil, errors.New("agente sem id")
		}
		if seen[id] {
			return nil, fmt.Errorf("id repetido: %s", id)
		}
		label := strings.TrimSpace(asStr(raw["label"]))
		if label == "" {
			return nil, fmt.Errorf(`agente "%s" sem nome`, id)
		}
		color := asStr(raw["color"])
		if !colorRe.MatchString(color) {
			return nil, fmt.Errorf(`cor inválida em "%s"`, label)
		}
		seen[id] = true
		p := Preset{ID: id, Label: label, Command: asStr(raw["command"]), Color: color}
		p.KeepShell, _ = raw["keepShell"].(bool)
		if t := asStr(raw["tool"]); t != "" && has(agentTools, t) {
			p.Tool = t
		}
		if m := strings.TrimSpace(asStr(raw["model"])); m != "" {
			p.Model = m
		}
		p.Args = strings.TrimSpace(asStr(raw["args"]))
		users = append(users, p)
		labels = append(labels, label)
	}
	for _, d := range withBuiltins(nil, labels) { // só vale como ajuste o que difere do padrão
		if o := over[d.ID]; o != nil {
			lb := strings.TrimSpace(asStr(o["label"]))
			defLabel := lb == d.Label || (d.ID == reviewerID && (lb == "Revisor" || lb == "Revisor (padrão)")) // rótulo padrão anterior não conta como edição
			o["edited"] = !defLabel || asStr(o["color"]) != d.Color ||
				asStr(o["tool"]) != d.Tool || strings.TrimSpace(asStr(o["model"])) != d.Model || strings.TrimSpace(asStr(o["args"])) != ""
		}
	}
	full := append(withBuiltins(over, labels), users...)
	writeJSON(presetsFile(), full)
	return full, nil
}

// ---- workspaces ----

func ListWorkspaces() []string {
	mu.Lock()
	defer mu.Unlock()
	return listWs()
}

func listWs() []string {
	ensureDirs()
	out := []string{}
	ents, _ := os.ReadDir(wsDir())
	for _, e := range ents {
		if n := e.Name(); strings.HasSuffix(n, ".json") {
			out = append(out, strings.TrimSuffix(n, ".json"))
		}
	}
	sort.Strings(out)
	return out
}

// AllCardIDs lê todos os workspaces do disco (sem tocar state.json).
func AllCardIDs() []string {
	mu.Lock()
	defer mu.Unlock()
	ids := map[string]bool{}
	for _, n := range listWs() {
		f := filepath.Join(wsDir(), n+".json")
		ws := parseWs(f)
		if ws == nil {
			ws = parseWs(f + ".bak")
		}
		for _, c := range cardsOf(ws) {
			if id := asStr(c["id"]); id != "" {
				ids[id] = true
			}
		}
	}
	out := []string{}
	for id := range ids {
		out = append(out, id)
	}
	return out
}

func cardsOf(ws Workspace) []map[string]any {
	var out []map[string]any
	cs, _ := ws["cards"].([]any)
	for _, c := range cs {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// EdgesOf: arestas do workspace como pares direcionados.
func EdgesOf(ws Workspace) []Edge {
	var out []Edge
	es, _ := ws["edges"].([]any)
	for _, e := range es {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		mode := "control"
		if asStr(m["mode"]) == "pipe" {
			mode = "pipe"
		}
		out = append(out, Edge{asStr(m["source"]), asStr(m["target"]), mode})
	}
	return out
}

// WorkspaceStr/WorkspaceCwd: acessores do mapa.
func WorkspaceName(ws Workspace) string { return asStr(ws["name"]) }
func WorkspaceCwd(ws Workspace) string  { return asStr(ws["cwd"]) }

// WorkspaceExists: já há arquivo (ou .bak) em disco para o nome (vazio = último, assume que existe).
func WorkspaceExists(name string) bool {
	if name == "" {
		return true
	}
	f := filepath.Join(wsDir(), SafeName(name)+".json")
	for _, p := range []string{f, f + ".bak"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func LoadWorkspace(name string) Workspace {
	mu.Lock()
	defer mu.Unlock()
	ensureDirs()
	if name == "" {
		var st struct {
			Last string `json:"last"`
		}
		readJSON(stateFile(), &st)
		name = st.Last
	}
	n := SafeName(name)
	ws := readWorkspaceFile(filepath.Join(wsDir(), n+".json"))
	setLast(n)
	if ws == nil {
		return Workspace{"name": n, "cwd": DefaultProjectsDir(), "cards": []any{}, "edges": []any{}}
	}
	cs, _ := ws["cards"].([]any)
	for _, c := range cs {
		if m, ok := c.(map[string]any); ok && m["preset"] == "opencode-deepseek" {
			m["preset"] = "opencode"
		}
	}
	return ws
}

func SaveWorkspace(ws Workspace) error {
	mu.Lock()
	defer mu.Unlock()
	ensureDirs()
	n := SafeName(asStr(ws["name"]))
	file := filepath.Join(wsDir(), n+".json")
	// .bak = último estado não vazio (nunca sobrescrito por um vazio)
	if prev := parseWs(file); prev != nil {
		es, _ := prev["edges"].([]any)
		if len(cardsOf(prev)) > 0 || len(es) > 0 {
			if b, err := os.ReadFile(file); err == nil {
				_ = os.WriteFile(file+".bak", b, 0o644)
			}
		}
	}
	out := map[string]any{}
	for k, v := range ws {
		out[k] = v
	}
	out["name"] = n
	// política de modelos não vem do renderer: preserva a do disco
	if _, ok := out["models"]; !ok {
		if prev := readDisk(file); prev != nil {
			if m, ok := prev["models"]; ok {
				out["models"] = m
			}
		}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if err := writeDurable(file, b); err != nil {
		return err
	}
	setLast(n)
	return nil
}

func DeleteWorkspace(name string) {
	mu.Lock()
	defer mu.Unlock()
	ensureDirs()
	f := filepath.Join(wsDir(), SafeName(name)+".json")
	_ = os.Remove(f)
	_ = os.Remove(f + ".bak")
}

func RenameWorkspace(from, to string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	ensureDirs()
	a, b := SafeName(from), SafeName(to)
	if a == b {
		return b, nil
	}
	src := filepath.Join(wsDir(), a+".json")
	dst := filepath.Join(wsDir(), b+".json")
	if _, err := os.Stat(src); err != nil {
		return "", errors.New("workspace não encontrado")
	}
	if _, err := os.Stat(dst); err == nil {
		return "", errors.New("já existe um workspace com esse nome")
	}
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	if _, err := os.Stat(src + ".bak"); err == nil {
		_ = os.Rename(src+".bak", dst+".bak")
	}
	if ws := parseWs(dst); ws != nil {
		ws["name"] = b
		writeJSON(dst, ws)
	}
	var st struct {
		Last string `json:"last"`
	}
	readJSON(stateFile(), &st)
	if st.Last == a {
		writeJSON(stateFile(), map[string]string{"last": b})
	}
	return b, nil
}

// ---- wsDisk: leitura somente-disco (permissão de cards de outro ws) ----

func readDisk(file string) Workspace {
	for _, f := range []string{file, file + ".bak"} {
		if ws := parseWs(f); ws != nil {
			return ws
		}
	}
	return nil
}

// WorkspaceOfCard: nome (arquivo) do workspace que contém o card, ou "".
func WorkspaceOfCard(cardID string) string {
	ents, err := os.ReadDir(wsDir())
	if err != nil {
		return ""
	}
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".json") {
			continue
		}
		for _, c := range cardsOf(readDisk(filepath.Join(wsDir(), n))) {
			if asStr(c["id"]) == cardID {
				return strings.TrimSuffix(n, ".json")
			}
		}
	}
	return ""
}

func LoadEdges(name string) []Edge {
	return EdgesOf(readDisk(filepath.Join(wsDir(), name+".json")))
}

// CardModel: --model persistido no registro do card ("" se ausente ou card desconhecido).
func CardModel(cardID string) string {
	mu.Lock()
	defer mu.Unlock()
	for _, n := range listWs() {
		for _, c := range cardsOf(readDisk(filepath.Join(wsDir(), n+".json"))) {
			if asStr(c["id"]) == cardID {
				return strings.TrimSpace(asStr(c["model"]))
			}
		}
	}
	return ""
}

// CardWorkspaces: id do card → nome do workspace, lido do disco (todos os ws).
func CardWorkspaces() map[string]string {
	mu.Lock()
	defer mu.Unlock()
	out := map[string]string{}
	for _, n := range listWs() {
		for _, c := range cardsOf(readDisk(filepath.Join(wsDir(), n+".json"))) {
			if id := asStr(c["id"]); id != "" {
				out[id] = n
			}
		}
	}
	return out
}

// Mutate lê o workspace do disco, aplica fn e grava (sem UI aberta). Não mexe em state.json.
func Mutate(name string, fn func(ws Workspace)) error {
	mu.Lock()
	defer mu.Unlock()
	ensureDirs()
	n := SafeName(name)
	file := filepath.Join(wsDir(), n+".json")
	ws := readDisk(file)
	if ws == nil {
		ws = Workspace{"name": n, "cwd": DefaultProjectsDir(), "cards": []any{}, "edges": []any{}}
	}
	fn(ws)
	ws["name"] = n
	b, _ := json.MarshalIndent(ws, "", "  ")
	return writeDurable(file, b)
}

// AddCard posiciona o card como o renderer (freeSpot): ao lado de `near` (se houver)
// ou à direita dos existentes; sem sobrepor, procurando em anéis crescentes.
func AddCard(name string, card map[string]any, near string) error {
	return Mutate(name, func(ws Workspace) {
		cs, _ := ws["cards"].([]any)
		const cw, ch, gap = 640.0, 400.0, 40.0
		type rect struct{ x, y, w, h float64 }
		var rs []rect
		var nr *rect
		maxRight := 0.0
		for _, c := range cardsOf(ws) {
			if asStr(c["id"]) == asStr(card["id"]) {
				return
			}
			x, _ := c["x"].(float64)
			y, _ := c["y"].(float64)
			w, _ := c["width"].(float64)
			h, _ := c["height"].(float64)
			if w == 0 {
				w = cw
			}
			if h == 0 {
				h = ch
			}
			r := rect{x, y, w, h}
			rs = append(rs, r)
			if x+w+gap > maxRight {
				maxRight = x + w + gap
			}
			if near != "" && asStr(c["id"]) == near {
				rr := r
				nr = &rr
			}
		}
		overlaps := func(px, py float64) bool {
			for _, r := range rs {
				if px < r.x+r.w+gap && px+cw+gap > r.x && py < r.y+r.h+gap && py+ch+gap > r.y {
					return true
				}
			}
			return false
		}
		ox, oy := maxRight, 0.0
		if nr != nil {
			ox, oy = nr.x+nr.w+80, nr.y
		}
		px, py := ox, oy
		if overlaps(px, py) {
			stepX, stepY := cw+gap, ch+gap
			found := false
			for r := 1; r < 12 && !found; r++ {
				for dx := -r; dx <= r && !found; dx++ {
					for dy := -r; dy <= r; dy++ {
						if max(abs(dx), abs(dy)) != r {
							continue
						}
						cx, cy := ox+float64(dx)*stepX, oy+float64(dy)*stepY
						if overlaps(cx, cy) || (nr != nil && cx < nr.x) {
							continue
						}
						px, py, found = cx, cy, true
						break
					}
				}
			}
			if !found {
				px, py = ox+40, oy+40
			}
		}
		c := map[string]any{"x": px, "y": py, "width": cw, "height": ch}
		hx, hasX := card["x"].(float64)
		hy, hasY := card["y"].(float64)
		keepPos := hasX && hasY && !overlaps(hx, hy) // posição sugerida só vale se livre
		for k, v := range card {
			if !keepPos && (k == "x" || k == "y" || k == "width" || k == "height") {
				continue
			}
			c[k] = v
		}
		ws["cards"] = append(cs, c)
	})
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// RemoveCard tira o card e as arestas que o tocam.
func RemoveCard(name, id string) error { return removeNode(name, id, true) }

// RemoveEdgesOf tira as arestas que tocam o id (nota removida).
func RemoveEdgesOf(name, id string) error { return removeNode(name, id, false) }

func removeNode(name, id string, card bool) error {
	return Mutate(name, func(ws Workspace) {
		if card {
			kept := []any{}
			cs, _ := ws["cards"].([]any)
			for _, c := range cs {
				if m, ok := c.(map[string]any); ok && asStr(m["id"]) == id {
					continue
				}
				kept = append(kept, c)
			}
			ws["cards"] = kept
		}
		ws["edges"] = filterEdges(ws, func(m map[string]any) bool { return asStr(m["source"]) == id || asStr(m["target"]) == id })
	})
}

func filterEdges(ws Workspace, drop func(map[string]any) bool) []any {
	kept := []any{}
	es, _ := ws["edges"].([]any)
	for _, e := range es {
		if m, ok := e.(map[string]any); ok && drop(m) {
			continue
		}
		kept = append(kept, e)
	}
	return kept
}

// AddEdge acrescenta/atualiza a aresta source→target.
func AddEdge(name, source, target, mode string) error {
	return Mutate(name, func(ws Workspace) {
		ws["edges"] = append(filterEdges(ws, func(m map[string]any) bool {
			return asStr(m["source"]) == source && asStr(m["target"]) == target
		}), map[string]any{"id": "e-" + source + "-" + target, "source": source, "target": target, "mode": mode})
	})
}

// RemoveEdge remove a aresta nos dois sentidos.
func RemoveEdge(name, a, b string) error {
	return Mutate(name, func(ws Workspace) {
		ws["edges"] = filterEdges(ws, func(m map[string]any) bool {
			s, t := asStr(m["source"]), asStr(m["target"])
			return (s == a && t == b) || (s == b && t == a)
		})
	})
}

// CardIDsOf: ids dos cards de um workspace (map do mapa carregado).
func CardIDsOf(ws Workspace) []string {
	var out []string
	for _, c := range cardsOf(ws) {
		if id := asStr(c["id"]); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// winPromptArg: expressão PowerShell que lê ORCHESTRATOR.md (UTF-8) como argumento nativo.
//   - shim .cmd/.bat (npm): passa pelo cmd.exe, que corta em quebra de linha e tem limite de ~8 KB →
//     manda só uma instrução curta apontando o arquivo.
//   - PS 5.1 / pwsh <7.3 não escapam `"` em args nativos: escapa no estilo CommandLineToArgvW
//     (barras antes de aspas dobradas, `"`→`\"`, barras finais dobradas). 7.3+ já escapa sozinho.
func winPromptArg(bin string) string { return winPromptFile(bin, "ORCHESTRATOR.md") }

// winPromptFile: igual a winPromptArg para qualquer arquivo de {docs} (ex.: WORKER.md).
func winPromptFile(bin, name string) string {
	f := `'{docs}\` + name + `'`
	return `$(& { $t = Get-Content -Raw -Encoding UTF8 ` + f + `; ` +
		`$c = Get-Command ` + bin + ` -ErrorAction SilentlyContinue | Select-Object -First 1; ` +
		`if ($c -and $c.Source -match '\.(cmd|bat)$') { 'Your operating instructions are in the file ' + ` + f + ` + '. Read it first and follow it as your system prompt.' } ` +
		`elseif ($PSVersionTable.PSVersion -lt [version]'7.3') { $t = [regex]::Replace($t, '(\\*)"', '$1$1\"'); [regex]::Replace($t, '(\\+)\z', '$1$1') } ` +
		`else { $t } })`
}

// ReserveDevice: no Windows, nomes de dispositivo (CON, NUL, COM1…) viram "<nome>_" (base antes do ponto).
func ReserveDevice(s string) string {
	if runtime.GOOS != "windows" {
		return s
	}
	base := strings.ToUpper(strings.SplitN(s, ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return "_" + s
	}
	return s
}

// Política de modelos do workspace: free | ask | all (padrão ask).
func NormPolicy(v string) string {
	switch v {
	case "free", "ask", "all":
		return v
	}
	return ""
}

func GetPolicy(name string) string {
	mu.Lock()
	defer mu.Unlock()
	ws := readDisk(filepath.Join(wsDir(), SafeName(name)+".json"))
	if p := NormPolicy(asStr(ws["models"])); p != "" {
		return p
	}
	return "ask"
}

func SetPolicy(name, v string) error {
	if NormPolicy(v) == "" {
		return errors.New("política inválida: use free, ask ou all")
	}
	return Mutate(name, func(ws Workspace) { ws["models"] = v })
}

// userMaps: agentes do usuário e Gerente salvo, no formato que SavePresets espera.
func userMaps(cur []Preset) []map[string]any {
	var out []map[string]any
	for _, p := range cur {
		if !isBuiltinAgent(p.ID) && p.Builtin {
			continue
		}
		b, _ := json.Marshal(p)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		out = append(out, m)
	}
	return out
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// AddUserPreset cria um agente nomeado (mesmos campos do menu Agentes). cur = presets atuais.
func AddUserPreset(cur []Preset, title, tool, model string, worker bool) ([]Preset, Preset, error) {
	title = strings.TrimSpace(title)
	model = strings.TrimSpace(model)
	if title == "" {
		return nil, Preset{}, errors.New("título obrigatório")
	}
	if tool == "" {
		tool = "claude"
	}
	if tool == "custom" || !has(agentTools, tool) {
		return nil, Preset{}, fmt.Errorf("--tool inválido: %s (claude|opencode|codex|gemini)", tool)
	}
	for _, p := range cur {
		if strings.EqualFold(p.Label, title) || p.ID == title {
			return nil, Preset{}, fmt.Errorf("já existe preset com esse nome: %s", p.ID)
		}
	}
	cmd := tool
	flag := map[string]string{"claude": "--model"}[tool]
	if flag == "" {
		flag = "-m"
	}
	if model != "" {
		cmd += " " + flag + " " + model
	}
	if worker {
		if tool != "claude" {
			return nil, Preset{}, errors.New("--worker só vale para --tool claude")
		}
		if model == "" {
			cmd = strings.Replace(WorkerCommand("sonnet"), "--model sonnet", "", 1)
		} else {
			cmd = WorkerCommand(model)
		}
	}
	id := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if id == "" {
		id = "agent"
	}
	ids := map[string]bool{}
	for _, p := range cur {
		ids[p.ID] = true
	}
	for base, n := id, 2; ids[id] || reservedIDs()[id]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	np := map[string]any{"id": id, "label": title, "command": cmd, "color": "#e8b04a", "keepShell": true, "tool": tool}
	if model != "" {
		np["model"] = model
	}
	list, err := SavePresets(append(userMaps(cur), np))
	if err != nil {
		return nil, Preset{}, err
	}
	for _, p := range list {
		if p.ID == id {
			return list, p, nil
		}
	}
	return list, Preset{ID: id}, nil
}

// RemoveUserPreset apaga um agente do usuário (título ou id); embutidos não.
func RemoveUserPreset(cur []Preset, key string) ([]Preset, string, error) {
	var target *Preset
	for i := range cur {
		if cur[i].ID == key || strings.EqualFold(cur[i].Label, key) {
			target = &cur[i]
			break
		}
	}
	if target == nil {
		return nil, "", fmt.Errorf("preset desconhecido: %s", key)
	}
	if target.Builtin {
		return nil, "", fmt.Errorf("preset embutido não pode ser removido: %s", target.ID)
	}
	var keep []Preset
	for _, p := range cur {
		if p.ID != target.ID {
			keep = append(keep, p)
		}
	}
	list, err := SavePresets(userMaps(keep))
	return list, target.ID, err
}

// SetCardRects grava x/y/width/height dos cards (tidy sem UI aberta).
func SetCardRects(name string, rects map[string]struct{ X, Y, W, H float64 }) error {
	return Mutate(name, func(ws Workspace) {
		for _, c := range cardsOf(ws) {
			if r, ok := rects[asStr(c["id"])]; ok {
				c["x"], c["y"], c["width"], c["height"] = r.X, r.Y, r.W, r.H
			}
		}
	})
}

// ResetBuiltinPreset restaura o padrão de um agente embutido (Gerente/Revisor), por id ou nome.
func ResetBuiltinPreset(cur []Preset, key string) ([]Preset, string, error) {
	var target *Preset
	for i := range cur {
		if cur[i].ID == key || strings.EqualFold(cur[i].Label, key) {
			target = &cur[i]
			break
		}
	}
	if target == nil || !isBuiltinAgent(target.ID) {
		return nil, "", fmt.Errorf("só Gerente/Revisor têm padrão a restaurar: %s", key)
	}
	maps := userMaps(cur)
	for i := range maps {
		if asStr(maps[i]["id"]) == target.ID {
			maps[i]["reset"] = true
		}
	}
	list, err := SavePresets(maps)
	return list, target.ID, err
}

// CardRects: retângulos dos cards do workspace em disco (sem UI aberta), exceto skip.
func CardRects(name, skip string) []struct{ X, Y, W, H float64 } {
	mu.Lock()
	defer mu.Unlock()
	ws := readDisk(filepath.Join(wsDir(), SafeName(name)+".json"))
	var out []struct{ X, Y, W, H float64 }
	for _, c := range cardsOf(ws) {
		if asStr(c["id"]) == skip {
			continue
		}
		x, _ := c["x"].(float64)
		y, _ := c["y"].(float64)
		w, _ := c["width"].(float64)
		h, _ := c["height"].(float64)
		if w == 0 {
			w = 640
		}
		if h == 0 {
			h = 400
		}
		out = append(out, struct{ X, Y, W, H float64 }{x, y, w, h})
	}
	return out
}
