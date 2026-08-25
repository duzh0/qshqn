package command

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gotd/td/tg"

	"qshqn/core/ai"
	"qshqn/core/locale"
	"qshqn/core/qsh"
)

var (
	TalkMetaPromptMsgID,
	TalkBasePromptMsgID locale.MsgID
)

type TalkResponse struct {
	Thoughts string   `json:"thoughts,omitempty"`
	Text     string   `json:"text"`
	Reaction string   `json:"reaction,omitempty"`
	Command  string   `json:"command,omitempty"`
	Args     []string `json:"args,omitempty"`
}

func (t TalkResponse) Schema() *ai.Schema {
	return talkResponseSchema
}

var talkResponseSchema = &ai.Schema{
	Type: ai.TypeObject,
	Properties: map[string]*ai.Schema{
		//		"thoughts": {
		//			Type:        ai.TypeString,
		//			Description: "Your internal reasoning. Keep this strictly under 4 sentences. Map user intent to command keyword or null here. Same with args. DO NOT write JSON blocks, schema types, or code syntax here",
		//		},
		"text": {
			Type:        ai.TypeString,
			Description: "Text reply to send to the user",
		},
		"reaction": {
			Type:        ai.TypeString,
			Description: "Optional single emoji reaction. Use null if no reaction is appropriate. Do not overuse reactions",
			Nullable:    true,
			Enum: []string{
				"👍", "👎", "❤", "🔥", "🥰", "👏", "😁", "🤔", "🤯",
				"😱", "🤬", "😢", "🎉", "🤩", "🤮", "💩", "🙏", "👌", "🕊",
				"🤡", "🥱", "🥴", "😍", "🐳", "❤️‍🔥", "🌚", "🌭", "💯", "🤣",
				"⚡", "🍌", "🏆", "💔", "🤨", "😐", "🍓", "🍾", "💋", "🖕",
				"😈", "😴", "😭", "🤓", "👻", "👨‍💻", "👀", "🎃", "🙈", "😇",
				"😨", "🤝", "✍", "🤗", "🫡", "🎅", "🎄", "☃", "💅", "🤪",
				"🗿", "🆒", "💘", "🙉", "🦄", "😘", "🙊", "😎", "👾", "🤷‍♂️",
				"🤷", "🤷‍♀️", "😡",
			},
		},
		"command": {
			Type:        ai.TypeString,
			Nullable:    true,
			Description: "Optional command keyword to execute after generating the response. Use null if no command should run. IMPORTANT: BE CAREFUL! COMMANDS EXECUTE IMMEDIATELY AFTER YOU TRIGGER THEM.",
		},
		"args": {
			Type:        ai.TypeArray,
			Nullable:    true,
			Description: "Optional list of arguments to pass to the selected command. Some commands REQUIRE ARGUMENTS so you MUST provide them to execute, AND THEY MUST BE CORRECT AND IN THE CORRECT ORDER. If you made a decision in your 'thoughts' to use an argument (such as a stake amount), you MUST explicitly place it inside this array. Do not leave this empty if you planned to use an argument.",
			Items: &ai.Schema{
				Type: ai.TypeString,
			},
		},
	},
}

func buildAvailableCommands(code locale.LangCode) string {
	cmds := make([]*Command, 0, len(commandsList))
	for _, cmd := range commandsList {
		cmds = append(cmds, cmd)
	}

	sort.Slice(cmds, func(i, j int) bool {
		return strings.Compare(cmds[i].Name(code), cmds[j].Name(code)) < 0
	})

	var b strings.Builder
	for _, cmd := range cmds {
		fmt.Fprintf(&b, "%s\nKeywords: %s\nUsage: %s\nHelp: %s\n\n",
			cmd.Name(code), strings.Join(cmd.Keywords(code), " "), cmd.Usage(code), cmd.Help(code))
	}

	return strings.TrimSpace(b.String())
}

func resolveCommandKeyword(input string, code locale.LangCode) (string, []string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", nil, false
	}

	parts := strings.Fields(strings.ToLower(trimmed))
	if len(parts) == 0 {
		return "", nil, false
	}

	keyword := parts[0]
	if _, ok := keywordTriggers[KeywordTriggerKey{Code: code, Keyword: keyword}]; !ok {
		return "", nil, false
	}

	return keyword, parts[1:], true
}

func buildCommandExecutionText(trigger, keyword string, args []string) string {
	if trigger == "" || keyword == "" {
		return ""
	}

	commandText := trigger + " " + keyword
	if len(args) > 0 {
		commandText += " " + strings.Join(args, " ")
	}

	return "> [" + commandText + "]"
}

func BuildHistory(ctx *Context) (history []ai.Content) {
	if ctx.RepliedTo == nil {
		history = make([]ai.Content, 0, 1)
		msgWithMetadata := buildMessageWithMetadata(ctx.From.ID, ctx.From.FullName, ctx.Payload)
		return append(history, ai.Content{Role: ai.RoleUser, Parts: []ai.Part{{Text: msgWithMetadata}}})
	}

	history = make([]ai.Content, 0, 2)
	if ctx.RepliedTo.FromID == ctx.Bot.ID {
		history = append(history, ai.Content{Role: ai.RoleModel, Parts: []ai.Part{{Text: ctx.RepliedTo.Text}}})
	} else {
		authorName := ctx.RepliedTo.FromName
		if authorName == "" {
			authorName = fmt.Sprintf("User%d", ctx.RepliedTo.FromID)
		}
		replyMsgWithMetadata := buildMessageWithMetadata(ctx.RepliedTo.FromID, authorName, ctx.RepliedTo.Text)
		history = append(history, ai.Content{Role: ai.RoleUser, Parts: []ai.Part{{Text: replyMsgWithMetadata}}})
	}

	msgWithMetadata := buildReplyWithMetadata(ctx.From.ID, ctx.From.FullName, ctx.Payload)
	return append(history, ai.Content{Role: ai.RoleUser, Parts: []ai.Part{{Text: msgWithMetadata}}})
}

func GetFullContext(ctx *Context) string {
	var sb strings.Builder
	sb.WriteString("[SYSTEM INSTRUCTION]\n")
	sb.WriteString(BuildFullSystemPrompt(ctx))
	sb.WriteString("\n\n[HISTORY]")

	history := BuildHistory(ctx)
	for i, c := range history {
		fmt.Fprintf(&sb, "\n\n[%d] %s:\n", i+1, strings.ToUpper(string(c.Role)))
		for _, part := range c.Parts {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}

func buildMessageWithMetadata(ID int64, author, msg string) string {
	return fmt.Sprintf("<ID>%d</ID> <Name>%s</Name> <Message>%s</Message>", ID, author, msg)
}

func buildReplyWithMetadata(ID int64, author, msg string) string {
	return "[replying to previous message] " + buildMessageWithMetadata(ID, author, msg)
}

func BuildFullSystemPrompt(ctx *Context) string {
	customPrompt := ctx.DBUser.CustomSystemPrompt
	customStart := ctx.LocaleMsgRaw(locale.GlobalIDs.CustomSystemPromptStart)
	customEnd := ctx.LocaleMsgRaw(locale.GlobalIDs.CustomSystemPromptEnd)
	nameStart := ctx.LocaleMsgRaw(locale.GlobalIDs.UserNameStart)
	nameEnd := ctx.LocaleMsgRaw(locale.GlobalIDs.UserNameEnd)

	safeName := ctx.From.FullName
	safeName = strings.ReplaceAll(safeName, customStart, "")
	safeName = strings.ReplaceAll(safeName, customEnd, "")
	safeName = strings.ReplaceAll(safeName, nameStart, "")
	safeName = strings.ReplaceAll(safeName, nameEnd, "")
	safeName = strings.ReplaceAll(safeName, "\n", " ")

	safeCustom := customPrompt
	if safeCustom == "" {
		safeCustom = ctx.LocaleMsgRaw(TalkBasePromptMsgID)
	} else {
		safeCustom = strings.ReplaceAll(safeCustom, customStart, "")
		safeCustom = strings.ReplaceAll(safeCustom, customEnd, "")
		safeCustom = strings.ReplaceAll(safeCustom, nameStart, "")
		safeCustom = strings.ReplaceAll(safeCustom, nameEnd, "")
	}

	userLang := ctx.DBUser.LangCode.Lang()
	availableCommands := buildAvailableCommands(ctx.DBUser.LangCode)

	return ctx.LocaleMsgf(TalkMetaPromptMsgID,
		locale.KV("current_time", time.Now().UTC().Format("02.01.2006 15:04:05.000")),
		locale.KV("bot_id", ctx.Bot.ID),
		locale.KV("bot_name", ctx.Bot.FullName),
		locale.KV("bot_username", ctx.Bot.Username),
		locale.KV("user_name", safeName),
		locale.KV("user_id", ctx.From.ID),
		locale.KV("balance", ctx.DBUser.Duzhocoins),
		locale.KV("dodep", ctx.DBUser.Dodep),
		locale.KV("commands", availableCommands),
		locale.KV("custom_system_prompt", safeCustom),
		locale.KV("custom_system_prompt_start", customStart),
		locale.KV("custom_system_prompt_end", customEnd),
		locale.KV("user_name_start", nameStart),
		locale.KV("user_name_end", nameEnd),
		locale.KV("lang", userLang.Name.English),
		locale.KV("lang_code", userLang.Code.String()),
		locale.KV("lang_flag", userLang.Flag),
	)
}

func init() {
	ID := locale.RootIDTalk
	otherMsgs := struct {
		QueryEmpty,
		MetaPrompt,
		BasePrompt locale.MsgID
	}{}

	exec := func(ctx *Context) (passthrough bool, err error) {
		if ctx.Payload == "" {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.QueryEmpty)
			return false, err
		}

		stopTyping := ctx.StartTyping()
		defer stopTyping()

		fullSystemPrompt := BuildFullSystemPrompt(ctx)
		promptText := ctx.Payload

		talkResp, err := ai.GenerateStructured[TalkResponse](ctx.Ctx, &ai.GenerateTextOptions{
			Text: promptText,
			SystemInstruction: &ai.SystemInstruction{
				Parts: []ai.Part{
					{Text: fullSystemPrompt},
				},
			},
			History: BuildHistory(ctx),
		})

		if err != nil {
			_, err2 := ctx.ReplyReportErrLocaleMsg(locale.GlobalIDs.Error)
			return false, errors.Join(err, err2)
		}

		thoughts := talkResp.Structure.Thoughts
		replyText := talkResp.Structure.Text
		reaction := talkResp.Structure.Reaction
		commandPayloadRaw := strings.TrimSpace(strings.ToLower(talkResp.Structure.Command))

		if thoughts != "" {
			qsh.Debugf("thoughts: %s", thoughts)
		}

		qsh.Debugf("in[%d] out[%d] total[%d]. time: [%.3fs]", talkResp.Resp.InputTokens, talkResp.Resp.OutputTokens, talkResp.Resp.TotalTokens, talkResp.Resp.Time)

		if reaction != "" {
			_, errReaction := ctx.Api.MessagesSendReaction(ctx.Ctx, &tg.MessagesSendReactionRequest{
				Peer:  ctx.InputPeer,
				MsgID: ctx.Message.ID,
				Reaction: []tg.ReactionClass{
					&tg.ReactionEmoji{Emoticon: reaction},
				},
			})
			if errReaction != nil {
				err = errors.Join(err, fmt.Errorf("failed to send reaction [%s]: %w", reaction, errReaction))
			}
		}

		var commandKeyword string
		var commandArgs []string
		var cmd *Command
		var shouldExecuteCommand bool

		if commandPayloadRaw != "" {
			keyword, cmdParts, ok := resolveCommandKeyword(commandPayloadRaw, ctx.DBUser.LangCode)
			if ok {
				commandKeyword = keyword
				commandArgs = append([]string{}, talkResp.Structure.Args...)
				commandArgs = append(commandArgs, cmdParts...)

				targetKey := KeywordTriggerKey{Code: ctx.DBUser.LangCode, Keyword: commandKeyword}
				var okCmd bool
				cmd, okCmd = keywordTriggers[targetKey]
				if !okCmd {
					qsh.Debugf("talk nested command skipped: unknown keyword [%s]", commandKeyword)
				} else if ctx.Command != nil && cmd.ID() == ctx.Command.ID() {
					qsh.Debugf("talk nested command skipped: same command as current (%s)", cmd.ID())
				} else {
					shouldExecuteCommand = true
					commandExecutionText := buildCommandExecutionText(ctx.DBUser.LangCode.Lang().PreferredTrigger, commandKeyword, commandArgs)
					if commandExecutionText != "" {
						if replyText != "" {
							replyText = strings.TrimRight(replyText, "\n\r ")
							replyText += "\n\n" + commandExecutionText
						} else {
							replyText = commandExecutionText
						}
					}
				}
			}
		}

		if replyText != "" {
			_, errReply := ctx.ReplyText(replyText)
			if errReply != nil {
				err = errors.Join(err, errReply)
			}
		} else {
			_, errReply := ctx.ReplyText("...")
			if errReply != nil {
				err = errors.Join(err, errReply)
			}
		}

		if shouldExecuteCommand {
			cmdCtx := GetContext(ctx.Ctx, ctx.Api, ctx.Sender, ctx.IsComment, ctx.From, ctx.Bot, ctx.DBUser, ctx.InputPeer, ctx.PeerID, ctx.Message, commandKeyword+" "+strings.Join(commandArgs, " "), ctx.RepliedTo)
			cmdCtx.SkipDispatchLock = true
			cmdCtx.Strict = true
			defer cmdCtx.Put()

			qsh.Debugf("talk command triggered nested command[%s] with payload[%s]", commandKeyword, cmdCtx.Payload)
			_, errDispatch := Dispatch(cmdCtx)
			if errDispatch != nil {
				qsh.Errorf("error dispatching nested command: %w", err)
				err = errors.Join(err, errDispatch)
			}
		}

		return false, err
	}
	register(ID, &otherMsgs, exec)
	OnPkgInit(func() error {
		TalkBasePromptMsgID = otherMsgs.BasePrompt
		TalkMetaPromptMsgID = otherMsgs.MetaPrompt
		return nil
	})
}
