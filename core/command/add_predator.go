package command

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"qshqn/core/config"
	"qshqn/core/db"
	"qshqn/core/qsh"

	"github.com/gotd/td/tg"
)

func init() {
	tMeRegex := regexp.MustCompile(`(?:https?://)?t\.me/([a-zA-Z0-9_]+)/(\d+)`)
	const targetFwdName = "𝙿𝚛𝚎𝚍𝚊𝚝𝚘𝚛 ✙"
	const batchSize = 200

	runFetcher := func(bgCtx *Context, username string, startID int) {
		notify := func(text string) {
			if _, err := bgCtx.SendText(text); err != nil {
				qsh.Errorf("fetcher failed to send report to chat: %v", err)
			}
		}

		resolved, err := bgCtx.Api.ContactsResolveUsername(bgCtx.Ctx, &tg.ContactsResolveUsernameRequest{Username: username})
		if err != nil {
			notify(fmt.Sprintf("❌ resolve err for %s: %v", username, err))
			return
		}

		var inputChannel *tg.InputChannel
		for _, c := range resolved.Chats {
			if ch, ok := c.(*tg.Channel); ok {
				inputChannel = &tg.InputChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
				break
			}
		}

		if inputChannel == nil {
			notify("❌ channel not found in resolved chats")
			return
		}

		currentID := startID
		lastSeenID := startID
		emptyBatches := 0

		for {
			var ids []tg.InputMessageClass
			for i := range batchSize {
				ids = append(ids, &tg.InputMessageID{ID: currentID + i})
			}

			notify(fmt.Sprintf("⏳ fetching %d - %d...", currentID, currentID+batchSize-1))

			resp, err := bgCtx.Api.ChannelsGetMessages(bgCtx.Ctx, &tg.ChannelsGetMessagesRequest{
				Channel: inputChannel,
				ID:      ids,
			})

			if err != nil {
				notify(fmt.Sprintf("❌ fetch err at %d - %d: %v\nlast link: https://t.me/%s/%d", currentID, currentID+batchSize-1, err, username, lastSeenID))
				break
			}

			var msgs []tg.MessageClass
			if m, ok := resp.(interface{ GetMessages() []tg.MessageClass }); ok {
				msgs = m.GetMessages()
			}

			var addedTexts []string
			validMsgsCount := 0

			for _, m := range msgs {
				switch msg := m.(type) {
				case *tg.Message:
					validMsgsCount++
					if msg.ID > lastSeenID {
						lastSeenID = msg.ID
					}
					if msg.Message != "" && !msg.FwdFrom.Zero() {
						if msg.FwdFrom.FromName == targetFwdName {
							inserted, err := db.AddPredatorMsg(msg.Message)
							if err != nil {
								notify(fmt.Sprintf("❌ db insert err for msg id %d: %v", msg.ID, err))
								continue
							}
							if inserted {
								addedTexts = append(addedTexts, msg.Message)
							}
						}
					}
				case *tg.MessageEmpty:
				}
			}

			if len(addedTexts) > 0 {
				report := fmt.Sprintf("✅ added %d messages from batch %d - %d:\n\n%s",
					len(addedTexts), currentID, currentID+batchSize-1, strings.Join(addedTexts, "\n---\n"))
				notify(report)
			} else {
				notify(fmt.Sprintf("ℹ️ no target messages found (or all duplicates) in %d - %d.", currentID, currentID+batchSize-1))
			}

			if validMsgsCount == 0 {
				emptyBatches++
			} else {
				emptyBatches = 0
			}

			if emptyBatches >= 2 {
				notify(fmt.Sprintf("🏁 reached end of channel (2 empty batches in a row). done.\nlast link: https://t.me/%s/%d", username, lastSeenID))
				break
			}

			currentID += batchSize
			time.Sleep(30 * time.Second)
		}
	}

	RegisterInterceptor(func(ctx *Context) bool {
		if ctx.From.ID != config.Tg.OwnerID {
			return false
		}
		if _, ok := ctx.InputPeer.(*tg.InputPeerUser); !ok {
			return false
		}
		if ctx.Message == nil || ctx.Message.Message == "" {
			return false
		}

		payload := strings.TrimSpace(ctx.Message.Message)
		matches := tMeRegex.FindStringSubmatch(payload)
		if len(matches) == 3 {
			username := matches[1]
			startID, _ := strconv.Atoi(matches[2])

			bgCtx := &Context{
				Ctx:       context.WithoutCancel(ctx.Ctx),
				Api:       ctx.Api,
				Sender:    ctx.Sender,
				InputPeer: ctx.InputPeer,
			}

			go runFetcher(bgCtx, username, startID)

			_, err := ctx.ReplyText("✅ started fetching in background")
			if err != nil {
				qsh.Errorf("failed to send fetcher start ack: %v", err)
			}
			return true
		}

		return false
	})
}
