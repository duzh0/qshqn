package callback

const (
	CmdIDUnknown CmdID = iota
	CmdIDLang
	CmdIDAskAddChat
	CmdIDApproveAddChat
	CmdIDDeclineAddChat
	CmdIDConfirmSend
	CmdIDCancelSend
	CmdIDLast
)
