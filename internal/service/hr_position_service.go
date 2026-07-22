package service

import (
	"strconv"
	"strings"

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

// positionFromInput 建立崗位 domain 物件。
func (c HRService) positionFromInput(ctx RequestContext, input CreatePositionInput) (Position, error) {
	now := c.Now()
	position := Position{
		ID:          utils.NewID("pos"),
		TenantID:    ctx.TenantID,
		Code:        strings.TrimSpace(input.Code),
		Name:        strings.TrimSpace(input.Name),
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
	if existing, ok, err := c.store.GetPositionByCode(goContext(ctx), ctx.TenantID, position.Code); err != nil {
		return err
	} else if ok && existing.ID != position.ID {
		return Conflict("position code already exists").WithPublicCode(domain.ErrorCodePositionConflict)
	}
	if position.Status == string(PositionStatusDisabled) {
		units, err := c.store.ListOrgUnits(goContext(ctx), ctx.TenantID)
		if err != nil {
			return err
		}
		if orgUnitUsesManagerPosition(units, position.ID) {
			fields = append(fields, FieldError{
				Field:   "status",
				Code:    "in_use",
				Message: "position is used as an organization manager position",
			})
		}
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
