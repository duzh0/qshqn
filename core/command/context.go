package command

func init() {
	ID := "context"
	otherMsgs := struct{}{}

	exec := func(ctx *Context) (passthrough bool, err error) {
		if len(ctx.Args) > 1 && !ctx.Strict {
			return true, nil
		}

		fullText := GetFullContext(ctx)
		_, err = ctx.ReplyText(fullText)
		return false, err
	}

	register(ID, &otherMsgs, exec)
}
