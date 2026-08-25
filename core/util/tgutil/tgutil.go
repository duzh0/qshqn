package tgutil

import (
	"github.com/gotd/td/tg"
)

const (
	CHANNEL_OFFSET int64 = -1000000000000
)

func GetInputPeer(peer tg.PeerClass, entities tg.Entities) tg.InputPeerClass {
	switch p := peer.(type) {
	case *tg.PeerUser:
		if user, ok := entities.Users[p.UserID]; ok {
			return user.AsInputPeer()
		}
	case *tg.PeerChat:
		if chat, ok := entities.Chats[p.ChatID]; ok {
			return chat.AsInputPeer()
		}
	case *tg.PeerChannel:
		if channel, ok := entities.Channels[p.ChannelID]; ok {
			return channel.AsInputPeer()
		}
	}

	return nil
}

func GetUserName(user *tg.User) string {
	if user.LastName == "" {
		return user.FirstName
	}
	return user.FirstName + " " + user.LastName
}

func GetFullName(peer tg.PeerClass, entities tg.Entities) string {
	switch p := peer.(type) {
	case *tg.PeerUser:
		if user, ok := entities.Users[p.UserID]; ok {
			return GetUserName(user)
		}
	case *tg.PeerChat:
		if chat, ok := entities.Chats[p.ChatID]; ok {
			return chat.Title
		}
	case *tg.PeerChannel:
		if channel, ok := entities.Channels[p.ChannelID]; ok {
			return channel.Title
		}
	}

	return ""
}

func GetPeerID(peer tg.PeerClass) int64 {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return p.UserID
	case *tg.PeerChat:
		return -p.ChatID
	case *tg.PeerChannel:
		return CHANNEL_OFFSET - p.ChannelID
	default:
		return 0
	}
}

func GetInputPeerID(inputPeer tg.InputPeerClass) int64 {
	switch p := inputPeer.(type) {
	case *tg.InputPeerUser:
		return p.UserID
	case *tg.InputPeerChat:
		return -p.ChatID
	case *tg.InputPeerChannel:
		return CHANNEL_OFFSET - p.ChannelID
	default:
		return 0
	}
}

func GetMessageID(upd tg.UpdatesClass) int {
	if upd == nil {
		return 0
	}

	if msgs := ExtractMessages(upd); len(msgs) > 0 {
		return msgs[0].ID
	}

	switch u := upd.(type) {
	case *tg.UpdateShortMessage:
		return u.ID
	case *tg.UpdateShortChatMessage:
		return u.ID
	case *tg.UpdateShortSentMessage:
		return u.ID
	}

	return 0
}

func ExtractMessages(upd tg.UpdatesClass) []*tg.Message {
	updates, ok := upd.(*tg.Updates)
	if !ok {
		return nil
	}

	var msgs []*tg.Message
	for _, u := range updates.Updates {
		switch inner := u.(type) {
		case *tg.UpdateNewMessage:
			if m, ok := inner.Message.(*tg.Message); ok {
				msgs = append(msgs, m)
			}
		case *tg.UpdateNewChannelMessage:
			if m, ok := inner.Message.(*tg.Message); ok {
				msgs = append(msgs, m)
			}
		}
	}
	return msgs
}
