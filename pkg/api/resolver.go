package api
type DNSRecord struct {
	Hostname string   `json:"hostname"`
	IPs      []string `json:"ips"`
}

