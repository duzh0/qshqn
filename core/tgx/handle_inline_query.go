package tgx

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gotd/td/telegram/message/inline"
	"github.com/gotd/td/tg"

	"qshqn/core/config"
	"qshqn/core/db"
	"qshqn/core/locale"
	"qshqn/core/netx"
	"qshqn/core/qsh"
	"qshqn/core/util/tgutil"
)

const (
	MAX_TITLE_LENGTH       = 33
	MAX_DESCRIPTION_LENGTH = 135
)

func getPredatorInlineResult(msg string) *inline.ArticleResultBuilder {
	var predatorTitle, predatorDescription string
	runes := []rune(msg)
	if len(runes) > MAX_TITLE_LENGTH {
		predatorTitle = string(runes[:MAX_TITLE_LENGTH])
		if len(runes) > MAX_DESCRIPTION_LENGTH {
			predatorDescription = string(runes[MAX_TITLE_LENGTH:MAX_DESCRIPTION_LENGTH])
		} else {
			predatorDescription = string(runes[MAX_TITLE_LENGTH:])
		}
	} else {
		predatorTitle = msg
	}

	predatorMsgWebDoc := tg.InputWebDocument{
		URL:      config.Links.RandomChatBubbleImage,
		MimeType: netx.MIME_JPEG,
	}
	return inline.Article(predatorTitle, inline.MessageText(msg)).Description(predatorDescription).Thumb(predatorMsgWebDoc)
}

func (m *Manager) handleInlineQuery(ctx context.Context, ent tg.Entities, upd *tg.UpdateBotInlineQuery) error {
	uID := upd.UserID
	dbUser, err := db.LoadOrDefault[db.User](uID)
	if err != nil {
		return fmt.Errorf("error loading or defaulting user: %w", err)
	}

	if dbUser.Banned {
		return nil
	}

	tgUser, ok := ent.Users[uID]
	if !ok {
		return fmt.Errorf("user [%d] not found in entities", uID)
	}

	fullName := tgutil.GetUserName(tgUser)
	qsh.Debugf("inline query from user ID[%d] [%s]: %s", uID, fullName, upd.Query)

	var nextOffset string
	var response []inline.ResultOption
	if upd.Query == "" {
		langCode := dbUser.LangCode
		duzhocoinsAmount := dbUser.Duzhocoins
		response = make([]inline.ResultOption, 2)

		duzhocoinsMsgID := locale.GlobalIDs.Nouns.Duzhocoins.Construct(langCode, locale.CaseNom, int(duzhocoinsAmount))
		duzhocoinsTitleMsgID := locale.GlobalIDs.InlineDuzhocoinsTitle
		duzhocoinsDescriptionMsgID := locale.GlobalIDs.InlineDuzhocoinsDescription
		resultMessageMsgID := locale.GlobalIDs.InlineDuzhocoinsMessageResult

		duzhocoinsAmountString := locale.String(duzhocoinsAmount)
		duzhocoinsAmountKV := locale.KV("amount", duzhocoinsAmountString)

		duzhocoinsTitle := locale.Msgf(langCode, duzhocoinsTitleMsgID, duzhocoinsAmountKV)
		duzhocoinsDescription := locale.Msg(langCode, duzhocoinsDescriptionMsgID)
		duzhocoinsMsgResult := locale.Msgf(langCode, resultMessageMsgID,
			duzhocoinsAmountKV,
			locale.KV("duzhocoins", duzhocoinsMsgID),
		)

		duzhocoinsWebDoc := tg.InputWebDocument{
			URL:      config.Links.DuzhocoinsImage,
			MimeType: netx.MIME_JPEG,
		}
		response[0] = inline.Article(duzhocoinsTitle, inline.MessageText(duzhocoinsMsgResult)).Description(duzhocoinsDescription).Thumb(duzhocoinsWebDoc)

		var predatorMsg string
		predatorMsgs, err := db.RandPredatorMsgs(1, 0, "")
		if err != nil || len(predatorMsgs) < 1 {
			predatorMsg = "русоскотосвинобиомусор"
		} else {
			predatorMsg = predatorMsgs[0]
		}
		response[1] = getPredatorInlineResult(predatorMsg)
	} else {
		offset := 0
		if upd.Offset != "" {
			if offset, err = strconv.Atoi(upd.Offset); err != nil {
				return fmt.Errorf("error parsing offset [%s]: %w", upd.Offset, err)
			}
		}

		msgs, err := db.PredatorMsgs(MAX_INLINE_ARTICLES, 0, offset, upd.Query)
		if err != nil {
			return err
		}

		if len(msgs) < 1 {
			return nil
		}

		response = make([]inline.ResultOption, 0, len(msgs))
		for _, msg := range msgs {
			response = append(response, getPredatorInlineResult(msg))
		}

		if len(msgs) == MAX_INLINE_ARTICLES {
			nextOffset = fmt.Sprint(offset + MAX_INLINE_ARTICLES)
		} else {
			nextOffset = ""
		}
	}

	if response == nil {
		return fmt.Errorf("response is nil")
	}

	if _, err = m.sender.Inline(upd).NextOffset(nextOffset).Set(ctx, response...); err != nil {
		return fmt.Errorf("error setting result: %w", err)
	}

	return nil
}
