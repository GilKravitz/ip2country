// Package geoip resolves an IP address to a physical Location.
//
// Store is the single seam that makes the service "easily extendable to
// support multiple ip2country databases": each backend implements Store, and
// New selects the active one by name. Adding a backend is one new file plus
// one case in New — no registry or plugin machinery required.
package geoip

import (
	"errors"
	"fmt"
	"net/netip"
)

// Location is the result of an IP lookup.
type Location struct {
	Country string `json:"country"`
	City    string `json:"city"`
}

// ErrNotFound is returned by Store.Lookup when the address is not in the
// datastore. Callers map this to a 404 response.
var ErrNotFound = errors.New("location not found")

// Store resolves an IP address to a Location. Implementations must be safe for
// concurrent use; lookups happen on every request.
type Store interface {
	Lookup(addr netip.Addr) (Location, error)
}

// Config selects and configures the active datastore backend.
type Config struct {
	DB      string
	CSVPath string
}

// New builds the Store selected by cfg.DB.
func New(cfg Config) (Store, error) {
	switch cfg.DB {
	case "csv":
		return NewCSVStore(cfg.CSVPath)
	default:
		return nil, fmt.Errorf("unknown datastore %q", cfg.DB)
	}
}
