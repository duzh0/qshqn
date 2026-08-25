package command

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"qshqn/core/db"
	"qshqn/core/locale"
)

func init() {
	const MAX_PROMPT_LENGTH = 4000
	ID := "prompt"
	otherMsgs := struct {
		ResetWord,
		CurrentPrompt,
		PromptEmpty,
		CurrentPromptReplied,
		PromptEmptyReplied,
		PromptSame,
		PromptTooLong,
		SetSuccess,
		ResetSuccess,
		DBSaveError locale.MsgID
	}{}

	exec := func(ctx *Context) (passthrough bool, err error) {
		if len(ctx.Args) == 1 {
			targetPrompt := ctx.DBUser.CustomSystemPrompt
			emptyMsg, currentMsg := otherMsgs.PromptEmpty, otherMsgs.CurrentPrompt

			if ctx.RepliedTo != nil {
				repliedUser, err := db.LoadOrDefault[db.User](ctx.RepliedTo.FromID)
				if err != nil {
					_, errReply := ctx.ReplyReportErrLocaleMsg(locale.GlobalIDs.Error)
					return false, errors.Join(err, errReply)
				}
				targetPrompt = repliedUser.CustomSystemPrompt
				emptyMsg, currentMsg = otherMsgs.PromptEmptyReplied, otherMsgs.CurrentPromptReplied
			}

			if targetPrompt == "" {
				_, err := ctx.ReplyReportErrLocaleMsg(emptyMsg)
				return false, err
			}

			_, err := ctx.ReplyReportErrLocaleMsgf(currentMsg,
				locale.KV("current_prompt", targetPrompt),
			)
			return false, err
		}

		idx := strings.IndexFunc(ctx.Payload, unicode.IsSpace)
		if idx == -1 {
			return false, fmt.Errorf("no space in payload? impossible: %s", ctx.Payload)
		}
		newPrompt := strings.TrimSpace(ctx.Payload[idx:])

		customStart := ctx.LocaleMsgRaw(locale.GlobalIDs.CustomSystemPromptStart)
		customEnd := ctx.LocaleMsgRaw(locale.GlobalIDs.CustomSystemPromptEnd)
		nameStart := ctx.LocaleMsgRaw(locale.GlobalIDs.UserNameStart)
		nameEnd := ctx.LocaleMsgRaw(locale.GlobalIDs.UserNameEnd)

		newPrompt = strings.ReplaceAll(newPrompt, customStart, "")
		newPrompt = strings.ReplaceAll(newPrompt, customEnd, "")
		newPrompt = strings.ReplaceAll(newPrompt, nameStart, "")
		newPrompt = strings.ReplaceAll(newPrompt, nameEnd, "")

		resetWord := strings.ToLower(ctx.LocaleMsgRaw(otherMsgs.ResetWord))
		switch strings.ToLower(newPrompt) {
		case resetWord:
			ctx.DBUser.CustomSystemPrompt = ""
			if err := ctx.DBUser.Save(); err != nil {
				_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.DBSaveError)
				return false, err
			}

			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.ResetSuccess)
			return false, err
		}

		newPromptLen := len(newPrompt)
		if newPromptLen > MAX_PROMPT_LENGTH {
			_, err := ctx.ReplyReportErrLocaleMsgf(otherMsgs.PromptTooLong,
				locale.KV("max_prompt_length", MAX_PROMPT_LENGTH),
				locale.KV("new_prompt_length", newPromptLen),
			)
			return false, err
		}

		if ctx.DBUser.CustomSystemPrompt == newPrompt {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.PromptSame)
			return false, err
		}

		ctx.DBUser.CustomSystemPrompt = newPrompt
		if err := ctx.DBUser.Save(); err != nil {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.DBSaveError)
			return false, err
		}

		_, err = ctx.ReplyReportErrLocaleMsg(otherMsgs.SetSuccess)
		return false, err
	}

	register(ID, &otherMsgs, exec)
}
