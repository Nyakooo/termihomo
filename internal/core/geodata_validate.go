package core

import (
	"encoding/binary"
	"os"
	"unicode/utf8"

	maxminddb "github.com/oschwald/maxminddb-golang/v2"
)

// A core-managed update is atomic but does not update Termihomo's manifest.
// Validate unfamiliar files instead of overwriting a newer/custom database.
func validGeodata(path, name string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<20 {
		return false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if name == "GeoSite.dat" || name == "GeoIP.dat" {
		return validGeodataList(body)
	}
	reader, err := maxminddb.OpenBytes(body)
	if err != nil {
		return false
	}
	defer reader.Close()
	// OpenBytes validates the metadata and search-tree bounds. Do not use
	// Verify's stricter MaxMind metadata policy: MetaCubeX's valid metadb files
	// deliberately omit the optional description map.
	return reader.Metadata.NodeCount > 0
}

// GeoIPList and GeoSiteList are protobuf messages containing repeated entries
// in field 1. Each entry has a nonempty country_code string in field 1. Check
// all wire frames to reject truncated entries left by interrupted downloads.
func validGeodataList(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	for len(body) > 0 {
		if body[0] != 0x0a {
			return false
		}
		size, n := binary.Uvarint(body[1:])
		if n <= 0 || size > uint64(len(body)-1-n) {
			return false
		}
		entry := body[1+n : 1+n+int(size)]
		if !validGeodataEntry(entry) {
			return false
		}
		body = body[1+n+int(size):]
	}
	return true
}

func validGeodataEntry(body []byte) bool {
	country := false
	for len(body) > 0 {
		tag, n := binary.Uvarint(body)
		if n <= 0 || tag>>3 == 0 {
			return false
		}
		body = body[n:]
		switch tag & 7 {
		case 0:
			_, n = binary.Uvarint(body)
			if n <= 0 {
				return false
			}
			body = body[n:]
		case 1:
			if len(body) < 8 {
				return false
			}
			body = body[8:]
		case 2:
			size, used := binary.Uvarint(body)
			if used <= 0 || size > uint64(len(body)-used) {
				return false
			}
			value := body[used : used+int(size)]
			if tag>>3 == 1 {
				country = len(value) > 0 && utf8.Valid(value)
			}
			body = body[used+int(size):]
		case 5:
			if len(body) < 4 {
				return false
			}
			body = body[4:]
		default:
			return false
		}
	}
	return country
}
