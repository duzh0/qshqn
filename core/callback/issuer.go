package callback

type Issuer interface {
	GetIssuerID() int64
}

type IssuerArgs struct {
	IssuerID int64
}

func (p *IssuerArgs) GetIssuerID() int64 {
	return p.IssuerID
}
