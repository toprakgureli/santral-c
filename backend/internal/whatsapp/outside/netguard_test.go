package outside

import (
	"net"
	"net/http"
	"testing"
)

func TestBlockedIP(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.10", "172.16.5.4", "169.254.169.254", "100.64.0.1", "::1", "fd00::1", "0.0.0.0"} {
		if !blockedIP(net.ParseIP(ip)) {
			t.Errorf("%s should be blocked", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "2606:4700::1111"} {
		if blockedIP(net.ParseIP(ip)) {
			t.Errorf("%s should be allowed", ip)
		}
	}
}

func TestRedirectStaysOnServer(t *testing.T) {
	first, _ := http.NewRequest(http.MethodGet, "https://api.example.com/a", nil)
	same, _ := http.NewRequest(http.MethodGet, "https://api.example.com/b", nil)
	other, _ := http.NewRequest(http.MethodGet, "https://collector.example.net/b", nil)
	if err := Client.CheckRedirect(same, []*http.Request{first}); err != nil {
		t.Errorf("same server refused: %v", err)
	}
	if err := Client.CheckRedirect(other, []*http.Request{first}); err == nil {
		t.Error("redirect to another server was followed")
	}
}
