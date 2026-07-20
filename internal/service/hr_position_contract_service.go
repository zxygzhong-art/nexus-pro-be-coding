package service

import (
	"strconv"
	"strings"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/utils"
)

// ListPositions 列出崗位的服務流程。
func (c HRService) ListPositions(ctx RequestContext) ([]Position, error) {
	if _, _, err := c.Service.requireServiceAuthz(ctx, AppHR, ResourcePosition, ActionRead, ""); err != nil {
		return nil, err
	}
	return c.store.ListPositions(goContext(ctx), ctx.TenantID)
}

// ListPositionPage 列出崗位分頁的服務流程。
func (c HRService) ListPositionPage(ctx RequestContext, page PageRequest) (PageResponse[Position], error) {
	items, err := c.ListPositions(ctx)
	if err != nil {
		return PageResponse[Position]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// CreatePosition persists the position and its authorization audit atomically.
func (c HRService) CreatePosition(ctx RequestContext, input CreatePositionInput) (Position, error) {
	_, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourcePosition, Action: ActionCreate},
		AuditTarget{Event: "hr.position.create", Resource: string(ResourcePosition)},
	)
	if err != nil {
		return Position{}, err
	}
	var position Position
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, err := tx.positionFromInput(ctx, input)
		if err != nil {
			return err
		}
		if err := tx.store.UpsertPosition(goContext(ctx), next); err != nil {
			return err
		}
		authzAudit.target.Target = next.ID
		if err := tx.audit(ctx, "hr.position.create", string(ResourcePosition), next.ID, string(SeverityMedium), auditDecisionDetails(ctx, decision, map[string]any{
			"code": next.Code, "name": next.Name, "status": next.Status,
		})); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		position = next
		return nil
	}); err != nil {
		return Position{}, err
	}
	return position, nil
}

// GetPosition 取得崗位的服務流程。
func (c HRService) GetPosition(ctx RequestContext, id string) (Position, error) {
	if _, _, err := c.Service.requireServiceAuthz(ctx, AppHR, ResourcePosition, ActionRead, id); err != nil {
		return Position{}, err
	}
	position, ok, err := c.store.GetPosition(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return Position{}, err
	}
	if !ok {
		return Position{}, positionNotFound(id)
	}
	return position, nil
}

// UpdatePosition applies the patch and its authorization audit atomically.
func (c HRService) UpdatePosition(ctx RequestContext, id string, input UpdatePositionInput) (Position, error) {
	positionID := strings.TrimSpace(id)
	_, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourcePosition, ResourceID: positionID, Action: ActionUpdate},
		AuditTarget{Event: "hr.position.update", Resource: string(ResourcePosition), Target: positionID},
	)
	if err != nil {
		return Position{}, err
	}
	var position Position
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetPosition(goContext(ctx), ctx.TenantID, positionID)
		if err != nil {
			return err
		}
		if !ok {
			return positionNotFound(id)
		}
		if err := tx.applyPositionPatch(ctx, &next, input); err != nil {
			return err
		}
		if err := tx.store.UpsertPosition(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.position.update", string(ResourcePosition), next.ID, string(SeverityMedium), auditDecisionDetails(ctx, decision, map[string]any{
			"code": next.Code, "name": next.Name, "status": next.Status,
		})); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		position = next
		return nil
	}); err != nil {
		return Position{}, err
	}
	return position, nil
}

// DeletePosition 軟禁用崗位的服務流程。
func (c HRService) DeletePosition(ctx RequestContext, id string) (Position, error) {
	_, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourcePosition, ResourceID: id, Action: ActionDelete},
		AuditTarget{Event: "hr.position.delete", Resource: string(ResourcePosition), Target: id},
	)
	if err != nil {
		return Position{}, err
	}
	var position Position
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetPosition(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
		if err != nil {
			return err
		}
		if !ok {
			return positionNotFound(id)
		}
		next.Status = string(PositionStatusDisabled)
		next.UpdatedAt = tx.Now()
		if err := tx.validatePosition(ctx, next); err != nil {
			return err
		}
		if err := tx.store.UpsertPosition(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.position.delete", string(ResourcePosition), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{"status": next.Status})); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		position = next
		return nil
	}); err != nil {
		return Position{}, err
	}
	return position, nil
}

// BackfillEmployeePositionsFromStrings 依員工既有 position 字串補齊 position_id。
func (c HRService) BackfillEmployeePositionsFromStrings(ctx RequestContext) (int, error) {
	if _, _, err := c.Service.requireServiceAuthz(ctx, AppHR, ResourcePosition, ActionCreate, ""); err != nil {
		return 0, err
	}
	updated := 0
	if err := c.withTransaction(ctx, func(tx HRService) error {
		employees, err := tx.store.ListEmployees(goContext(ctx), ctx.TenantID)
		if err != nil {
			return err
		}
		for _, employee := range employees {
			if strings.TrimSpace(employee.PositionID) != "" || strings.TrimSpace(employee.Position) == "" {
				continue
			}
			next := employee
			if err := tx.ensureEmployeePosition(ctx, &next, true); err != nil {
				return err
			}
			if strings.TrimSpace(next.PositionID) == "" {
				continue
			}
			if err := tx.store.UpsertEmployee(goContext(ctx), next); err != nil {
				return err
			}
			updated++
		}
		return nil
	}); err != nil {
		return 0, err
	}
	return updated, nil
}

// ListEmploymentContractsByEmployee 列出員工合約的服務流程。
func (c HRService) ListEmploymentContractsByEmployee(ctx RequestContext, employeeID string) ([]EmploymentContract, error) {
	account, decision, err := c.Service.requireServiceAuthz(ctx, AppHR, ResourceEmploymentContract, ActionRead, strings.TrimSpace(employeeID))
	if err != nil {
		return nil, err
	}
	if _, err := c.visibleEmployeeForContract(ctx, account, decision, employeeID); err != nil {
		return nil, err
	}
	return c.store.ListEmploymentContractsByEmployee(goContext(ctx), ctx.TenantID, strings.TrimSpace(employeeID))
}

// CreateEmploymentContract 建立員工合約的服務流程。
func (c HRService) CreateEmploymentContract(ctx RequestContext, employeeID string, input CreateEmploymentContractInput) (EmploymentContract, error) {
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmploymentContract, ResourceID: strings.TrimSpace(employeeID), Action: ActionCreate},
		AuditTarget{Event: "hr.contract.create", Resource: string(ResourceEmploymentContract), Target: strings.TrimSpace(employeeID)},
	)
	if err != nil {
		return EmploymentContract{}, err
	}
	if _, err := c.visibleEmployeeForContract(ctx, account, decision, employeeID); err != nil {
		return EmploymentContract{}, err
	}
	contract, err := c.employmentContractFromInput(ctx, employeeID, input)
	if err != nil {
		return EmploymentContract{}, err
	}
	if err := c.withTransaction(ctx, func(tx HRService) error {
		if err := tx.store.UpsertEmploymentContract(goContext(ctx), contract); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.contract.create", string(ResourceEmploymentContract), contract.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{"employee_id": contract.EmployeeID, "status": contract.Status})); err != nil {
			return err
		}
		return authzAudit.CommitWith(ctx, tx.Service)
	}); err != nil {
		return EmploymentContract{}, err
	}
	return contract, nil
}

// GetEmploymentContract 取得員工合約的服務流程。
func (c HRService) GetEmploymentContract(ctx RequestContext, id string) (EmploymentContract, error) {
	account, decision, err := c.Service.requireServiceAuthz(ctx, AppHR, ResourceEmploymentContract, ActionRead, strings.TrimSpace(id))
	if err != nil {
		return EmploymentContract{}, err
	}
	contract, ok, err := c.store.GetEmploymentContract(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
	if err != nil {
		return EmploymentContract{}, err
	}
	if !ok {
		return EmploymentContract{}, employmentContractNotFound(id)
	}
	if _, err := c.visibleEmployeeForContract(ctx, account, decision, contract.EmployeeID); err != nil {
		return EmploymentContract{}, err
	}
	return contract, nil
}

// UpdateEmploymentContract 更新員工合約的服務流程。
func (c HRService) UpdateEmploymentContract(ctx RequestContext, id string, input UpdateEmploymentContractInput) (EmploymentContract, error) {
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmploymentContract, ResourceID: strings.TrimSpace(id), Action: ActionUpdate},
		AuditTarget{Event: "hr.contract.update", Resource: string(ResourceEmploymentContract), Target: strings.TrimSpace(id)},
	)
	if err != nil {
		return EmploymentContract{}, err
	}
	var contract EmploymentContract
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetEmploymentContract(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
		if err != nil {
			return err
		}
		if !ok {
			return employmentContractNotFound(id)
		}
		if _, err := tx.visibleEmployeeForContract(ctx, account, decision, next.EmployeeID); err != nil {
			return err
		}
		if err := tx.applyEmploymentContractPatch(ctx, &next, input); err != nil {
			return err
		}
		if err := tx.store.UpsertEmploymentContract(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.contract.update", string(ResourceEmploymentContract), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{"employee_id": next.EmployeeID, "status": next.Status, "version": next.Version})); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		contract = next
		return nil
	}); err != nil {
		return EmploymentContract{}, err
	}
	return contract, nil
}

// DeleteEmploymentContract 軟終止員工合約的服務流程。
func (c HRService) DeleteEmploymentContract(ctx RequestContext, id string) (EmploymentContract, error) {
	account, decision, authzAudit, err := c.Authorize(ctx,
		CheckRequest{ApplicationCode: AppHR, ResourceType: ResourceEmploymentContract, ResourceID: strings.TrimSpace(id), Action: ActionDelete},
		AuditTarget{Event: "hr.contract.delete", Resource: string(ResourceEmploymentContract), Target: strings.TrimSpace(id)},
	)
	if err != nil {
		return EmploymentContract{}, err
	}
	var contract EmploymentContract
	if err := c.withTransaction(ctx, func(tx HRService) error {
		next, ok, err := tx.store.GetEmploymentContract(goContext(ctx), ctx.TenantID, strings.TrimSpace(id))
		if err != nil {
			return err
		}
		if !ok {
			return employmentContractNotFound(id)
		}
		if _, err := tx.visibleEmployeeForContract(ctx, account, decision, next.EmployeeID); err != nil {
			return err
		}
		if next.Status == string(EmploymentContractStatusTerminated) {
			return Conflict("employment contract is already terminated").WithPublicCode(domain.ErrorCodeEmploymentContractInvalidTransition)
		}
		next.Status = string(EmploymentContractStatusTerminated)
		next.Version++
		next.UpdatedAt = tx.Now()
		if err := tx.store.UpsertEmploymentContract(goContext(ctx), next); err != nil {
			return err
		}
		if err := tx.audit(ctx, "hr.contract.delete", string(ResourceEmploymentContract), next.ID, string(SeverityHigh), auditDecisionDetails(ctx, decision, map[string]any{"employee_id": next.EmployeeID, "status": next.Status, "version": next.Version})); err != nil {
			return err
		}
		if err := authzAudit.CommitWith(ctx, tx.Service); err != nil {
			return err
		}
		contract = next
		return nil
	}); err != nil {
		return EmploymentContract{}, err
	}
	return contract, nil
}

// positionFromInput 建立崗位 domain 物件。
func (c HRService) positionFromInput(ctx RequestContext, input CreatePositionInput) (Position, error) {
	now := c.Now()
	position := Position{
		ID:          utils.NewID("pos"),
		TenantID:    ctx.TenantID,
		Code:        strings.TrimSpace(input.Code),
		Name:        strings.TrimSpace(input.Name),
		OrgUnitID:   strings.TrimSpace(input.OrgUnitID),
		Level:       strings.TrimSpace(input.Level),
		Status:      normalizePositionStatus(input.Status),
		Description: strings.TrimSpace(input.Description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if position.Code == "" {
		code, err := c.availablePositionCode(ctx, position.Name)
		if err != nil {
			return Position{}, err
		}
		position.Code = code
	}
	if err := c.validatePosition(ctx, position); err != nil {
		return Position{}, err
	}
	return position, nil
}

// applyPositionPatch 套用崗位 patch。
func (c HRService) applyPositionPatch(ctx RequestContext, position *Position, input UpdatePositionInput) error {
	if input.Code != nil {
		position.Code = strings.TrimSpace(*input.Code)
	}
	if input.Name != nil {
		position.Name = strings.TrimSpace(*input.Name)
	}
	if input.OrgUnitID != nil {
		position.OrgUnitID = strings.TrimSpace(*input.OrgUnitID)
	}
	if input.Level != nil {
		position.Level = strings.TrimSpace(*input.Level)
	}
	if input.Status != nil {
		position.Status = normalizePositionStatus(*input.Status)
	}
	if input.Description != nil {
		position.Description = strings.TrimSpace(*input.Description)
	}
	position.UpdatedAt = c.Now()
	return c.validatePosition(ctx, *position)
}

// validatePosition 驗證崗位。
func (c HRService) validatePosition(ctx RequestContext, position Position) error {
	fields := make([]FieldError, 0)
	if strings.TrimSpace(position.Name) == "" {
		fields = append(fields, FieldError{Field: "name", Code: "required", Message: "name is required"})
	}
	if strings.TrimSpace(position.Code) == "" {
		fields = append(fields, FieldError{Field: "code", Code: "required", Message: "code is required"})
	}
	if !validPositionStatus(position.Status) {
		fields = append(fields, FieldError{Field: "status", Code: "invalid", Message: "status must be active or disabled"})
	}
	if position.OrgUnitID != "" {
		if _, ok, err := c.store.GetOrgUnit(goContext(ctx), ctx.TenantID, position.OrgUnitID); err != nil {
			return err
		} else if !ok {
			fields = append(fields, FieldError{Field: "org_unit_id", Code: "not_found", Message: "org unit not found"})
		}
	}
	units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
	if err != nil {
		return err
	}
	for _, unit := range units {
		if strings.TrimSpace(unit.ManagerPositionID) != position.ID {
			continue
		}
		if position.Status == string(PositionStatusDisabled) {
			fields = append(fields, FieldError{Field: "status", Code: "in_use", Message: "manager position cannot be disabled"})
		}
		if position.OrgUnitID != unit.ID {
			fields = append(fields, FieldError{Field: "org_unit_id", Code: "in_use", Message: "manager position must remain in its org unit"})
		}
		break
	}
	if position.OrgUnitID != "" {
		employees, err := c.store.ListEmployees(goContext(ctx), ctx.TenantID)
		if err != nil {
			return err
		}
		for _, employee := range employees {
			if strings.TrimSpace(employee.PositionID) != position.ID || strings.TrimSpace(employee.OrgUnitID) == "" {
				continue
			}
			if employee.OrgUnitID != position.OrgUnitID {
				fields = append(fields, FieldError{Field: "org_unit_id", Code: "in_use", Message: "position is used by employees in another org unit"})
				break
			}
		}
	}
	if existing, ok, err := c.store.GetPositionByCode(goContext(ctx), ctx.TenantID, position.Code); err != nil {
		return err
	} else if ok && existing.ID != position.ID {
		return Conflict("position code already exists").WithPublicCode(domain.ErrorCodePositionConflict)
	}
	if len(fields) > 0 {
		return domainValidation("position validation failed", fields...)
	}
	return nil
}

// ensureEmployeePosition 同步員工 position_id 和 position 字串。
func (c HRService) ensureEmployeePosition(ctx RequestContext, employee *Employee, createMissing bool) error {
	positionID := strings.TrimSpace(utils.FirstNonEmpty(employee.PositionID, stringFromMap(employee.EmploymentInfo, "position_id")))
	positionName := strings.TrimSpace(utils.FirstNonEmpty(employee.Position, stringFromMap(employee.EmploymentInfo, "position"), stringFromMap(employee.EmploymentInfo, "job_title")))
	if positionID != "" {
		position, ok, err := c.store.GetPosition(goContext(ctx), ctx.TenantID, positionID)
		if err != nil {
			return err
		}
		if !ok {
			return domainValidation("employee validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "position_id", Code: "not_found", Message: "position not found"})
		}
		if position.Status == string(PositionStatusDisabled) {
			return domainValidation("employee validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "position_id", Code: "invalid", Message: "position is disabled"})
		}
		setEmployeePosition(employee, position)
		return nil
	}
	if positionName == "" {
		return nil
	}
	position, ok, err := c.store.GetPositionByName(goContext(ctx), ctx.TenantID, positionName)
	if err != nil {
		return err
	}
	if ok {
		if position.Status == string(PositionStatusDisabled) {
			return domainValidation("employee validation failed", FieldError{Tab: employeeTabEmploymentInfo, Field: "position", Code: "invalid", Message: "position is disabled"})
		}
		setEmployeePosition(employee, position)
		return nil
	}
	if !createMissing {
		employee.Position = positionName
		return nil
	}
	position, err = c.createPositionFromEmployeeString(ctx, positionName)
	if err != nil {
		return err
	}
	setEmployeePosition(employee, position)
	return nil
}

// createPositionFromEmployeeString 從員工字串建立崗位。
func (c HRService) createPositionFromEmployeeString(ctx RequestContext, name string) (Position, error) {
	code, err := c.availablePositionCode(ctx, name)
	if err != nil {
		return Position{}, err
	}
	now := c.Now()
	position := Position{
		ID:        utils.NewID("pos"),
		TenantID:  ctx.TenantID,
		Code:      code,
		Name:      strings.TrimSpace(name),
		Status:    string(PositionStatusActive),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := c.store.UpsertPosition(goContext(ctx), position); err != nil {
		return Position{}, err
	}
	return position, nil
}

// setEmployeePosition 寫入員工崗位雙寫欄位。
func setEmployeePosition(employee *Employee, position Position) {
	employee.PositionID = position.ID
	employee.Position = position.Name
	if employee.EmploymentInfo == nil {
		employee.EmploymentInfo = map[string]any{}
	}
	employee.EmploymentInfo["position_id"] = position.ID
	employee.EmploymentInfo["position"] = position.Name
}

// employmentContractFromInput 建立員工合約 domain 物件。
func (c HRService) employmentContractFromInput(ctx RequestContext, employeeID string, input CreateEmploymentContractInput) (EmploymentContract, error) {
	startDate, err := requiredContractDate(input.StartDate, "start_date")
	if err != nil {
		return EmploymentContract{}, err
	}
	endDate, err := optionalDateTime(input.EndDate)
	if err != nil {
		return EmploymentContract{}, BadRequest("end_date must be RFC3339 or YYYY-MM-DD")
	}
	now := c.Now()
	contract := EmploymentContract{
		ID:                  utils.NewID("ctr"),
		TenantID:            ctx.TenantID,
		EmployeeID:          strings.TrimSpace(employeeID),
		ContractType:        normalizeEmploymentContractType(input.ContractType),
		ContractNo:          strings.TrimSpace(input.ContractNo),
		StartDate:           *startDate,
		EndDate:             endDate,
		Status:              normalizeEmploymentContractStatus(utils.FirstNonEmpty(input.Status, string(EmploymentContractStatusDraft))),
		AttachmentObjectKey: strings.TrimSpace(input.AttachmentObjectKey),
		Notes:               strings.TrimSpace(input.Notes),
		Version:             1,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := validateEmploymentContract(contract); err != nil {
		return EmploymentContract{}, err
	}
	return contract, nil
}

// applyEmploymentContractPatch 套用員工合約 patch。
func (c HRService) applyEmploymentContractPatch(_ RequestContext, contract *EmploymentContract, input UpdateEmploymentContractInput) error {
	beforeStatus := contract.Status
	if input.ContractType != nil {
		contract.ContractType = normalizeEmploymentContractType(*input.ContractType)
	}
	if input.ContractNo != nil {
		contract.ContractNo = strings.TrimSpace(*input.ContractNo)
	}
	if input.StartDate != nil {
		startDate, err := requiredContractDate(*input.StartDate, "start_date")
		if err != nil {
			return err
		}
		contract.StartDate = *startDate
	}
	if input.EndDate != nil {
		endDate, err := optionalDateTime(*input.EndDate)
		if err != nil {
			return BadRequest("end_date must be RFC3339 or YYYY-MM-DD")
		}
		contract.EndDate = endDate
	}
	if input.Status != nil {
		nextStatus := normalizeEmploymentContractStatus(*input.Status)
		if err := ensureEmploymentContractStatusTransition(beforeStatus, nextStatus); err != nil {
			return err
		}
		contract.Status = nextStatus
	}
	if input.AttachmentObjectKey != nil {
		contract.AttachmentObjectKey = strings.TrimSpace(*input.AttachmentObjectKey)
	}
	if input.Notes != nil {
		contract.Notes = strings.TrimSpace(*input.Notes)
	}
	contract.Version++
	contract.UpdatedAt = c.Now()
	return validateEmploymentContract(*contract)
}

// visibleEmployeeForContract 確認合約所屬員工在授權資料範圍內。
func (c HRService) visibleEmployeeForContract(ctx RequestContext, account Account, decision CheckResult, employeeID string) (Employee, error) {
	employee, ok, err := c.store.GetEmployee(goContext(ctx), ctx.TenantID, strings.TrimSpace(employeeID))
	if err != nil {
		return Employee{}, err
	}
	if !ok {
		return Employee{}, NotFound("employee", employeeID)
	}
	visible, err := c.filterEmployeesByDecision(ctx, account, []Employee{employee}, decision)
	if err != nil {
		return Employee{}, err
	}
	if len(visible) == 0 {
		return Employee{}, ForbiddenDataScope("employee is outside data scope")
	}
	return visible[0], nil
}

// validateEmploymentContract 驗證員工合約。
func validateEmploymentContract(contract EmploymentContract) error {
	fields := make([]FieldError, 0)
	if strings.TrimSpace(contract.EmployeeID) == "" {
		fields = append(fields, FieldError{Field: "employee_id", Code: "required", Message: "employee_id is required"})
	}
	if !validEmploymentContractType(contract.ContractType) {
		fields = append(fields, FieldError{Field: "contract_type", Code: "invalid", Message: "contract_type must be fulltime, parttime, contractor or intern"})
	}
	if contract.StartDate.IsZero() {
		fields = append(fields, FieldError{Field: "start_date", Code: "required", Message: "start_date is required"})
	}
	if contract.EndDate != nil && contract.EndDate.Before(contract.StartDate) {
		fields = append(fields, FieldError{Field: "end_date", Code: "invalid", Message: "end_date must be on or after start_date"})
	}
	if !validEmploymentContractStatus(contract.Status) {
		fields = append(fields, FieldError{Field: "status", Code: "invalid", Message: "status must be draft, active, expired, terminated or renewed"})
	}
	if len(fields) > 0 {
		return domainValidation("employment contract validation failed", fields...)
	}
	return nil
}

// requiredContractDate 解析必填合約日期。
func requiredContractDate(value string, field string) (*time.Time, error) {
	parsed, err := optionalDateTime(value)
	if err != nil {
		return nil, BadRequest(field + " must be RFC3339 or YYYY-MM-DD")
	}
	if parsed == nil {
		return nil, domainValidation("employment contract validation failed", FieldError{Field: field, Code: "required", Message: field + " is required"})
	}
	return parsed, nil
}

// ensureEmploymentContractStatusTransition 驗證合約狀態流轉。
func ensureEmploymentContractStatusTransition(current, next string) error {
	current = normalizeEmploymentContractStatus(current)
	next = normalizeEmploymentContractStatus(next)
	if current == next {
		return nil
	}
	allowed := false
	switch current {
	case string(EmploymentContractStatusDraft):
		allowed = next == string(EmploymentContractStatusActive) || next == string(EmploymentContractStatusTerminated)
	case string(EmploymentContractStatusActive):
		allowed = next == string(EmploymentContractStatusExpired) || next == string(EmploymentContractStatusTerminated) || next == string(EmploymentContractStatusRenewed)
	case string(EmploymentContractStatusExpired):
		allowed = next == string(EmploymentContractStatusRenewed) || next == string(EmploymentContractStatusTerminated)
	case string(EmploymentContractStatusRenewed), string(EmploymentContractStatusTerminated):
		allowed = false
	default:
		allowed = validEmploymentContractStatus(next)
	}
	if allowed {
		return nil
	}
	return BadRequest("invalid employment contract status transition").WithPublicCode(domain.ErrorCodeEmploymentContractInvalidTransition)
}

// normalizePositionStatus 正規化崗位狀態。
func normalizePositionStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", string(PositionStatusActive):
		return string(PositionStatusActive)
	case string(PositionStatusDisabled):
		return string(PositionStatusDisabled)
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

// validPositionStatus 驗證崗位狀態。
func validPositionStatus(status string) bool {
	switch normalizePositionStatus(status) {
	case string(PositionStatusActive), string(PositionStatusDisabled):
		return true
	default:
		return false
	}
}

// normalizeEmploymentContractType 正規化合約類型。
func normalizeEmploymentContractType(contractType string) string {
	switch strings.ToLower(strings.TrimSpace(contractType)) {
	case "full_time", "full-time", string(EmploymentContractTypeFulltime):
		return string(EmploymentContractTypeFulltime)
	case "part_time", "part-time", string(EmploymentContractTypeParttime):
		return string(EmploymentContractTypeParttime)
	case string(EmploymentContractTypeContractor):
		return string(EmploymentContractTypeContractor)
	case string(EmploymentContractTypeIntern):
		return string(EmploymentContractTypeIntern)
	default:
		return strings.ToLower(strings.TrimSpace(contractType))
	}
}

// validEmploymentContractType 驗證合約類型。
func validEmploymentContractType(contractType string) bool {
	switch normalizeEmploymentContractType(contractType) {
	case string(EmploymentContractTypeFulltime), string(EmploymentContractTypeParttime), string(EmploymentContractTypeContractor), string(EmploymentContractTypeIntern):
		return true
	default:
		return false
	}
}

// normalizeEmploymentContractStatus 正規化合約狀態。
func normalizeEmploymentContractStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", string(EmploymentContractStatusDraft):
		return string(EmploymentContractStatusDraft)
	case string(EmploymentContractStatusActive):
		return string(EmploymentContractStatusActive)
	case string(EmploymentContractStatusExpired):
		return string(EmploymentContractStatusExpired)
	case string(EmploymentContractStatusTerminated):
		return string(EmploymentContractStatusTerminated)
	case string(EmploymentContractStatusRenewed):
		return string(EmploymentContractStatusRenewed)
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

// validEmploymentContractStatus 驗證合約狀態。
func validEmploymentContractStatus(status string) bool {
	switch normalizeEmploymentContractStatus(status) {
	case string(EmploymentContractStatusDraft), string(EmploymentContractStatusActive), string(EmploymentContractStatusExpired), string(EmploymentContractStatusTerminated), string(EmploymentContractStatusRenewed):
		return true
	default:
		return false
	}
}

// availablePositionCode 產生可用崗位 code。
func (c HRService) availablePositionCode(ctx RequestContext, name string) (string, error) {
	base := positionCodeFromName(name)
	for i := 0; i < 1000; i++ {
		code := base
		if i > 0 {
			code = base + "-" + strconv.Itoa(i+1)
		}
		_, ok, err := c.store.GetPositionByCode(goContext(ctx), ctx.TenantID, code)
		if err != nil {
			return "", err
		}
		if !ok {
			return code, nil
		}
	}
	return "", Conflict("position code space exhausted").WithPublicCode(domain.ErrorCodePositionConflict)
}

// positionCodeFromName 將名稱轉為 code。
func positionCodeFromName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastHyphen := false
	for _, r := range name {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen && b.Len() > 0 {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	code := strings.Trim(b.String(), "-")
	if code == "" {
		return "position"
	}
	return code
}

// positionNotFound 建立崗位 not found 錯誤。
func positionNotFound(id string) error {
	return NotFound("position", id).WithPublicCode(domain.ErrorCodePositionNotFound)
}

// employmentContractNotFound 建立員工合約 not found 錯誤。
func employmentContractNotFound(id string) error {
	return NotFound("employment contract", id).WithPublicCode(domain.ErrorCodeEmploymentContractNotFound)
}
