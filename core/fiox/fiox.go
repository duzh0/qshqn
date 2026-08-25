package fiox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"path/filepath"

	"qshqn/core/typex"
)

const (
	CreateOnly     SaveMode = iota // error if file exists
	UpdateOnly                     // error if file does not exist
	CreateOrUpdate                 // creates or updates

	ReadCache   ReadCacheOpt = true  // reads from cache; if not found, reads from disk
	NoReadCache ReadCacheOpt = false // reads from disk directly

	SetCache   SetCacheOpt = true  // caches data at key=path
	NoSetCache SetCacheOpt = false // does not cache data
)

var (
	cache      = typex.NewMap[string, any](0)
	directives = map[string]Directive{}
)

type SaveMode uint8
type SetCacheOpt bool
type ReadCacheOpt bool

func GetDirective(path string) (Directive, bool) {
	d, ok := directives[filepath.Ext(path)]
	return d, ok
}

func CacheDelete(path string) {
	cache.Delete(filepath.Clean(path))
}

func SafeWrite(path string, data any, writeFunc WriteFunc) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "tmp-*")
	if err != nil {
		return fmt.Errorf("error creating temp file: %w", err)
	}

	tmpPath := f.Name()
	defer os.Remove(tmpPath)

	if err := writeFunc(f, data); err != nil {
		f.Close()
		return fmt.Errorf("write func error: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("error closing temp file [%s]: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("error renaming temp file from [%s] to [%s]: %w", tmpPath, path, err)
	}

	return nil
}

func Save(path string, data any, mode SaveMode, setCache SetCacheOpt) error {
	path = filepath.Clean(path)
	switch mode {
	case UpdateOnly:
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("file at [%s] does not exist", path)
			}
			return fmt.Errorf("file info at [%s] error: %w", path, err)
		}
	case CreateOnly:
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("file at [%s] already exists", path)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("file info at [%s] error: %w", path, err)
		}
	}

	d, ok := GetDirective(path)
	if !ok {
		return fmt.Errorf("no directive to save to file at [%s]", path)
	}

	if err := SafeWrite(path, data, d.Write); err != nil {
		return fmt.Errorf("safe write error: %w", err)
	}

	if setCache {
		cache.Set(path, data)
	}

	return nil
}

func Load[T any](path string, readCache ReadCacheOpt, setCache SetCacheOpt) (T, error) {
	var v T
	path = filepath.Clean(path)
	if readCache {
		if cached, found := cache.Get(path); found {
			if data, ok := cached.(T); ok {
				return data, nil
			} else {
				return v, fmt.Errorf("type mismatch in cache for file at [%s]: want [%T], cached [%T]", path, v, data)
			}
		}
	}

	d, ok := GetDirective(path)
	if !ok {
		return v, fmt.Errorf("no directive to load file at [%s]", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return v, fmt.Errorf("error opening file: %w", err)
	}

	defer f.Close()

	if err := d.Read(f, &v); err != nil {
		return v, fmt.Errorf("load file error at [%s]: %w", path, err)
	}

	if setCache {
		cache.Set(path, v)
	}

	return v, nil
}

func IsAccessible(path string) bool {
	_, err := os.Stat(filepath.Clean(path))
	return err == nil
}
