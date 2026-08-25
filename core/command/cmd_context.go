package command

import (
	"context"
	"errors"
	"time"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"qshqn/core/db"
	"qshqn/core/locale"
	"qshqn/core/qsh"
	"qshqn/core/typex"
	"qshqn/core/util/stringx"
)

var (
	ctxPool = typex.NewPool(func() *Context {
		return &Context{
			Args: make([]string, 0, 4),
		}
	})
)

func extractIDFromUpdates(upd tg.UpdatesClass) int {
	if upd == nil {
		return 0
	}
	switch u := upd.(type) {
	case *tg.Updates:
		for _, inner := range u.Updates {
			switch ev := inner.(type) {
			case *tg.UpdateNewMessage:
				if msg, ok := ev.Message.(*tg.Message); ok {
					return msg.ID
				}
			case *tg.UpdateNewChannelMessage:
				if msg, ok := ev.Message.(*tg.Message); ok {
					return msg.ID
				}
			}
		}
	}
	return 0
}

func sendWithRetry(ctx context.Context, sendFunc func() (tg.UpdatesClass, error)) (tg.UpdatesClass, error) {
	for {
		upd, err := sendFunc()
		if err == nil {
			return upd, nil
		}

		if d, ok := tgerr.AsFloodWait(err); ok {
			qsh.Warnf("Rate limited (FLOOD_WAIT). Pausing execution for %v", d)
			select {
			case <-time.After(d + 500*time.Millisecond):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		if rpcErr, ok := tgerr.AsType(err, "SLOWMODE_WAIT"); ok {
			d := time.Second * time.Duration(rpcErr.Argument)
			qsh.Warnf("Group has slowmode enabled. Waiting for %v", d)
			select {
			case <-time.After(d + 500*time.Millisecond):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		return nil, err
	}
}

func splitText(text string, limit int) []string {
	var chunks []string
	runes := []rune(text)
	for len(runes) > limit {
		splitAt := limit
		found := false

		for i := limit - 1; i >= limit-400 && i > 0; i-- {
			if runes[i] == '\n' {
				splitAt = i + 1
				found = true
				break
			}
		}

		if !found {
			for i := limit - 1; i >= limit-150 && i > 0; i-- {
				if runes[i] == ' ' {
					splitAt = i + 1
					found = true
					break
				}
			}
		}

		chunks = append(chunks, string(runes[:splitAt]))
		runes = runes[splitAt:]
	}

	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}

func (c *Context) sendTextInternal(text string, firstReplyID int) (tg.UpdatesClass, error) {
	const limit = 4000
	const maxSendDuration = 60 * time.Second

	sendCtx, cancel := context.WithTimeout(c.Ctx, maxSendDuration)
	defer cancel()

	if len(text) <= limit {
		return sendWithRetry(sendCtx, func() (tg.UpdatesClass, error) {
			if firstReplyID != 0 {
				return c.Reply().Text(sendCtx, text)
			}
			return c.ToPeer().Text(sendCtx, text)
		})
	}

	chunks := splitText(text, limit)
	var lastUpdate tg.UpdatesClass
	var err error

	lastUpdate, err = sendWithRetry(sendCtx, func() (tg.UpdatesClass, error) {
		if firstReplyID != 0 {
			return c.Reply().Text(sendCtx, chunks[0])
		}
		return c.ToPeer().Text(sendCtx, chunks[0])
	})
	if err != nil {
		return nil, err
	}

	lastMsgID := extractIDFromUpdates(lastUpdate)

	for _, chunk := range chunks[1:] {
		lastUpdate, err = sendWithRetry(sendCtx, func() (tg.UpdatesClass, error) {
			if lastMsgID != 0 {
				return c.ToPeer().Reply(lastMsgID).Text(sendCtx, chunk)
			}
			return c.ToPeer().Text(sendCtx, chunk)
		})

		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				qsh.Warnf("Sending budget (%d) exceeded. Dropped remaining %d chunks.", maxSendDuration, len(chunks)-len(chunks[:2]))
				return lastUpdate, nil
			}
			return nil, err
		}

		lastMsgID = extractIDFromUpdates(lastUpdate)
	}

	return lastUpdate, nil
}

type Context struct {
	Ctx     context.Context
	Command *Command

	Api    *tg.Client
	Sender *message.Sender

	IsComment bool
	From      typex.From
	Bot       typex.BotInfo
	DBUser    *db.User

	InputPeer tg.InputPeerClass
	PeerID    int64
	Message   *tg.Message

	Args    []string
	Payload string

	Strict           bool
	SkipDispatchLock bool

	RepliedTo *typex.CachedMsg
}

func (c *Context) LocaleMsgExistsFor(ID locale.MsgID) bool {
	return locale.MsgExistsFor(c.DBUser.LangCode, ID)
}

func (c *Context) LocaleMsgRaw(ID locale.MsgID) string { return locale.MsgRaw(c.DBUser.LangCode, ID) }
func (c *Context) LocaleMsg(ID locale.MsgID) string    { return locale.Msg(c.DBUser.LangCode, ID) }
func (c *Context) LocaleMsgf(ID locale.MsgID, kv ...locale.KVPair) string {
	return locale.Msgf(c.DBUser.LangCode, ID, kv...)
}

func (c *Context) ToPeer() *message.RequestBuilder {
	return c.Sender.To(c.InputPeer)
}

func (c *Context) Reply() *message.Builder {
	return c.ToPeer().Reply(c.Message.ID)
}

func (c *Context) ReplyText(text string) (tg.UpdatesClass, error) {
	return c.sendTextInternal(text, c.Message.ID)
}

func (c *Context) SendText(text string) (tg.UpdatesClass, error) {
	return c.sendTextInternal(text, 0)
}

func (c *Context) ReplyTextRaw(text string) (tg.UpdatesClass, error) {
	return c.Reply().Text(c.Ctx, text)
}

func (c *Context) ReplyMediaRaw(media message.MediaOption) (tg.UpdatesClass, error) {
	return c.Reply().Media(c.Ctx, media)
}

func (c *Context) SendTextRaw(text string) (tg.UpdatesClass, error) {
	return c.ToPeer().Text(c.Ctx, text)
}

func (c *Context) ReplyReportErr(msg string) (tg.UpdatesClass, error) {
	upd, err := c.ReplyText(msg)
	if err != nil {
		qsh.Errorf("error sending message [%s]: %w", msg, err)
	}
	return upd, err
}

func (c *Context) ReplyLocaleMsg(ID locale.MsgID) (tg.UpdatesClass, error) {
	return c.ReplyText(locale.Msg(c.DBUser.LangCode, ID))
}

func (c *Context) ReplyLocaleMsgf(ID locale.MsgID, kv ...locale.KVPair) (tg.UpdatesClass, error) {
	return c.ReplyText(locale.Msgf(c.DBUser.LangCode, ID, kv...))
}

func (c *Context) ReplyReportErrLocaleMsgf(ID locale.MsgID, kv ...locale.KVPair) (tg.UpdatesClass, error) {
	upd, err := c.ReplyLocaleMsgf(ID, kv...)
	if err != nil {
		qsh.Errorf("error sending message ID[%s]: %w", ID.String(), err)
	}
	return upd, err
}

func (c *Context) ReplyReportErrLocaleMsg(ID locale.MsgID) (tg.UpdatesClass, error) {
	upd, err := c.ReplyLocaleMsg(ID)
	if err != nil {
		qsh.Errorf("error sending message ID[%s]: %w", ID.String(), err)
	}
	return upd, err
}

func (c *Context) StartAction(action tg.SendMessageActionClass) context.CancelFunc {
	ctx, cancel := context.WithTimeout(c.Ctx, 5*time.Minute)

	err := c.Sender.To(c.InputPeer).TypingAction().Custom(ctx, action)
	if err != nil {
		cancel()
		return cancel
	}

	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := c.Sender.To(c.InputPeer).TypingAction().Custom(ctx, action); err != nil {
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return cancel
}

func (c *Context) StartTyping() context.CancelFunc {
	return c.StartAction(&tg.SendMessageTypingAction{})
}

func (c *Context) Put() {
	args := c.Args[:0]
	clear(args)

	*c = Context{}

	c.Args = args
	ctxPool.Put(c)
}

func GetContext(
	ctx context.Context,
	api *tg.Client,
	sender *message.Sender,
	isComment bool,
	from typex.From,
	bot typex.BotInfo,
	dbUser *db.User,
	inputPeer tg.InputPeerClass,
	peerID int64,
	msg *tg.Message,
	payload string,
	repliedTo *typex.CachedMsg,
) *Context {
	c := ctxPool.Get()
	c.Ctx = ctx
	c.Api = api
	c.Sender = sender
	c.IsComment = isComment
	c.From = from
	c.Bot = bot
	c.DBUser = dbUser
	c.InputPeer = inputPeer
	c.PeerID = peerID
	c.Message = msg
	c.RepliedTo = repliedTo
	c.Args = c.Args[:0]
	stringx.Parts(payload, &c.Args)
	if len(c.Args) > 0 {
		rawKeyword := c.Args[0]
		if len(rawKeyword) >= StrictCommandIdentifierLen && rawKeyword[0:StrictCommandIdentifierLen] == StrictCommandIdentifier {
			c.Strict = true
			c.Args[0] = rawKeyword[StrictCommandIdentifierLen:]
		}
	}
	c.Payload = payload
	return c
}
