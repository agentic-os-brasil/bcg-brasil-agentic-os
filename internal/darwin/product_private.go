package darwin

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// productPrivateFiles is operation-local custody, never persisted authority.
// A nil owner preserves the original Unix private-file implementation.
type productPrivateFiles struct {
	root        string
	directories map[string]*scheduler.HeldDirectory
	order       []*scheduler.HeldDirectory
}

func (p *productPrivateFiles) close() error {
	var result error
	for i := len(p.order) - 1; i >= 0; i-- {
		result = errors.Join(result, p.order[i].Close())
	}
	return result
}

func (p *productPrivateFiles) parent(path string) (*scheduler.HeldDirectory, string, error) {
	root := p.directories[p.root]
	if _, err := root.Relative(path); err != nil {
		return nil, "", err
	}
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		if directory := p.directories[parent]; directory != nil {
			relative, err := directory.Relative(path)
			return directory, relative, err
		}
		if parent == p.root || parent == filepath.Dir(parent) {
			return nil, "", errors.New("Darwin private parent is not retained")
		}
	}
}

// prepare retains each named parent through the complete journal, artifact,
// evidence, pointer and readback transaction. It runs only under a live grant.
func (p *productPrivateFiles) prepare(job string) error {
	if p == nil {
		return nil
	}
	if !maintenance.DarwinJob(job) {
		return errors.New("unknown Darwin publication job")
	}
	type parent struct {
		name   string
		create bool
	}
	names := []parent{{"managed-state", true}, {"proposals", job != maintenance.DarwinDailyJobID}, {"reports", true}}
	for _, id := range []string{maintenance.DarwinDailyJobID, maintenance.DarwinWeeklyJobID, maintenance.DarwinMonthlyJobID} {
		names = append(names, parent{filepath.Join("reports", id), id == job}, parent{filepath.Join("reports", id, "versions"), id == job})
	}
	for _, entry := range names {
		path := filepath.Join(p.root, entry.name)
		if p.directories[path] != nil {
			continue
		}
		directory, relative, err := p.parent(path)
		if err != nil {
			return err
		}
		if entry.create {
			if err = directory.EnsureDirectory(relative); err != nil {
				return err
			}
		}
		held, err := directory.HoldDirectory(relative)
		if !entry.create && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		p.directories[path] = held
		p.order = append(p.order, held)
	}
	return nil
}

func (p *productPrivateFiles) read(path string, maximum int64) ([]byte, error) {
	if p == nil {
		return scheduler.ReadPrivateStateFile(path, maximum)
	}
	directory, name, err := p.parent(path)
	if err != nil {
		return nil, err
	}
	body, err := directory.ReadPrivateFile(name, maximum)
	if err == nil && len(body) == 0 {
		return nil, errors.New("private state file is empty")
	}
	return body, err
}

func (p *productPrivateFiles) ensureDirectory(path string) error {
	if p == nil {
		return scheduler.EnsurePrivateDirectory(path)
	}
	if p.directories[path] == nil {
		return errors.New("Darwin publication directory is not retained")
	}
	return nil
}

func (p *productPrivateFiles) publish(path string, body []byte) error {
	if p == nil {
		return scheduler.PublishNewPrivateFile(path, body)
	}
	directory, name, err := p.parent(path)
	if err != nil {
		return err
	}
	return directory.PublishNewFile(name, body)
}

func (p *productPrivateFiles) update(path string, maximum int64, guard func(func() error) error, update func([]byte) ([]byte, error)) error {
	if p == nil {
		return scheduler.UpdatePrivateFileGuarded(path, maximum, guard, update)
	}
	directory, name, err := p.parent(path)
	if err != nil {
		return err
	}
	return directory.UpdatePrivateFileGuarded(name, maximum, guard, update)
}

func (p *productPrivateFiles) withLock(path string, action func() error) error {
	if p == nil {
		return scheduler.WithPrivateLock(path, action)
	}
	directory, name, err := p.parent(path)
	if err != nil {
		return err
	}
	return directory.WithPrivateLock(name, action)
}
