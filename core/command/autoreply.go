package command

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"

	"github.com/gotd/td/fileid"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"

	"qshqn/core/config"
	"qshqn/core/db"
	"qshqn/core/qsh"
)

type AutoReply struct {
	ScanName  bool
	Condition func(ctx *Context) bool
	Exec      func(ctx *Context, name string, matches []string) bool
}

var (
	autoreplies         = make(map[string]AutoReply)
	autoreplyRegex      *regexp.Regexp
	autoreplyGroupNames []string

	uabelRegex  *regexp.Regexp
	triggerGifs map[string]*tg.InputMediaDocument
)

func RegisterAutoReply(name string, ar AutoReply) {
	if _, exists := autoreplies[name]; exists {
		panic(fmt.Sprintf("autoreply [%s] registered twice", name))
	}
	autoreplies[name] = ar
}

func init() {
	mediaOptionFromData := func(ID int64, AH int64) message.MediaOption {
		return message.Media(&tg.InputMediaDocument{
			ID: &tg.InputDocument{
				ID:         ID,
				AccessHash: AH,
			},
		})
	}
	sendMedia := func(ctx *Context, name string, media message.MediaOption) bool {
		qsh.Debugf("sending [%s] media", name)
		if _, err := ctx.ReplyMediaRaw(media); err != nil {
			qsh.Errorf("error sending [%s] media: %w", name, err)
		}
		return true
	}

	var golangMedia, luhanskMedia, petushokMedia, predatorMedia message.MediaOption

	RegisterAutoReply("golang", AutoReply{Exec: func(ctx *Context, name string, _ []string) bool {
		return sendMedia(ctx, name, golangMedia)
	}})
	RegisterAutoReply("luhansk", AutoReply{Exec: func(ctx *Context, name string, _ []string) bool {
		return sendMedia(ctx, name, luhanskMedia)
	}})
	RegisterAutoReply("petushok", AutoReply{Exec: func(ctx *Context, name string, _ []string) bool {
		return sendMedia(ctx, name, petushokMedia)
	}})
	RegisterAutoReply("predator", AutoReply{Exec: func(ctx *Context, name string, _ []string) bool {
		return sendMedia(ctx, name, predatorMedia)
	}})
	RegisterAutoReply("nixos", AutoReply{Exec: func(ctx *Context, name string, _ []string) bool {
		qsh.Debug("sending nixosdotcom string")
		ctx.ReplyReportErr("nixos.com")
		return true
	}})

	RegisterAutoReply("rusnia_gif", AutoReply{
		ScanName: true,
		Condition: func(ctx *Context) bool {
			if !ctx.IsComment {
				return false
			}

			fullText := strings.ToLower(ctx.Payload + " " + ctx.From.FullName)
			if uabelRegex.MatchString(fullText) {
				return false
			}

			return true
		},
		Exec: func(ctx *Context, name string, matches []string) bool {
			matchedChars := make(map[string]bool, len(matches))
			for _, m := range matches {
				matchedChars[strings.ToLower(m)] = true
			}

			var possibleGifs []*tg.InputMediaDocument
			if matchedChars["ъ"] || matchedChars["🇷🇺"] || matchedChars["ѣ"] {
				for _, doc := range triggerGifs {
					possibleGifs = append(possibleGifs, doc)
				}
			} else {
				for char := range matchedChars {
					if doc, ok := triggerGifs[char]; ok {
						possibleGifs = append(possibleGifs, doc)
					}
				}
			}

			if len(possibleGifs) == 0 {
				for _, doc := range triggerGifs {
					possibleGifs = append(possibleGifs, doc)
				}
			}

			doc := possibleGifs[rand.Intn(len(possibleGifs))]

			var caption string
			predatorMsgs, err := db.RandPredatorMsgs(1, 200, "русоскот", "русосвин", "крокус", "русня", "хуйло", "хуило", "чучело", "пассажирка", "русовыблядь", "крым", "заднеприводн", "zаднеприводн", "обосса", "терпил")
			if err == nil && len(predatorMsgs) > 0 {
				caption = predatorMsgs[0]
			}

			return sendMedia(ctx, name, message.Media(doc, styling.Plain(caption)))
		},
	})

	OnPkgInit(func() error {
		golangMedia = mediaOptionFromData(config.Tg.Media.Golang.ID, config.Tg.Media.Golang.AH)
		luhanskMedia = mediaOptionFromData(config.Tg.Media.Luhansk.ID, config.Tg.Media.Luhansk.AH)
		petushokMedia = mediaOptionFromData(config.Tg.Media.Petushok.ID, config.Tg.Media.Petushok.AH)
		predatorMedia = mediaOptionFromData(config.Tg.Media.Predator.ID, config.Tg.Media.Predator.AH)

		uabelRegex = regexp.MustCompile(`(?i)[іўєґї🇺🇦]`)

		decodeDoc := func(fileID string) *tg.InputMediaDocument {
			parsed, err := fileid.DecodeFileID(fileID)
			if err != nil {
				panic(fmt.Sprintf("invalid gif file id [%s]: %v", fileID, err))
			}
			return &tg.InputMediaDocument{
				ID: &tg.InputDocument{
					ID:            parsed.ID,
					AccessHash:    parsed.AccessHash,
					FileReference: parsed.FileReference,
				},
			}
		}

		triggerGifs = map[string]*tg.InputMediaDocument{
			"ы": decodeDoc("CgACAgQAAxkBAAIDompjlHs8DDYLM_BCd35u0nGFq8S9AALCFAACU_yYU_dbe9qujvR1IgQ"),
			"ё": decodeDoc("CgACAgIAAxkBAAIDoGpjlHuox0FkbEy0lArUQz2amEmMAAIsFAACaq35S12wwP9nKLeIIgQ"),
			"э": decodeDoc("CgACAgQAAxkBAAIDoWpjlHu1L4wMUS3ynFzCnNLKUBaKAAKSEAAC0YvQUDgyTFc82dEfIgQ"),
		}

		return initAutoReplies()
	})
}

func initAutoReplies() error {
	var validNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

	if len(autoreplies) == 0 && len(config.Tg.Autoreplies) == 0 {
		return nil
	}

	var errs []string
	for cfgName := range config.Tg.Autoreplies {
		if !validNameRegex.MatchString(cfgName) {
			errs = append(errs, fmt.Sprintf("config key [%s] contains invalid characters (only a-z, A-Z, 0-9, _ allowed)", cfgName))
		}
		if _, ok := autoreplies[cfgName]; !ok {
			errs = append(errs, fmt.Sprintf("key [%s] exists in config but has no handler", cfgName))
		}
	}

	var groups []string
	for name := range autoreplies {
		if !validNameRegex.MatchString(name) {
			errs = append(errs, fmt.Sprintf("handler [%s] contains invalid characters (only a-z, A-Z, 0-9, _ allowed)", name))
		}
		pattern, ok := config.Tg.Autoreplies[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("handler [%s] registered in code but missing from config", name))
			continue
		}
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			errs = append(errs, fmt.Sprintf("handler [%s] has empty pattern in config", name))
			continue
		}
		groups = append(groups, "(?P<"+name+">"+pattern+")")
	}

	if len(errs) > 0 {
		return fmt.Errorf("autoreply config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	autoreplyRegex = regexp.MustCompile("(?i)" + strings.Join(groups, "|"))
	autoreplyGroupNames = autoreplyRegex.SubexpNames()

	RegisterInterceptor(func(ctx *Context) bool {
		if autoreplyRegex == nil {
			return false
		}

		hitsMap := make(map[string][]string)

		if ctx.Payload != "" {
			for _, match := range autoreplyRegex.FindAllStringSubmatch(ctx.Payload, -1) {
				for i, name := range autoreplyGroupNames {
					if i != 0 && name != "" && match[i] != "" {
						hitsMap[name] = append(hitsMap[name], match[i])
					}
				}
			}
		}

		if ctx.From.FullName != "" {
			for _, match := range autoreplyRegex.FindAllStringSubmatch(ctx.From.FullName, -1) {
				for i, name := range autoreplyGroupNames {
					if i != 0 && name != "" && match[i] != "" {
						if ar, ok := autoreplies[name]; ok && ar.ScanName {
							hitsMap[name] = append(hitsMap[name], match[i])
						}
					}
				}
			}
		}

		if len(hitsMap) == 0 {
			return false
		}

		var validHits []string
		for name := range hitsMap {
			if ar, ok := autoreplies[name]; ok {
				if ar.Condition == nil || ar.Condition(ctx) {
					validHits = append(validHits, name)
				} else {
					delete(hitsMap, name)
				}
			}
		}

		if len(validHits) == 0 {
			return false
		}

		hit := validHits[rand.Intn(len(validHits))]
		return autoreplies[hit].Exec(ctx, hit, hitsMap[hit])
	})

	return nil
}
