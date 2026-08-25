package tgx

import (
	"context"
	"strings"

	"qshqn/core/qsh"
	"qshqn/core/typex"
	"qshqn/core/util/tgutil"

	"github.com/gotd/td/fileid"
	"github.com/gotd/td/tg"
)

var (
	CachedMsgNotFetched = &typex.CachedMsg{}
)

func newCachedMsg(ID int, fromID int64, fromName, text string, isFwd bool, fwdFromID int64, hasViews bool, repliedTo *typex.CachedMsg) typex.CachedMsg {
	return typex.CachedMsg{
		ID:        ID,
		FromID:    fromID,
		FromName:  fromName,
		Text:      strings.Clone(text),
		IsFwd:     isFwd,
		FwdFromID: fwdFromID,
		HasViews:  hasViews,
		RepliedTo: repliedTo,
	}
}

func msgCacheKV(peerID int64, msgID int, fromID int64, fromName, text string, isFwd bool, fwdFromID int64, hasViews bool, repliedTo *typex.CachedMsg) (typex.MsgKey, typex.CachedMsg) {
	key := typex.MsgKey{PeerID: peerID, MsgID: msgID}
	cached := newCachedMsg(msgID, fromID, fromName, text, isFwd, fwdFromID, hasViews, repliedTo)
	return key, cached
}

func extractFwdAndViews(msg *tg.Message) (isFwd bool, fwdFromID int64, hasViews bool) {
	if msg == nil {
		return false, 0, false
	}
	hasViews = msg.Views > 0
	if !msg.FwdFrom.Zero() {
		isFwd = true
		if msg.FwdFrom.FromID != nil {
			fwdFromID = tgutil.GetPeerID(msg.FwdFrom.FromID)
		}
	}
	return isFwd, fwdFromID, hasViews
}

func isMsgFwd(msg *tg.Message) bool {
	return msg != nil && !msg.FwdFrom.Zero()
}

func (m *Manager) getRepliedMsg(ctx context.Context, replyTo tg.MessageReplyHeaderClass, peerID int64, inputPeer tg.InputPeerClass, skipCache bool) (*typex.CachedMsg, bool) {
	if replyTo == nil {
		return nil, false
	}
	header, ok := replyTo.(*tg.MessageReplyHeader)
	if !ok {
		return nil, false
	}
	replyID := header.ReplyToMsgID
	replyPeerID := peerID
	if header.ReplyToPeerID != nil {
		if pid := tgutil.GetPeerID(header.ReplyToPeerID); pid != 0 {
			replyPeerID = pid
		}
	}
	if !skipCache {
		key := typex.MsgKey{PeerID: replyPeerID, MsgID: replyID}
		if cached, found := m.ownMsgCache.Get(key); found {
			return &cached, true
		}
		if cached, found := m.otherMsgCache.Get(key); found {
			return &cached, false
		}
	}
	var response tg.MessagesMessagesClass
	var err error
	if channel, ok := inputPeer.(*tg.InputPeerChannel); ok {
		response, err = m.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{
				ChannelID:  channel.ChannelID,
				AccessHash: channel.AccessHash,
			},
			ID: []tg.InputMessageClass{&tg.InputMessageID{ID: replyID}},
		})
	} else {
		response, err = m.api.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: replyID}})
	}
	if err != nil {
		qsh.Debugf("failed to fetch replied msg via API peer[%d] msg[%d]: %v", replyPeerID, replyID, err)
		return nil, false
	}
	var msgs []tg.MessageClass
	var users []tg.UserClass
	var chats []tg.ChatClass
	switch resp := response.(type) {
	case *tg.MessagesMessages:
		msgs = resp.Messages
		users = resp.Users
		chats = resp.Chats
	case *tg.MessagesMessagesSlice:
		msgs = resp.Messages
		users = resp.Users
		chats = resp.Chats
	case *tg.MessagesChannelMessages:
		msgs = resp.Messages
		users = resp.Users
		chats = resp.Chats
	default:
		return nil, false
	}
	if len(msgs) == 0 {
		return nil, false
	}
	msg, ok := msgs[0].(*tg.Message)
	if !ok {
		return nil, false
	}
	fromID := tgutil.GetPeerID(msg.FromID)
	if fromID == 0 {
		fromID = peerID
	}
	var fromName string
	for _, u := range users {
		if user, ok := u.(*tg.User); ok && user.ID == fromID {
			fromName = tgutil.GetUserName(user)
			break
		}
	}
	if fromName == "" {
		for _, c := range chats {
			switch ch := c.(type) {
			case *tg.Chat:
				if -ch.ID == fromID {
					fromName = ch.Title
				}
			case *tg.Channel:
				if (tgutil.CHANNEL_OFFSET - ch.ID) == fromID {
					fromName = ch.Title
				}
			}
		}
	}
	if fromName == "" {
		fromName = "Author Name Unknown"
	}
	isSelf := (fromID == m.self.ID) || msg.Out
	var parentReply *typex.CachedMsg
	if msg.ReplyTo != nil {
		parentReply = CachedMsgNotFetched
	}
	isFwd, fwdFromID, hasViews := extractFwdAndViews(msg)

	cached := newCachedMsg(msg.ID, fromID, fromName, msg.Message, isFwd, fwdFromID, hasViews, parentReply)

	key := typex.MsgKey{PeerID: replyPeerID, MsgID: replyID}
	if isSelf {
		m.ownMsgCache.Add(key, cached)
		return &cached, true
	}
	m.otherMsgCache.Add(key, cached)
	return &cached, false
}

func extractMediaInfo(msg *tg.Message) (hasMedia bool, mediaType string, docID int64, accessHash int64, fileIDStr string) {
	if msg == nil {
		return false, "none", 0, 0, ""
	}
	media := msg.Media
	if media == nil {
		return false, "none", 0, 0, ""
	}
	switch m := media.(type) {
	case *tg.MessageMediaPhoto:
		if photo, ok := m.Photo.(*tg.Photo); ok {
			fid := fileid.FromPhoto(photo, 'm')
			enc, _ := fileid.EncodeFileID(fid)
			return true, "photo", photo.ID, photo.AccessHash, enc
		}
		return true, "photo_empty", 0, 0, ""
	case *tg.MessageMediaDocument:
		doc, ok := m.Document.(*tg.Document)
		if !ok {
			return true, "document_empty", 0, 0, ""
		}
		mType := "document"
		for _, attr := range doc.Attributes {
			switch a := attr.(type) {
			case *tg.DocumentAttributeAudio:
				if a.Voice {
					mType = "voice"
				} else {
					mType = "audio"
				}
			case *tg.DocumentAttributeVideo:
				if a.RoundMessage {
					mType = "round_video"
				} else {
					mType = "video"
				}
			case *tg.DocumentAttributeSticker:
				mType = "sticker"
			case *tg.DocumentAttributeAnimated:
				mType = "gif"
			}
		}
		fid := fileid.FromDocument(doc)
		enc, _ := fileid.EncodeFileID(fid)
		return true, mType, doc.ID, doc.AccessHash, enc
	case *tg.MessageMediaWebPage:
		return true, "webpage", 0, 0, ""
	case *tg.MessageMediaDice:
		return true, "dice", 0, 0, ""
	case *tg.MessageMediaGeo:
		return true, "geo", 0, 0, ""
	case *tg.MessageMediaContact:
		return true, "contact", 0, 0, ""
	default:
		return true, "unknown", 0, 0, ""
	}
}

func safeHandle(f func() error) {
	defer func() {
		if r := recover(); r != nil {
			qsh.Errorf("PANIC recovered in handler: %v", r)
		}
	}()
	if err := f(); err != nil {
		qsh.Errorf("handler error: %w", err)
	}
}
