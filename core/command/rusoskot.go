package command

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/gotd/td/telegram/message/html"

	"qshqn/core/config"
	"qshqn/core/fiox"
	"qshqn/core/locale"
	"qshqn/core/netx"
	"qshqn/core/qsh"
)

func init() {
	type RuLossesResponse struct {
		Legend map[string]string         `json:"legend"`
		Data   map[string]map[string]int `json:"data"`
	}

	type StatEntry struct {
		Emoji string
		MsgID locale.MsgID
	}

	warStart, err := time.Parse("02.01.2006", "24.02.2022")
	if err != nil {
		panic(fmt.Errorf("error parsing war start date in rusoskot: %w", err))
	}

	fetchFunc := func(ctx context.Context, path, endpoint string) (RuLossesResponse, error) {
		data, err := fiox.Load[RuLossesResponse](path, fiox.ReadCache, fiox.SetCache)
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006.01.02")
		if !(err != nil || (data.Data != nil && data.Data[yesterday] == nil)) {
			return data, nil
		}

		data, err = netx.GetJSON[RuLossesResponse](ctx, endpoint, nil)
		if err != nil {
			return data, err
		}

		if err = fiox.Save(path, data, fiox.CreateOrUpdate, fiox.SetCache); err != nil {
			qsh.Errorf("error saving rusoskot data at [%s]: %w", path, err)
		}

		return data, nil
	}

	ID := "rusoskot"
	otherMsgs := struct {
		AllFetchErrored,
		LegendError,
		ResponseMessage,
		ValueUnknown,
		RangeCorrectUsage,
		OutOfRange,
		StatTanks,
		StatApv,
		StatArtillery,
		StatMlrs,
		StatAaws,
		StatAircraft,
		StatHelicopters,
		StatUav,
		StatVehicles,
		StatBoats,
		StatSe,
		StatMissiles,
		StatPersonnel,
		StatCaptive locale.MsgID
	}{}
	var statMap map[string]StatEntry
	OnPkgInit(func() error {
		statMap = map[string]StatEntry{
			"tanks":       {"🚜", otherMsgs.StatTanks},
			"apv":         {"🛡️", otherMsgs.StatApv},
			"artillery":   {"💥", otherMsgs.StatArtillery},
			"mlrs":        {"🚀", otherMsgs.StatMlrs},
			"aaws":        {"📡", otherMsgs.StatAaws},
			"aircraft":    {"🛩️", otherMsgs.StatAircraft},
			"helicopters": {"🚁", otherMsgs.StatHelicopters},
			"uav":         {"🛸", otherMsgs.StatUav},
			"vehicles":    {"🚚", otherMsgs.StatVehicles},
			"boats":       {"🚤", otherMsgs.StatBoats},
			"se":          {"⚙️", otherMsgs.StatSe},
			"missiles":    {"☄️", otherMsgs.StatMissiles},
			"personnel":   {"🐷", otherMsgs.StatPersonnel},
			"captive":     {"🏳️", otherMsgs.StatCaptive},
		}
		return nil
	})
	var orderedKeys = []string{
		"personnel",
		"tanks",
		"apv",
		"artillery",
		"mlrs",
		"aaws",
		"aircraft",
		"helicopters",
		"uav",
		"missiles",
		"boats",
		"vehicles",
		"se",
		// "captive", not present in the api but present in legend?
	}
	parseToRange := func(s string) (time.Time, time.Time, error) {
		layouts := []struct {
			layout string
			isDay  bool
		}{
			{"02.01.2006", true},
			{"02.01.06", true},
			{"01.2006", false},
			{"01.06", false},
		}
		for _, l := range layouts {
			if t, err := time.Parse(l.layout, s); err == nil {
				if l.isDay {
					return t, t, nil
				}

				end := t.AddDate(0, 1, 0).AddDate(0, 0, -1)
				return t, end, nil
			}
		}
		return time.Time{}, time.Time{}, fmt.Errorf("invalid format")
	}
	exec := func(ctx *Context) (passthrough bool, err error) {
		if len(ctx.Args) > 2 {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.RangeCorrectUsage)
			return false, err
		}
		ctxWithTimeout, cancel := context.WithTimeout(ctx.Ctx, 10*time.Second)
		defer cancel()

		daily, err1 := fetchFunc(ctxWithTimeout, config.Services.Rusoskot.CachePathDaily, config.Services.Rusoskot.EndpointDaily)
		monthly, err2 := fetchFunc(ctxWithTimeout, config.Services.Rusoskot.CachePathMonthly, config.Services.Rusoskot.EndpointMonthly)
		if err1 != nil && err2 != nil {
			_, err3 := ctx.ReplyReportErrLocaleMsg(otherMsgs.AllFetchErrored)
			return false, errors.Join(err1, err2, err3)
		}

		unknownVal := ctx.LocaleMsg(otherMsgs.ValueUnknown)
		legend := daily.Legend
		if len(legend) == 0 {
			legend = monthly.Legend
		}
		if len(legend) == 0 {
			_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.LegendError)
			return false, errors.Join(fmt.Errorf("no legend found in neither daily nor monthly data"), err)
		}

		var periodTitle string
		results := make(map[string]string)

		if len(ctx.Args) <= 1 {
			now := time.Now()
			yesterday := now.AddDate(0, 0, -1)
			yKey := yesterday.Format("2006.01.02")
			mKey := yesterday.Format("2006.01")
			periodTitle = fmt.Sprintf("%s / %s", yesterday.Format("02.01.2006"), yesterday.Format("01.2006"))

			for key := range legend {
				dVal, mVal := unknownVal, unknownVal
				if v, ok := daily.Data[yKey][key]; ok {
					dVal = fmt.Sprint(v)
				}
				if v, ok := monthly.Data[mKey][key]; ok {
					mVal = fmt.Sprint(v)
				}
				results[key] = dVal + " / " + mVal
			}
		} else {
			arg := ctx.Args[1]
			parts := strings.Split(arg, "-")

			startS, endS, err := parseToRange(parts[0])
			if err != nil {
				_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.RangeCorrectUsage)
				return false, err
			}

			finalStart, finalEnd := startS, endS
			if len(parts) == 2 {
				_, endRange2, err := parseToRange(parts[1])
				if err != nil {
					_, err2 := ctx.ReplyReportErrLocaleMsg(otherMsgs.RangeCorrectUsage)
					return false, errors.Join(err, err2)
				}
				finalEnd = endRange2
			}

			if finalStart.Before(warStart) || finalEnd.After(time.Now()) || finalStart.After(finalEnd) {
				_, err := ctx.ReplyReportErrLocaleMsg(otherMsgs.OutOfRange)
				return false, err
			}

			periodTitle = fmt.Sprintf("%s — %s", finalStart.Format("02.01.06"), finalEnd.Format("02.01.06"))
			if finalStart.Equal(finalEnd) {
				periodTitle = finalStart.Format("02.01.06")
			}

			sums := make(map[string]int)
			foundAny := false
			for curr := finalStart; !curr.After(finalEnd); curr = curr.AddDate(0, 0, 1) {
				dayKey := curr.Format("2006.01.02")
				if dayData, ok := daily.Data[dayKey]; ok {
					foundAny = true
					for k := range legend {
						sums[k] += dayData[k]
					}
				}
			}

			for k := range legend {
				if !foundAny {
					results[k] = unknownVal
				} else {
					results[k] = fmt.Sprint(sums[k])
				}
			}
		}

		stats := &strings.Builder{}
		for _, key := range orderedKeys {
			apiName, ok := legend[key]
			if !ok {
				continue
			}

			var emoji, name string
			var entry StatEntry
			if entry, ok = statMap[key]; !ok {
				name = apiName
			} else {
				name = ctx.LocaleMsgRaw(entry.MsgID)
				emoji = entry.Emoji
				if emoji != "" {
					emoji += " "
				}
			}

			fmt.Fprintf(stats, "%s<b>%s</b>: %s\n", emoji, name, results[key])
		}

		var ruName string
		names := strings.Split(ctx.LocaleMsg(locale.GlobalIDs.RuNames), locale.ARRAY_SEPARATOR)
		if namesLen := len(names); namesLen > 0 {
			ruName = names[rand.Intn(namesLen)]
		} else {
			ruName = config.DEFAULT_RU_NAME_LAT
		}

		responseRaw := ctx.LocaleMsgf(otherMsgs.ResponseMessage,
			locale.KV("flag", config.RU_FLAGS[rand.Intn(len(config.RU_FLAGS))]),
			locale.KV("ru_name", ruName),
			locale.KV("period", periodTitle),
			locale.KV("stats", stats.String()),
		)

		_, err = ctx.Reply().StyledText(ctx.Ctx, html.String(nil, responseRaw))
		return false, err
	}
	register(ID, &otherMsgs, exec)
}
