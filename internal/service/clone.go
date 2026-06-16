package service

func copyStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func copyStringMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyEmployeeExperiences(src []EmployeeExperience) []EmployeeExperience {
	if len(src) == 0 {
		return nil
	}
	dst := make([]EmployeeExperience, len(src))
	for i, item := range src {
		if item.StartDate != nil {
			t := *item.StartDate
			item.StartDate = &t
		}
		if item.EndDate != nil {
			t := *item.EndDate
			item.EndDate = &t
		}
		dst[i] = item
	}
	return dst
}
