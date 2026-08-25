package typex

import (
	"github.com/gotd/td/tg"
	lru "github.com/hashicorp/golang-lru/v2"
)

type From struct {
	ID       int64
	FullName string
	Obj      tg.Entities
}

type BotInfo struct {
	ID       int64
	FullName string
	Username string
}

type MsgKey struct {
	PeerID int64
	MsgID  int
}

type CachedMsg struct {
	ID        int
	IsFwd     bool
	FwdFromID int64
	HasViews  bool
	FromID    int64
	FromName  string
	Text      string
	RepliedTo *CachedMsg
}

type MsgCache = lru.Cache[MsgKey, CachedMsg]

func NewMsgCache(size int) (*MsgCache, error) { return lru.New[MsgKey, CachedMsg](size) }
