package geoip

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/netip"
	"os"
	"sort"
)

// csvStore is a Store backed by a CSV file of IP ranges.
//
// Each row is: cidr,City,Country
// Rows are loaded once at startup into a slice sorted by network address; lookups
// are a binary search and the store is therefore immutable and concurrency-safe.
//
// CIDR ranges are assumed to be non-overlapping.
type csvStore struct {
	ranges []ipRange
}

type ipRange struct {
	prefix netip.Prefix
	loc    Location
}

// NewCSVStore loads the CSV datastore at path.
func NewCSVStore(path string) (Store, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open csv datastore: %w", err)
	}
	defer f.Close()

	ranges, err := parseCSV(f)
	if err != nil {
		return nil, err
	}
	if len(ranges) == 0 {
		return nil, fmt.Errorf("csv datastore %q is empty", path)
	}

	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].prefix.Addr().Compare(ranges[j].prefix.Addr()) < 0
	})
	return &csvStore{ranges: ranges}, nil
}

func parseCSV(r io.Reader) ([]ipRange, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = 3
	cr.TrimLeadingSpace = true

	var ranges []ipRange
	for line := 1; ; line++ {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("csv line %d: %w", line, err)
		}

		prefix, err := netip.ParsePrefix(rec[0])
		if err != nil {
			return nil, fmt.Errorf("csv line %d: bad cidr %q", line, rec[0])
		}
		prefix = prefix.Masked()

		ranges = append(ranges, ipRange{
			prefix: prefix,
			loc:    Location{City: rec[1], Country: rec[2]},
		})
	}
	return ranges, nil
}

// Lookup finds the range containing addr via binary search.
func (s *csvStore) Lookup(addr netip.Addr) (Location, error) {
	addr = addr.Unmap()

	// First range whose start is strictly greater than addr; the candidate
	// containing range, if any, is the one just before it.
	i := sort.Search(len(s.ranges), func(i int) bool {
		return s.ranges[i].prefix.Addr().Compare(addr) > 0
	})
	if i == 0 {
		return Location{}, ErrNotFound
	}
	r := s.ranges[i-1]
	if r.prefix.Contains(addr) {
		return r.loc, nil
	}
	return Location{}, ErrNotFound
}
