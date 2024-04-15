package domain

type DNSProvider interface {
	SetRecord(in *SetRecordRequest) (out *SetRecordResponse, err error)
}

type SetRecordRequest struct {
	Subdomain string
	Ipaddress string
}

type SetRecordResponse struct {
}

func New(domain string) DNSProvider {
	switch domain {
	case "cloudflare":
		return NewCloudFlareService()
	default:
		return nil
	}
}
