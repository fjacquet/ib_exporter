package config

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Holder owns the live configuration and reloads it from disk.
type Holder struct {
	path string
	mu   sync.RWMutex
	cfg  *Config
}

// NewHolder loads and validates path.
func NewHolder(path string) (*Holder, error) {
	cfg, err := loadValid(path)
	if err != nil {
		return nil, err
	}
	return &Holder{path: path, cfg: cfg}, nil
}

func loadValid(path string) (*Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	return cfg, cfg.Validate()
}

// Get returns the current config; treat it as immutable.
func (h *Holder) Get() *Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

// Reload swaps in the file's config when it loads and validates; otherwise it
// returns the error and keeps the old one.
func (h *Holder) Reload() error {
	cfg, err := loadValid(h.path)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.cfg = cfg
	h.mu.Unlock()
	return nil
}

// Watch reloads on SIGHUP and on changes to the config file (debounced), until
// ctx ends. onReload is called with the old and new config after each successful
// reload; failures are logged and the old config stays. The parent directory is
// watched, not the file, so atomic renames and Kubernetes ConfigMap symlink swaps
// are seen.
func (h *Holder) Watch(ctx context.Context, debounce time.Duration, onReload func(old, cur *Config)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(h.path)
	if err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Add(filepath.Dir(abs)); err != nil {
		_ = w.Close()
		return err
	}
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		defer func() { _ = w.Close() }()
		defer signal.Stop(hup)
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-hup:
				timer.Reset(0)
			case ev := <-w.Events:
				if filepath.Base(ev.Name) == filepath.Base(abs) || filepath.Base(ev.Name) == "..data" {
					timer.Reset(debounce)
				}
			case err := <-w.Errors:
				slog.Warn("config watcher", "err", err)
			case <-timer.C:
				old := h.Get()
				if err := h.Reload(); err != nil {
					slog.Error("config reload failed; keeping previous config", "err", err)
					continue
				}
				slog.Info("config reloaded", "path", h.path)
				onReload(old, h.Get())
			}
		}
	}()
	return nil
}
