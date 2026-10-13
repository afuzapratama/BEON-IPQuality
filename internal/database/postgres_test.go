package database

import (
	"net/netip"
	"testing"
)

func TestIPRangeFromPrefix(t *testing.T) {
	tests := []struct {
		prefix string
		start  string
		end    string
	}{
		{prefix: "192.0.2.128/25", start: "192.0.2.128", end: "192.0.2.255"},
		{prefix: "2001:db8:abcd:1200::/56", start: "2001:db8:abcd:1200::", end: "2001:db8:abcd:12ff:ffff:ffff:ffff:ffff"},
		{prefix: "2001:db8::1/128", start: "2001:db8::1", end: "2001:db8::1"},
	}
	for _, test := range tests {
		prefix := netip.MustParsePrefix(test.prefix)
		start, end := IPRangeFromPrefix(prefix)
		if start != test.start || end != test.end {
			t.Errorf("IPRangeFromPrefix(%s) = (%s, %s), want (%s, %s)", test.prefix, start, end, test.start, test.end)
		}
	}
}
