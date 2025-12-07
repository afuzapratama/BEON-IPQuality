package iputil

import (
	"net/netip"
	"testing"
)

func TestParseIP(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"Valid IPv4", "192.168.1.1", "192.168.1.1", false},
		{"Valid IPv6", "2001:db8::1", "2001:db8::1", false},
		{"Invalid IP", "not-an-ip", "", true},
		{"Empty string", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIP(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.String() != tt.want {
				t.Errorf("ParseIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePrefix(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"Valid CIDR /24", "192.168.1.0/24", "192.168.1.0/24", false},
		{"Valid CIDR /32", "10.0.0.1/32", "10.0.0.1/32", false},
		{"Valid IPv6 CIDR", "2001:db8::/32", "2001:db8::/32", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePrefix(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePrefix() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.String() != tt.want {
				t.Errorf("ParsePrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseIPOrPrefix(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantIP     string
		wantPrefix string
		isPrefix   bool
		wantErr    bool
	}{
		{"Single IP", "8.8.8.8", "8.8.8.8", "", false, false},
		{"CIDR /24", "10.0.0.0/24", "", "10.0.0.0/24", true, false},
		{"IPv6 address", "::1", "::1", "", false, false},
		{"Invalid", "garbage", "", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, prefix, isPrefix, err := ParseIPOrPrefix(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseIPOrPrefix() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if isPrefix != tt.isPrefix {
				t.Errorf("ParseIPOrPrefix() isPrefix = %v, want %v", isPrefix, tt.isPrefix)
			}
			if !isPrefix && addr.String() != tt.wantIP {
				t.Errorf("ParseIPOrPrefix() addr = %v, want %v", addr, tt.wantIP)
			}
			if isPrefix && prefix.String() != tt.wantPrefix {
				t.Errorf("ParseIPOrPrefix() prefix = %v, want %v", prefix, tt.wantPrefix)
			}
		})
	}
}

func TestIsPrivate(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"Private 10.x", "10.0.0.1", true},
		{"Private 172.16.x", "172.16.0.1", true},
		{"Private 192.168.x", "192.168.1.1", true},
		{"Public IP", "8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, _ := netip.ParseAddr(tt.ip)
			got := IsPrivate(addr)
			if got != tt.want {
				t.Errorf("IsPrivate(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestNormalizeIP(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"IPv4 mapped IPv6", "::ffff:192.168.1.1", "192.168.1.1"},
		{"Regular IPv4", "8.8.8.8", "8.8.8.8"},
		{"Regular IPv6", "2001:db8::1", "2001:db8::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, _ := netip.ParseAddr(tt.input)
			got := NormalizeIP(addr)
			if got.String() != tt.want {
				t.Errorf("NormalizeIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseIPPort(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantIP   string
		wantPort int
		wantErr  bool
	}{
		{"Valid IP:Port", "192.168.1.1:8080", "192.168.1.1", 8080, false},
		{"Valid IP:Port 443", "10.0.0.1:443", "10.0.0.1", 443, false},
		{"No port", "192.168.1.1", "", 0, true},
		{"Invalid port", "192.168.1.1:99999", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, port, err := ParseIPPort(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseIPPort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if addr.String() != tt.wantIP {
					t.Errorf("ParseIPPort() addr = %v, want %v", addr, tt.wantIP)
				}
				if port != tt.wantPort {
					t.Errorf("ParseIPPort() port = %v, want %v", port, tt.wantPort)
				}
			}
		})
	}
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"Public IP", "8.8.8.8", true},
		{"Private IP", "192.168.1.1", false},
		{"Loopback", "127.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, _ := netip.ParseAddr(tt.ip)
			got := IsValid(addr)
			if got != tt.want {
				t.Errorf("IsValid(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// Benchmark tests
func BenchmarkParseIP(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParseIP("192.168.1.1")
	}
}

func BenchmarkParsePrefix(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParsePrefix("192.168.0.0/16")
	}
}

func BenchmarkIsPrivate(b *testing.B) {
	addr, _ := netip.ParseAddr("192.168.1.1")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsPrivate(addr)
	}
}
