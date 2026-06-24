package geoip

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `1.0.0.0/24,Los Angeles,US
2.22.233.0/24,London,GB
8.8.8.0/24,Mountain View,US
2001:db8::/112,Berlin,DE
`

func loadStore(t *testing.T, data string) Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "db.csv")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewCSVStore(path)
	if err != nil {
		t.Fatalf("NewCSVStore: %v", err)
	}
	return s
}

func TestLookup(t *testing.T) {
	s := loadStore(t, sample)

	tests := []struct {
		ip          string
		wantCountry string
		wantErr     bool
	}{
		{"2.22.233.0", "GB", false},   // start of range
		{"2.22.233.128", "GB", false}, // middle
		{"2.22.233.255", "GB", false}, // end of range
		{"8.8.8.8", "US", false},      // different range
		{"2001:db8::5", "DE", false},  // ipv6
		{"2.22.234.0", "", true},      // just past a range -> gap
		{"0.0.0.1", "", true},         // before everything
		{"255.255.255.255", "", true}, // after everything
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			loc, err := s.Lookup(netip.MustParseAddr(tt.ip))
			if tt.wantErr {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("err = %v, want ErrNotFound", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err = %v", err)
			}
			if loc.Country != tt.wantCountry {
				t.Errorf("country = %q, want %q", loc.Country, tt.wantCountry)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	tests := map[string]string{
		"bad cidr":          "999.0.0.0/24,X,US",
		"wrong field count": "1.0.0.0/24,1.0.0.255,X,US",
		"empty":             "",
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "db.csv")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewCSVStore(path); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestParseCSVUnordered(t *testing.T) {
	// Rows out of order must still resolve correctly after the sort.
	unordered := "8.8.8.0/24,MV,US\n1.0.0.0/24,LA,US\n"
	s := loadStore(t, unordered)
	if loc, err := s.Lookup(netip.MustParseAddr("1.0.0.50")); err != nil || loc.Country != "US" {
		t.Fatalf("loc=%v err=%v", loc, err)
	}
}

func TestNewUnknownDB(t *testing.T) {
	if _, err := New("redis", ""); err == nil {
		t.Error("expected error for unknown db")
	}
}

func TestParseDirect(t *testing.T) {
	rs, err := parseCSV(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 4 {
		t.Errorf("got %d ranges, want 4", len(rs))
	}
}
