package publicip

import (
	"errors"
	"fmt"
	"net"

	"github.com/miekg/dns"
)

const opendnsMyIP = "myip.opendns.com."
const opendnsResolver = "resolver1.opendns.com."
const cloudflareResolver = "1.1.1.1:53"

// OpenDNSProvider is a Public IP address provider which makes use of OpenDNS
type OpenDNSProvider struct {
	IPv4Addr net.IP
	IPv6Addr net.IP
	client   *dns.Client
}

func NewOpenDNSProvider(coreResolver string) (*OpenDNSProvider, error) {
	client := new(dns.Client)

	queryA := new(dns.Msg)
	queryA.SetQuestion(dns.Fqdn(opendnsResolver), dns.TypeA)

	inA, _, err := client.Exchange(queryA, coreResolver)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return nil, err
	}
	resultClient := OpenDNSProvider{
		client: new(dns.Client),
	}

	if t, ok := inA.Answer[0].(*dns.A); ok {
		fmt.Printf("inA.Answer: %v\n", t.A)
		resultClient.IPv4Addr = t.A
	}

	queryAAAA := new(dns.Msg)
	queryAAAA.SetQuestion(opendnsResolver, dns.TypeAAAA)

	inAAAA, _, err := client.Exchange(queryAAAA, coreResolver)
	if err != nil {
		return nil, err
	}
	if t, ok := inAAAA.Answer[0].(*dns.AAAA); ok {
		resultClient.IPv6Addr = t.AAAA
	}
	return &resultClient, nil
}

// GetIPv4 returns the public IPv4 Address of the current machine
func (opdns OpenDNSProvider) GetIPv4() (string, error) {
	myIpQuery := new(dns.Msg)
	myIpQuery.SetQuestion(opendnsMyIP, dns.TypeA)
	res, _, err := opdns.client.Exchange(myIpQuery, opdns.IPv4Addr.String()+":53")
	if err != nil {
		return "", err
	}

	record, ok := res.Answer[0].(*dns.A)
	if !ok {
		return "", errors.New("OpenDNS failed to return a valid A record")
	}

	return record.A.String(), nil
}

// GetIPv6 returns the public IPv6 Address of the current machine
func (opdns OpenDNSProvider) GetIPv6() (string, error) {
	myIpQuery := new(dns.Msg)
	myIpQuery.SetQuestion(opendnsMyIP, dns.TypeAAAA)
	res, _, err := opdns.client.Exchange(myIpQuery, fmt.Sprintf("[%v]:53", opdns.IPv6Addr.String()))
	if err != nil {
		return "", err
	}

	if erry, ok := err.(*net.OpError); ok && erry.Err.Error() == "connect: no route to host" {
		return "", errors.New("no route to OpenDNS IPv6. Does your connection support IPv6?")
	} else if err != nil {
		return "", err
	}

	record, ok := res.Answer[0].(*dns.AAAA)
	if !ok {
		return "", errors.New("OpenDNS failed to return a valid AAAA record")
	}

	return record.AAAA.String(), nil
}
