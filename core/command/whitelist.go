package command

import (
	"fmt"

	"github.com/gotd/td/tg"

	"qshqn/core/callback"
	"qshqn/core/config"
	"qshqn/core/db"
	"qshqn/core/locale"
	"qshqn/core/util/tgutil"
)

type AddChatParams struct {
	callback.IssuerArgs
	ChatID,
	RequesterUserID,
	RequesterUserAH int64
}

func init() {
	approveHandler := func(ctx *callback.TypedContext[*AddChatParams]) error {
		p := ctx.Params
		chat, err := db.LoadOrDefault[db.Chat](p.ChatID)
		if err != nil {
			return fmt.Errorf("error loading chat: %w", err)
		}

		chat.Whitelisted = true
		if err := chat.Save(); err != nil {
			return fmt.Errorf("error saving chat: %w", err)
		}

		targetPeer := tgutil.GetInputPeer(ctx.Query.Peer, ctx.From.Obj)
		ownerText := locale.Msgf(ctx.DBUser.LangCode, locale.GlobalIDs.WhitelistApprovedOwner,
			locale.KV("chat_id", p.ChatID),
		)

		_, err = ctx.Sender.To(targetPeer).Edit(ctx.Query.MsgID).Text(ctx.Ctx, ownerText)
		if err != nil {
			return err
		}

		if p.RequesterUserID != 0 && p.RequesterUserAH != 0 {
			requesterPeer := &tg.InputPeerUser{UserID: p.RequesterUserID, AccessHash: p.RequesterUserAH}
			userText := locale.Msgf(ctx.DBUser.LangCode, locale.GlobalIDs.WhitelistApprovedUser,
				locale.KV("chat_id", p.ChatID),
			)
			_, _ = ctx.Sender.To(requesterPeer).Text(ctx.Ctx, userText)
		}

		return nil
	}

	callback.RegisterAuto(
		callback.CmdIDApproveAddChat,
		approveHandler,
	)

	declineHandler := func(ctx *callback.TypedContext[*AddChatParams]) error {
		p := ctx.Params
		targetPeer := tgutil.GetInputPeer(ctx.Query.Peer, ctx.From.Obj)
		ownerText := locale.Msgf(ctx.DBUser.LangCode, locale.GlobalIDs.WhitelistDeclinedOwner,
			locale.KV("chat_id", p.ChatID),
		)
		_, err := ctx.Sender.To(targetPeer).Edit(ctx.Query.MsgID).Text(ctx.Ctx, ownerText)
		if p.RequesterUserID != 0 && p.RequesterUserAH != 0 {
			requesterPeer := &tg.InputPeerUser{UserID: p.RequesterUserID, AccessHash: p.RequesterUserAH}
			userText := locale.Msgf(ctx.DBUser.LangCode, locale.GlobalIDs.WhitelistDeclinedUser,
				locale.KV("chat_id", p.ChatID),
				locale.KV("support_link", config.Tg.SupportLink),
			)
			_, _ = ctx.Sender.To(requesterPeer).Text(ctx.Ctx, userText)
		}
		return err
	}

	callback.RegisterAuto(
		callback.CmdIDDeclineAddChat,
		declineHandler,
	)
}
