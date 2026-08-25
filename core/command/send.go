package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/tg"

	"qshqn/core/callback"
	"qshqn/core/db"
	"qshqn/core/locale"
	"qshqn/core/qsh"
	"qshqn/core/typex"
	"qshqn/core/util/tgutil"
)

func init() {
	type SendCallbackParams struct {
		callback.IssuerArgs
		ReceiverID  int64
		Amount      int64
		ConfirmStep uint8
	}

	type msgKey struct {
		peerID int64
		msgID  int
	}

	type pendingState struct {
		expiresAt time.Time
		cancel    context.CancelFunc
	}

	const confirmDuration = 60 * time.Second

	ID := "send"
	otherMsgs := struct {
		NotPositiveInteger,
		CantSendToChatOrChannel,
		CantSendToSelfOrBot,
		UserNotInDb,
		NotEnoughDuzhocoins,
		UnknownReceiver,
		ConfirmPrompt,
		ConfirmSecondTimePrompt,
		Success,
		SenderNotification,
		ReceiverNotification,
		Cancelled locale.MsgID
	}{}

	pendingConfirmations := typex.NewMap[msgKey, pendingState](0)

	getName := func(ctx context.Context, api *tg.Client, ent tg.Entities, receiverID int64) string {
		if u, ok := ent.Users[receiverID]; ok {
			return tgutil.GetUserName(u)
		}

		dbUser, err := db.Load[db.User](receiverID)
		if err == nil && dbUser.AH != 0 {
			users, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUser{
				UserID:     receiverID,
				AccessHash: dbUser.AH,
			}})
			if err == nil && len(users) > 0 {
				if u, ok := users[0].(*tg.User); ok {
					return tgutil.GetUserName(u)
				}
			}
		}

		return fmt.Sprintf("!NAME_MISSING[ID%d]!", receiverID)
	}

	formatConfirmPrompt := func(ctx context.Context, api *tg.Client, ent tg.Entities, langCode locale.LangCode, senderName string, senderID, receiverID, amount int64, seconds int) string {
		receiverName := getName(ctx, api, ent, receiverID)
		duzhocoinsNoun := locale.GlobalIDs.Nouns.Duzhocoins.ConstructSafe(langCode, locale.CaseNom, int(amount))
		duzhocoinsText := locale.Msg(langCode, duzhocoinsNoun)

		return locale.Msgf(langCode, otherMsgs.ConfirmPrompt,
			locale.KV("sender_name", senderName),
			locale.KV("sender_id", senderID),
			locale.KV("receiver_name", receiverName),
			locale.KV("receiver_id", receiverID),
			locale.KV("amount", amount),
			locale.KV("duzhocoins", duzhocoinsText),
			locale.KV("seconds", seconds),
		)
	}

	exec := func(ctx *Context) (bool, error) {
		if len(ctx.Args) < 2 || len(ctx.Args) > 3 {
			_, err := ctx.ReplyReportErr(ctx.Command.Usage(ctx.DBUser.LangCode))
			return false, err
		}

		amount, err := strconv.ParseInt(ctx.Args[1], 10, 64)
		if err != nil || amount < 1 {
			if !ctx.Strict {
				return true, nil
			}
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.NotPositiveInteger)
			return false, err
		}

		if amount > ctx.DBUser.Duzhocoins {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.NotEnoughDuzhocoins)
			return false, err
		}

		var receiverID int64
		var receiverAH int64
		if len(ctx.Args) == 3 {
			someIDBuf := ctx.Args[2]
			if parsedID, err := strconv.ParseInt(someIDBuf, 10, 64); err == nil {
				receiverID = parsedID
			} else {
				someIDBuf = strings.TrimRight(someIDBuf, "/")
				if idx := strings.LastIndexAny(someIDBuf, "/@"); idx != -1 {
					someIDBuf = someIDBuf[idx+1:]
				}

				resolvedPeer, err := ctx.Api.ContactsResolveUsername(ctx.Ctx, &tg.ContactsResolveUsernameRequest{
					Username: someIDBuf,
				})

				if err == nil {
					if peerUser, ok := resolvedPeer.Peer.(*tg.PeerUser); ok {
						receiverID = peerUser.UserID
						for _, u := range resolvedPeer.Users {
							if user, ok := u.(*tg.User); ok && user.ID == receiverID {
								receiverAH = user.AccessHash
								break
							}
						}
					} else {
						_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.CantSendToChatOrChannel)
						return false, err
					}
				} else if ctx.Message != nil && len(ctx.Message.Entities) > 0 {
					for _, ent := range ctx.Message.Entities {
						if m, ok := ent.(*tg.MessageEntityMentionName); ok {
							receiverID = m.UserID
							break
						}
					}
				}
			}
		} else if ctx.RepliedTo != nil {
			receiverID = ctx.RepliedTo.FromID
		}

		if receiverID == 0 {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.UnknownReceiver)
			return false, err
		}

		if receiverID == ctx.From.ID || receiverID == ctx.Bot.ID {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.CantSendToSelfOrBot)
			return false, err
		}

		if _, err := db.Exists[db.User](receiverID); err != nil {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.UserNotInDb)
			return false, err
		}

		if receiverAH != 0 {
			if dbUser, err := db.Load[db.User](receiverID); err == nil {
				if dbUser.AH != receiverAH {
					dbUser.AH = receiverAH
					_ = dbUser.Save()
				}
			}
		}

		params := &SendCallbackParams{
			IssuerArgs: callback.IssuerArgs{
				IssuerID: ctx.From.ID,
			},
			ReceiverID: receiverID,
			Amount:     amount,
		}

		confirmData, err := callback.Encode(callback.CmdIDConfirmSend, params)
		if err != nil {
			ctx.ReplyReportErrLocaleMsg(locale.GlobalIDs.Error)
			return false, fmt.Errorf("error encoding confirm: %w", err)
		}

		cancelData, err := callback.Encode(callback.CmdIDCancelSend, params)
		if err != nil {
			ctx.ReplyReportErrLocaleMsg(locale.GlobalIDs.Error)
			return false, fmt.Errorf("error encoding cancel: %w", err)
		}

		markup := &tg.ReplyInlineMarkup{
			Rows: []tg.KeyboardButtonRow{
				{
					Buttons: []tg.KeyboardButtonClass{
						&tg.KeyboardButtonCallback{Text: "✅", Data: confirmData},
						&tg.KeyboardButtonCallback{Text: "❌", Data: cancelData},
					},
				},
			},
		}

		promptText := formatConfirmPrompt(
			ctx.Ctx, ctx.Api, ctx.From.Obj, ctx.DBUser.LangCode,
			ctx.From.FullName, ctx.From.ID, receiverID, amount,
			int(confirmDuration.Seconds()),
		)

		result, err := ctx.Reply().Markup(markup).StyledText(ctx.Ctx, html.String(nil, promptText))

		if err == nil && result != nil && ctx.Message != nil {
			peer := tgutil.GetInputPeer(ctx.Message.PeerID, ctx.From.Obj)
			msgID := tgutil.GetMessageID(result)

			if msgID != 0 && peer != nil {
				key := msgKey{peerID: tgutil.GetInputPeerID(peer), msgID: msgID}

				ctxWait, cancelTimer := context.WithCancel(context.Background())

				pendingConfirmations.Set(key, pendingState{
					expiresAt: time.Now().Add(confirmDuration),
					cancel:    cancelTimer,
				})

				api := ctx.Api
				go func() {
					select {
					case <-time.After(confirmDuration):
						pendingConfirmations.Delete(key)
						deleteMsgAfter(api, peer, msgID, 15*time.Second)
					case <-ctxWait.Done():
						// button clicked
					}
				}()
			}
		}

		return false, err
	}

	confirmHandler := func(ctx *callback.TypedContext[*SendCallbackParams]) error {
		p := ctx.Params
		peer := tgutil.GetInputPeer(ctx.Query.Peer, ctx.From.Obj)
		msgID := ctx.Query.MsgID
		key := msgKey{peerID: tgutil.GetInputPeerID(peer), msgID: msgID}

		state, loaded := pendingConfirmations.Get(key)
		if !loaded {
			deleteMsgAfter(ctx.Api, peer, msgID, 15*time.Second)
			return nil
		}

		if p.ConfirmStep == 0 {
			state.expiresAt = time.Now().Add(confirmDuration)
			pendingConfirmations.Set(key, state)

			p2 := &SendCallbackParams{
				IssuerArgs: callback.IssuerArgs{
					IssuerID: p.IssuerID,
				},
				ReceiverID:  p.ReceiverID,
				Amount:      p.Amount,
				ConfirmStep: 1,
			}

			confirmData, err := callback.Encode(callback.CmdIDConfirmSend, p2)
			if err != nil {
				return fmt.Errorf("error encoding second confirm data: %w; params: %v", err, p2)
			}
			cancelData, err := callback.Encode(callback.CmdIDCancelSend, p2)
			if err != nil {
				return fmt.Errorf("error encoding second cancel data: %w; params: %v", err, p2)
			}

			markup := &tg.ReplyInlineMarkup{
				Rows: []tg.KeyboardButtonRow{
					{
						Buttons: []tg.KeyboardButtonClass{
							&tg.KeyboardButtonCallback{Text: "✅", Data: confirmData},
							&tg.KeyboardButtonCallback{Text: "❌", Data: cancelData},
						},
					},
				},
			}

			remainingSeconds := max(0, int(time.Until(state.expiresAt).Seconds()))
			originalPrompt := formatConfirmPrompt(
				ctx.Ctx, ctx.Api, ctx.From.Obj, ctx.DBUser.LangCode,
				ctx.From.FullName, p.IssuerID, p.ReceiverID, p.Amount,
				remainingSeconds,
			)
			warningText := originalPrompt + "\n\n" + locale.Msg(ctx.DBUser.LangCode, otherMsgs.ConfirmSecondTimePrompt)

			_, err = ctx.Sender.To(peer).Markup(markup).Edit(msgID).StyledText(ctx.Ctx, html.String(nil, warningText))
			return err
		}

		pendingConfirmations.Delete(key)
		state.cancel()

		if time.Now().After(state.expiresAt) {
			deleteMsgAfter(ctx.Api, peer, msgID, 15*time.Second)
			return nil
		}

		duzhocoins := ctx.DBUser.Duzhocoins
		if duzhocoins < p.Amount {
			return editAndScheduleRemove(ctx.Api, ctx.Ctx, peer, msgID,
				locale.Msgf(ctx.DBUser.LangCode, otherMsgs.NotEnoughDuzhocoins, locale.KV("amount", duzhocoins)), 15*time.Second)
		}

		receiver, err := db.Load[db.User](p.ReceiverID)
		if err != nil {
			editAndScheduleRemove(ctx.Api, ctx.Ctx, peer, msgID, locale.Msgf(ctx.DBUser.LangCode, locale.GlobalIDs.Error), 15*time.Second)
			return fmt.Errorf("failed to load receiver: %w", err)
		}

		ctx.DBUser.Duzhocoins -= p.Amount
		if err := ctx.DBUser.Save(); err != nil {
			editAndScheduleRemove(ctx.Api, ctx.Ctx, peer, msgID, locale.Msgf(ctx.DBUser.LangCode, locale.GlobalIDs.Error), 15*time.Second)
			return fmt.Errorf("failed to save sender: %w", err)
		}

		receiver.Duzhocoins += p.Amount
		if err := receiver.Save(); err != nil {
			editAndScheduleRemove(ctx.Api, ctx.Ctx, peer, msgID, locale.Msgf(ctx.DBUser.LangCode, locale.GlobalIDs.Error), 15*time.Second)
			return fmt.Errorf("failed to save receiver: %w", err)
		}

		receiverName := getName(ctx.Ctx, ctx.Api, ctx.From.Obj, p.ReceiverID)
		duzhocoinsNoun := locale.GlobalIDs.Nouns.Duzhocoins.ConstructSafe(ctx.DBUser.LangCode, locale.CaseNom, int(p.Amount))
		duzhocoinsText := locale.Msg(ctx.DBUser.LangCode, duzhocoinsNoun)

		successText := locale.Msgf(ctx.DBUser.LangCode, otherMsgs.Success,
			locale.KV("amount", p.Amount),
			locale.KV("duzhocoins", duzhocoinsText),
			locale.KV("receiver_name", receiverName),
			locale.KV("receiver_id", p.ReceiverID),
		)

		senderName := ctx.From.FullName
		senderID := p.IssuerID
		senderAH := ctx.DBUser.AH
		senderLang := ctx.DBUser.LangCode

		receiverID := p.ReceiverID
		receiverAH := receiver.AH
		receiverLang := receiver.LangCode

		senderApi := ctx.Sender
		bgCtx := context.WithoutCancel(ctx.Ctx)

		go func() {
			if senderAH != 0 {
				senderNoun := locale.GlobalIDs.Nouns.Duzhocoins.ConstructSafe(senderLang, locale.CaseNom, int(p.Amount))
				senderText := locale.Msgf(senderLang, otherMsgs.SenderNotification,
					locale.KV("amount", p.Amount),
					locale.KV("duzhocoins", locale.Msg(senderLang, senderNoun)),
					locale.KV("receiver_name", receiverName),
					locale.KV("receiver_id", receiverID),
				)
				_, err := senderApi.To(&tg.InputPeerUser{UserID: senderID, AccessHash: senderAH}).StyledText(bgCtx, html.String(nil, senderText))
				if err != nil {
					qsh.Errorf("error sending sender text: %w", err)
				}
			}

			if receiverAH != 0 {
				receiverNoun := locale.GlobalIDs.Nouns.Duzhocoins.ConstructSafe(receiverLang, locale.CaseNom, int(p.Amount))
				receiverText := locale.Msgf(receiverLang, otherMsgs.ReceiverNotification,
					locale.KV("amount", p.Amount),
					locale.KV("duzhocoins", locale.Msg(receiverLang, receiverNoun)),
					locale.KV("sender_name", senderName),
					locale.KV("sender_id", senderID),
				)
				_, err := senderApi.To(&tg.InputPeerUser{UserID: receiverID, AccessHash: receiverAH}).StyledText(bgCtx, html.String(nil, receiverText))
				if err != nil {
					qsh.Errorf("error sending receiver text: %w", err)
				}
			}
		}()

		return editAndScheduleRemove(ctx.Api, ctx.Ctx, peer, msgID, successText, 15*time.Second)
	}

	cancelHandler := func(cbCtx *callback.TypedContext[*SendCallbackParams]) error {
		_ = cbCtx.Params
		peer := tgutil.GetInputPeer(cbCtx.Query.Peer, cbCtx.From.Obj)
		msgID := cbCtx.Query.MsgID
		key := msgKey{peerID: tgutil.GetInputPeerID(peer), msgID: msgID}

		state, loaded := pendingConfirmations.Get(key)
		if !loaded {
			deleteMsgAfter(cbCtx.Api, peer, msgID, 15*time.Second)
			return nil
		}

		pendingConfirmations.Delete(key)
		state.cancel()

		if time.Now().After(state.expiresAt) {
			deleteMsgAfter(cbCtx.Api, peer, msgID, 15*time.Second)
			return nil
		}

		return editAndScheduleRemove(cbCtx.Api, cbCtx.Ctx, peer, msgID,
			locale.Msg(cbCtx.DBUser.LangCode, otherMsgs.Cancelled), 15*time.Second)
	}

	register(ID, &otherMsgs, exec)

	callback.RegisterAuto(
		callback.CmdIDConfirmSend,
		confirmHandler,
	)
	callback.RegisterAuto(
		callback.CmdIDCancelSend,
		cancelHandler,
	)
}

func editAndScheduleRemove(api *tg.Client, ctx context.Context, peer tg.InputPeerClass, msgID int, text string, delay time.Duration) error {
	_, err := api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:    peer,
		ID:      msgID,
		Message: text,
	})
	if err == nil {
		go func() {
			time.Sleep(delay)
			deleteMsgAfter(api, peer, msgID, 15*time.Second)
		}()
	}
	return err
}

func deleteMsgAfter(api *tg.Client, peer tg.InputPeerClass, msgID int, duration time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	if channel, ok := peer.(*tg.InputPeerChannel); ok {
		_, _ = api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: &tg.InputChannel{
				ChannelID:  channel.ChannelID,
				AccessHash: channel.AccessHash,
			},
			ID: []int{msgID},
		})
	} else {
		_, _ = api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			Revoke: true,
			ID:     []int{msgID},
		})
	}
}
