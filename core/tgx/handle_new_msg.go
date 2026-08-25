package tgx

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"qshqn/core/command"
	"qshqn/core/config"
	"qshqn/core/db"
	"qshqn/core/locale"
	"qshqn/core/qsh"
	"qshqn/core/typex"
	"qshqn/core/util/tgutil"

	"github.com/gotd/td/tg"
)

func (m *Manager) handleNewMsg(ctx context.Context, ent tg.Entities, msgClass tg.MessageClass) error {
	var msg *tg.Message
	var svc *tg.MessageService
	var peer, from tg.PeerClass // chat/sender entity
	var mID int

	switch inner := msgClass.(type) {
	case *tg.Message:
		msg = inner
		peer = inner.PeerID
		from = inner.FromID
		mID = inner.ID
	case *tg.MessageService:
		if inner.Out {
			return nil
		}
		svc = inner
		peer = inner.PeerID
		from = inner.FromID
		mID = inner.ID
		msg = &tg.Message{ID: inner.ID, PeerID: inner.PeerID, FromID: inner.FromID}
	default:
		return nil
	}

	peerID := tgutil.GetPeerID(peer)
	if peerID == 0 {
		return fmt.Errorf("failed to extract ID for peer[%v]", peer)
	}
	inputPeer := tgutil.GetInputPeer(peer, ent)
	if inputPeer == nil {
		return fmt.Errorf("failed to get inputPeer from [%s]. entities: %v", peer.TypeName(), ent)
	}

	isFwd, fwdFromID, hasViews := extractFwdAndViews(msg)
	if msg != nil && msg.Out {
		repliedMsg, _ := m.getRepliedMsg(ctx, msg.ReplyTo, peerID, inputPeer, false)
		m.ownMsgCache.Add(msgCacheKV(peerID, mID, m.self.ID, m.self.FullName, msg.Message, isFwd, fwdFromID, hasViews, repliedMsg))
		return nil
	}

	if from == nil {
		from = peer
	}

	isPrivate := false
	if _, ok := peer.(*tg.PeerUser); ok {
		isPrivate = true
		from = peer
	}

	fromID := tgutil.GetPeerID(from)
	if fromID == 0 {
		return fmt.Errorf("failed to extract ID from[%v]", from)
	}

	if isPrivate && fromID != config.Tg.OwnerID {
		qsh.Debugf("ignoring private message from non-owner user ID[%d]", fromID)
		return nil
	}

	dbUser, err := db.LoadOrDefault[db.User](fromID)
	if err != nil {
		return fmt.Errorf("error loading or defaulting user: %w", err)
	}

	if u, ok := ent.Users[fromID]; ok {
		if u.AccessHash != 0 && dbUser.AH != u.AccessHash {
			dbUser.AH = u.AccessHash
			if err := dbUser.Save(); err != nil {
				qsh.Errorf("failed to save updated AH for user[%d]: %v", fromID, err)
			}
		}
	}

	var text string
	var repliedMsg *typex.CachedMsg
	var isReplyToSelf bool

	if msg != nil {
		text = msg.Message
		repliedMsg, isReplyToSelf = m.getRepliedMsg(ctx, msg.ReplyTo, peerID, inputPeer, false)
	} else {
		text = ""
	}
	fromName := tgutil.GetFullName(from, ent)

	m.otherMsgCache.Add(msgCacheKV(peerID, mID, fromID, fromName, text, isFwd, fwdFromID, hasViews, repliedMsg))

	if qsh.IsDebug() {
		peerName := tgutil.GetFullName(peer, ent)
		qsh.Debugf("msg in [%s][%d] by [%s][%d]: %s", peerName, peerID, fromName, fromID, text)
		qsh.Debugf("len(otherMsgCache)=%d", m.otherMsgCache.Len())
	}

	var trigger, payload string
	if idx := strings.IndexFunc(text, unicode.IsSpace); idx == -1 {
		trigger = text
		payload = ""
	} else {
		trigger = text[:idx]
		payload = text[idx+1:]
	}

	trigger = strings.ToLower(trigger)
	isTrigger := locale.IsTrigger(trigger)
	if !isTrigger {
		payload = text
		trigger = ""
	}
	shouldProcess := (isTrigger || isReplyToSelf) && !isFwd

	replyToID := 0
	if msg != nil {
		if header, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok {
			replyToID = header.ReplyToMsgID
		}
	}

	hasMedia, mediaType, mediaID, mediaAH, fileIDStr := extractMediaInfo(msg)
	qsh.Debugf("msg[%d] isFwd[%t] replyTo[%d] media[has=%t, type=%s, id=%d, ah=%d, file_id=%s] trigger[is=%t, \"%s\"] reply[fetched=%t, isSelf=%t] payload[%q] shouldProcess[%t]",
		mID, isFwd, replyToID, hasMedia, mediaType, mediaID, mediaAH, fileIDStr, isTrigger, trigger, repliedMsg != nil, isReplyToSelf, payload, shouldProcess)

	var linkedChatID int64
	if !isPrivate {
		whitelistedChatData, ok := m.whitelistedChats.Get(peerID)
		if !ok {
			qsh.Warnf("msg in a non-whitelisted chat [%s] ID[%d], notifying and leaving", tgutil.GetFullName(peer, ent), peerID)
			ctxFrom := typex.From{ID: fromID, FullName: fromName, Obj: ent}
			cmdCtx := command.GetContext(ctx, m.api, m.sender, false, ctxFrom, m.self, dbUser, inputPeer, peerID, msg, payload, repliedMsg)
			defer cmdCtx.Put()
			return m.NotifyAndLeaveChat(cmdCtx)
		}
		if !whitelistedChatData.LinkedChannel.Checked {
			if channel, ok := inputPeer.(*tg.InputPeerChannel); ok {
				full, err := m.api.ChannelsGetFullChannel(ctx, &tg.InputChannel{
					ChannelID:  channel.ChannelID,
					AccessHash: channel.AccessHash,
				})
				whitelistedChatData.LinkedChannel.Checked = true
				if err == nil {
					if chFull, ok := full.FullChat.(*tg.ChannelFull); ok && chFull.LinkedChatID != 0 {
						whitelistedChatData.LinkedChannel.ID = tgutil.CHANNEL_OFFSET - chFull.LinkedChatID
					}
				}
			}
		}
		linkedChatID = whitelistedChatData.LinkedChannel.ID
	}

	var isComment bool
	if msg != nil && msg.ReplyTo != nil && linkedChatID != 0 {
		if repliedMsg != nil && (repliedMsg.FromID == linkedChatID || repliedMsg.FwdFromID == linkedChatID) && repliedMsg.HasViews {
			isComment = true
		}
	}

	qsh.Debugf("isComment[%t]", isComment)

	ctxFrom := typex.From{
		ID:       fromID,
		FullName: tgutil.GetFullName(from, ent),
		Obj:      ent,
	}
	cmdCtx := command.GetContext(
		ctx,
		m.api,
		m.sender,
		isComment,
		ctxFrom,
		m.self,
		dbUser,
		inputPeer,
		peerID,
		msg,
		payload,
		repliedMsg,
	)
	defer cmdCtx.Put()

	if svc != nil {
		if action, ok := svc.Action.(*tg.MessageActionChatAddUser); ok {
			helpKw := "help"
			if helpCmd, ok := command.ByID(locale.RootIDHelp); ok {
				kws := helpCmd.Keywords(dbUser.LangCode)
				if len(kws) > 0 {
					helpKw = kws[0]
				}
			}
			if slices.Contains(action.Users, m.self.ID) {
				qsh.Infof("bot added to a whitelisted chat [%s] ID[%d], greeting", tgutil.GetFullName(peer, ent), peerID)
				_, err := cmdCtx.ReplyReportErrLocaleMsgf(locale.GlobalIDs.WhitelistWelcomeMsg,
					locale.KV("trigger", dbUser.LangCode.Lang().PreferredTrigger),
					locale.KV("help", helpKw),
					locale.KV("support_link", config.Tg.SupportLink),
				)
				return err
			} else {
				_, err := cmdCtx.ReplyReportErrLocaleMsgf(locale.GlobalIDs.WhitelistUserJoinedMsg,
					locale.KV("user_name", tgutil.GetFullName(from, ent)),
					locale.KV("chat_name", tgutil.GetFullName(peer, ent)),
					locale.KV("trigger", dbUser.LangCode.Lang().PreferredTrigger),
					locale.KV("help", helpKw),
				)
				return err
			}
		}
		return nil
	}

	if msg == nil {
		return nil
	}

	if dbUser.Banned {
		return nil
	}

	if !shouldProcess {
		if command.PreDispatch(cmdCtx) {
			return nil
		}
		return nil
	}

	_, err = command.Dispatch(cmdCtx)
	if err != nil {
		return fmt.Errorf("command dispatch error: %w", err)
	}

	return nil
}
