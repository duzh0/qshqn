//go:build !embed

package locale

import (
	"qshqn/core/fiox"
)

func loadAssetMsgMap(path string) (MsgMap, error) {
	return fiox.Load[MsgMap](path, fiox.NoReadCache, fiox.NoSetCache)
}

func loadAssetString(path string) (string, error) {
	return fiox.Load[string](path, fiox.ReadCache, fiox.SetCache)
}
