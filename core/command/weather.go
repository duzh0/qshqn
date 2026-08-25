package command

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/tg"

	"qshqn/core/config"
	"qshqn/core/locale"
	"qshqn/core/netx"
)

func init() {
	const baseURL = "http://api.openweathermap.org/data/2.5/weather?q=%s&appid=%s&units=metric&lang=%s"
	const cityNotFoundString = "city not found"

	type WeatherError struct {
		Cod     string `json:"cod"`
		Message string `json:"message"`
	}

	type WeatherData struct {
		Name     string `json:"name"`
		Timezone int    `json:"timezone"`
		Coord    struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"coord"`
		Main struct {
			Temp      float64 `json:"temp"`
			FeelsLike float64 `json:"feels_like"`
			Humidity  int     `json:"humidity"`
			Pressure  int     `json:"pressure"`
		} `json:"main"`
		Wind struct {
			Speed float64 `json:"speed"`
		} `json:"wind"`
		Sys struct {
			Country string `json:"country"`
			Sunrise int64  `json:"sunrise"`
			Sunset  int64  `json:"sunset"`
		} `json:"sys"`
		Weather []struct {
			ID   int    `json:"id"`
			Desc string `json:"description"`
			Icon string `json:"icon"`
		} `json:"weather"`
	}

	getWeatherEmoji := func(ID int, icon string) string {
		switch {
		case ID >= 200 && ID < 300:
			return "⛈️"
		case ID >= 300 && ID < 400:
			return "🌦️"
		case ID >= 500 && ID < 600:
			return "🌧️"
		case ID >= 600 && ID < 700:
			return "❄️"
		case ID >= 700 && ID < 800:
			return "🌫️"
		case ID == 800:
			if strings.HasSuffix(icon, "n") {
				return "🌙"
			}
			return "☀️"
		case ID == 801:
			if strings.HasSuffix(icon, "n") {
				return "🌥️"
			}
			return "🌤️"
		case ID == 802:
			return "⛅"
		case ID >= 803 && ID < 808:
			return "☁️"
		default:
			return "🌤️"
		}
	}

	var clocks = [12][2]string{
		{"🕛", "🕧"}, // 0/12
		{"🕐", "🕜"}, // 1
		{"🕑", "🕝"}, // 2
		{"🕒", "🕞"}, // 3
		{"🕓", "🕟"}, // 4
		{"🕔", "🕠"}, // 5
		{"🕕", "🕡"}, // 6
		{"🕖", "🕢"}, // 7
		{"🕗", "🕣"}, // 8
		{"🕘", "🕤"}, // 9
		{"🕙", "🕥"}, // 10
		{"🕚", "🕦"}, // 11
	}

	capitalize := func(s string) string {
		if s == "" {
			return ""
		}

		lower := strings.ToLower(s)

		r, size := utf8.DecodeRuneInString(lower)
		if r == utf8.RuneError {
			return lower
		}

		return string(unicode.ToUpper(r)) + lower[size:]
	}

	getClock := func(now time.Time) string {
		return clocks[now.Hour()%12][now.Minute()/30]
	}

	isRu := func(countryCode string) bool {
		return strings.ToUpper(countryCode) == "RU"
	}

	getFlag := func(countryCode string) string {
		if isRu(countryCode) {
			return config.RU_FLAGS[rand.Intn(config.RU_FLAGS_LEN)]
		}

		var res strings.Builder
		for _, r := range strings.ToUpper(countryCode) {
			res.WriteRune(r + 127397)
		}
		return res.String()
	}

	getCountryName := func(countryCode string, langCode locale.LangCode) string {
		if isRu(countryCode) {
			names := strings.Split(locale.Msg(langCode, locale.GlobalIDs.RuNames), locale.ARRAY_SEPARATOR)
			if namesLen := len(names); namesLen > 0 {
				return names[rand.Intn(namesLen)]
			}

			return config.DEFAULT_RU_NAME_LAT
		}

		tag, err := language.Parse(langCode.String())
		if err != nil {
			tag = language.English
		}

		region, err := language.ParseRegion(countryCode)
		if err != nil {
			return countryCode
		}

		namer := display.Regions(tag)
		if name := namer.Name(region); name != "" {
			return name
		}

		return countryCode
	}

	timeFmt := func(t time.Time) string { return t.Format("15:04") }

	ID := "weather"
	otherMsgs := struct {
		CityNotSpecified,
		APIError,
		NetxError,
		CityNotFound,
		NoWeatherDescription,
		ResponseTemplate locale.MsgID
	}{}
	exec := func(ctx *Context) (passthrough bool, err error) {
		langCode := ctx.DBUser.LangCode
		if config.Services.Weather.Token == "" {
			ctx.ReplyReportErrLocaleMsg(locale.GlobalIDs.Error)
			return false, fmt.Errorf("weather api key missing")
		}

		if len(ctx.Args) < 2 {
			ctx.ReplyReportErrLocaleMsg(otherMsgs.CityNotSpecified)
			return false, nil
		}

		cityRaw := strings.Join(ctx.Args[1:], " ")
		cityEscaped := url.QueryEscape(cityRaw)
		targetURL := fmt.Sprintf(baseURL, cityEscaped, config.Services.Weather.Token, langCode)

		ctxWithTimeout, cancel := context.WithTimeout(ctx.Ctx, 10*time.Second)
		defer cancel()

		data, err := netx.GetJSON[WeatherData](ctxWithTimeout, targetURL, nil)
		if err != nil {
			if apiErr, ok := err.(*netx.HTTPError); ok {
				var wErr WeatherError
				jErr := json.Unmarshal(apiErr.Body, &wErr)
				if jErr == nil && wErr.Message == cityNotFoundString {

					// API failed, query is long, and not strict mode = passthrough
					if len(ctx.Args) > 4 && !ctx.Strict {
						return true, nil
					}

					// otherwise (short query typo/forced strict mode) return error
					ctx.ReplyReportErrLocaleMsg(otherMsgs.CityNotFound)
					return false, nil
				}

				ctx.ReplyReportErrLocaleMsg(otherMsgs.APIError)
				return false, fmt.Errorf("weather api error: %w", err)
			}

			ctx.ReplyReportErrLocaleMsg(otherMsgs.NetxError)
			return false, fmt.Errorf("netx get json error: %w", err)
		}

		countryCode := data.Sys.Country

		flag := getFlag(countryCode)
		countryName := getCountryName(countryCode, langCode)

		tz := time.Duration(data.Timezone) * time.Second
		nowLocal := time.Now().UTC().Add(tz)

		clock := getClock(nowLocal)

		sign := "+"
		if tz < 0 {
			sign = "-"
		}

		absTz := tz.Abs()
		hours := int(absTz / time.Hour)
		minutes := int((absTz % time.Hour) / time.Minute)

		var tzString string
		if minutes == 0 {
			tzString = fmt.Sprintf("%s%d", sign, hours)
		} else {
			tzString = fmt.Sprintf("%s%d:%02d", sign, hours, minutes)
		}

		weatherDesc := ""
		weatherEmoji := "🌤️"
		if len(data.Weather) > 0 {
			weatherDesc = strings.ToUpper(data.Weather[0].Desc)
			weatherEmoji = getWeatherEmoji(data.Weather[0].ID, data.Weather[0].Icon)
		} else {
			weatherDesc = locale.Msg(langCode, otherMsgs.NoWeatherDescription)
		}

		sunriseTime := time.Unix(data.Sys.Sunrise, 0).UTC().Add(tz)
		sunsetTime := time.Unix(data.Sys.Sunset, 0).UTC().Add(tz)

		location := &tg.InputMediaGeoPoint{
			GeoPoint: &tg.InputGeoPoint{
				Lat:  data.Coord.Lat,
				Long: data.Coord.Lon,
			},
		}

		formattedText := locale.Msgf(
			langCode,
			otherMsgs.ResponseTemplate,
			locale.KV("flag", flag),
			locale.KV("city", data.Name),
			locale.KV("country", countryName),
			locale.KV("clock", clock),
			locale.KV("local_time", timeFmt(nowLocal)),
			locale.KV("utc_tz", tzString),
			locale.KV("weather_emoji", weatherEmoji),
			locale.KV("weather_description", capitalize(weatherDesc)),
			locale.KV("temperature", data.Main.Temp),
			locale.KV("feels_like", data.Main.FeelsLike),
			locale.KV("humidity", data.Main.Humidity),
			locale.KV("pressure", data.Main.Pressure),
			locale.KV("wind_speed", data.Wind.Speed),
			locale.KV("sunrise", timeFmt(sunriseTime)),
			locale.KV("sunset", timeFmt(sunsetTime)),
		)
		caption := html.String(nil, formattedText)

		locMedia := message.Media(
			location,
			caption,
		)

		_, err = ctx.Reply().Media(ctx.Ctx, locMedia)
		return false, err
	}

	register(ID, &otherMsgs, exec)
}
