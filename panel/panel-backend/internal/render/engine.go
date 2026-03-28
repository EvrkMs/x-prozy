package render

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// Engine — обёртка над html/template с горячей перезагрузкой и общими данными.
type Engine struct {
	templates *template.Template
	funcMap   template.FuncMap
	dir       string
	log       *slog.Logger
	isDev     bool
	mu        sync.RWMutex
}

// EngineConfig — настройки template engine.
type EngineConfig struct {
	Dir   string       // путь к папке templates (default: "web/templates")
	Dev   bool         // true = перечитывать шаблоны на каждый запрос
	Log   *slog.Logger // логгер
}

// NewEngine создаёт template engine.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	if cfg.Dir == "" {
		cfg.Dir = filepath.Join("web", "templates")
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}

	e := &Engine{
		dir:   cfg.Dir,
		log:   cfg.Log,
		isDev: cfg.Dev,
	}

	e.funcMap = template.FuncMap{
		"safe": func(s string) template.HTML {
			return template.HTML(s)
		},
		"dict": func(pairs ...any) map[string]any {
			m := make(map[string]any, len(pairs)/2)
			for i := 0; i+1 < len(pairs); i += 2 {
				key, _ := pairs[i].(string)
				m[key] = pairs[i+1]
			}
			return m
		},
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
	}

	if err := e.load(); err != nil {
		return nil, err
	}

	return e, nil
}

// AddFunc добавляет пользовательскую функцию в шаблоны.
// Вызывать ДО первого Render.
func (e *Engine) AddFunc(name string, fn any) {
	e.funcMap[name] = fn
}

// Render рендерит шаблон с данными и пишет в ResponseWriter.
func (e *Engine) Render(w http.ResponseWriter, name string, data any) {
	e.RenderWithStatus(w, http.StatusOK, name, data)
}

// RenderWithStatus рендерит шаблон с конкретным HTTP статусом.
func (e *Engine) RenderWithStatus(w http.ResponseWriter, status int, name string, data any) {
	// В dev-режиме — перечитываем шаблоны
	if e.isDev {
		if err := e.load(); err != nil {
			e.log.Error("reload templates", "error", err)
			http.Error(w, "template reload error", http.StatusInternalServerError)
			return
		}
	}

	e.mu.RLock()
	tmpl := e.templates
	e.mu.RUnlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		e.log.Error("render template", "name", name, "error", err)
		// Если заголовки ещё не отправлены — вернуть 500
		// Но WriteHeader уже вызван, так что просто логируем
	}
}

// load парсит все .html файлы из директории.
func (e *Engine) load() error {
	files, err := collectHTML(e.dir)
	if err != nil {
		return fmt.Errorf("render: collect templates: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("render: no templates found in %s", e.dir)
	}

	tmpl, err := template.New("").Funcs(e.funcMap).ParseFiles(files...)
	if err != nil {
		return fmt.Errorf("render: parse templates: %w", err)
	}

	e.mu.Lock()
	e.templates = tmpl
	e.mu.Unlock()

	e.log.Debug("templates loaded", "count", len(files), "dir", e.dir)
	return nil
}

func collectHTML(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && filepath.Ext(path) == ".html" {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// IsDev проверяет env переменную.
func IsDev() bool {
	v := os.Getenv("APP_ENV")
	return v == "dev" || v == "development"
}
