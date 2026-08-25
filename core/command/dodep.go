package command

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"qshqn/core/locale"
	"qshqn/core/typex"
)

const DayInSeconds = 86400

type DodepConfirmResult struct {
	Confirmed bool
	MsgID     int
}

var (
	pendingDodeps     = typex.NewMap[int64, chan DodepConfirmResult](0)
	dodepConfirmWords = typex.NewSet[string](0)
)

func init() {
	ID := "dodep"
	otherMsgs := struct {
		ConfirmWords,
		Pending,
		AlreadyReceived,
		Prompt,
		Timeout,
		Success locale.MsgID
	}{}
	execDodep := func(ctx *Context) (passthroguh bool, err error) {
		if len(ctx.Args) != 1 && !ctx.Strict {
			return true, nil
		}

		now := time.Now().Unix()
		nextDodepTimestamp := ctx.DBUser.DodepTimestamp + DayInSeconds
		if now < nextDodepTimestamp {
			timeLeft := time.Duration(nextDodepTimestamp-now) * time.Second
			h := int(timeLeft.Hours())
			m := int(timeLeft.Minutes()) % 60
			s := int(timeLeft.Seconds()) % 60
			timeLeftString := fmt.Sprintf("%d%s %d%s %d%s",
				h, ctx.LocaleMsgRaw(locale.GlobalIDs.TimeHourShort),
				m, ctx.LocaleMsgRaw(locale.GlobalIDs.TimeMinuteShort),
				s, ctx.LocaleMsgRaw(locale.GlobalIDs.TimeSecondShort),
			)
			_, err := ctx.ReplyLocaleMsgf(otherMsgs.AlreadyReceived, locale.KV("time_left", timeLeftString))
			return false, err
		}

		confirmChan, set := pendingDodeps.SetIfAbsentLazy(ctx.DBUser.ID,
			func() chan DodepConfirmResult {
				return make(chan DodepConfirmResult, 1)
			},
		)
		if !set {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.Pending)
			return false, err
		}

		randAmount := rand.Intn(25) + 1
		defer pendingDodeps.Delete(ctx.DBUser.ID)

		promptMsg := ctx.LocaleMsgf(otherMsgs.Prompt,
			locale.KV("amount", randAmount),
			locale.KV("confirm", otherMsgs.ConfirmWords),
		)
		if _, err := ctx.ReplyReportErr(promptMsg); err != nil {
			return false, err
		}

		select {
		case r := <-confirmChan:
			if !r.Confirmed {
				_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.Timeout)
				return false, err
			}
			ctx.DBUser.Dodep = int64(randAmount)
			ctx.DBUser.DodepTimestamp = now

			if err := ctx.DBUser.Save(); err != nil {
				return false, fmt.Errorf("dodep failed to save dbuser[%d]: %w", ctx.DBUser.ID, err)
			}

			_, err := ctx.ReplyReportErrLocaleMsgf(otherMsgs.Success, locale.KV("amount", randAmount))
			return false, err

		case <-time.After(15 * time.Second):
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.Timeout)
			return false, err
		}
	}

	OnPkgInit(func() error {
		for code := range locale.AllLangs() {
			raw := locale.MsgRaw(code, otherMsgs.ConfirmWords)
			for word := range strings.SplitSeq(raw, locale.ARRAY_SEPARATOR) {
				if w := strings.TrimSpace(strings.ToLower(word)); w != "" {
					dodepConfirmWords.Add(w)
				}
			}
		}
		return nil
	})

	register(ID, &otherMsgs, execDodep)

	RegisterInterceptor(func(ctx *Context) bool {
		ch, exists := pendingDodeps.Get(ctx.DBUser.ID)
		if !exists {
			return false
		}

		text := strings.ToLower(strings.TrimSpace(ctx.Payload))
		select {
		case ch <- DodepConfirmResult{dodepConfirmWords.Has(text), ctx.Message.ID}:
		default:
		}
		return true
	})
}
