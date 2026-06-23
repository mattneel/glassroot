package materialize

import (
	"strconv"
	"strings"
)

const lfsPointerVersionLine = "version https://git-lfs.github.com/spec/v1"

func parseLFSPointer(data []byte) (LFSPointerMetadata, bool) {
	if len(data) == 0 || len(data) > MaxSymlinkTargetBytes {
		return LFSPointerMetadata{}, false
	}
	text := string(data)
	if strings.ContainsRune(text, 0) {
		return LFSPointerMetadata{}, false
	}
	text = strings.TrimSuffix(text, "\n")
	lines := strings.Split(text, "\n")
	if len(lines) != 3 || lines[0] != lfsPointerVersionLine {
		return LFSPointerMetadata{}, false
	}
	const oidPrefix = "oid sha256:"
	if !strings.HasPrefix(lines[1], oidPrefix) {
		return LFSPointerMetadata{}, false
	}
	hexPart := strings.TrimPrefix(lines[1], oidPrefix)
	if len(hexPart) != 64 || !isLowerHex(hexPart) {
		return LFSPointerMetadata{}, false
	}
	const sizePrefix = "size "
	if !strings.HasPrefix(lines[2], sizePrefix) {
		return LFSPointerMetadata{}, false
	}
	sizeText := strings.TrimPrefix(lines[2], sizePrefix)
	if sizeText == "" || strings.HasPrefix(sizeText, "+") || strings.HasPrefix(sizeText, "-") {
		return LFSPointerMetadata{}, false
	}
	size, err := strconv.ParseInt(sizeText, 10, 64)
	if err != nil || size < 0 {
		return LFSPointerMetadata{}, false
	}
	return LFSPointerMetadata{OID: "sha256:" + hexPart, Size: size}, true
}

func isLowerHex(s string) bool {
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}
