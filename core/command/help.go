package command

import (
	"fmt"
	"slices"
	"strings"

	"qshqn/core/config"
	"qshqn/core/locale"
)

func init() {
	ID := locale.RootIDHelp
	otherMsgs := struct {
		CommandMissing,
		CommandHelpMsg,
		AllCommandsHelpMsg locale.MsgID
	}{}
	commandDescriptionFunc := func(cmd *Command, code locale.LangCode) []locale.KVPair {
		return []locale.KVPair{
			locale.KV("command_name", cmd.Name(code)),
			locale.KV("command_keywords", "["+strings.Join(cmd.Keywords(code), ", ")+"]"),
			locale.KV("command_help", cmd.Help(code)),
			locale.KV("command_usage", cmd.Usage(code)),
		}
	}
	exec := func(ctx *Context) (passthrough bool, err error) {
		langCode := ctx.DBUser.LangCode
		if len(ctx.Args) > 1 {
			target := strings.ToLower(ctx.Args[1])
			key := KeywordTriggerKey{Code: langCode, Keyword: target}
			if cmd, ok := keywordTriggers[key]; ok {
				_, err := ctx.ReplyReportErrLocaleMsgf(otherMsgs.CommandHelpMsg, commandDescriptionFunc(cmd, langCode)...)
				return false, err
			} else if ctx.Strict {
				_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.CommandMissing)
				return false, err
			}
			return true, nil
		}

		cmds := make([]*Command, 0, len(commandsList))
		for _, cmd := range commandsList {
			cmds = append(cmds, cmd)
		}

		slices.SortFunc(cmds, func(a, b *Command) int {
			return strings.Compare(a.Name(langCode), b.Name(langCode))
		})

		var b strings.Builder
		for i, cmd := range cmds {
			fmt.Fprintf(&b, "\n\n%d. %s", i+1, ctx.LocaleMsgf(otherMsgs.CommandHelpMsg, commandDescriptionFunc(cmd, langCode)...))
		}

		_, err = ctx.ReplyReportErrLocaleMsgf(otherMsgs.AllCommandsHelpMsg,
			locale.KV("current_model", config.Services.Gemini.DefaultModel),
			locale.KV("all_commands", b.String()),
		)
		return false, err
	}
	register(ID, &otherMsgs, exec)
}
