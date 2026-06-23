package config

import (
	"path"
	"strings"
	"unicode/utf8"
)

func validateAbsoluteLexicalPath(p string) error {
	if err := validatePathString(p); err != nil {
		return err
	}
	if !strings.HasPrefix(p, "/") {
		return errInvalidPath
	}
	if strings.Contains(p, "\\") {
		return errInvalidPath
	}
	if path.Clean(p) != p {
		return errInvalidPath
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == "." || segment == ".." {
			return errInvalidPath
		}
	}
	return nil
}

func validateArtifactGlob(p string) error {
	if err := validatePathString(p); err != nil {
		return err
	}
	if !strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return errInvalidPath
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == ".." {
			return errInvalidPath
		}
	}
	if _, err := path.Match(p, "/"); err != nil {
		return errInvalidPath
	}
	return nil
}

func validatePathString(p string) error {
	if p == "" || len(p) > MaxPathBytes || !utf8.ValidString(p) {
		return errInvalidPath
	}
	for _, r := range p {
		if r == 0 || r < 0x20 || r == 0x7f {
			return errInvalidPath
		}
	}
	return nil
}

type invalidPathError struct{}

func (invalidPathError) Error() string { return "invalid path" }

var errInvalidPath = invalidPathError{}
