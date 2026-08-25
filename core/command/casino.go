package command

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gotd/td/tg"

	"qshqn/core/locale"
	"qshqn/core/qsh"
)

func init() {
	const (
		Bar int = iota
		Grape
		Lemon
		Seven
	)

	type Result struct {
		Multiplier float64
		MsgID      locale.MsgID
	}

	ID := "casino"
	otherMsgs := struct {
		NotPositiveInteger,
		InsufficientBalance,
		SendCasinoError,

		Paskhalochka67,
		Paskhalochka69,
		Paskhalochka1312,
		Paskhalochka1488,

		ResultTripleBar,
		ResultTripleGrape,
		ResultTripleLemon,
		ResultTripleSeven,

		ResultDoubleBar,
		ResultDoubleGrape,
		ResultDoubleLemon,
		ResultDoubleSeven,

		ResultNoMatch,

		MessageWin,
		MessageLoss,
		MessageTemplate locale.MsgID
	}{}

	getSlotsValues := func(rawValue int) (left, center, right int) {
		val := rawValue - 1
		left = val & 3
		center = (val >> 2) & 3
		right = val >> 4
		return left, center, right
	}

	var paskhalochky map[int64]locale.MsgID
	var resultsTriple [4]Result
	var resultsDouble [4]Result
	var resultNoMatch Result
	OnPkgInit(func() error {
		paskhalochky = map[int64]locale.MsgID{
			67:   otherMsgs.Paskhalochka67,
			69:   otherMsgs.Paskhalochka69,
			1312: otherMsgs.Paskhalochka1312,
			1488: otherMsgs.Paskhalochka1488,
		}
		resultsTriple = [4]Result{
			Bar:   {0.75, otherMsgs.ResultTripleBar},
			Grape: {3.0, otherMsgs.ResultTripleGrape},
			Lemon: {5.0, otherMsgs.ResultTripleLemon},
			Seven: {7.0, otherMsgs.ResultTripleSeven},
		}
		resultsDouble = [4]Result{
			Bar:   {0.5, otherMsgs.ResultDoubleBar},
			Grape: {1.5, otherMsgs.ResultDoubleGrape},
			Lemon: {2.0, otherMsgs.ResultDoubleLemon},
			Seven: {3.0, otherMsgs.ResultDoubleSeven},
		}
		resultNoMatch = Result{0, otherMsgs.ResultNoMatch}
		return nil
	})

	getResult := func(left, center, right int) Result {
		switch {
		case left == center && center == right:
			return resultsTriple[center]
		case left == center || center == right:
			return resultsDouble[center]
		}

		return resultNoMatch
	}

	exec := func(ctx *Context) (passthrough bool, err error) {
		langCode := ctx.DBUser.LangCode

		if len(ctx.Args) < 2 {
			_, err := ctx.ReplyText(ctx.Command.Usage(langCode))
			return false, err
		}

		if len(ctx.Args) > 2 && !ctx.Strict {
			return true, nil
		}

		bet, err := strconv.ParseInt(ctx.Args[1], 10, 64)
		if err != nil || bet < 1 {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.NotPositiveInteger)
			return false, err
		}

		duzhocoinsAmount := ctx.DBUser.Duzhocoins
		dodep := ctx.DBUser.Dodep
		total := duzhocoinsAmount + dodep
		if bet > total {
			errMsg := ctx.LocaleMsgf(otherMsgs.InsufficientBalance,
				locale.KV("amount", duzhocoinsAmount),
				locale.KV("duzhocoins", locale.GlobalIDs.Nouns.Duzhocoins.Construct(langCode, locale.CaseNom, int(duzhocoinsAmount))),
				locale.KV("dodep_amount", dodep),
				locale.KV("dodep_duzhocoins", locale.GlobalIDs.Nouns.Duzhocoins.Construct(langCode, locale.CaseNom, int(dodep))),
			)
			_, err := ctx.ReplyReportErr(errMsg)
			return false, err
		}

		if bet <= dodep {
			ctx.DBUser.Dodep = dodep - bet
		} else {
			ctx.DBUser.Dodep = 0
			ctx.DBUser.Duzhocoins -= (bet - dodep)
		}

		result, err := ctx.ToPeer().Casino(ctx.Ctx)
		if err != nil {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.SendCasinoError)
			return false, err
		}

		var diceMsgID, diceValue int
		if upds, ok := result.(*tg.Updates); ok {
			for _, u := range upds.Updates {
				var msgClass tg.MessageClass
				switch inner := u.(type) {
				case *tg.UpdateNewMessage:
					msgClass = inner.Message
				case *tg.UpdateNewChannelMessage:
					msgClass = inner.Message
				}

				if msg, ok := msgClass.(*tg.Message); ok {
					diceMsgID = msg.ID
					if dice, ok := msg.Media.(*tg.MessageMediaDice); ok {
						diceValue = dice.Value
						break
					}
				}
			}
		}

		if diceValue == 0 {
			return false, fmt.Errorf("failed to extract dice value")
		}

		leftVal, centerVal, rightVal := getSlotsValues(diceValue)
		resultStruct := getResult(leftVal, centerVal, rightVal)
		multiplier, resultMsgID := resultStruct.Multiplier, resultStruct.MsgID

		winAmount := int64(math.Round(float64(bet) * multiplier))
		ctx.DBUser.Duzhocoins += winAmount

		if err := ctx.DBUser.Save(); err != nil {
			return false, fmt.Errorf("casino failed to save user[%d] in db: %w", ctx.DBUser.ID, err)
		}

		slotsResultText := locale.Msg(langCode, resultMsgID)
		var winLossText string
		if winAmount > 0 {
			winLossText = locale.Msgf(langCode, otherMsgs.MessageWin,
				locale.KV("win_amount", winAmount),
				locale.KV("duzhocoins", locale.GlobalIDs.Nouns.Duzhocoins.ConstructSafe(langCode, locale.CaseAcc, int(winAmount))),
				locale.KV("multiplier", multiplier),
			)
		} else {
			winLossText = locale.Msgf(langCode, otherMsgs.MessageLoss,
				locale.KV("bet", bet),
				locale.KV("duzhocoins", locale.GlobalIDs.Nouns.Duzhocoins.ConstructSafe(langCode, locale.CaseGen, int(bet))),
			)
		}

		easterEggStr := ""
		if eggMsgID, ok := paskhalochky[bet]; ok {
			easterEggStr = "\n" + locale.Msg(langCode, eggMsgID)
		} else if eggMsgID, ok := paskhalochky[winAmount]; ok {
			easterEggStr = "\n" + locale.Msg(langCode, eggMsgID)
		}

		finalMessage := locale.Msgf(langCode, otherMsgs.MessageTemplate,
			locale.KV("slots_result", slotsResultText),
			locale.KV("win_loss_text", winLossText),
			locale.KV("easter_egg", easterEggStr),
			locale.KV("balance", ctx.DBUser.Duzhocoins),
			locale.KV("duzhocoins", locale.GlobalIDs.Nouns.Duzhocoins.ConstructSafe(langCode, locale.CaseGen, int(ctx.DBUser.Duzhocoins))),
		)

		bgCtx := context.Background()
		api := ctx.Api
		sender := ctx.Sender
		inputPeer := ctx.InputPeer
		replyToMsgID := ctx.Message.ID

		go func() {
			time.Sleep(5 * time.Second)
			upd, err := sender.To(inputPeer).Reply(replyToMsgID).Text(bgCtx, finalMessage)
			if err != nil {
				qsh.Errorf("casino background result send error: %w", err)
				return
			}

			var sentMsgID int
			if updates, ok := upd.(*tg.Updates); ok {
				for _, u := range updates.Updates {
					switch inner := u.(type) {
					case *tg.UpdateNewMessage:
						if m, ok := inner.Message.(*tg.Message); ok {
							sentMsgID = m.ID
						}
					case *tg.UpdateNewChannelMessage:
						if m, ok := inner.Message.(*tg.Message); ok {
							sentMsgID = m.ID
						}
					}
				}
			}

			time.Sleep(12 * time.Second)

			idsToDelete := []int{replyToMsgID, diceMsgID}
			if sentMsgID != 0 {
				idsToDelete = append(idsToDelete, sentMsgID)
			}

			if _, err := api.MessagesDeleteMessages(bgCtx, &tg.MessagesDeleteMessagesRequest{
				Revoke: true,
				ID:     idsToDelete,
			}); err != nil {
				qsh.Warnf("casino bulk context delete failed (%v), falling back to one-by-one", err)
				for _, id := range idsToDelete {
					if _, singleErr := api.MessagesDeleteMessages(bgCtx, &tg.MessagesDeleteMessagesRequest{
						Revoke: true,
						ID:     []int{id},
					}); singleErr != nil {
						qsh.Errorf("casino fallback delete for msg[%d] failed: %v", id, singleErr)
					}
				}
			}
		}()

		return false, nil
	}

	register(ID, &otherMsgs, exec)
}
