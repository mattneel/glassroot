package observe

import (
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

func normalizeObservedPath(value string, roots []PathRootAlias) NormalizedPath {
	p := NormalizedPath{Literal: value, Namespace: PathNamespaceOpaqueInvalid, Display: value}
	if value == "" || !utf8.ValidString(value) || strings.Contains(value, "\x00") || strings.Contains(value, "\\") || hasControl(value) {
		p.Display = "<invalid>"
		return p
	}
	if strings.HasPrefix(value, "/") {
		best := PathRootAlias{}
		bestRel := ""
		matched := false
		ordered := append([]PathRootAlias(nil), roots...)
		sort.SliceStable(ordered, func(i, j int) bool {
			if len(ordered[i].Root) != len(ordered[j].Root) {
				return len(ordered[i].Root) > len(ordered[j].Root)
			}
			if ordered[i].Namespace != ordered[j].Namespace {
				return ordered[i].Namespace == PathNamespaceWorkdirRoot
			}
			return ordered[i].RootIndex < ordered[j].RootIndex
		})
		for _, r := range ordered {
			if rootMatch(value, r.Root) {
				best = r
				if value == r.Root {
					bestRel = ""
				} else {
					bestRel = strings.TrimPrefix(value[len(r.Root):], "/")
				}
				matched = true
				break
			}
		}
		if matched {
			p.Namespace = best.Namespace
			p.RootIndex = best.RootIndex
			p.Relative = bestRel
			p.Display = best.Alias
			if bestRel != "" {
				p.Display += "/" + bestRel
			}
			return p
		}
		p.Namespace = PathNamespaceAbsoluteUnmapped
		p.Display = value
		return p
	}
	if path.Clean(value) != value || strings.HasPrefix(value, "../") || value == ".." {
		p.Display = "<invalid>"
		return p
	}
	p.Namespace = PathNamespaceRelative
	p.Relative = value
	p.Display = value
	return p
}
func rootMatch(value, root string) bool {
	if root == "" || !strings.HasPrefix(root, "/") {
		return false
	}
	return value == root || strings.HasPrefix(value, root+"/")
}
func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
