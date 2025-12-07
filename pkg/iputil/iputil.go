package iputil

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// ParseIP parses an IP address string and returns a netip.Addr
func ParseIP(s string) (netip.Addr, error) {
	s = strings.TrimSpace(s)

	// Handle IPv6 with brackets [::1]:8080
	if strings.HasPrefix(s, "[") {
		if idx := strings.Index(s, "]"); idx != -1 {
			s = s[1:idx]
		}
	} else if strings.Contains(s, ".") && strings.Contains(s, ":") {
		// IPv4 with port like 192.168.1.1:8080
		if idx := strings.LastIndex(s, ":"); idx != -1 {
			s = s[:idx]
		}
	}

	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid IP address: %s", s)
	}

	return addr, nil
}

// ParsePrefix parses a CIDR prefix string and returns a netip.Prefix
func ParsePrefix(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)

	// If it's a single IP, convert to /32 or /128
	if !strings.Contains(s, "/") {
		addr, err := ParseIP(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		if addr.Is4() {
			s = s + "/32"
		} else {
			s = s + "/128"
		}
	}

	prefix, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("invalid CIDR prefix: %s", s)
	}

	return prefix, nil
}

// ParseIPOrPrefix parses a string that could be either an IP or a CIDR prefix
func ParseIPOrPrefix(s string) (addr netip.Addr, prefix netip.Prefix, isPrefix bool, err error) {
	s = strings.TrimSpace(s)

	if strings.Contains(s, "/") {
		prefix, err = ParsePrefix(s)
		if err != nil {
			return netip.Addr{}, netip.Prefix{}, false, err
		}
		return netip.Addr{}, prefix, true, nil
	}

	addr, err = ParseIP(s)
	if err != nil {
		return netip.Addr{}, netip.Prefix{}, false, err
	}

	return addr, netip.Prefix{}, false, nil
}

// ParseIPPort parses an IP:PORT string
func ParseIPPort(s string) (netip.Addr, int, error) {
	s = strings.TrimSpace(s)

	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return netip.Addr{}, 0, fmt.Errorf("invalid IP:PORT format: %s", s)
	}

	addr, err := ParseIP(parts[0])
	if err != nil {
		return netip.Addr{}, 0, err
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil || port < 0 || port > 65535 {
		return netip.Addr{}, 0, fmt.Errorf("invalid port number: %s", parts[1])
	}

	return addr, port, nil
}

// IPToUint32 converts an IPv4 address to uint32
func IPToUint32(addr netip.Addr) uint32 {
	if !addr.Is4() {
		return 0
	}
	b := addr.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// Uint32ToIP converts a uint32 to an IPv4 address
func Uint32ToIP(n uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{
		byte(n >> 24),
		byte(n >> 16),
		byte(n >> 8),
		byte(n),
	})
}

// IsPrivate checks if an IP address is private (RFC 1918)
func IsPrivate(addr netip.Addr) bool {
	return addr.IsPrivate()
}

// IsLoopback checks if an IP address is a loopback address
func IsLoopback(addr netip.Addr) bool {
	return addr.IsLoopback()
}

// IsGlobalUnicast checks if an IP address is a global unicast address
func IsGlobalUnicast(addr netip.Addr) bool {
	return addr.IsGlobalUnicast()
}

// IsValid checks if an IP address is valid for reputation checking
func IsValid(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	if addr.IsLoopback() {
		return false
	}
	if addr.IsPrivate() {
		return false
	}
	if addr.IsMulticast() {
		return false
	}
	if addr.IsUnspecified() {
		return false
	}
	return true
}

// NormalizeIP normalizes an IP address (IPv4-mapped IPv6 to IPv4)
func NormalizeIP(addr netip.Addr) netip.Addr {
	if addr.Is4In6() {
		return addr.Unmap()
	}
	return addr
}

// ContainsIP checks if a prefix contains an IP address
func ContainsIP(prefix netip.Prefix, addr netip.Addr) bool {
	return prefix.Contains(addr)
}

// GetSubnetSize returns the number of IPs in a subnet
func GetSubnetSize(prefix netip.Prefix) uint64 {
	bits := prefix.Bits()
	if prefix.Addr().Is4() {
		return 1 << (32 - bits)
	}
	// For IPv6, cap at uint64 max
	if bits <= 64 {
		return 1 << (128 - bits)
	}
	return 1 << (128 - bits)
}

// GetNetworkAddress returns the network address of a prefix
func GetNetworkAddress(prefix netip.Prefix) netip.Addr {
	return prefix.Masked().Addr()
}

// LegacyIPToNetIP converts a net.IP to netip.Addr
func LegacyIPToNetIP(ip net.IP) (netip.Addr, bool) {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	return NormalizeIP(addr), true
}

// NetIPToLegacy converts a netip.Addr to net.IP
func NetIPToLegacy(addr netip.Addr) net.IP {
	return addr.AsSlice()
}

// ValidateIPString validates if a string is a valid IP address
func ValidateIPString(s string) bool {
	_, err := ParseIP(s)
	return err == nil
}

// ValidateCIDRString validates if a string is a valid CIDR notation
func ValidateCIDRString(s string) bool {
	_, err := ParsePrefix(s)
	return err == nil
}
