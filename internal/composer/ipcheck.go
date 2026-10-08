package composer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

// IPCheckAction checks where this device's public IP address is, by GeoIP: that it is in a place
// wanted, or not in a place to avoid (is the VPN on?). A place is a country code (US, DE), a
// country, region or city name, ignoring case.
type IPCheckAction struct {
	// Want: the IP must be in one of these places.
	Want []string `json:"want,omitempty"`
	// Avoid: the IP must not be in any of these places.
	Avoid []string `json:"avoid,omitempty"`
}

func (a *IPCheckAction) Kind() string { return "ip-check" }

func (a *IPCheckAction) Describe() string {
	var parts []string
	if len(a.Want) > 0 {
		parts = append(parts, "in "+strings.Join(a.Want, " or "))
	}
	if len(a.Avoid) > 0 {
		parts = append(parts, "not in "+strings.Join(a.Avoid, " or "))
	}
	if len(parts) == 0 {
		return "IP check"
	}
	return "IP is " + strings.Join(parts, ", ")
}

func (a *IPCheckAction) Check(c *Checker) {
	if len(a.Want) == 0 && len(a.Avoid) == 0 {
		c.Error("want", "say where the IP should be, or should not be")
	}
	for _, list := range [][]string{a.Want, a.Avoid} {
		if slices.ContainsFunc(list, func(p string) bool { return strings.TrimSpace(p) == "" }) {
			c.Error("want", "a place is empty")
			break
		}
	}
}

// Geo is where an IP address is.
type Geo struct {
	IP          string
	Country     string // ISO code: US, DE
	CountryName string
	Region      string
	City        string
	Org         string
}

func (g Geo) String() string {
	var place []string
	for _, p := range []string{g.City, g.Region, g.CountryName} {
		if p != "" && !slices.Contains(place, p) {
			place = append(place, p)
		}
	}
	s := fmt.Sprintf("%s in %s", g.IP, g.Country)
	if len(place) > 0 {
		s += " (" + strings.Join(place, ", ") + ")"
	}
	if g.Org != "" {
		s += " · " + g.Org
	}
	return s
}

// in reports whether g is in place: its country code, country, region or city, ignoring case.
func (g Geo) in(place string) bool {
	place = strings.TrimSpace(place)
	for _, p := range []string{g.Country, g.CountryName, g.Region, g.City} {
		if p != "" && strings.EqualFold(p, place) {
			return true
		}
	}
	return false
}

// geoService is a GeoIP service that answers about the IP asking, and how to read its answer.
type geoService struct {
	url  string
	read func([]byte) (Geo, error)
}

// geoServices are asked in order, the next when one fails. Neither needs a key.
var geoServices = []geoService{
	{"https://ipinfo.io/json", func(b []byte) (Geo, error) {
		var r struct{ IP, City, Region, Country, Org string }
		err := json.Unmarshal(b, &r)
		return Geo{IP: r.IP, Country: r.Country, Region: r.Region, City: r.City, Org: r.Org}, err
	}},
	{"https://ipapi.co/json/", func(b []byte) (Geo, error) {
		var r struct {
			IP          string `json:"ip"`
			City        string `json:"city"`
			Region      string `json:"region"`
			CountryCode string `json:"country_code"`
			CountryName string `json:"country_name"`
			Org         string `json:"org"`
		}
		err := json.Unmarshal(b, &r)
		return Geo{IP: r.IP, Country: r.CountryCode, CountryName: r.CountryName, Region: r.Region, City: r.City, Org: r.Org}, err
	}},
}

// Locate asks the GeoIP services where this device's public IP address is.
func Locate(ctx context.Context) (Geo, error) {
	var errs []error
	for _, s := range geoServices {
		g, err := s.locate(ctx)
		if err == nil {
			return g, nil
		}
		if ctx.Err() != nil {
			return Geo{}, ctx.Err()
		}
		errs = append(errs, err)
	}
	return Geo{}, errors.Join(errs...)
}

// geoClient is a client with no connections kept from before: one opened before the VPN came up
// would still go out the old way, and the service would see the old IP.
func geoClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableKeepAlives = true
	return &http.Client{Transport: t}
}

func (s geoService) locate(ctx context.Context) (Geo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return Geo{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	res, err := geoClient().Do(req)
	if err != nil {
		return Geo{}, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return Geo{}, err
	}
	if res.StatusCode != http.StatusOK {
		return Geo{}, fmt.Errorf("%s: %s", s.url, res.Status)
	}
	g, err := s.read(b)
	if err != nil || g.IP == "" || g.Country == "" {
		return Geo{}, fmt.Errorf("%s: no IP and country in its answer", s.url)
	}
	return g, nil
}

func (a *IPCheckAction) Run(r *Run) error {
	g, err := Locate(r.Ctx)
	if err != nil {
		return fmt.Errorf("could not tell where the IP is: %w", err)
	}
	r.Printf("IP %s", g)
	if i := slices.IndexFunc(a.Avoid, g.in); i >= 0 {
		return fmt.Errorf("the IP is in %s, to avoid", strings.TrimSpace(a.Avoid[i]))
	}
	if len(a.Want) > 0 && !slices.ContainsFunc(a.Want, g.in) {
		return fmt.Errorf("the IP is in %s, not in %s", g.Country, strings.Join(a.Want, " or "))
	}
	return nil
}
