package artifactcollect

import "path"

func matchRelativePattern(pattern, rel string, limits Limits) (bool, error) {
	pcomps := splitRel(pattern)
	rcomps := splitRel(rel)
	states := 0
	memo := map[[2]int]bool{}
	var rec func(i, j int) (bool, error)
	rec = func(i, j int) (bool, error) {
		states++
		if states > limits.MaxPatternStates {
			return false, errCode(CodePatternLimit, "match", "", "artifact pattern matching exceeded state limit", nil)
		}
		key := [2]int{i, j}
		if v, ok := memo[key]; ok {
			return v, nil
		}
		var ok bool
		defer func() { memo[key] = ok }()
		if i == len(pcomps) {
			ok = j == len(rcomps)
			return ok, nil
		}
		if pcomps[i] == "**" {
			if m, err := rec(i+1, j); err != nil || m {
				ok = m
				return m, err
			}
			if j < len(rcomps) {
				if m, err := rec(i, j+1); err != nil || m {
					ok = m
					return m, err
				}
			}
			return false, nil
		}
		if j >= len(rcomps) {
			return false, nil
		}
		m, err := path.Match(pcomps[i], rcomps[j])
		if err != nil || !m {
			return false, err
		}
		ok, err = rec(i+1, j+1)
		return ok, err
	}
	return rec(0, 0)
}

func patternCanMatchDescendant(pattern, rel string, limits Limits) (bool, error) {
	pcomps := splitRel(pattern)
	rcomps := splitRel(rel)
	states := 0
	memo := map[[2]int]bool{}
	var remainingCanMatchOne func(int) bool
	remainingCanMatchOne = func(i int) bool { return i < len(pcomps) }
	var rec func(i, j int) (bool, error)
	rec = func(i, j int) (bool, error) {
		states++
		if states > limits.MaxPatternStates {
			return false, errCode(CodePatternLimit, "match", "", "artifact pattern matching exceeded state limit", nil)
		}
		key := [2]int{i, j}
		if v, ok := memo[key]; ok {
			return v, nil
		}
		var ok bool
		defer func() { memo[key] = ok }()
		if j == len(rcomps) {
			ok = remainingCanMatchOne(i)
			return ok, nil
		}
		if i == len(pcomps) {
			return false, nil
		}
		if pcomps[i] == "**" {
			if m, err := rec(i+1, j); err != nil || m {
				ok = m
				return m, err
			}
			if m, err := rec(i, j+1); err != nil || m {
				ok = m
				return m, err
			}
			return false, nil
		}
		m, err := path.Match(pcomps[i], rcomps[j])
		if err != nil || !m {
			return false, err
		}
		ok, err = rec(i+1, j+1)
		return ok, err
	}
	return rec(0, 0)
}
