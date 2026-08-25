package command

import "qshqn/core/locale"

func init() {
	ID := "balance"
	otherMsgs := struct {
		BalanceMessage locale.MsgID
	}{}
	exec := func(ctx *Context) (bool, error) {
		if len(ctx.Args) > 1 && !ctx.Strict {
			return true, nil
		}
		_, err := ctx.ReplyReportErrLocaleMsgf(otherMsgs.BalanceMessage,
			locale.KV("amount", ctx.DBUser.Duzhocoins),
			locale.KV("duzhocoins", ctx.LocaleMsg(locale.GlobalIDs.Nouns.Duzhocoins.ConstructSafe(ctx.DBUser.LangCode, locale.CaseGen, int(ctx.DBUser.Duzhocoins)))),
		)
		return false, err
	}
	register(ID, &otherMsgs, exec)
}
