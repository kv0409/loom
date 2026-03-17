package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/karanagi/loom/internal/store"
	"gopkg.in/yaml.v3"
)

type Lock struct {
	File       string    `yaml:"file"`
	Agent      string    `yaml:"agent"`
	AcquiredAt time.Time `yaml:"acquired_at"`
	Issue      string    `yaml:"issue"`
}

type AcquireOpts struct {
	File   string
	Agent  string
	Issue  string
}

type ReleaseOpts struct {
	File string
}

type CheckOpts struct {
	File string
}

func lockPath(loomRoot, file string) string {
	encoded := strings.ReplaceAll(file, "/", "__")
	return filepath.Join(loomRoot, "locks", encoded+".lock.yaml")
}

func Acquire(loomRoot string, opts AcquireOpts) error {
	path := lockPath(loomRoot, opts.File)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	l := &Lock{
		File:       opts.File,
		Agent:      opts.Agent,
		AcquiredAt: time.Now(),
		Issue:      opts.Issue,
	}
	data, err := yaml.Marshal(l)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			existing, readErr := Check(loomRoot, opts.File)
			if readErr != nil {
				return readErr
			}
			if existing != nil {
				return fmt.Errorf("LOCKED by %s (%s) since %s", existing.Agent, existing.Issue, existing.AcquiredAt.Format("15:04:05"))
			}
		}
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func Release(loomRoot string, file string) error {
	path := lockPath(loomRoot, file)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s is not locked", file)
		}
		return err
	}
	return nil
}

func Check(loomRoot string, file string) (*Lock, error) {
	path := lockPath(loomRoot, file)
	var l Lock
	if err := store.ReadYAML(path, &l); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &l, nil
}

func ListLocks(loomRoot string) ([]*Lock, error) {
	dir := filepath.Join(loomRoot, "locks")
	files, err := store.ListYAMLFiles(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var locks []*Lock
	for _, f := range files {
		var l Lock
		if err := store.ReadYAML(f, &l); err != nil {
			continue
		}
		locks = append(locks, &l)
	}
	return locks, nil
}
