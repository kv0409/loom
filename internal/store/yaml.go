package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"
)

func ReadYAML(path string, out interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}

func WriteYAML(path string, data interface{}) error {
	out, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func ListYAMLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

func NextCounter(counterFile string) (int, error) {
	f, err := os.OpenFile(counterFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	fd := int(f.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return 0, err
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)

	data, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}
	n := 0
	if s := strings.TrimSpace(string(data)); s != "" {
		n, err = strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("invalid counter value in %s: %w", counterFile, err)
		}
	}
	n++
	if err := f.Truncate(0); err != nil {
		return 0, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return 0, err
	}
	if _, err := f.WriteString(strconv.Itoa(n)); err != nil {
		return 0, err
	}
	return n, nil
}
