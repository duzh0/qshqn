package tgx

import (
	"context"

	"fmt"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"

	"qshqn/core/db"
	"qshqn/core/qsh"
	"qshqn/core/typex"
	"qshqn/core/util/tgutil"
)

const (
	OWN_MSG_CACHE_SIZE   = 2000
	OTHER_MSG_CACHE_SIZE = 2000

	MAX_INLINE_ARTICLES = 50
)

type LinkedChannel struct {
	Checked bool
	ID      int64
}

type WhitelistedChat struct {
	LinkedChannel LinkedChannel
}

type Manager struct {
	botToken string

	ownMsgCache   *typex.MsgCache
	otherMsgCache *typex.MsgCache

	self           typex.BotInfo
	api            *tg.Client
	client         *telegram.Client
	sessionStorage *session.FileStorage
	dispatcher     *tg.UpdateDispatcher
	sender         *message.Sender
	updater        *updates.Manager

	whitelistedChats *typex.Map[int64, *WhitelistedChat]
}

func (m *Manager) run(ctx context.Context) error {
	if _, err := m.client.Auth().Bot(ctx, m.botToken); err != nil {
		return fmt.Errorf("tg bot auth error: %w\n", err)
	}

	self, err := m.client.Self(ctx)
	if err != nil {
		return fmt.Errorf("tg get self error: %w\n", err)
	}

	m.self = typex.BotInfo{
		ID:       self.ID,
		FullName: tgutil.GetUserName(self),
		Username: self.Username,
	}
	qsh.Infof("logged in successfully as %s (%d, @%s)\n", m.self.FullName, self.ID, self.Username)

	m.api = m.client.API()
	m.sender = message.NewSender(m.api)

	m.dispatcher.OnBotInlineQuery(m.catchInlineQuery)
	m.dispatcher.OnBotCallbackQuery(m.catchBotCallbackQuery)

	m.dispatcher.OnEditMessage(m.catchEditMessageUpdate)
	m.dispatcher.OnEditChannelMessage(m.catchEditChannelMessageUpdate)

	m.dispatcher.OnNewMessage(m.catchNewMessageUpdate)
	m.dispatcher.OnNewChannelMessage(m.catchNewChannelMessageUpdate)

	<-ctx.Done()
	return nil

	// return m.updater.Run(ctx, m.api, self.ID, updates.AuthOptions{
	// 	IsBot: true,
	// 	OnStart: func(ctx context.Context) {
	// 		qsh.Infof("updater started")
	// 	},
	// })
}
func (m *Manager) Run(ctx context.Context) error {
	return m.client.Run(ctx, m.run)
}

func (m *Manager) catchInlineQuery(ctx context.Context, ent tg.Entities, upd *tg.UpdateBotInlineQuery) error {
	go safeHandle(func() error {
		err := m.handleInlineQuery(ctx, ent, upd)
		if err != nil {
			return fmt.Errorf("error handling inline query: %w", err)
		}

		return nil
	})

	return nil
}

func (m *Manager) catchEditMessageUpdate(ctx context.Context, ent tg.Entities, upd *tg.UpdateEditMessage) error {
	go safeHandle(func() error {
		err := m.handleEditMsg(ctx, ent, upd.Message)
		if err != nil {
			return fmt.Errorf("error handling edit message: %w", err)
		}
		return nil
	})
	return nil
}

func (m *Manager) catchEditChannelMessageUpdate(ctx context.Context, ent tg.Entities, upd *tg.UpdateEditChannelMessage) error {
	go safeHandle(func() error {
		err := m.handleEditMsg(ctx, ent, upd.Message)
		if err != nil {
			return fmt.Errorf("error handling edit channel message: %w", err)
		}
		return nil
	})
	return nil
}

func (m *Manager) catchNewMessageUpdate(ctx context.Context, ent tg.Entities, upd *tg.UpdateNewMessage) error {
	go safeHandle(func() error {
		err := m.handleNewMsg(ctx, ent, upd.Message)
		if err != nil {
			return fmt.Errorf("error handling new message: %w", err)
		}

		return nil
	})
	return nil
}
func (m *Manager) catchNewChannelMessageUpdate(ctx context.Context, ent tg.Entities, upd *tg.UpdateNewChannelMessage) error {
	go safeHandle(func() error {
		err := m.handleNewMsg(ctx, ent, upd.Message)
		if err != nil {
			return fmt.Errorf("error handling new channel message: %w", err)
		}

		return nil
	})

	return nil
}

func (m *Manager) LeaveChat(ctx context.Context, peer tg.InputPeerClass) (tg.UpdatesClass, error) {
	switch p := peer.(type) {
	case *tg.InputPeerChannel:
		return m.api.ChannelsLeaveChannel(ctx, &tg.InputChannel{
			ChannelID:  p.ChannelID,
			AccessHash: p.AccessHash,
		})

	case *tg.InputPeerChat:
		return m.api.MessagesDeleteChatUser(ctx, &tg.MessagesDeleteChatUserRequest{
			ChatID: p.ChatID,
			UserID: &tg.InputUserSelf{},
		})

	default:
		return nil, fmt.Errorf("m.LeaveChat() expects InputPeerChannel or InputPeerChat, got %T", p)
	}
}

func New(apiID int, apiHash, botToken, sessionPath string) (*Manager, error) {
	owncache, err := typex.NewMsgCache(OWN_MSG_CACHE_SIZE)
	if err != nil {
		return nil, fmt.Errorf("own msgs cache init error: %w", err)
	}
	othercache, err := typex.NewMsgCache(OTHER_MSG_CACHE_SIZE)
	if err != nil {
		return nil, fmt.Errorf("other msgs cache init error: %w", err)
	}
	dispatcher := tg.NewUpdateDispatcher()
	updater := updates.New(updates.Config{Handler: dispatcher})
	sessionStorage := &session.FileStorage{Path: sessionPath}
	opts := telegram.Options{
		SessionStorage: sessionStorage,
		UpdateHandler:  updater,
	}
	client := telegram.NewClient(apiID, apiHash, opts)
	chatIDs, err := db.Keys[db.Chat]()
	if err != nil {
		return nil, fmt.Errorf("db chats keys fetch error: %w", err)
	}

	chatIDsMap := typex.NewMap[int64, *WhitelistedChat](0)
	for _, id := range chatIDs {
		chatIDsMap.Set(id, &WhitelistedChat{})
	}

	return &Manager{
		botToken:         botToken,
		ownMsgCache:      owncache,
		otherMsgCache:    othercache,
		client:           client,
		sessionStorage:   sessionStorage,
		dispatcher:       &dispatcher,
		updater:          updater,
		whitelistedChats: chatIDsMap,
	}, nil
}
