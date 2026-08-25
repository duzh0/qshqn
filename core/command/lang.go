package command

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gotd/td/tg"

	"qshqn/core/callback"
	"qshqn/core/locale"
	"qshqn/core/util/tgutil"
)

func init() {
	type LangCallbackParams struct {
		callback.IssuerArgs
		LangCodeRaw [2]byte
	}

	ID := "lang"
	const callbackCmdID = callback.CmdIDLang
	otherMsgs := struct {
		LangNotSupported,
		Prompt,
		Success locale.MsgID
	}{}

	newLangCallbackParams := func(code locale.LangCode) *LangCallbackParams {
		p := &LangCallbackParams{}
		copy(p.LangCodeRaw[:], string(code))
		return p
	}

	exec := func(ctx *Context) (passthrough bool, err error) {
		if len(ctx.Args) > 1 {
			newLangCode := strings.ToLower(ctx.Args[1])
			if lang, ok := locale.SupportedCode(newLangCode); ok {
				ctx.DBUser.LangCode = lang.Code
				if err := ctx.DBUser.Save(); err != nil {
					return false, err
				}
				_, err := ctx.ReplyReportErrLocaleMsgf(otherMsgs.Success,
					locale.KV("flag", lang.Flag),
					locale.KV("lang_name", lang.Name.Local),
				)
				return false, err
			} else if ctx.Strict {
				_, err := ctx.ReplyReportErrLocaleMsgf(otherMsgs.LangNotSupported)
				return false, err
			}
		}

		userLangCode := ctx.DBUser.LangCode
		var rows []tg.KeyboardButtonRow
		for code, lang := range locale.AllLangs() {
			if code == userLangCode {
				continue
			}
			params := newLangCallbackParams(lang.Code)
			params.IssuerID = ctx.From.ID
			cbData, err := callback.Encode(callbackCmdID, params)
			if err != nil {
				return false, fmt.Errorf("encode callback error: %w", err)
			}
			rows = append(rows, tg.KeyboardButtonRow{
				Buttons: []tg.KeyboardButtonClass{
					&tg.KeyboardButtonCallback{
						Text: lang.Flag,
						Data: cbData,
					},
				},
			})
		}

		userLang := userLangCode.Lang()
		markup := &tg.ReplyInlineMarkup{Rows: rows}
		_, err = ctx.Reply().Markup(markup).Text(ctx.Ctx,
			ctx.LocaleMsgf(otherMsgs.Prompt,
				locale.KV("current_flag", userLang.Flag),
				locale.KV("current_lang_name", userLang.Name.Local),
			),
		)
		return false, err
	}

	callbackHandler := func(ctx *callback.TypedContext[*LangCallbackParams]) error {
		p := ctx.Params
		newLangCode := locale.LangCode(bytes.TrimRight(p.LangCodeRaw[:], "\x00"))
		ctx.DBUser.LangCode = newLangCode
		if err := ctx.DBUser.Save(); err != nil {
			return err
		}
		lang := newLangCode.Lang()
		successText := locale.Msgf(newLangCode, otherMsgs.Success,
			locale.KV("flag", lang.Flag),
			locale.KV("lang_name", lang.Name.Local),
		)
		_, err := ctx.Api.MessagesEditMessage(ctx.Ctx, &tg.MessagesEditMessageRequest{
			Peer:    tgutil.GetInputPeer(ctx.Query.Peer, ctx.From.Obj),
			ID:      ctx.Query.MsgID,
			Message: successText,
		})
		return err
	}

	register(ID, &otherMsgs, exec)

	callback.RegisterAuto(
		callbackCmdID,
		callbackHandler,
	)
}
