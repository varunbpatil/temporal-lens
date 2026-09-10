package service

import (
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	progressFormatVersion = 1
	progressFileMode      = 0o600
)

// terminalProgress records successfully indexed terminal executions for one
// index generation. It is optional: a nil tracker preserves full reindexing.
type terminalProgress struct {
	path       string
	generation string

	mu        sync.Mutex
	completed map[string]struct{}
	pending   map[string]struct{}
	dirty     bool
}

type progressFile struct {
	FormatVersion int
	Generation    string
	Completed     map[string]struct{}
}

func newTerminalProgress(path, generation string) (*terminalProgress, error) {
	progress := &terminalProgress{
		path: path, generation: generation, completed: make(map[string]struct{}), pending: make(map[string]struct{}),
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return progress, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var persisted progressFile
	if decodeErr := gob.NewDecoder(file).Decode(&persisted); decodeErr != nil {
		return nil, fmt.Errorf("decode %q: %w", path, decodeErr)
	}
	if persisted.FormatVersion != progressFormatVersion || persisted.Generation != generation {
		return progress, nil
	}
	progress.completed = persisted.Completed
	if progress.completed == nil {
		progress.completed = make(map[string]struct{})
	}
	return progress, nil
}

func (p *terminalProgress) reserve(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.completed[id]; exists {
		return false
	}
	if _, exists := p.pending[id]; exists {
		return false
	}
	p.pending[id] = struct{}{}
	return true
}

func (p *terminalProgress) release(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pending, id)
}

func (p *terminalProgress) complete(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pending, id)
	p.completed[id] = struct{}{}
	p.dirty = true
}

func (p *terminalProgress) save() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.dirty {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(p.path), filepath.Base(p.path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if chmodErr := temporary.Chmod(progressFileMode); chmodErr != nil {
		return errors.Join(chmodErr, temporary.Close())
	}
	persisted := progressFile{FormatVersion: progressFormatVersion, Generation: p.generation, Completed: p.completed}
	if encodeErr := gob.NewEncoder(temporary).Encode(persisted); encodeErr != nil {
		return errors.Join(encodeErr, temporary.Close())
	}
	if closeErr := temporary.Close(); closeErr != nil {
		return closeErr
	}
	if renameErr := os.Rename(temporaryPath, p.path); renameErr != nil {
		return renameErr
	}
	p.dirty = false
	return nil
}
