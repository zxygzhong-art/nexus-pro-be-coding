package utils

import "nexus-pro-be/internal/domain"

func CopyStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func CopyStringMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func CopyStringStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func CopyEmployeeExperiences(src []domain.EmployeeExperience) []domain.EmployeeExperience {
	if len(src) == 0 {
		return nil
	}
	dst := make([]domain.EmployeeExperience, len(src))
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
