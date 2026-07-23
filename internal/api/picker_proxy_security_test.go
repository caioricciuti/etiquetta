package api

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"testing"
)

func TestIsPublicPickerIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		address string
		want    bool
	}{
		{"93.184.216.34", true},
		{"2606:4700:4700::1111", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false},
		{"168.63.129.16", false},
		{"100.100.100.200", false},
		{"198.18.0.1", false},
		{"203.0.113.10", false},
		{"0.0.0.0", false},
		{"::1", false},
		{"fc00::1", false},
		{"fe80::1", false},
		{"::ffff:127.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			if got := isPublicPickerIP(netip.MustParseAddr(tt.address)); got != tt.want {
				t.Fatalf("isPublicPickerIP(%s) = %v, want %v", tt.address, got, tt.want)
			}
		})
	}
}

func TestValidatePickerURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{"public HTTPS", "https://www.example.com/products?q=one", false},
		{"public HTTPS custom port", "https://www.example.com:8443/", false},
		{"public HTTP remains supported", "http://www.example.com/", false},
		{"missing host", "https:///path", true},
		{"file scheme", "file:///etc/passwd", true},
		{"embedded credentials", "https://user:password@www.example.com/", true},
		{"metadata hostname", "http://metadata.google.internal/computeMetadata/v1/", true},
		{"metadata hostname trailing dot", "http://metadata.google.internal./", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := url.Parse(tt.rawURL)
			if err == nil {
				err = validatePickerURL(target)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("validatePickerURL(%q) error = %v, wantErr %v", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}

func TestSecurePickerDialPinsResolvedPublicIP(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("dial stopped by test")
	var dialedAddress string
	lookup := func(_ context.Context, network, host string) ([]netip.Addr, error) {
		if network != "ip" || host != "public.example" {
			t.Fatalf("unexpected lookup: network=%q host=%q", network, host)
		}
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	dial := func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" {
			t.Fatalf("unexpected dial network %q", network)
		}
		dialedAddress = address
		return nil, wantErr
	}

	_, err := securePickerDialContext(lookup, dial)(context.Background(), "tcp", "public.example:443")
	if !errors.Is(err, wantErr) {
		t.Fatalf("dial error = %v, want wrapped test error", err)
	}
	if dialedAddress != "93.184.216.34:443" {
		t.Fatalf("dialed %q; hostname was not pinned to the validated IP", dialedAddress)
	}
}

func TestSecurePickerDialRejectsPrivateAndMixedDNSAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		addresses []netip.Addr
	}{
		{"private", []netip.Addr{netip.MustParseAddr("10.0.0.8")}},
		{"loopback", []netip.Addr{netip.MustParseAddr("127.0.0.1")}},
		{"link local metadata", []netip.Addr{netip.MustParseAddr("169.254.169.254")}},
		{"mixed public and private", []netip.Addr{
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("192.168.1.8"),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialCalled := false
			lookup := func(context.Context, string, string) ([]netip.Addr, error) {
				return tt.addresses, nil
			}
			dial := func(context.Context, string, string) (net.Conn, error) {
				dialCalled = true
				return nil, errors.New("unexpected dial")
			}

			_, err := securePickerDialContext(lookup, dial)(context.Background(), "tcp", "attacker.example:443")
			if !errors.Is(err, errUnsafePickerDestination) {
				t.Fatalf("error = %v, want unsafe destination", err)
			}
			if dialCalled {
				t.Fatal("unsafe DNS answer reached the network dialer")
			}
		})
	}
}

func TestSecurePickerDialRevalidatesEveryConnection(t *testing.T) {
	t.Parallel()

	lookupCount := 0
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		lookupCount++
		if lookupCount == 1 {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	dialAttempts := 0
	dial := func(context.Context, string, string) (net.Conn, error) {
		dialAttempts++
		return nil, errors.New("dial stopped by test")
	}
	secureDial := securePickerDialContext(lookup, dial)

	_, firstErr := secureDial(context.Background(), "tcp", "rebinding.example:443")
	if errors.Is(firstErr, errUnsafePickerDestination) {
		t.Fatalf("initial public answer was rejected: %v", firstErr)
	}
	_, secondErr := secureDial(context.Background(), "tcp", "rebinding.example:443")
	if !errors.Is(secondErr, errUnsafePickerDestination) {
		t.Fatalf("rebound private answer error = %v, want unsafe destination", secondErr)
	}
	if dialAttempts != 1 {
		t.Fatalf("network dial attempts = %d, want only the public answer dialed", dialAttempts)
	}
}
