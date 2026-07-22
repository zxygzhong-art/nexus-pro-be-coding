package service

import (
	"strings"

	"nexus-pro-api/internal/utils"
)

// ListAttendanceWorksitePage 列出考勤工作地點分頁的服務流程。
func (c AttendanceService) ListAttendanceWorksitePage(ctx RequestContext, page PageRequest) (PageResponse[AttendanceWorksite], error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceWorksite, ActionRead, ""); err != nil {
		return PageResponse[AttendanceWorksite]{}, err
	}
	items, err := c.store.ListAttendanceWorksites(goContext(ctx), ctx.TenantID)
	if err != nil {
		return PageResponse[AttendanceWorksite]{}, err
	}
	return utils.PageResponse(items, page), nil
}

// CreateAttendanceWorksite 建立考勤工作地點的服務流程。
func (c AttendanceService) CreateAttendanceWorksite(ctx RequestContext, input CreateAttendanceWorksiteInput) (AttendanceWorksite, error) {
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceWorksite, ActionCreate, ""); err != nil {
		return AttendanceWorksite{}, err
	}
	status := normalizeAttendanceStatus(input.Status)
	if err := validateWorksiteInput(input.Name, input.Latitude, input.Longitude, input.RadiusMeters, status); err != nil {
		return AttendanceWorksite{}, err
	}
	now := c.Now()
	item := AttendanceWorksite{
		ID:           utils.NewID("aws"),
		TenantID:     ctx.TenantID,
		Name:         strings.TrimSpace(input.Name),
		Address:      strings.TrimSpace(input.Address),
		Latitude:     input.Latitude,
		Longitude:    input.Longitude,
		RadiusMeters: input.RadiusMeters,
		Status:       status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := c.store.UpsertAttendanceWorksite(goContext(ctx), item); err != nil {
		return AttendanceWorksite{}, err
	}
	if err := c.audit(ctx, "attendance.worksite.create", string(ResourceAttendanceWorksite), item.ID, string(SeverityMedium), map[string]any{"name": item.Name}); err != nil {
		return AttendanceWorksite{}, err
	}
	return item, nil
}

// UpdateAttendanceWorksite 更新考勤工作地點的服務流程。
func (c AttendanceService) UpdateAttendanceWorksite(ctx RequestContext, input UpdateAttendanceWorksiteInput) (AttendanceWorksite, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return AttendanceWorksite{}, BadRequest("id is required")
	}
	if _, _, err := c.requireAttendanceAuthz(ctx, ResourceAttendanceWorksite, ActionUpdate, id); err != nil {
		return AttendanceWorksite{}, err
	}
	item, ok, err := c.store.GetAttendanceWorksite(goContext(ctx), ctx.TenantID, id)
	if err != nil {
		return AttendanceWorksite{}, err
	}
	if !ok {
		return AttendanceWorksite{}, NotFound("attendance worksite", id)
	}
	if input.Name != nil {
		item.Name = strings.TrimSpace(*input.Name)
	}
	if input.Address != nil {
		item.Address = strings.TrimSpace(*input.Address)
	}
	if input.Latitude != nil {
		item.Latitude = *input.Latitude
	}
	if input.Longitude != nil {
		item.Longitude = *input.Longitude
	}
	if input.RadiusMeters != nil {
		item.RadiusMeters = *input.RadiusMeters
	}
	if input.Status != nil {
		item.Status = normalizeAttendanceStatus(*input.Status)
	}
	if err := validateWorksiteInput(item.Name, item.Latitude, item.Longitude, item.RadiusMeters, item.Status); err != nil {
		return AttendanceWorksite{}, err
	}
	item.UpdatedAt = c.Now()
	if err := c.store.UpsertAttendanceWorksite(goContext(ctx), item); err != nil {
		return AttendanceWorksite{}, err
	}
	if err := c.audit(ctx, "attendance.worksite.update", string(ResourceAttendanceWorksite), item.ID, string(SeverityMedium), map[string]any{"name": item.Name}); err != nil {
		return AttendanceWorksite{}, err
	}
	return item, nil
}

// normalizeAttendanceStatus 正規化考勤狀態。
func normalizeAttendanceStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", attendanceStatusActive:
		return attendanceStatusActive
	case attendanceStatusInactive:
		return attendanceStatusInactive
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// validateWorksiteInput 驗證工作地點輸入。
func validateWorksiteInput(name string, latitude, longitude float64, radiusMeters int, status string) error {
	if strings.TrimSpace(name) == "" {
		return BadRequest("name is required")
	}
	if err := validateCoordinates(latitude, longitude); err != nil {
		return err
	}
	if radiusMeters <= 0 {
		return BadRequest("radius_meters must be greater than zero")
	}
	switch status {
	case attendanceStatusActive, attendanceStatusInactive:
		return nil
	default:
		return BadRequest("status must be active or inactive")
	}
}
