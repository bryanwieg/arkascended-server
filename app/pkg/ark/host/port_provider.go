package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FetchTextImpl fetches a URL and returns the trimmed response body. A non-2xx
// status is an error, so callers never parse an error page as data.
func FetchTextImpl(url string) (string, error) {
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("GET %s: HTTP %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// FetchText fetches a URL using deps.FetchText if set, otherwise falls back to
// FetchTextImpl.
func FetchText(deps HostDeps, url string) (string, error) {
	if dep := deps.FetchText; dep != nil {
		return dep(url)
	}
	return FetchTextImpl(url)
}

// ResolvePublicIP discovers the machine's public IP using multiple providers.
func ResolvePublicIP(deps HostDeps) (string, error) {
	urls := []string{
		"https://portchecker.io/api/me",
		"https://api.ipify.org",
	}
	var lastErr error
	for _, url := range urls {
		ip, err := FetchText(deps, url)
		if err != nil {
			lastErr = err
			continue
		}
		ip = strings.TrimSpace(ip)
		if ip != "" {
			return ip, nil
		}
		lastErr = fmt.Errorf("Error: empty response from %s", url)
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("Error: failed to resolve public IP")
}

type starruptureStatus struct {
	IsOpen bool
	Status string
}

type starrupturePortResult struct {
	TCP starruptureStatus
	UDP starruptureStatus
	Raw string
}

// PortCheckProviderClient checks external port status via remote providers.
type PortCheckProviderClient struct {
	Deps              HostDeps
	IP                string
	starruptureByPort map[int]starrupturePortResult
}

// NewPortCheckProviderClient creates a new provider client for the given IP.
func NewPortCheckProviderClient(deps HostDeps, ip string) *PortCheckProviderClient {
	return &PortCheckProviderClient{
		Deps:              deps,
		IP:                ip,
		starruptureByPort: map[int]starrupturePortResult{},
	}
}

// CheckExternalPortStatus checks whether a port is open from the internet.
func (c *PortCheckProviderClient) CheckExternalPortStatus(target PortCheckTarget) (bool, string, string, error) {
	star, starErr, starParsed := c.getStarruptureResult(target.HostPort)

	if target.Protocol != ProtocolUDP && target.Protocol != ProtocolTCP {
		raw := ""
		if starErr == nil && starParsed {
			raw = star.Raw
		}
		return false, "starrupture-utilities.com", raw, fmt.Errorf("Error: unsupported protocol for external check: %s", target.Protocol)
	}

	if starErr == nil && starParsed {
		if target.Protocol == ProtocolUDP {
			return star.UDP.IsOpen, "starrupture-utilities.com", star.UDP.Status, nil
		}
		// Use StarRupture first for TCP too; fallback to portchecker.io if TCP result is absent.
		if star.TCP.Status != "" || star.TCP.IsOpen {
			return star.TCP.IsOpen, "starrupture-utilities.com", star.TCP.Status, nil
		}
	}

	if target.Protocol == ProtocolUDP {
		return udpExternalFallback(star, starErr)
	}

	return c.tcpExternalFallback(target.HostPort, starErr)
}

func udpExternalFallback(star starrupturePortResult, starErr error) (bool, string, string, error) {
	if starErr != nil {
		return false, "starrupture-utilities.com", "", starErr
	}
	return false, "starrupture-utilities.com", star.Raw, nil
}

func (c *PortCheckProviderClient) tcpExternalFallback(port int, starErr error) (bool, string, string, error) {
	fallbackBody, fallbackErr := FetchText(c.Deps, fmt.Sprintf("https://portchecker.io/api/me/%s", strconv.Itoa(port)))
	if fallbackErr != nil {
		if starErr != nil {
			return false, "portchecker.io", "", starErr
		}
		return false, "portchecker.io", "", fallbackErr
	}
	return false, "portchecker.io", fallbackBody, nil
}

func (c *PortCheckProviderClient) getStarruptureResult(port int) (starrupturePortResult, error, bool) {
	if cached, ok := c.starruptureByPort[port]; ok {
		return cached, nil, true
	}
	body, err := FetchText(c.Deps, fmt.Sprintf(
		"https://starrupture-utilities.com/port_check/index.php?host=%s&port=%d&format=json",
		c.IP,
		port,
	))
	if err != nil {
		return starrupturePortResult{}, err, false
	}
	type starruptureJSON struct {
		Success bool `json:"success"`
		Result  struct {
			TCP struct {
				IsOpen bool   `json:"is_open"`
				Status string `json:"status"`
			} `json:"tcp"`
			UDP struct {
				IsOpen bool   `json:"is_open"`
				Status string `json:"status"`
			} `json:"udp"`
		} `json:"result"`
	}
	var parsed starruptureJSON
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return starrupturePortResult{Raw: body}, nil, false
	}
	result := starrupturePortResult{
		TCP: starruptureStatus{IsOpen: parsed.Result.TCP.IsOpen, Status: parsed.Result.TCP.Status},
		UDP: starruptureStatus{IsOpen: parsed.Result.UDP.IsOpen, Status: parsed.Result.UDP.Status},
		Raw: body,
	}
	c.starruptureByPort[port] = result
	return result, nil, true
}
