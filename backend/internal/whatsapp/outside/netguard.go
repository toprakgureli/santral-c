// Package outside makes the requests the WhatsApp module sends to systems
// outside the panel (a chatbot's outside system, an automatic rule's
// address): never to the server's own network, and with customer answers
// escaped where they land.
package outside

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// Addresses typed into the panel (a chatbot's outside system, a rule's
// notification address) may only reach the internet. The server itself
// and the private network around it stay out of reach, so nobody can use
// a chatbot to peek at internal services. The check runs on the address
// actually dialled, after the name is resolved, so a name that later
// points inward is caught too.

// ErrInternalAddress refuses a request to the server's own network.
var ErrInternalAddress = errors.New("iç ağdaki adreslere istek gönderilemez")

func blockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return true
	}
	// shared address space used by carriers and some clouds
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xC0 == 64 {
		return true
	}
	return false
}

func guardedDialer() *net.Dialer {
	return &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if blockedIP(net.ParseIP(host)) {
				return ErrInternalAddress
			}
			return nil
		},
	}
}

// Client is the HTTP client for addresses entered in the panel.
var Client = &http.Client{
	Transport: &http.Transport{
		DialContext:           guardedDialer().DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConns:          20,
		IdleConnTimeout:       60 * time.Second,
	},
	// Redirects stay on the same server: the request may carry an outside
	// system's secret headers, which must never reach another address.
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("çok fazla yönlendirme")
		}
		if req.URL.Scheme != via[0].URL.Scheme || req.URL.Host != via[0].URL.Host {
			return fmt.Errorf("başka bir sunucuya yönlendirildi; güvenlik için izlenmedi")
		}
		return nil
	},
}

// Explain turns a dial refusal into words for the panel.
func Explain(err error) error {
	if errors.Is(err, ErrInternalAddress) {
		return ErrInternalAddress
	}
	return err
}
