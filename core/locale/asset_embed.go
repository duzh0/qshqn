//go:build embed

package locale

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"qshqn/data"
)

var (
	embedStringCache = make(map[string]string)
	embedCacheMu     sync.RWMutex
)

func loadAssetMsgMap(path string) (MsgMap, error) {
	embedPath := filepath.ToSlash(filepath.Clean(path))
	embedPath = strings.TrimPrefix(embedPath, "data/")

	bytes, err := data.Locales.ReadFile(embedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded locale file [%s]: %w", embedPath, err)
	}

	var m MsgMap
	if err := json.Unmarshal(bytes, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal embedded locale file [%s]: %w", embedPath, err)
	}
	return m, nil
}

func loadAssetString(path string) (string, error) {
	embedPath := filepath.ToSlash(filepath.Clean(path))
	embedPath = strings.TrimPrefix(embedPath, "data/")

	embedCacheMu.RLock()
	cached, found := embedStringCache[embedPath]
	embedCacheMu.RUnlock()
	if found {
		return cached, nil
	}

	bytes, err := data.Locales.ReadFile(embedPath)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded text file [%s]: %w", embedPath, err)
	}

	content := string(bytes)

	embedCacheMu.Lock()
	embedStringCache[embedPath] = content
	embedCacheMu.Unlock()

	return content, nil
}
