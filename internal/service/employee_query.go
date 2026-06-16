package service

import (
	"sort"
	"strings"
	"time"
)

func (c *Service) listEmployeesForQuery(ctx RequestContext, query EmployeeQuery) ([]Employee, error) {
	return c.store.ListEmployeesByQuery(goContext(ctx), ctx.TenantID, query)
}

func employeeDecisionCanUseStorePage(decision CheckResult) bool {
	switch decision.Scope {
	case "", ScopeAll, ScopeTenant:
		return true
	default:
		return false
	}
}

func normalizeEmployeeQuery(query EmployeeQuery) EmployeeQuery {
	if query.Page <= 0 {
		query.Page = defaultEmployeePage
	}
	if query.PageSize <= 0 {
		query.PageSize = defaultEmployeePageSize
	}
	if query.PageSize > maxEmployeePageSize {
		query.PageSize = maxEmployeePageSize
	}
	if query.Sort == "" {
		query.Sort = "created_at_asc"
	}
	query.EmploymentStatus = normalizeEmployeeStatus(query.EmploymentStatus)
	query.Category = normalizeEmployeeCategory(query.Category)
	return query
}

func filterEmployeeQuery(items []Employee, query EmployeeQuery) []Employee {
	out := make([]Employee, 0, len(items))
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	for _, item := range items {
		if query.EmploymentStatus != string(EmployeeStatusDeleted) && employeeStatus(item) == string(EmployeeStatusDeleted) {
			continue
		}
		if query.DepartmentID != "" && item.OrgUnitID != query.DepartmentID {
			continue
		}
		if query.EmploymentStatus != "" && employeeStatus(item) != query.EmploymentStatus {
			continue
		}
		if query.Category != "" && item.Category != query.Category {
			continue
		}
		if keyword != "" {
			haystack := strings.ToLower(strings.Join([]string{item.Name, item.CompanyEmail, item.PersonalEmail, item.EmployeeNo, item.Phone}, " "))
			if !strings.Contains(haystack, keyword) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func sortEmployees(items []Employee, sortKey string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch sortKey {
		case "created_at_desc":
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.ID > b.ID
			}
			return a.CreatedAt.After(b.CreatedAt)
		case "hire_date_desc":
			return timeValue(a.HireDate).After(timeValue(b.HireDate))
		case "hire_date_asc":
			return timeValue(a.HireDate).Before(timeValue(b.HireDate))
		default:
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.ID < b.ID
			}
			return a.CreatedAt.Before(b.CreatedAt)
		}
	})
}

func paginateEmployees(items []Employee, page, pageSize int) []Employee {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []Employee{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func employeeStatus(item Employee) string {
	return firstNonEmpty(item.EmploymentStatus, item.Status)
}

func normalizeEmployeeStatus(value string) string {
	return NormalizeEmployeeStatus(value)
}

func normalizeEmployeeCategory(value string) string {
	return NormalizeEmployeeCategory(value)
}

func validEmployeeStatus(value string, includeDeleted bool) bool {
	status, ok := ParseEmployeeStatus(value)
	return ok && status.Valid(includeDeleted)
}

func validEmployeeCategory(value string) bool {
	category, ok := ParseEmployeeCategory(value)
	return ok && category.Valid()
}

func sameMonth(t time.Time, ref time.Time) bool {
	t = t.UTC()
	ref = ref.UTC()
	return t.Year() == ref.Year() && t.Month() == ref.Month()
}

func timeValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func formatDate(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func uniqueSorted(values []string) []string {
	return uniqueStrings(values)
}

func employeeStringValues(items []Employee, fn func(Employee) string) []string {
	out := make([]string, 0)
	for _, item := range items {
		if value := strings.TrimSpace(fn(item)); value != "" {
			out = append(out, value)
		}
	}
	return out
}
