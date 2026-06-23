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
// Each row is: from,to,City,Country
// Rows are loaded once at startup into a slice sorted by start address; lookups
// are a binary search and the store is therefore immutable and concurrency-safe.
//
// Ranges are assumed to be non-overlapping.
type csvStore struct {
	ranges []ipRange
}

type ipRange struct {
	start netip.Addr
	end   netip.Addr
	loc   Location
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
		return ranges[i].start.Compare(ranges[j].start) < 0
	})
	return &csvStore{ranges: ranges}, nil
}

func parseCSV(r io.Reader) ([]ipRange, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = 4
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

		start, err := netip.ParseAddr(rec[0])
		if err != nil {
			return nil, fmt.Errorf("csv line %d: bad start ip %q", line, rec[0])
		}
		end, err := netip.ParseAddr(rec[1])
		if err != nil {
			return nil, fmt.Errorf("csv line %d: bad end ip %q", line, rec[1])
		}
		start, end = start.Unmap(), end.Unmap()
		if end.Less(start) {
			return nil, fmt.Errorf("csv line %d: end ip %s precedes start ip %s", line, end, start)
		}

		ranges = append(ranges, ipRange{
			start: start,
			end:   end,
			loc:   Location{City: rec[2], Country: rec[3]},
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
		return s.ranges[i].start.Compare(addr) > 0
	})
	if i == 0 {
		return Location{}, ErrNotFound
	}
	r := s.ranges[i-1]
	if addr.Compare(r.end) <= 0 {
		return r.loc, nil
	}
	return Location{}, ErrNotFound
}
