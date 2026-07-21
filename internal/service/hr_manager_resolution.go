package service

import (
	"sort"
	"strings"
	"time"
)

const (
	managerSourceOverride = "override"
	managerSourcePosition = "position"
	managerSourceNone     = "none"

	managerIssueOrgUnitNotFound       = "org_unit_not_found"
	managerIssueManagerPositionMissing = "manager_position_missing"
	managerIssueManagerUnfilled       = "manager_unfilled"
	managerIssueManagerAmbiguous      = "manager_ambiguous"
)

// EffectiveManager describes the resolved reporting manager for an employee.
type EffectiveManager struct {
	ManagerEmployeeID string
	Source            string
	Issue             string
	PositionID        string
	DefiningOrgUnitID string
}

// ResolveEffectiveManager derives manager via override, else org → manager position → holder.
// Empty manager_position_id inherits the first non-empty ancestor binding (甲方：子組織沿用上級主管崗).
func ResolveEffectiveManager(employee Employee, employees []Employee, units []OrgUnit, now time.Time) EffectiveManager {
	if override := strings.TrimSpace(employee.ManagerEmployeeID); override != "" {
		return EffectiveManager{ManagerEmployeeID: override, Source: managerSourceOverride}
	}
	return resolvePositionManager(employee.ID, strings.TrimSpace(employee.OrgUnitID), employees, units, now, nil)
}

func resolvePositionManager(employeeID, orgUnitID string, employees []Employee, units []OrgUnit, now time.Time, seenUnits map[string]struct{}) EffectiveManager {
	if orgUnitID == "" {
		return EffectiveManager{Source: managerSourceNone, Issue: managerIssueOrgUnitNotFound}
	}
	if seenUnits == nil {
		seenUnits = map[string]struct{}{}
	}
	if _, loop := seenUnits[orgUnitID]; loop {
		return EffectiveManager{Source: managerSourceNone, Issue: managerIssueOrgUnitNotFound}
	}
	seenUnits[orgUnitID] = struct{}{}

	byID := map[string]OrgUnit{}
	for _, unit := range units {
		byID[unit.ID] = unit
	}
	start, ok := byID[orgUnitID]
	if !ok {
		return EffectiveManager{Source: managerSourceNone, Issue: managerIssueOrgUnitNotFound}
	}

	positionID, definingUnit, found := inheritManagerPosition(start, byID)
	if !found {
		return EffectiveManager{Source: managerSourceNone, Issue: managerIssueManagerPositionMissing}
	}

	holders := managerPositionHolders(definingUnit.ID, positionID, employees, now)
	issue := ""
	if len(holders) == 0 {
		return EffectiveManager{
			Source:            managerSourceNone,
			Issue:             managerIssueManagerUnfilled,
			PositionID:        positionID,
			DefiningOrgUnitID: definingUnit.ID,
		}
	}
	if len(holders) > 1 {
		issue = managerIssueManagerAmbiguous
	}
	holder := holders[0]
	if holder.ID == employeeID {
		parentID := strings.TrimSpace(definingUnit.ParentID)
		if parentID == "" {
			return EffectiveManager{
				Source:            managerSourceNone,
				Issue:             issue,
				PositionID:        positionID,
				DefiningOrgUnitID: definingUnit.ID,
			}
		}
		parentResult := resolvePositionManager(employeeID, parentID, employees, units, now, seenUnits)
		if parentResult.Issue == "" && issue != "" {
			parentResult.Issue = issue
		}
		return parentResult
	}
	return EffectiveManager{
		ManagerEmployeeID: holder.ID,
		Source:            managerSourcePosition,
		Issue:             issue,
		PositionID:        positionID,
		DefiningOrgUnitID: definingUnit.ID,
	}
}

func inheritManagerPosition(start OrgUnit, byID map[string]OrgUnit) (positionID string, defining OrgUnit, ok bool) {
	current := start
	seen := map[string]struct{}{}
	for {
		if _, loop := seen[current.ID]; loop {
			return "", OrgUnit{}, false
		}
		seen[current.ID] = struct{}{}
		if positionID = strings.TrimSpace(current.ManagerPositionID); positionID != "" {
			return positionID, current, true
		}
		parentID := strings.TrimSpace(current.ParentID)
		if parentID == "" {
			return "", OrgUnit{}, false
		}
		parent, exists := byID[parentID]
		if !exists {
			return "", OrgUnit{}, false
		}
		current = parent
	}
}

func managerPositionHolders(orgUnitID, positionID string, employees []Employee, now time.Time) []Employee {
	holders := make([]Employee, 0)
	for _, employee := range employees {
		if strings.TrimSpace(employee.OrgUnitID) != orgUnitID {
			continue
		}
		if strings.TrimSpace(employee.PositionID) != positionID {
			continue
		}
		if !workspaceDirectoryEmployeeVisible(employee, now) {
			continue
		}
		holders = append(holders, employee)
	}
	sort.SliceStable(holders, func(i, j int) bool {
		leftNo := strings.TrimSpace(holders[i].EmployeeNo)
		rightNo := strings.TrimSpace(holders[j].EmployeeNo)
		if leftNo != rightNo {
			return leftNo < rightNo
		}
		return holders[i].ID < holders[j].ID
	})
	return holders
}

// ResolveEffectiveManagerChain walks N levels of effective managers (for relative approvers).
func ResolveEffectiveManagerChain(employee Employee, employees []Employee, units []OrgUnit, now time.Time, levels int) EffectiveManager {
	if levels <= 0 {
		levels = 1
	}
	byID := map[string]Employee{}
	for _, item := range employees {
		byID[item.ID] = item
	}
	current := employee
	var last EffectiveManager
	for i := 0; i < levels; i++ {
		last = ResolveEffectiveManager(current, employees, units, now)
		if strings.TrimSpace(last.ManagerEmployeeID) == "" {
			return last
		}
		next, ok := byID[last.ManagerEmployeeID]
		if !ok {
			return last
		}
		current = next
	}
	return last
}

func orgUnitUsesManagerPosition(units []OrgUnit, positionID string) bool {
	positionID = strings.TrimSpace(positionID)
	if positionID == "" {
		return false
	}
	for _, unit := range units {
		if strings.TrimSpace(unit.ManagerPositionID) == positionID {
			return true
		}
	}
	return false
}
