package tgx

import (
	"fmt"

	"github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/tg"

	"qshqn/core/callback"
	"qshqn/core/command"
	"qshqn/core/config"
	"qshqn/core/db"
	"qshqn/core/locale"
	"qshqn/core/qsh"
	"qshqn/core/util/tgutil"
)

func (m *Manager) NotifyAndLeaveChat(ctx *command.Context) (err error) {
	defer func() {
		m.LeaveChat(ctx.Ctx, ctx.InputPeer)
	}()

	peerID := ctx.PeerID
	if _, ok := ctx.InputPeer.(*tg.InputPeerChannel); !ok {
		_, err = ctx.ReplyReportErrLocaleMsg(locale.GlobalIDs.WhitelistNotSupergroup)
		return err
	}

	_, _ = ctx.ReplyReportErrLocaleMsgf(locale.GlobalIDs.WhitelistNoticeMsg,
		locale.KV("support_link", config.Tg.SupportLink),
	)

	dbOwner, err := db.Load[db.User](config.Tg.OwnerID)
	if err != nil {
		return fmt.Errorf("error loading ownerID[%d]: %w", config.Tg.OwnerID, err)
	}
	ownerInputPeer := &tg.InputPeerUser{UserID: dbOwner.ID, AccessHash: dbOwner.AH}

	addChatParams := &command.AddChatParams{
		IssuerArgs: callback.IssuerArgs{
			IssuerID: dbOwner.ID,
		},
		ChatID:          peerID,
		RequesterUserID: ctx.From.ID,
		RequesterUserAH: ctx.DBUser.AH,
	}
	approveData, err := callback.Encode(callback.CmdIDApproveAddChat, addChatParams)
	if err != nil {
		return fmt.Errorf("error encoding approve data: %w", err)
	}

	declineData, err := callback.Encode(callback.CmdIDDeclineAddChat, addChatParams)
	if err != nil {
		return fmt.Errorf("error encoding decline data: %w", err)
	}

	markup := &tg.ReplyInlineMarkup{
		Rows: []tg.KeyboardButtonRow{
			{
				Buttons: []tg.KeyboardButtonClass{
					&tg.KeyboardButtonCallback{Text: "✅", Data: approveData},
					&tg.KeyboardButtonCallback{Text: "❌", Data: declineData},
				},
			},
		},
	}

	requestText := locale.Msgf(dbOwner.LangCode, locale.GlobalIDs.WhitelistRequestOwner,
		locale.KV("sender_name", ctx.From.FullName),
		locale.KV("sender_id", ctx.From.ID),
		locale.KV("chat_name", tgutil.GetFullName(ctx.Message.PeerID, ctx.From.Obj)),
		locale.KV("chat_id", peerID),
		locale.KV("support_link", config.Tg.SupportLink),
	)
	_, errRequest := ctx.Sender.To(ownerInputPeer).Markup(markup).StyledText(ctx.Ctx, html.String(nil, requestText))
	if errRequest != nil {
		qsh.Errorf("failed to notify owner: %w", errRequest)
	}

	return nil
}
