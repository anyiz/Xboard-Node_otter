// Package devicelease implements fail-closed, panel-authorized IP leases.
package devicelease

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Item struct {
	CredentialID int    `json:"credential_id"`
	UUID         string `json:"uuid"`
	IP           string `json:"ip"`
	InstanceID   string `json:"instance_id"`
	LeaseID      string `json:"lease_id"`
}
type Result struct {
	Allowed bool    `json:"allowed"`
	Reason  string  `json:"reason"`
	TTL     float64 `json:"ttl"`
}
type API interface {
	Call(context.Context, string, []Item) ([]Result, error)
}
type Client struct {
	URL, Token        string
	NodeID, MachineID int
	HTTP              *http.Client
}

func (c *Client) Call(ctx context.Context, op string, items []Item) ([]Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	body, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimRight(c.URL, "/") + "/api/v2/server/device-leases/" + op)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("token", c.Token)
	q.Set("node_id", strconv.Itoa(c.NodeID))
	if c.MachineID > 0 {
		q.Set("machine_id", strconv.Itoa(c.MachineID))
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-Leases", "v1")
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lease request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lease status %d", resp.StatusCode)
	}
	var out struct {
		Results []Result `json:"results"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Results) != len(items) {
		return nil, fmt.Errorf("invalid lease response length")
	}
	return out.Results, nil
}
