// Package notes: índice de notas por workspace + arquivos .md (port de notesStore.ts).
package notes

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"colmeia/daemon/internal/auth"
	"colmeia/daemon/internal/store"
)

type NoteCard struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	File      string  `json:"file"`
	Workspace string  `json:"workspace"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	// Extra guarda campos desconhecidos (ex.: novos campos do renderer) sem perdê-los.
	Extra map[string]json.RawMessage `json:"-"`
}

type noteAlias NoteCard

func (n *NoteCard) UnmarshalJSON(b []byte) error {
	var a noteAlias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	for _, k := range []string{"id", "title", "file", "workspace", "x", "y", "width", "height"} {
		delete(all, k)
	}
	a.Extra = nil
	if len(all) > 0 {
		a.Extra = all
	}
	*n = NoteCard(a)
	return nil
}

func (n NoteCard) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(noteAlias(n))
	if err != nil || len(n.Extra) == 0 {
		return b, err
	}
	m := map[string]json.RawMessage{}
	_ = json.Unmarshal(b, &m)
	for k, v := range n.Extra {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

type index struct {
	Workspace string     `json:"workspace"`
	Notes     []NoteCard `json:"notes"`
}

var (
	mu    sync.Mutex
	cache = map[string]*index{}
	root  struct {
		at time.Time
		v  string
	}
)

func indexDir() string     { return filepath.Join(auth.ConfigDir(), "notes") }
func configFile() string   { return filepath.Join(auth.ConfigDir(), "notes-config.json") }
func fallbackRoot() string { return filepath.Join(auth.ConfigDir(), "notes-files") }
func obsidianFile() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "obsidian", "obsidian.json")
}
func indexFile(ws string) string { return filepath.Join(indexDir(), store.SafeName(ws)+".json") }

func readJSON(file string, v any) bool {
	b, err := os.ReadFile(file)
	return err == nil && json.Unmarshal(b, v) == nil
}

func writeAtomic(file string, text []byte) error {
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, text, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

var slugRe = regexp.MustCompile(`[^\w.-]+`)

func slugify(title string) string {
	s := slugRe.ReplaceAllString(strings.ToLower(title), "-")
	if r := []rune(s); len(r) > 64 {
		s = string(r[:64])
	}
	if s == "" {
		return "nota"
	}
	return s
}

// Root: env → notes-config.json → vault do Obsidian → fallback (cache 3s).
func Root() string {
	mu.Lock()
	defer mu.Unlock()
	return rootLocked()
}

func rootLocked() string {
	if root.v != "" && time.Since(root.at) < 3*time.Second {
		return root.v
	}
	root.v, root.at = computeRoot(), time.Now()
	return root.v
}

func abs(p string) string { a, _ := filepath.Abs(p); return a }

func computeRoot() string {
	if e := os.Getenv("COLMEIA_NOTES_DIR"); e != "" {
		return abs(e)
	}
	var cfg struct {
		Root string `json:"root"`
	}
	readJSON(configFile(), &cfg)
	if cfg.Root != "" {
		return abs(cfg.Root)
	}
	var obs struct {
		Vaults map[string]struct {
			Path string `json:"path"`
			Open bool   `json:"open"`
		} `json:"vaults"`
	}
	readJSON(obsidianFile(), &obs)
	pick := ""
	for _, v := range obs.Vaults {
		if v.Open && v.Path != "" {
			pick = v.Path
			break
		}
	}
	if pick == "" {
		for _, v := range obs.Vaults {
			if v.Path != "" {
				pick = v.Path
				break
			}
		}
	}
	if pick != "" {
		return filepath.Join(pick, "Colmeia")
	}
	return fallbackRoot()
}

// NoteDir: diretório das notas do workspace, criado se faltar.
func NoteDir(ws string) string {
	mu.Lock()
	defer mu.Unlock()
	return noteDirLocked(ws)
}

func noteDirLocked(ws string) string {
	d := filepath.Join(rootLocked(), store.SafeName(ws))
	_ = os.MkdirAll(d, 0o755)
	return d
}

func loadIndex(ws string) *index {
	ws = store.SafeName(ws)
	if i, ok := cache[ws]; ok {
		return i
	}
	var idx index
	if !readJSON(indexFile(ws), &idx) || idx.Notes == nil {
		idx = index{Notes: []NoteCard{}}
	}
	idx.Workspace = ws
	cache[ws] = &idx
	return &idx
}

func saveIndex(i *index) {
	cache[i.Workspace] = i
	b, _ := json.MarshalIndent(i, "", "  ")
	_ = writeAtomic(indexFile(i.Workspace), b)
}

// confine: o arquivo precisa ficar dentro da raiz das notas.
func confine(file string) (string, error) {
	a := abs(file)
	rel, err := filepath.Rel(rootLocked(), a)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("caminho fora da pasta de notas: %s", file)
	}
	return a, nil
}

// Mtime em ms (0 se o arquivo não existe).
func Mtime(file string) float64 {
	fi, err := os.Stat(file)
	if err != nil {
		return 0
	}
	return float64(fi.ModTime().UnixNano()) / 1e6
}

func List(ws string) []NoteCard {
	mu.Lock()
	defer mu.Unlock()
	return append([]NoteCard{}, loadIndex(ws).Notes...)
}

func getLocked(ws, ref string) *NoteCard {
	r := strings.TrimSpace(ref)
	if r == "" {
		return nil
	}
	ns := loadIndex(ws).Notes
	for i := range ns {
		if ns[i].ID == r {
			return &ns[i]
		}
	}
	for i := range ns {
		if strings.EqualFold(ns[i].Title, r) {
			return &ns[i]
		}
	}
	for i := range ns {
		if strings.HasPrefix(ns[i].ID, r) {
			return &ns[i]
		}
	}
	return nil
}

// Get resolve id exato, título (sem caixa) ou prefixo de id.
func Get(ws, ref string) (NoteCard, bool) {
	mu.Lock()
	defer mu.Unlock()
	if n := getLocked(ws, ref); n != nil {
		return *n, true
	}
	return NoteCard{}, false
}

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func NewID() string { return newID() }

func Create(ws, title string, body *string) (NoteCard, error) {
	mu.Lock()
	defer mu.Unlock()
	ws = store.SafeName(ws)
	dir := noteDirLocked(ws)
	idx := loadIndex(ws)
	base := slugify(title)
	taken := map[string]bool{}
	for _, n := range idx.Notes {
		taken[n.File] = true
	}
	slug := base
	exists := func(s string) bool {
		f := filepath.Join(dir, s+".md")
		_, err := os.Stat(f)
		return err == nil || taken[f]
	}
	for i := 2; exists(slug); i++ {
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	file := filepath.Join(dir, slug+".md")
	text := fmt.Sprintf("# %s\n\n", title)
	if body != nil {
		text = *body
	}
	if err := writeAtomic(file, []byte(text)); err != nil {
		return NoteCard{}, err
	}
	n := NoteCard{ID: newID(), Title: title, File: file, Workspace: ws, Width: 420, Height: 320}
	idx.Notes = append(idx.Notes, n)
	saveIndex(idx)
	return n, nil
}

// Update aplica um patch parcial (id e workspace não mudam).
func Update(ws, id string, patch map[string]any) (NoteCard, error) {
	mu.Lock()
	defer mu.Unlock()
	idx := loadIndex(ws)
	for i := range idx.Notes {
		if idx.Notes[i].ID != id {
			continue
		}
		cur := idx.Notes[i]
		m := map[string]any{}
		b, _ := json.Marshal(cur)
		_ = json.Unmarshal(b, &m)
		for k, v := range patch {
			m[k] = v
		}
		m["id"], m["workspace"] = cur.ID, cur.Workspace
		b, _ = json.Marshal(m)
		next := cur
		if err := json.Unmarshal(b, &next); err != nil {
			next = cur
		}
		idx.Notes[i] = next
		saveIndex(idx)
		return next, nil
	}
	return NoteCard{}, fmt.Errorf("nota não encontrada: %s", id)
}

func Remove(ws, id string, deleteFile bool) error {
	mu.Lock()
	defer mu.Unlock()
	idx := loadIndex(ws)
	var note *NoteCard
	kept := make([]NoteCard, 0, len(idx.Notes))
	for i := range idx.Notes {
		if idx.Notes[i].ID == id {
			n := idx.Notes[i]
			note = &n
			continue
		}
		kept = append(kept, idx.Notes[i])
	}
	if note == nil {
		return nil
	}
	idx.Notes = kept
	saveIndex(idx)
	if deleteFile {
		f, err := confine(note.File)
		if err != nil {
			return err
		}
		_ = os.Remove(f)
	}
	return nil
}

type Read struct {
	Body  string  `json:"body"`
	Mtime float64 `json:"mtime"`
}

func ReadNote(ws, ref string) (Read, error) {
	mu.Lock()
	defer mu.Unlock()
	n := getLocked(ws, ref)
	if n == nil {
		return Read{}, fmt.Errorf("nota não encontrada: %s", ref)
	}
	f, err := confine(n.File)
	if err != nil {
		return Read{}, err
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return Read{}, nil // .md apagado por fora: leitura nunca falha
	}
	return Read{string(b), Mtime(f)}, nil
}

// Write grava; baseMtime != nil e o disco mudou depois dele → erro "conflict".
func Write(ws, ref, text string, baseMtime *float64) (float64, error) {
	mu.Lock()
	defer mu.Unlock()
	n := getLocked(ws, ref)
	if n == nil {
		return 0, fmt.Errorf("nota não encontrada: %s", ref)
	}
	f, err := confine(n.File)
	if err != nil {
		return 0, err
	}
	if baseMtime != nil && Mtime(f) > *baseMtime {
		return 0, errors.New("conflict")
	}
	if err := writeAtomic(f, []byte(text)); err != nil {
		return 0, err
	}
	return Mtime(f), nil
}

func Append(ws, ref, text string) (float64, error) {
	mu.Lock()
	defer mu.Unlock()
	n := getLocked(ws, ref)
	if n == nil {
		return 0, fmt.Errorf("nota não encontrada: %s", ref)
	}
	f, err := confine(n.File)
	if err != nil {
		return 0, err
	}
	cur, _ := os.ReadFile(f)
	if err := writeAtomic(f, append(cur, text...)); err != nil {
		return 0, err
	}
	return Mtime(f), nil
}

// RenameWorkspace move índice + pasta ao renomear o workspace (mescla se o destino existe).
func RenameWorkspace(from, to string) {
	mu.Lock()
	defer mu.Unlock()
	a, b := store.SafeName(from), store.SafeName(to)
	if a == b {
		return
	}
	idxA, idxB := loadIndex(a), loadIndex(b)
	r := rootLocked()
	oldDir, newDir := filepath.Join(r, a), filepath.Join(r, b)
	mapped := map[string]string{}
	if ents, err := os.ReadDir(oldDir); err == nil {
		_ = os.MkdirAll(newDir, 0o755)
		for _, e := range ents {
			f := e.Name()
			ext := filepath.Ext(f)
			base := strings.TrimSuffix(f, ext)
			dest := filepath.Join(newDir, f)
			for i := 2; ; i++ {
				if _, err := os.Stat(dest); err != nil {
					break
				}
				dest = filepath.Join(newDir, fmt.Sprintf("%s-%d%s", base, i, ext))
			}
			if os.Rename(filepath.Join(oldDir, f), dest) == nil {
				mapped[filepath.Join(oldDir, f)] = dest
			}
		}
		_ = os.Remove(oldDir)
	}
	taken := map[string]bool{}
	for _, n := range idxB.Notes {
		taken[n.ID] = true
	}
	next := &index{Workspace: b, Notes: append([]NoteCard{}, idxB.Notes...)}
	for _, n := range idxA.Notes {
		n.Workspace = b
		if d, ok := mapped[n.File]; ok {
			n.File = d
		}
		if !taken[n.ID] {
			next.Notes = append(next.Notes, n)
		}
	}
	_, statErr := os.Stat(indexFile(a))
	if len(next.Notes) > 0 || statErr == nil {
		saveIndex(next)
	}
	delete(cache, a)
	_ = os.Remove(indexFile(a))
}

// DeleteWorkspace remove o índice e guarda os .md em <raiz>/_deleted/<ws>-<ts>/.
func DeleteWorkspace(ws string) {
	mu.Lock()
	defer mu.Unlock()
	ws = store.SafeName(ws)
	delete(cache, ws)
	_ = os.Remove(indexFile(ws))
	dir := filepath.Join(rootLocked(), ws)
	if _, err := os.Stat(dir); err != nil {
		return
	}
	dest := filepath.Join(rootLocked(), "_deleted", fmt.Sprintf("%s-%d", ws, time.Now().UnixMilli()))
	_ = os.MkdirAll(filepath.Dir(dest), 0o755)
	_ = os.Rename(dir, dest)
}
