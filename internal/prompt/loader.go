// Package prompt loads AI prompt templates from disk, with hot reload and
// embedded defaults. No prompt text lives in Go source.
package prompt

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"
)

//go:embed defaults/*.md
var defaultFS embed.FS

// Names lists every prompt the application expects to exist.
var Names = []string{
	"system_chat",
	"intent_extraction",
	"progress_extraction",
	"reminder_message",
	"end_reminder_message",
	"daily_recap",
	"completion_reply",
}

type entry struct {
	tmpl     *template.Template
	modTime  time.Time
	fromDisk bool
}

// Loader renders named templates.
type Loader struct {
	dir       string
	hotReload bool
	mu        sync.RWMutex
	cache     map[string]*entry
}

// New validates every known prompt at startup.
func New(dir string, hotReload bool) (*Loader, error) {
	l := &Loader{
		dir:       dir,
		hotReload: hotReload,
		cache:     make(map[string]*entry, len(Names)),
	}

	var problems []string
	for _, name := range Names {
		if _, err := l.load(name); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("prompt tidak valid:\n- %s", strings.Join(problems, "\n- "))
	}
	return l, nil
}

// Render renders a prompt with the supplied data.
func (l *Loader) Render(name string, data any) (string, error) {
	e, err := l.get(name)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := e.tmpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("render prompt %s: %w", name, err)
	}
	return strings.TrimSpace(b.String()), nil
}

// Files returns the prompt file names, sorted.
func (l *Loader) Files() []string {
	out := append([]string(nil), Names...)
	sort.Strings(out)
	return out
}

func (l *Loader) get(name string) (*entry, error) {
	l.mu.RLock()
	e, ok := l.cache[name]
	l.mu.RUnlock()
	if !ok {
		return l.load(name)
	}
	if !l.hotReload || !e.fromDisk {
		return e, nil
	}

	info, err := os.Stat(filepath.Join(l.dir, name+".md"))
	if err != nil {
		return e, nil
	}
	if !info.ModTime().After(e.modTime) {
		return e, nil
	}
	return l.load(name)
}

func (l *Loader) load(name string) (*entry, error) {
	source, modTime, fromDisk, err := l.read(name)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New(name).Option("missingkey=error").Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	e := &entry{tmpl: tmpl, modTime: modTime, fromDisk: fromDisk}
	l.mu.Lock()
	l.cache[name] = e
	l.mu.Unlock()
	return e, nil
}

func (l *Loader) read(name string) (string, time.Time, bool, error) {
	path := filepath.Join(l.dir, name+".md")
	info, err := os.Stat(path)
	if err == nil {
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", time.Time{}, false, fmt.Errorf("baca file: %w", readErr)
		}
		return string(b), info.ModTime(), true, nil
	}
	if !os.IsNotExist(err) {
		return "", time.Time{}, false, fmt.Errorf("stat file: %w", err)
	}

	b, readErr := defaultFS.ReadFile("defaults/" + name + ".md")
	if readErr != nil {
		return "", time.Time{}, false, fmt.Errorf("default prompt tidak ditemukan: %w", readErr)
	}
	return string(b), time.Time{}, false, nil
}
