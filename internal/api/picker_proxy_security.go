package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var errUnsafePickerDestination = errors.New("unsafe picker destination")

// These ranges are not suitable destinations for a server-side web proxy. In
// addition to private and link-local space, this includes shared, benchmarking,
// documentation, and other special-purpose ranges that can route differently
// inside a hosting provider.
var pickerDeniedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("168.63.129.16/32"), // Azure platform metadata/wire server
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

var pickerMetadataHostnames = map[string]struct{}{
	"instance-data":                    {},
	"instance-data.ec2.internal":       {},
	"metadata":                         {},
	"metadata.azure.internal":          {},
	"metadata.google.internal":         {},
	"metadata.google.internal.corp":    {},
	"metadata.packet.net":              {},
	"metadata.platformequinix.com":     {},
	"metadata.tencentyun.com":          {},
	"metadata.tencentyun.com.internal": {},
}

type lookupNetIPFunc func(context.Context, string, string) ([]netip.Addr, error)
type dialContextFunc func(context.Context, string, string) (net.Conn, error)

// pickerSafeTransport validates the URL before the underlying Transport gets a
// chance to use it. The transport's dialer independently resolves and pins a
// public IP, so redirects and the router's picker-resource fallback get the
// same protection as the main picker endpoint.
type pickerSafeTransport struct {
	transport *http.Transport
}

func (t *pickerSafeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := validatePickerURL(req.URL); err != nil {
		return nil, err
	}
	return t.transport.RoundTrip(req)
}

func newPickerProxyClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   8 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A forward proxy would resolve the target again and defeat IP pinning.
	transport.Proxy = nil
	transport.DialContext = securePickerDialContext(net.DefaultResolver.LookupNetIP, dialer.DialContext)
	transport.DialTLSContext = nil
	transport.ResponseHeaderTimeout = 8 * time.Second

	return &http.Client{
		Transport: &pickerSafeTransport{transport: transport},
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return validatePickerURL(req.URL)
		},
	}
}

func validatePickerURL(target *url.URL) error {
	if target == nil {
		return fmt.Errorf("%w: missing URL", errUnsafePickerDestination)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return fmt.Errorf("%w: unsupported scheme", errUnsafePickerDestination)
	}
	if target.User != nil {
		return fmt.Errorf("%w: credentials are not allowed", errUnsafePickerDestination)
	}

	hostname := target.Hostname()
	if hostname == "" {
		return fmt.Errorf("%w: missing hostname", errUnsafePickerDestination)
	}
	normalizedHostname := strings.TrimSuffix(strings.ToLower(hostname), ".")
	if _, blocked := pickerMetadataHostnames[normalizedHostname]; blocked {
		return fmt.Errorf("%w: metadata hostname", errUnsafePickerDestination)
	}
	if strings.ContainsAny(hostname, "\x00/\\") {
		return fmt.Errorf("%w: invalid hostname", errUnsafePickerDestination)
	}

	if port := target.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("%w: invalid port", errUnsafePickerDestination)
		}
	}
	return nil
}

func securePickerDialContext(lookup lookupNetIPFunc, dial dialContextFunc) dialContextFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid address", errUnsafePickerDestination)
		}

		var addresses []netip.Addr
		if literal, parseErr := netip.ParseAddr(host); parseErr == nil {
			addresses = []netip.Addr{literal}
		} else {
			lookupNetwork := "ip"
			if network == "tcp4" {
				lookupNetwork = "ip4"
			} else if network == "tcp6" {
				lookupNetwork = "ip6"
			}
			addresses, err = lookup(ctx, lookupNetwork, host)
			if err != nil {
				return nil, fmt.Errorf("resolve picker destination: %w", err)
			}
		}

		if len(addresses) == 0 {
			return nil, fmt.Errorf("resolve picker destination: no addresses")
		}
		for _, addr := range addresses {
			if !isPublicPickerIP(addr) {
				return nil, fmt.Errorf("%w: %s", errUnsafePickerDestination, addr)
			}
		}

		var dialErr error
		for _, addr := range addresses {
			addr = addr.Unmap()
			if network == "tcp4" && !addr.Is4() {
				continue
			}
			if network == "tcp6" && !addr.Is6() {
				continue
			}
			conn, err := dial(ctx, network, net.JoinHostPort(addr.String(), port))
			if err == nil {
				return conn, nil
			}
			dialErr = err
		}
		if dialErr == nil {
			dialErr = fmt.Errorf("no address compatible with %s", network)
		}
		return nil, fmt.Errorf("dial picker destination: %w", dialErr)
	}
}

func isPublicPickerIP(addr netip.Addr) bool {
	if !addr.IsValid() || addr.Zone() != "" {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range pickerDeniedPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}
