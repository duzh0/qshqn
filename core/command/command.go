package command

import (
	"fmt"
	"reflect"
	"strings"

	"qshqn/core/locale"
	"qshqn/core/qsh"
	"qshqn/core/typex"
)

const (
	StrictCommandIdentifier    = "."
	StrictCommandIdentifierLen = len(StrictCommandIdentifier)
)

var (
	commandsList     = map[string]*Command{}
	keywordTriggers  = map[KeywordTriggerKey]*Command{}
	executingUserIDs = typex.NewMap[int64, string](0)
	interceptors     = []InterceptorFunc{}
	pkgInitFuncs     = []func() error{}
)

type InterceptorFunc func(ctx *Context) bool

func RegisterInterceptor(function InterceptorFunc) { interceptors = append(interceptors, function) }

func PreDispatch(ctx *Context) bool {
	for _, intercept := range interceptors {
		if intercept(ctx) {
			return true
		}
	}
	return false
}

type KeywordTriggerKey struct {
	Code    locale.LangCode
	Keyword string
}

type ExecFunc func(ctx *Context) (passthrough bool, err error)

// concrete struct instead of interface + generic base
type Command struct {
	id       string
	keywords map[locale.LangCode][]string
	nameID   locale.MsgID
	helpID   locale.MsgID
	usageID  locale.MsgID
	msgIDs   []locale.MsgID
	exec     ExecFunc
}

func (c *Command) ID() string                             { return c.id }
func (c *Command) Keywords(code locale.LangCode) []string { return c.keywords[code] }
func (c *Command) Name(code locale.LangCode) string       { return locale.Msg(code, c.nameID) }
func (c *Command) MsgIDs() []locale.MsgID                 { return c.msgIDs }
func (c *Command) Exec(ctx *Context) (bool, error)        { return c.exec(ctx) }

func (c *Command) Help(code locale.LangCode) string {
	return locale.Msgf(code, c.helpID,
		locale.KV("name", c.nameID),
		locale.KV("trigger", locale.Stringf("%s [%s]", code.Lang().PreferredTrigger, c.keywords[code][0])),
	)
}

func (c *Command) Usage(code locale.LangCode) string {
	return locale.Msgf(code, c.usageID,
		locale.KV("trigger", code.Lang().PreferredTrigger),
		locale.KV("keyword", c.keywords[code][0]),
	)
}

func (c *Command) setKeywords(code locale.LangCode, keywords []string) {
	if len(keywords) < 1 {
		panic(fmt.Sprintf("empty keywords list for lang code [%s]", code))
	}
	if kws, ok := c.keywords[code]; ok {
		panic(fmt.Sprintf("keywords for lang code [%s] already exist: existing [%s], new [%s]", code, strings.Join(kws, ", "), strings.Join(keywords, ", ")))
	}
	c.keywords[code] = keywords
}

func registerBaseOnly(ID string, exec ExecFunc) {
	register(ID, &struct{}{}, exec)
}

func register[T any](ID string, otherMsgsPtr *T, exec ExecFunc) {
	if _, ok := commandsList[ID]; ok {
		panic("duplicate command id [" + ID + "]")
	}

	if err := locale.FillMsgIDs(ID, otherMsgsPtr); err != nil {
		panic("error filling locale msg ids for command id [" + ID + "]: " + err.Error())
	}

	nameID := locale.MsgID(ID).Name()
	helpID := locale.MsgID(ID).Help()
	usageID := locale.MsgID(ID).Usage()

	allIDs := []locale.MsgID{nameID, helpID, usageID}
	seen := map[locale.MsgID]struct{}{nameID: {}, helpID: {}, usageID: {}}

	if otherMsgsPtr != nil {
		val := reflect.ValueOf(otherMsgsPtr).Elem()
		if val.Kind() == reflect.Struct {
			for _, provider := range locale.CollectMsgIDs(val.Interface()) {
				for code := range locale.AllLangs() {
					for _, id := range provider.AllIDs(code) {
						if _, ok := seen[id]; !ok {
							seen[id] = struct{}{}
							allIDs = append(allIDs, id)
						}
					}
				}
			}
		}
	}

	cmd := &Command{
		id:       ID,
		keywords: make(map[locale.LangCode][]string),
		nameID:   nameID,
		helpID:   helpID,
		usageID:  usageID,
		msgIDs:   allIDs,
		exec:     exec,
	}

	commandsList[ID] = cmd
}

func execute(ctx *Context, key KeywordTriggerKey) (executed bool, err error) {
	if cmd, ok := keywordTriggers[key]; ok {
		qsh.Debugf("executing command: [%s]", cmd.ID())
		ctx.Command = cmd
		passthrough, err := cmd.Exec(ctx)
		if passthrough && !ctx.Strict {
			qsh.Debug("command passthrough, not executed")
			return false, nil
		}
		return true, err
	}

	qsh.Debugf("no command match: [%s:%s]", key.Code, key.Keyword)
	return false, nil
}

func Dispatch(ctx *Context) (dispatched bool, err error) {
	qsh.Debug("qshqn command triggered. dispatching")
	key := KeywordTriggerKey{
		Code:    ctx.DBUser.LangCode,
		Keyword: "",
	}

	if len(ctx.Args) > 0 {
		key.Keyword = ctx.Args[0]
	}

	uid := ctx.DBUser.ID
	if !ctx.SkipDispatchLock {
		if set := executingUserIDs.SetIfAbsent(uid, key.Keyword); !set {
			qsh.Debugf("uid[%d] already executing command, dropping", uid)
			return false, nil
		}
		defer executingUserIDs.Delete(uid)
	}

	executed, err := execute(ctx, key)
	if err != nil {
		return executed, err
	}

	if executed {
		return true, nil
	}

	key.Keyword = ""
	return execute(ctx, key)
}

func ByID(ID string) (*Command, bool) {
	cmd, ok := commandsList[ID]
	return cmd, ok
}

func OnPkgInit(f func() error) { pkgInitFuncs = append(pkgInitFuncs, f) }

func OnPkgInitSet[T any](ptr *T, f func() (T, error)) struct{} {
	OnPkgInit(func() error {
		val, err := f()
		if err != nil {
			return err
		}
		*ptr = val
		return nil
	})
	return struct{}{}
}

func OnPkgInitMustSet[T any](ptr *T, f func() T) struct{} {
	OnPkgInit(func() error {
		*ptr = f()
		return nil
	})
	return struct{}{}
}

func Init() error {
	qsh.Debug("initializing command module")

	var allIDs []locale.MsgID
	for ID, cmd := range commandsList {
		allIDs = append(allIDs, locale.MsgID(ID).Keywords())
		allIDs = append(allIDs, cmd.MsgIDs()...)
	}

	if missing, err := locale.VerifyMsgIDs(allIDs); err != nil {
		qsh.Error(err)
		return locale.ResolveMissingMsgIDs(missing)
	}

	qsh.Debug("locale messages verified successfully")

	langs := locale.AllLangs()
	for ID, cmd := range commandsList {
		for code := range langs {
			var keywords []string
			keywordsString := locale.MsgRaw(code, locale.MsgID(ID).Keywords())
			if keywords = strings.Fields(keywordsString); len(keywords) < 1 {
				keywords = []string{""}
			}
			cmd.setKeywords(code, keywords)
			for _, kw := range keywords {
				kw = strings.ToLower(kw)
				for kwcode := range langs {
					key := KeywordTriggerKey{
						Code:    kwcode,
						Keyword: kw,
					}
					if existingCmd, ok := keywordTriggers[key]; ok && existingCmd.ID() != cmd.ID() {
						panic(fmt.Sprintf("keyword conflict: [%s:%s:%s] and [%s:%s:%s] overwrite eachother", kwcode, kw, existingCmd.ID(), code, kw, cmd.ID()))
					}

					keywordTriggers[key] = cmd
				}
			}
		}
	}

	qsh.Debugf("commands list:\n%v", keywordTriggers)

	for _, f := range pkgInitFuncs {
		if err := f(); err != nil {
			return fmt.Errorf("error at pkg init func: %w", err)
		}
	}

	return nil
}
