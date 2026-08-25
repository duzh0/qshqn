package locale

type MsgID string

func (ID MsgID) String() string               { return string(ID) }
func (ID MsgID) Name() MsgID                  { return MakeMsgID(ID, "name") }
func (ID MsgID) Keywords() MsgID              { return MakeMsgID(ID, "keywords") }
func (ID MsgID) Help() MsgID                  { return MakeMsgID(ID, "help") }
func (ID MsgID) Usage() MsgID                 { return MakeMsgID(ID, "usage") }
func (ID MsgID) Resolve(code LangCode) string { return Msg(code, ID) }
func (ID MsgID) AllIDs(code LangCode) []MsgID { return []MsgID{ID} }
