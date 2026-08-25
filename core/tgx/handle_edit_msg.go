package tgx

import (
	"context"
	"strings"

	"qshqn/core/typex"
	"qshqn/core/util/tgutil"

	"github.com/gotd/td/tg"
)

func (m *Manager) handleEditMsg(_ context.Context, _ tg.Entities, msgClass tg.MessageClass) error {
	msg, ok := msgClass.(*tg.Message)
	if !ok || msg == nil {
		return nil
	}

	peerID := tgutil.GetPeerID(msg.PeerID)
	key := typex.MsgKey{PeerID: peerID, MsgID: msg.ID}

	if cached, found := m.otherMsgCache.Get(key); found {
		cached.Text = strings.Clone(msg.Message)
		m.otherMsgCache.Add(key, cached)
	} else if cached, found := m.ownMsgCache.Get(key); found {
		cached.Text = strings.Clone(msg.Message)
		m.ownMsgCache.Add(key, cached)
	}

	return nil
}
