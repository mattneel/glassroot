package dockerengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxPathBytes = 4096

func ValidateSocketPath(path string) error {
	if err := validateSocketPathSyntax(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return errCode(CodeInvalidSocketPath, "socket", "lstat", "socket is not accessible", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errCode(CodeSocketSymlink, "socket", "lstat", "socket final component is a symlink", nil)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return errCode(CodeSocketNotUnix, "socket", "lstat", "socket final component is not a Unix socket", nil)
	}
	return nil
}

func validateSocketPathSyntax(path string) error {
	if path == "" {
		return errCode(CodeInvalidSocketPath, "socket", "syntax", "socket path is required", nil)
	}
	if len(path) > maxPathBytes || !utf8.ValidString(path) {
		return errCode(CodeInvalidSocketPath, "socket", "syntax", "socket path is invalid", nil)
	}
	lower := strings.ToLower(path)
	if strings.Contains(lower, "://") || strings.HasPrefix(lower, "npipe:") {
		return errCode(CodeInvalidSocketPath, "socket", "syntax", "only absolute Unix socket paths are supported", nil)
	}
	for _, r := range path {
		if r == 0 || r < 0x20 || r == 0x7f {
			return errCode(CodeInvalidSocketPath, "socket", "syntax", "socket path contains a control character", nil)
		}
	}
	if !filepath.IsAbs(path) {
		return errCode(CodeInvalidSocketPath, "socket", "syntax", "socket path must be absolute", nil)
	}
	if filepath.Clean(path) != path {
		return errCode(CodeInvalidSocketPath, "socket", "syntax", "socket path must be lexical-clean", nil)
	}
	return nil
}

func ValidateImmutableLocalImage(image string) (string, error) {
	if image == "" || len(image) > 4096 || !utf8.ValidString(image) {
		return "", errCode(CodeImageReferenceInvalid, "image", "syntax", "image reference is invalid", nil)
	}
	for _, r := range image {
		if r <= 0x20 || r == 0x7f {
			return "", errCode(CodeImageReferenceInvalid, "image", "syntax", "image reference contains whitespace or control characters", nil)
		}
	}
	idx := strings.LastIndex(image, "@sha256:")
	if idx <= 0 || idx+len("@sha256:")+64 != len(image) {
		return "", errCode(CodeImageReferenceInvalid, "image", "syntax", "image must include exactly one sha256 digest", nil)
	}
	digest := image[idx+len("@sha256:"):]
	for _, c := range digest {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return "", errCode(CodeImageReferenceInvalid, "image", "syntax", "image digest must be lowercase hexadecimal", nil)
		}
	}
	if strings.Contains(image[:idx], "@") {
		return "", errCode(CodeImageReferenceInvalid, "image", "syntax", "image contains multiple digests", nil)
	}
	return image, nil
}

func compareAPIVersion(got, want string) (int, error) {
	var gm, gn, wm, wn int
	if _, err := fmt.Sscanf(got, "%d.%d", &gm, &gn); err != nil {
		return 0, err
	}
	if _, err := fmt.Sscanf(want, "%d.%d", &wm, &wn); err != nil {
		return 0, err
	}
	if gm != wm {
		if gm < wm {
			return -1, nil
		}
		return 1, nil
	}
	if gn < wn {
		return -1, nil
	}
	if gn > wn {
		return 1, nil
	}
	return 0, nil
}
