package sliceutil

func CopyStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func ContainsString(src []string, target string) bool {
	for _, v := range src {
		if v == target {
			return true
		}
	}
	return false
}

func RemoveString(src []string, target string) []string {
	if len(src) == 0 {
		return nil
	}
	out := src[:0]
	for _, v := range src {
		if v != target {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return append([]string(nil), out...)
}
