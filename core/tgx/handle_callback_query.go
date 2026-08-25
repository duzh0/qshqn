package tgx

import (
	"context"
	"fmt"

	"github.com/gotd/td/tg"

	"qshqn/core/callback"
	"qshqn/core/db"
	"qshqn/core/locale"
	"qshqn/core/qsh"
	"qshqn/core/typex"
	"qshqn/core/util/tgutil"
)

func (m *Manager) catchBotCallbackQuery(ctx context.Context, ent tg.Entities, upd *tg.UpdateBotCallbackQuery) error {
	go safeHandle(func() error {
		return m.handleCallbackQuery(ctx, ent, upd)
	})
	return nil
}

func (m *Manager) handleCallbackQuery(ctx context.Context, ent tg.Entities, upd *tg.UpdateBotCallbackQuery) error {
	defer func() {
		_, err := m.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: upd.QueryID,
		})
		if err != nil {
			qsh.Errorf("failed to answer callback query: %v", err)
		}
	}()

	cmdID, params, err := callback.Decode[callback.Issuer](upd.Data)
	if err != nil {
		return fmt.Errorf("decode callback error: %w", err)
	}

	dbUser, err := db.LoadOrDefault[db.User](upd.UserID)
	if err != nil {
		return fmt.Errorf("db load error: %w", err)
	}

	if dbUser.Banned {
		return nil
	}

	issuerID := params.GetIssuerID()
	if issuerID != 0 && issuerID != upd.UserID {
		_, _ = m.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: upd.QueryID,
			Message: locale.Msg(dbUser.LangCode, locale.GlobalIDs.NotCallbackButtonOwner),
			Alert:   true,
		})
		return nil
	}

	var fromFullName string
	if tgUser, ok := ent.Users[upd.UserID]; ok {
		fromFullName = tgutil.GetUserName(tgUser)
	}

	ctxFrom := typex.From{
		ID:       upd.UserID,
		FullName: fromFullName,
		Obj:      ent,
	}

	cbCtx := &callback.Context{
		Ctx:       ctx,
		Api:       m.api,
		Sender:    m.sender,
		From:      ctxFrom,
		DBUser:    dbUser,
		Query:     upd,
		CmdID:     cmdID,
		RawParams: params,
	}

	return callback.Dispatch(cbCtx)
}
