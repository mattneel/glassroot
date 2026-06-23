package githubsourcestore

import "strings"

func ValidateShallowMetadata(data []byte, objectFormat string, maxEntries int) error {
	if maxEntries <= 0 || maxEntries > 2 {
		return errCode(CodeMetadataInvalid, "shallow", "shallow limit rejected", nil)
	}
	want := 40
	if objectFormat == "sha256" {
		want = 64
	} else if objectFormat != "sha1" {
		return errCode(CodeMetadataInvalid, "shallow", "object format rejected", nil)
	}
	if len(data) == 0 || len(data) > maxEntries*(want+1) {
		return errCode(CodeMetadataInvalid, "shallow", "shallow metadata rejected", nil)
	}
	seen := map[string]struct{}{}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "" || !isLowerHex(line, want) {
			return errCode(CodeMetadataInvalid, "shallow", "shallow object rejected", nil)
		}
		if _, ok := seen[line]; ok {
			return errCode(CodeMetadataInvalid, "shallow", "duplicate shallow object rejected", nil)
		}
		seen[line] = struct{}{}
		if len(seen) > maxEntries {
			return errCode(CodeMetadataInvalid, "shallow", "too many shallow entries", nil)
		}
	}
	return nil
}
