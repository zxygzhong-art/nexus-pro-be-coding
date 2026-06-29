package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"nexus-pro-be/internal/domain"
	sqlc "nexus-pro-be/internal/platform/postgres/db"
	"nexus-pro-be/internal/repository"
	"nexus-pro-be/internal/utils"
	"nexus-pro-be/internal/utils/jsoncodec"
	"nexus-pro-be/internal/utils/tenantctx"
)

// Store implements repository.Store using PostgreSQL and generated sqlc queries.
type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
	db   sqlc.DBTX
}

var _ repository.Store = (*Store)(nil)

// NewStore creates a PostgreSQL repository with tenant-scoped query execution.
func NewStore(pool *pgxpool.Pool) *Store {
	db := newTenantDBTX(pool)
	return &Store{pool: pool, q: sqlc.New(db), db: db}
}

func tenantContext(ctx context.Context, tenantID string) context.Context {
	return tenantctx.WithTenantID(ctx, tenantID)
}

// WithTenantTransaction runs fn inside a PostgreSQL transaction with tenant context set.
func (s *Store) WithTenantTransaction(execCtx context.Context, tenantID string, fn func(repository.Store) error) error {
	if s.pool == nil {
		return fn(s)
	}
	if execCtx == nil {
		execCtx = context.Background()
	}
	tx, err := s.pool.Begin(execCtx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(execCtx)
			panic(p)
		}
		// Rollback after commit is harmless in pgx and protects every early return path.
		_ = tx.Rollback(execCtx)
	}()
	if _, err := tx.Exec(execCtx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return err
	}
	txStore := &Store{q: sqlc.New(tx), db: tx}
	if err := fn(txStore); err != nil {
		return err
	}
	return tx.Commit(execCtx)
}

func (s *Store) UpsertTenant(execCtx context.Context, v domain.Tenant) error {
	_, err := s.q.UpsertTenant(execCtx, sqlc.UpsertTenantParams{
		ID:        v.ID,
		Name:      v.Name,
		CreatedAt: timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetTenant(execCtx context.Context, id string) (domain.Tenant, bool, error) {
	v, err := s.q.GetTenant(execCtx, id)
	if isNotFound(err) {
		return domain.Tenant{}, false, nil
	}
	if err != nil {
		return domain.Tenant{}, false, err
	}
	return fromTenant(v), true, nil
}

func (s *Store) ListTenants(execCtx context.Context) ([]domain.Tenant, error) {
	items, err := s.q.ListTenants(execCtx)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromTenant), nil
}

func (s *Store) UpsertAccount(execCtx context.Context, v domain.Account) error {
	_, err := s.q.UpsertAccount(execCtx, sqlc.UpsertAccountParams{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		DisplayName:            v.DisplayName,
		Email:                  v.Email,
		EmployeeID:             v.EmployeeID,
		Status:                 v.Status,
		UserGroupIds:           textArray(v.UserGroupIDs),
		DirectPermissionSetIds: textArray(v.DirectPermissionSetIDs),
		ActiveAssumableRoleID:  v.ActiveAssumableRoleID,
		CreatedAt:              timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetAccount(execCtx context.Context, tenantID, id string) (domain.Account, bool, error) {
	v, err := s.q.GetAccount(execCtx, sqlc.GetAccountParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.Account{}, false, nil
	}
	if err != nil {
		return domain.Account{}, false, err
	}
	return fromAccount(v), true, nil
}

func (s *Store) ListAccounts(execCtx context.Context, tenantID string) ([]domain.Account, error) {
	items, err := s.q.ListAccounts(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAccount), nil
}

func (s *Store) UpsertUserIdentity(execCtx context.Context, v domain.UserIdentity) error {
	_, err := s.q.UpsertUserIdentity(tenantContext(execCtx, v.TenantID), sqlc.UpsertUserIdentityParams{
		ID:        v.ID,
		TenantID:  v.TenantID,
		AccountID: v.AccountID,
		Provider:  v.Provider,
		Subject:   v.Subject,
		Email:     v.Email,
		CreatedAt: timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetUserIdentity(execCtx context.Context, tenantID, provider, subject string) (domain.UserIdentity, bool, error) {
	v, err := s.q.GetUserIdentity(tenantContext(execCtx, tenantID), sqlc.GetUserIdentityParams{
		TenantID: tenantID,
		Provider: provider,
		Subject:  subject,
	})
	if isNotFound(err) {
		return domain.UserIdentity{}, false, nil
	}
	if err != nil {
		return domain.UserIdentity{}, false, err
	}
	return fromUserIdentity(v), true, nil
}

func (s *Store) ListUserIdentities(execCtx context.Context, tenantID, accountID string) ([]domain.UserIdentity, error) {
	items, err := s.q.ListUserIdentities(tenantContext(execCtx, tenantID), sqlc.ListUserIdentitiesParams{TenantID: tenantID, AccountID: accountID})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromUserIdentity), nil
}

func (s *Store) RemoveAccountGroup(execCtx context.Context, tenantID, accountID, groupID string) error {
	account, ok, err := s.GetAccount(execCtx, tenantID, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	account.UserGroupIDs = utils.RemoveString(account.UserGroupIDs, groupID)
	return s.UpsertAccount(execCtx, account)
}

func (s *Store) AddAccountGroup(execCtx context.Context, tenantID, accountID, groupID string) error {
	account, ok, err := s.GetAccount(execCtx, tenantID, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if utils.ContainsString(account.UserGroupIDs, groupID) {
		return nil
	}
	account.UserGroupIDs = append(account.UserGroupIDs, groupID)
	return s.UpsertAccount(execCtx, account)
}

func (s *Store) UpsertUserGroup(execCtx context.Context, v domain.UserGroup) error {
	_, err := s.q.UpsertUserGroup(execCtx, sqlc.UpsertUserGroupParams{
		ID:               v.ID,
		TenantID:         v.TenantID,
		Name:             v.Name,
		Description:      v.Description,
		MemberAccountIds: textArray(v.MemberAccountIDs),
		PermissionSetIds: textArray(v.PermissionSetIDs),
		CreatedAt:        timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetUserGroup(execCtx context.Context, tenantID, id string) (domain.UserGroup, bool, error) {
	v, err := s.q.GetUserGroup(execCtx, sqlc.GetUserGroupParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.UserGroup{}, false, nil
	}
	if err != nil {
		return domain.UserGroup{}, false, err
	}
	return fromUserGroup(v), true, nil
}

func (s *Store) ListUserGroups(execCtx context.Context, tenantID string) ([]domain.UserGroup, error) {
	items, err := s.q.ListUserGroups(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromUserGroup), nil
}

func (s *Store) UpsertPermissionSet(execCtx context.Context, v domain.PermissionSet) error {
	_, err := s.q.UpsertPermissionSet(execCtx, sqlc.UpsertPermissionSetParams{
		ID:          v.ID,
		TenantID:    v.TenantID,
		Name:        v.Name,
		Description: v.Description,
		Column5:     mustJSON(v.Permissions),
		CreatedAt:   timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetPermissionSet(execCtx context.Context, tenantID, id string) (domain.PermissionSet, bool, error) {
	v, err := s.q.GetPermissionSet(execCtx, sqlc.GetPermissionSetParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.PermissionSet{}, false, nil
	}
	if err != nil {
		return domain.PermissionSet{}, false, err
	}
	return fromPermissionSet(v), true, nil
}

func (s *Store) ListPermissionSets(execCtx context.Context, tenantID string) ([]domain.PermissionSet, error) {
	items, err := s.q.ListPermissionSets(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPermissionSet), nil
}

func (s *Store) UpsertPermissionSetAssignment(execCtx context.Context, v domain.PermissionSetAssignment) error {
	_, err := s.q.CreateAuthzPermissionSetAssignment(execCtx, sqlc.CreateAuthzPermissionSetAssignmentParams{
		ID:              v.ID,
		TenantID:        v.TenantID,
		PrincipalType:   v.PrincipalType,
		PrincipalID:     v.PrincipalID,
		PermissionSetID: v.PermissionSetID,
		Effect:          v.Effect,
		DataScopeID:     v.DataScopeID,
		ConditionID:     v.ConditionID,
		StartsAt:        nullableTimestamptz(v.StartsAt),
		ExpiresAt:       nullableTimestamptz(v.ExpiresAt),
		CreatedAt:       timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) ListPermissionSetAssignments(execCtx context.Context, tenantID string) ([]domain.PermissionSetAssignment, error) {
	items, err := s.q.ListAuthzPermissionSetAssignments(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPermissionSetAssignment), nil
}

func (s *Store) ListPermissionSetAssignmentsForPrincipal(execCtx context.Context, tenantID, principalType, principalID string) ([]domain.PermissionSetAssignment, error) {
	items, err := s.q.ListAuthzPermissionSetAssignmentsForPrincipal(execCtx, sqlc.ListAuthzPermissionSetAssignmentsForPrincipalParams{
		TenantID:      tenantID,
		PrincipalType: principalType,
		PrincipalID:   principalID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromPermissionSetAssignment), nil
}

func (s *Store) UpsertDataScope(execCtx context.Context, v domain.DataScope) error {
	_, err := s.q.UpsertAuthzDataScope(execCtx, sqlc.UpsertAuthzDataScopeParams{
		ID:        v.ID,
		TenantID:  v.TenantID,
		Code:      v.Code,
		Name:      v.Name,
		ScopeType: v.ScopeType,
		Column6:   mustJSON(v.Params),
		CreatedAt: timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetDataScope(execCtx context.Context, tenantID, id string) (domain.DataScope, bool, error) {
	v, err := s.q.GetAuthzDataScope(execCtx, sqlc.GetAuthzDataScopeParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.DataScope{}, false, nil
	}
	if err != nil {
		return domain.DataScope{}, false, err
	}
	return fromDataScope(v), true, nil
}

func (s *Store) GetDataScopeByCode(execCtx context.Context, tenantID, code string) (domain.DataScope, bool, error) {
	v, err := s.q.GetAuthzDataScopeByCode(execCtx, sqlc.GetAuthzDataScopeByCodeParams{TenantID: tenantID, Code: code})
	if isNotFound(err) {
		return domain.DataScope{}, false, nil
	}
	if err != nil {
		return domain.DataScope{}, false, err
	}
	return fromDataScope(v), true, nil
}

func (s *Store) ListDataScopes(execCtx context.Context, tenantID string) ([]domain.DataScope, error) {
	items, err := s.q.ListAuthzDataScopes(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromDataScope), nil
}

func (s *Store) UpsertFieldPolicy(execCtx context.Context, v domain.FieldPolicy) error {
	_, err := s.q.UpsertAuthzFieldPolicy(execCtx, sqlc.UpsertAuthzFieldPolicyParams{
		ID:              v.ID,
		TenantID:        v.TenantID,
		ApplicationCode: v.ApplicationCode,
		ResourceType:    v.ResourceType,
		FieldName:       v.FieldName,
		Effect:          v.Effect,
		MaskStrategy:    v.MaskStrategy,
		PermissionID:    v.PermissionID,
		CreatedAt:       timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) ListFieldPolicies(execCtx context.Context, tenantID, applicationCode, resourceType string) ([]domain.FieldPolicy, error) {
	items, err := s.q.ListAuthzFieldPolicies(execCtx, sqlc.ListAuthzFieldPoliciesParams{
		TenantID:        tenantID,
		ApplicationCode: applicationCode,
		ResourceType:    resourceType,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromFieldPolicy), nil
}

func (s *Store) UpsertAssumableRole(execCtx context.Context, v domain.AssumableRole) error {
	duration := v.SessionDurationSeconds
	if duration <= 0 {
		duration = 28800
	}
	_, err := s.q.UpsertAssumableRole(execCtx, sqlc.UpsertAssumableRoleParams{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		Name:                   v.Name,
		Description:            v.Description,
		PermissionSetIds:       textArray(v.PermissionSetIDs),
		Trusted:                v.Trusted,
		Column7:                mustJSON(v.TrustPolicy),
		Column8:                mustJSON(v.PermissionBoundary),
		SessionDurationSeconds: int32(duration),
		CreatedAt:              timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetAssumableRole(execCtx context.Context, tenantID, id string) (domain.AssumableRole, bool, error) {
	v, err := s.q.GetAssumableRole(execCtx, sqlc.GetAssumableRoleParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AssumableRole{}, false, nil
	}
	if err != nil {
		return domain.AssumableRole{}, false, err
	}
	return fromAssumableRole(v), true, nil
}

func (s *Store) ListAssumableRoles(execCtx context.Context, tenantID string) ([]domain.AssumableRole, error) {
	items, err := s.q.ListAssumableRoles(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAssumableRole), nil
}

func (s *Store) UpsertAssumableRoleSession(execCtx context.Context, v domain.AssumableRoleSession) error {
	_, err := s.q.CreateAuthzAssumableRoleSession(execCtx, sqlc.CreateAuthzAssumableRoleSessionParams{
		ID:              v.ID,
		TenantID:        v.TenantID,
		AccountID:       v.AccountID,
		AssumableRoleID: v.AssumableRoleID,
		Column5:         mustJSON(v.SessionPolicy),
		ExpiresAt:       timestamptz(v.ExpiresAt),
		RevokedAt:       nullableTimestamptz(v.RevokedAt),
		CreatedAt:       timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetActiveAssumableRoleSession(execCtx context.Context, tenantID, id string) (domain.AssumableRoleSession, bool, error) {
	v, err := s.q.GetActiveAuthzAssumableRoleSession(execCtx, sqlc.GetActiveAuthzAssumableRoleSessionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AssumableRoleSession{}, false, nil
	}
	if err != nil {
		return domain.AssumableRoleSession{}, false, err
	}
	return fromAssumableRoleSession(v), true, nil
}

func (s *Store) UpsertOrgUnit(execCtx context.Context, v domain.OrgUnit) error {
	_, err := s.q.UpsertOrgUnit(execCtx, sqlc.UpsertOrgUnitParams{
		ID:        v.ID,
		TenantID:  v.TenantID,
		Code:      v.Code,
		Name:      v.Name,
		ParentID:  v.ParentID,
		Path:      utils.CopyStrings(v.Path),
		CreatedAt: timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetOrgUnit(execCtx context.Context, tenantID, id string) (domain.OrgUnit, bool, error) {
	v, err := s.q.GetOrgUnit(execCtx, sqlc.GetOrgUnitParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.OrgUnit{}, false, nil
	}
	if err != nil {
		return domain.OrgUnit{}, false, err
	}
	return fromOrgUnit(v), true, nil
}

func (s *Store) ListOrgUnits(execCtx context.Context, tenantID string) ([]domain.OrgUnit, error) {
	items, err := s.q.ListOrgUnits(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromOrgUnit), nil
}

func (s *Store) UpsertEmployee(execCtx context.Context, v domain.Employee) error {
	_, err := s.q.UpsertEmployee(execCtx, sqlc.UpsertEmployeeParams{
		ID:                    v.ID,
		TenantID:              v.TenantID,
		EmployeeNo:            v.EmployeeNo,
		Name:                  v.Name,
		CompanyEmail:          v.CompanyEmail,
		PersonalEmail:         v.PersonalEmail,
		Phone:                 v.Phone,
		OrgUnitID:             v.OrgUnitID,
		AccountID:             v.AccountID,
		ManagerEmployeeID:     nullableText(v.ManagerEmployeeID),
		Position:              v.Position,
		Category:              v.Category,
		Status:                v.Status,
		EmploymentStatus:      v.EmploymentStatus,
		HireDate:              nullableTimestamptz(v.HireDate),
		ResignDate:            nullableTimestamptz(v.ResignDate),
		BasicInfo:             mustJSON(v.BasicInfo),
		EmploymentInfo:        mustJSON(v.EmploymentInfo),
		EducationMilitaryInfo: mustJSON(v.EducationMilitaryInfo),
		ContactInfo:           mustJSON(v.ContactInfo),
		InsuranceInfo:         mustJSON(v.InsuranceInfo),
		InternalExperiences:   mustJSON(v.InternalExperiences),
		CreatedAt:             timestamptz(v.CreatedAt),
		UpdatedAt:             timestamptz(v.UpdatedAt),
	})
	return err
}

func (s *Store) GetEmployee(execCtx context.Context, tenantID, id string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployee(execCtx, sqlc.GetEmployeeParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

func (s *Store) GetEmployeeByEmployeeNo(execCtx context.Context, tenantID, employeeNo string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployeeByEmployeeNo(execCtx, sqlc.GetEmployeeByEmployeeNoParams{TenantID: tenantID, EmployeeNo: employeeNo})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

func (s *Store) GetEmployeeByCompanyEmail(execCtx context.Context, tenantID, companyEmail string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployeeByCompanyEmail(execCtx, sqlc.GetEmployeeByCompanyEmailParams{TenantID: tenantID, CompanyEmail: companyEmail})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

func (s *Store) GetEmployeeByPersonalEmail(execCtx context.Context, tenantID, personalEmail string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployeeByPersonalEmail(execCtx, sqlc.GetEmployeeByPersonalEmailParams{TenantID: tenantID, PersonalEmail: personalEmail})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

func (s *Store) GetEmployeeByAccountID(execCtx context.Context, tenantID, accountID string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployeeByAccountID(execCtx, sqlc.GetEmployeeByAccountIDParams{TenantID: tenantID, AccountID: accountID})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

func (s *Store) GetEmployeeByBasicInfoField(execCtx context.Context, tenantID, fieldName, fieldValue string) (domain.Employee, bool, error) {
	v, err := s.q.GetEmployeeByBasicInfoField(execCtx, sqlc.GetEmployeeByBasicInfoFieldParams{TenantID: tenantID, FieldName: fieldName, FieldValue: fieldValue})
	if isNotFound(err) {
		return domain.Employee{}, false, nil
	}
	if err != nil {
		return domain.Employee{}, false, err
	}
	return fromEmployee(v), true, nil
}

func (s *Store) ListEmployees(execCtx context.Context, tenantID string) ([]domain.Employee, error) {
	items, err := s.q.ListEmployees(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromEmployee), nil
}

func (s *Store) ListEmployeesByQuery(execCtx context.Context, tenantID string, query domain.EmployeeQuery) ([]domain.Employee, error) {
	items, err := s.q.ListEmployeesFiltered(execCtx, sqlc.ListEmployeesFilteredParams{
		TenantID:         tenantID,
		Keyword:          query.Keyword,
		DepartmentID:     query.DepartmentID,
		EmploymentStatus: query.EmploymentStatus,
		Category:         query.Category,
		Sort:             query.Sort,
	})
	if err != nil {
		return nil, err
	}
	return filterPostgresEmployeesByScope(mapSlice(items, fromEmployee), query.Scope), nil
}

func (s *Store) ListEmployeePageByQuery(execCtx context.Context, tenantID string, query domain.EmployeeQuery) ([]domain.Employee, int, error) {
	if employeeQueryHasScope(query) {
		items, err := s.ListEmployeesByQuery(execCtx, tenantID, query)
		if err != nil {
			return nil, 0, err
		}
		page, pageSize := normalizePostgresEmployeePage(query)
		return paginatePostgresEmployees(items, page, pageSize), len(items), nil
	}
	params := sqlc.CountEmployeesFilteredParams{
		TenantID:         tenantID,
		Keyword:          query.Keyword,
		DepartmentID:     query.DepartmentID,
		EmploymentStatus: query.EmploymentStatus,
		Category:         query.Category,
	}
	total, err := s.q.CountEmployeesFiltered(execCtx, params)
	if err != nil {
		return nil, 0, err
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	items, err := s.q.ListEmployeesFilteredPage(execCtx, sqlc.ListEmployeesFilteredPageParams{
		TenantID:         params.TenantID,
		Keyword:          params.Keyword,
		DepartmentID:     params.DepartmentID,
		EmploymentStatus: params.EmploymentStatus,
		Category:         params.Category,
		Sort:             query.Sort,
		OffsetCount:      int32((page - 1) * pageSize),
		LimitCount:       int32(pageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromEmployee), int(total), nil
}

func (s *Store) CountEmployeesByQuery(execCtx context.Context, tenantID string, query domain.EmployeeQuery) (int, error) {
	if employeeQueryHasScope(query) {
		items, err := s.ListEmployeesByQuery(execCtx, tenantID, query)
		if err != nil {
			return 0, err
		}
		return len(items), nil
	}
	total, err := s.q.CountEmployeesFiltered(execCtx, sqlc.CountEmployeesFilteredParams{
		TenantID:         tenantID,
		Keyword:          query.Keyword,
		DepartmentID:     query.DepartmentID,
		EmploymentStatus: query.EmploymentStatus,
		Category:         query.Category,
	})
	if err != nil {
		return 0, err
	}
	return int(total), nil
}

func employeeQueryHasScope(query domain.EmployeeQuery) bool {
	return query.Scope.DenyAll || len(query.Scope.EmployeeIDs) > 0 || len(query.Scope.OrgUnitIDs) > 0 || len(query.Scope.Statuses) > 0
}

func filterPostgresEmployeesByScope(items []domain.Employee, scope domain.EmployeeScopeConstraint) []domain.Employee {
	if scope.DenyAll {
		return []domain.Employee{}
	}
	employeeAllowed := postgresStringSet(scope.EmployeeIDs)
	orgAllowed := postgresStringSet(scope.OrgUnitIDs)
	statusAllowed := postgresStringSet(scope.Statuses)
	if len(employeeAllowed) == 0 && len(orgAllowed) == 0 && len(statusAllowed) == 0 {
		return items
	}
	out := make([]domain.Employee, 0, len(items))
	for _, item := range items {
		status := utils.FirstNonEmpty(item.EmploymentStatus, item.Status)
		if len(employeeAllowed) > 0 {
			if _, ok := employeeAllowed[item.ID]; !ok {
				continue
			}
		}
		if len(orgAllowed) > 0 {
			if _, ok := orgAllowed[item.OrgUnitID]; !ok {
				continue
			}
		}
		if len(statusAllowed) > 0 {
			if _, ok := statusAllowed[status]; !ok {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func postgresStringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func normalizePostgresEmployeePage(query domain.EmployeeQuery) (int, int) {
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	return page, pageSize
}

func paginatePostgresEmployees(items []domain.Employee, page, pageSize int) []domain.Employee {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []domain.Employee{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	out := make([]domain.Employee, end-start)
	copy(out, items[start:end])
	return out
}

func (s *Store) NextEmployeeNo(execCtx context.Context, tenantID, prefix string) (string, error) {
	nextSeq, err := s.q.NextEmployeeNoSequence(execCtx, sqlc.NextEmployeeNoSequenceParams{
		TenantID:    tenantID,
		Prefix:      prefix,
		InitialNext: 1,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%03d", prefix, nextSeq), nil
}

func (s *Store) UpsertEmployeeImportSession(execCtx context.Context, v domain.EmployeeImportSession) error {
	_, err := s.q.UpsertEmployeeImportSession(execCtx, sqlc.UpsertEmployeeImportSessionParams{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		Filename:             v.Filename,
		ObjectProvider:       v.ObjectProvider,
		ObjectBucket:         v.ObjectBucket,
		ObjectKey:            v.ObjectKey,
		ContentType:          v.ContentType,
		SizeBytes:            v.SizeBytes,
		Sha256:               v.SHA256,
		Status:               v.Status,
		Rows:                 mustJSON(v.Rows),
		Summary:              mustJSON(v.Summary),
		CreatedByAccountID:   v.CreatedByAccountID,
		ConfirmedByAccountID: v.ConfirmedByAccountID,
		CreatedAt:            timestamptz(v.CreatedAt),
		ExpiresAt:            timestamptz(v.ExpiresAt),
		ConfirmedAt:          nullableTimestamptz(v.ConfirmedAt),
	})
	return err
}

func (s *Store) GetEmployeeImportSession(execCtx context.Context, tenantID, id string) (domain.EmployeeImportSession, bool, error) {
	v, err := s.q.GetEmployeeImportSession(execCtx, sqlc.GetEmployeeImportSessionParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.EmployeeImportSession{}, false, nil
	}
	if err != nil {
		return domain.EmployeeImportSession{}, false, err
	}
	return fromEmployeeImportSession(v), true, nil
}

func (s *Store) UpsertLeaveBalance(execCtx context.Context, v domain.LeaveBalance) error {
	_, err := s.q.UpsertLeaveBalance(execCtx, sqlc.UpsertLeaveBalanceParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		EmployeeID:     v.EmployeeID,
		LeaveType:      v.LeaveType,
		RemainingHours: v.RemainingHours,
		UpdatedAt:      timestamptz(v.UpdatedAt),
	})
	return err
}

func (s *Store) GetLeaveBalance(execCtx context.Context, tenantID, id string) (domain.LeaveBalance, bool, error) {
	v, err := s.q.GetLeaveBalance(execCtx, sqlc.GetLeaveBalanceParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.LeaveBalance{}, false, nil
	}
	if err != nil {
		return domain.LeaveBalance{}, false, err
	}
	return fromLeaveBalance(v), true, nil
}

func (s *Store) ListLeaveBalances(execCtx context.Context, tenantID string) ([]domain.LeaveBalance, error) {
	items, err := s.q.ListLeaveBalances(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromLeaveBalance), nil
}

func (s *Store) ReserveLeaveBalance(execCtx context.Context, tenantID, employeeID, leaveType string, hours float64, updatedAt time.Time) (domain.LeaveBalance, bool, bool, error) {
	leaveType = strings.TrimSpace(leaveType)
	v, err := s.q.ReserveLeaveBalance(execCtx, sqlc.ReserveLeaveBalanceParams{
		TenantID:   tenantID,
		EmployeeID: employeeID,
		LeaveType:  leaveType,
		Hours:      hours,
		UpdatedAt:  timestamptz(updatedAt),
	})
	if err == nil {
		return fromLeaveBalance(v), true, true, nil
	}
	if !isNotFound(err) {
		return domain.LeaveBalance{}, false, false, err
	}
	items, listErr := s.q.ListLeaveBalances(tenantContext(execCtx, tenantID), tenantID)
	if listErr != nil {
		return domain.LeaveBalance{}, false, false, listErr
	}
	for _, item := range items {
		if item.EmployeeID == employeeID && strings.EqualFold(item.LeaveType, strings.TrimSpace(leaveType)) {
			return fromLeaveBalance(item), false, true, nil
		}
	}
	return domain.LeaveBalance{}, false, false, nil
}

func (s *Store) UpsertLeaveRequest(execCtx context.Context, v domain.LeaveRequest) error {
	_, err := s.q.UpsertLeaveRequest(execCtx, sqlc.UpsertLeaveRequestParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		EmployeeID:     v.EmployeeID,
		LeaveType:      v.LeaveType,
		StartAt:        timestamptz(v.StartAt),
		EndAt:          timestamptz(v.EndAt),
		Hours:          v.Hours,
		Reason:         v.Reason,
		Status:         v.Status,
		FormInstanceID: v.FormInstanceID,
		CreatedAt:      timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetLeaveRequest(execCtx context.Context, tenantID, id string) (domain.LeaveRequest, bool, error) {
	v, err := s.q.GetLeaveRequest(execCtx, sqlc.GetLeaveRequestParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.LeaveRequest{}, false, nil
	}
	if err != nil {
		return domain.LeaveRequest{}, false, err
	}
	return fromLeaveRequest(v), true, nil
}

func (s *Store) ListLeaveRequests(execCtx context.Context, tenantID string) ([]domain.LeaveRequest, error) {
	items, err := s.q.ListLeaveRequests(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromLeaveRequest), nil
}

func (s *Store) UpsertAttendanceWorksite(execCtx context.Context, v domain.AttendanceWorksite) error {
	_, err := s.q.UpsertAttendanceWorksite(execCtx, sqlc.UpsertAttendanceWorksiteParams{
		ID:           v.ID,
		TenantID:     v.TenantID,
		Name:         v.Name,
		Address:      v.Address,
		Latitude:     v.Latitude,
		Longitude:    v.Longitude,
		RadiusMeters: int32(v.RadiusMeters),
		Status:       v.Status,
		CreatedAt:    timestamptz(v.CreatedAt),
		UpdatedAt:    timestamptz(v.UpdatedAt),
	})
	return err
}

func (s *Store) GetAttendanceWorksite(execCtx context.Context, tenantID, id string) (domain.AttendanceWorksite, bool, error) {
	v, err := s.q.GetAttendanceWorksite(execCtx, sqlc.GetAttendanceWorksiteParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AttendanceWorksite{}, false, nil
	}
	if err != nil {
		return domain.AttendanceWorksite{}, false, err
	}
	return fromAttendanceWorksite(v), true, nil
}

func (s *Store) ListAttendanceWorksites(execCtx context.Context, tenantID string) ([]domain.AttendanceWorksite, error) {
	items, err := s.q.ListAttendanceWorksites(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceWorksite), nil
}

func (s *Store) UpsertAttendanceShift(execCtx context.Context, v domain.AttendanceShift) error {
	_, err := s.q.UpsertAttendanceShift(execCtx, sqlc.UpsertAttendanceShiftParams{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		Name:                   v.Name,
		ClockInStart:           v.ClockInStart,
		ClockInEnd:             v.ClockInEnd,
		ClockOutStart:          v.ClockOutStart,
		ClockOutEnd:            v.ClockOutEnd,
		LateGraceMinutes:       int32(v.LateGraceMinutes),
		EarlyLeaveGraceMinutes: int32(v.EarlyLeaveGraceMinutes),
		Status:                 v.Status,
		CreatedAt:              timestamptz(v.CreatedAt),
		UpdatedAt:              timestamptz(v.UpdatedAt),
	})
	return err
}

func (s *Store) GetAttendanceShift(execCtx context.Context, tenantID, id string) (domain.AttendanceShift, bool, error) {
	v, err := s.q.GetAttendanceShift(execCtx, sqlc.GetAttendanceShiftParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AttendanceShift{}, false, nil
	}
	if err != nil {
		return domain.AttendanceShift{}, false, err
	}
	return fromAttendanceShift(v), true, nil
}

func (s *Store) ListAttendanceShifts(execCtx context.Context, tenantID string) ([]domain.AttendanceShift, error) {
	items, err := s.q.ListAttendanceShifts(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceShift), nil
}

func (s *Store) UpsertAttendanceShiftAssignment(execCtx context.Context, v domain.AttendanceShiftAssignment) error {
	_, err := s.q.UpsertAttendanceShiftAssignment(execCtx, sqlc.UpsertAttendanceShiftAssignmentParams{
		ID:            v.ID,
		TenantID:      v.TenantID,
		EmployeeID:    v.EmployeeID,
		ShiftID:       v.ShiftID,
		WorksiteID:    v.WorksiteID,
		EffectiveFrom: timestamptz(v.EffectiveFrom),
		EffectiveTo:   nullableTimestamptz(v.EffectiveTo),
		Status:        v.Status,
		CreatedAt:     timestamptz(v.CreatedAt),
		UpdatedAt:     timestamptz(v.UpdatedAt),
	})
	return err
}

func (s *Store) GetAttendanceShiftAssignment(execCtx context.Context, tenantID, id string) (domain.AttendanceShiftAssignment, bool, error) {
	v, err := s.q.GetAttendanceShiftAssignment(execCtx, sqlc.GetAttendanceShiftAssignmentParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AttendanceShiftAssignment{}, false, nil
	}
	if err != nil {
		return domain.AttendanceShiftAssignment{}, false, err
	}
	return fromAttendanceShiftAssignment(v), true, nil
}

func (s *Store) ListAttendanceShiftAssignments(execCtx context.Context, tenantID string) ([]domain.AttendanceShiftAssignment, error) {
	items, err := s.q.ListAttendanceShiftAssignments(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceShiftAssignment), nil
}

func (s *Store) FindEffectiveAttendanceShiftAssignment(execCtx context.Context, tenantID, employeeID string, at time.Time) (domain.AttendanceShiftAssignment, bool, error) {
	v, err := s.q.FindEffectiveAttendanceShiftAssignment(execCtx, sqlc.FindEffectiveAttendanceShiftAssignmentParams{
		TenantID:      tenantID,
		EmployeeID:    employeeID,
		EffectiveFrom: timestamptz(at),
	})
	if isNotFound(err) {
		return domain.AttendanceShiftAssignment{}, false, nil
	}
	if err != nil {
		return domain.AttendanceShiftAssignment{}, false, err
	}
	return fromAttendanceShiftAssignment(v), true, nil
}

func (s *Store) UpsertAttendanceClockRecord(execCtx context.Context, v domain.AttendanceClockRecord) error {
	_, err := s.q.UpsertAttendanceClockRecord(execCtx, sqlc.UpsertAttendanceClockRecordParams{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		EmployeeID:          v.EmployeeID,
		ShiftAssignmentID:   v.ShiftAssignmentID,
		ShiftID:             v.ShiftID,
		WorksiteID:          v.WorksiteID,
		WorkDate:            v.WorkDate,
		Direction:           v.Direction,
		ClockedAt:           timestamptz(v.ClockedAt),
		Latitude:            v.Latitude,
		Longitude:           v.Longitude,
		AccuracyMeters:      v.AccuracyMeters,
		DistanceMeters:      v.DistanceMeters,
		RecordStatus:        v.RecordStatus,
		RejectionReason:     v.RejectionReason,
		Source:              v.Source,
		DeviceID:            v.DeviceID,
		Column18:            mustJSON(v.DeviceInfo),
		CorrectionRequestID: v.CorrectionRequestID,
		CreatedAt:           timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetAttendanceClockRecord(execCtx context.Context, tenantID, id string) (domain.AttendanceClockRecord, bool, error) {
	v, err := s.q.GetAttendanceClockRecord(execCtx, sqlc.GetAttendanceClockRecordParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AttendanceClockRecord{}, false, nil
	}
	if err != nil {
		return domain.AttendanceClockRecord{}, false, err
	}
	return fromAttendanceClockRecord(v), true, nil
}

func (s *Store) GetAcceptedAttendanceClockRecord(execCtx context.Context, tenantID, employeeID, workDate, direction string) (domain.AttendanceClockRecord, bool, error) {
	v, err := s.q.GetAcceptedAttendanceClockRecord(execCtx, sqlc.GetAcceptedAttendanceClockRecordParams{
		TenantID:   tenantID,
		EmployeeID: employeeID,
		WorkDate:   workDate,
		Direction:  direction,
	})
	if isNotFound(err) {
		return domain.AttendanceClockRecord{}, false, nil
	}
	if err != nil {
		return domain.AttendanceClockRecord{}, false, err
	}
	return fromAttendanceClockRecord(v), true, nil
}

func (s *Store) ListAttendanceClockRecords(execCtx context.Context, tenantID string, query domain.AttendanceClockRecordQuery) ([]domain.AttendanceClockRecord, error) {
	items, err := s.q.ListAttendanceClockRecords(tenantContext(execCtx, tenantID), sqlc.ListAttendanceClockRecordsParams{
		TenantID:     tenantID,
		EmployeeID:   query.EmployeeID,
		FromDate:     query.FromDate,
		ToDate:       query.ToDate,
		Direction:    query.Direction,
		RecordStatus: query.RecordStatus,
		Source:       query.Source,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceClockRecord), nil
}

func (s *Store) UpsertAttendanceCorrectionRequest(execCtx context.Context, v domain.AttendanceCorrectionRequest) error {
	_, err := s.q.UpsertAttendanceCorrectionRequest(execCtx, sqlc.UpsertAttendanceCorrectionRequestParams{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		EmployeeID:          v.EmployeeID,
		Direction:           v.Direction,
		RequestedClockedAt:  timestamptz(v.RequestedClockedAt),
		WorkDate:            v.WorkDate,
		Reason:              v.Reason,
		Status:              v.Status,
		FormInstanceID:      v.FormInstanceID,
		ClockRecordID:       v.ClockRecordID,
		ReviewedByAccountID: v.ReviewedByAccountID,
		ReviewReason:        v.ReviewReason,
		ReviewedAt:          nullableTimestamptz(v.ReviewedAt),
		CreatedAt:           timestamptz(v.CreatedAt),
		UpdatedAt:           timestamptz(v.UpdatedAt),
	})
	return err
}

func (s *Store) GetAttendanceCorrectionRequest(execCtx context.Context, tenantID, id string) (domain.AttendanceCorrectionRequest, bool, error) {
	v, err := s.q.GetAttendanceCorrectionRequest(execCtx, sqlc.GetAttendanceCorrectionRequestParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AttendanceCorrectionRequest{}, false, nil
	}
	if err != nil {
		return domain.AttendanceCorrectionRequest{}, false, err
	}
	return fromAttendanceCorrectionRequest(v), true, nil
}

func (s *Store) ListAttendanceCorrectionRequests(execCtx context.Context, tenantID string, query domain.AttendanceCorrectionQuery) ([]domain.AttendanceCorrectionRequest, error) {
	items, err := s.q.ListAttendanceCorrectionRequests(tenantContext(execCtx, tenantID), sqlc.ListAttendanceCorrectionRequestsParams{
		TenantID:   tenantID,
		EmployeeID: query.EmployeeID,
		FromDate:   query.FromDate,
		ToDate:     query.ToDate,
		Status:     query.Status,
		Direction:  query.Direction,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAttendanceCorrectionRequest), nil
}

func (s *Store) UpsertFormTemplate(execCtx context.Context, v domain.FormTemplate) error {
	_, err := s.q.UpsertFormTemplate(execCtx, sqlc.UpsertFormTemplateParams{
		ID:          v.ID,
		TenantID:    v.TenantID,
		Key:         v.Key,
		Name:        v.Name,
		Description: v.Description,
		Column6:     mustJSON(v.Schema),
		CreatedAt:   timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) GetFormTemplate(execCtx context.Context, tenantID, id string) (domain.FormTemplate, bool, error) {
	v, err := s.q.GetFormTemplate(execCtx, sqlc.GetFormTemplateParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.FormTemplate{}, false, nil
	}
	if err != nil {
		return domain.FormTemplate{}, false, err
	}
	return fromFormTemplate(v), true, nil
}

func (s *Store) GetFormTemplateByKey(execCtx context.Context, tenantID, key string) (domain.FormTemplate, bool, error) {
	v, err := s.q.GetFormTemplateByKey(execCtx, sqlc.GetFormTemplateByKeyParams{TenantID: tenantID, Key: key})
	if isNotFound(err) {
		return domain.FormTemplate{}, false, nil
	}
	if err != nil {
		return domain.FormTemplate{}, false, err
	}
	return fromFormTemplate(v), true, nil
}

func (s *Store) ListFormTemplates(execCtx context.Context, tenantID string) ([]domain.FormTemplate, error) {
	items, err := s.q.ListFormTemplates(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromFormTemplate), nil
}

func (s *Store) UpsertFormInstance(execCtx context.Context, v domain.FormInstance) error {
	_, err := s.q.UpsertFormInstance(execCtx, sqlc.UpsertFormInstanceParams{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		TemplateID:         v.TemplateID,
		ApplicantAccountID: v.ApplicantAccountID,
		Status:             v.Status,
		Column6:            mustJSON(v.Payload),
		SubmittedAt:        timestamptz(v.SubmittedAt),
		ApprovedBy:         v.ApprovedBy,
		UpdatedAt:          timestamptz(v.UpdatedAt),
	})
	return err
}

func (s *Store) GetFormInstance(execCtx context.Context, tenantID, id string) (domain.FormInstance, bool, error) {
	v, err := s.q.GetFormInstance(execCtx, sqlc.GetFormInstanceParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.FormInstance{}, false, nil
	}
	if err != nil {
		return domain.FormInstance{}, false, err
	}
	return fromFormInstance(v), true, nil
}

func (s *Store) ListFormInstances(execCtx context.Context, tenantID string) ([]domain.FormInstance, error) {
	items, err := s.q.ListFormInstances(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromFormInstance), nil
}

func (s *Store) UpsertKnowledgeArticle(execCtx context.Context, v domain.KnowledgeArticle) error {
	_, err := s.q.UpsertKnowledgeArticle(execCtx, sqlc.UpsertKnowledgeArticleParams{
		ID:        v.ID,
		TenantID:  v.TenantID,
		Title:     v.Title,
		Content:   v.Content,
		Tags:      utils.CopyStrings(v.Tags),
		CreatedAt: timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) ListKnowledgeArticles(execCtx context.Context, tenantID string) ([]domain.KnowledgeArticle, error) {
	items, err := s.q.ListKnowledgeArticles(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromKnowledgeArticle), nil
}

func (s *Store) UpsertAgentRun(execCtx context.Context, v domain.AgentRun) error {
	_, err := s.q.UpsertAgentRun(execCtx, sqlc.UpsertAgentRunParams{
		ID:        v.ID,
		TenantID:  v.TenantID,
		AccountID: v.AccountID,
		Mode:      v.Mode,
		Prompt:    v.Prompt,
		Answer:    v.Answer,
		Status:    v.Status,
		Column8:   mustJSON(v.References),
		CreatedAt: timestamptz(v.CreatedAt),
		UpdatedAt: timestamptz(v.UpdatedAt),
	})
	return err
}

func (s *Store) GetAgentRun(execCtx context.Context, tenantID, id string) (domain.AgentRun, bool, error) {
	v, err := s.q.GetAgentRun(execCtx, sqlc.GetAgentRunParams{TenantID: tenantID, ID: id})
	if isNotFound(err) {
		return domain.AgentRun{}, false, nil
	}
	if err != nil {
		return domain.AgentRun{}, false, err
	}
	return fromAgentRun(v), true, nil
}

func (s *Store) ListAgentRuns(execCtx context.Context, tenantID string) ([]domain.AgentRun, error) {
	items, err := s.q.ListAgentRuns(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAgentRun), nil
}

func (s *Store) ListAgentRunPage(execCtx context.Context, tenantID string, page domain.PageRequest) ([]domain.AgentRun, int, error) {
	page = utils.NormalizePageRequest(page)
	total, err := s.q.CountAgentRuns(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.q.ListAgentRunsPage(tenantContext(execCtx, tenantID), sqlc.ListAgentRunsPageParams{
		TenantID:    tenantID,
		Sort:        page.Sort,
		LimitCount:  int32(page.PageSize),
		OffsetCount: int32((page.Page - 1) * page.PageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromAgentRun), int(total), nil
}

func (s *Store) AppendAuditLog(execCtx context.Context, v domain.AuditLog) error {
	_, err := s.q.AppendAuditLog(execCtx, sqlc.AppendAuditLogParams{
		ID:             v.ID,
		TenantID:       v.TenantID,
		ActorAccountID: v.ActorAccountID,
		Action:         v.Action,
		Resource:       v.Resource,
		Target:         v.Target,
		Result:         v.Result,
		TraceID:        v.TraceID,
		Severity:       v.Severity,
		Column10:       mustJSON(v.Details),
		CreatedAt:      timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) ListAuditLogs(execCtx context.Context, tenantID string) ([]domain.AuditLog, error) {
	items, err := s.q.ListAuditLogs(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAuditLog), nil
}

func (s *Store) ListAuditLogPage(execCtx context.Context, tenantID string, page domain.PageRequest) ([]domain.AuditLog, int, error) {
	page = utils.NormalizePageRequest(page)
	total, err := s.q.CountAuditLogs(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.q.ListAuditLogsPage(execCtx, sqlc.ListAuditLogsPageParams{
		TenantID:    tenantID,
		Sort:        page.Sort,
		LimitCount:  int32(page.PageSize),
		OffsetCount: int32((page.Page - 1) * page.PageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	return mapSlice(items, fromAuditLog), int(total), nil
}

func (s *Store) GetPermissionVersion(execCtx context.Context, tenantID string) (int64, error) {
	v, err := s.q.GetAuthzPermissionVersion(tenantContext(execCtx, tenantID), tenantID)
	if isNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return v.Version, nil
}

func (s *Store) IncrementPermissionVersion(execCtx context.Context, tenantID string) (int64, error) {
	v, err := s.q.IncrementAuthzPermissionVersion(execCtx, sqlc.IncrementAuthzPermissionVersionParams{
		TenantID:  tenantID,
		UpdatedAt: timestamptz(time.Now()),
	})
	if err != nil {
		return 0, err
	}
	return v.Version, nil
}

func (s *Store) UpsertAuthzRelationshipTuple(execCtx context.Context, v domain.AuthzRelationshipTuple) error {
	_, err := s.q.UpsertAuthzRelationshipTuple(execCtx, sqlc.UpsertAuthzRelationshipTupleParams{
		ID:          v.ID,
		TenantID:    v.TenantID,
		ObjectType:  v.ObjectType,
		ObjectID:    v.ObjectID,
		Relation:    v.Relation,
		SubjectType: v.SubjectType,
		SubjectID:   v.SubjectID,
		CreatedAt:   timestamptz(v.CreatedAt),
	})
	return err
}

func (s *Store) DeleteAuthzRelationshipTuple(execCtx context.Context, v domain.AuthzRelationshipTuple) error {
	return s.q.DeleteAuthzRelationshipTuple(execCtx, sqlc.DeleteAuthzRelationshipTupleParams{
		TenantID:    v.TenantID,
		ObjectType:  v.ObjectType,
		ObjectID:    v.ObjectID,
		Relation:    v.Relation,
		SubjectType: v.SubjectType,
		SubjectID:   v.SubjectID,
	})
}

func (s *Store) ListAuthzRelationshipTuplesForObject(execCtx context.Context, tenantID, objectType, objectID string) ([]domain.AuthzRelationshipTuple, error) {
	items, err := s.q.ListAuthzRelationshipTuplesForObject(tenantContext(execCtx, tenantID), sqlc.ListAuthzRelationshipTuplesForObjectParams{
		TenantID:   tenantID,
		ObjectType: objectType,
		ObjectID:   objectID,
	})
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAuthzRelationshipTuple), nil
}

func (s *Store) AppendAuthzOutboxEvent(execCtx context.Context, v domain.AuthzOutboxEvent) error {
	_, err := s.q.AppendAuthzOutboxEvent(execCtx, sqlc.AppendAuthzOutboxEventParams{
		ID:          v.ID,
		TenantID:    v.TenantID,
		EventType:   v.EventType,
		Column4:     mustJSON(v.Payload),
		Status:      v.Status,
		RetryCount:  int32(v.RetryCount),
		LastError:   v.LastError,
		CreatedAt:   timestamptz(v.CreatedAt),
		ProcessedAt: nullableTimestamptz(v.ProcessedAt),
	})
	return err
}

func (s *Store) ListAuthzOutboxEvents(execCtx context.Context, tenantID string) ([]domain.AuthzOutboxEvent, error) {
	items, err := s.q.ListAuthzOutboxEvents(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromAuthzOutboxEvent), nil
}

func (s *Store) UpdateAuthzOutboxEvent(execCtx context.Context, v domain.AuthzOutboxEvent) error {
	_, err := s.q.UpdateAuthzOutboxEvent(tenantContext(execCtx, v.TenantID), sqlc.UpdateAuthzOutboxEventParams{
		TenantID:    v.TenantID,
		ID:          v.ID,
		Status:      v.Status,
		RetryCount:  int32(v.RetryCount),
		LastError:   v.LastError,
		ProcessedAt: nullableTimestamptz(v.ProcessedAt),
	})
	return err
}

func (s *Store) AppendOutboxEvent(execCtx context.Context, v domain.OutboxEvent) error {
	_, err := s.q.AppendOutboxEvent(execCtx, sqlc.AppendOutboxEventParams{
		ID:            v.ID,
		TenantID:      v.TenantID,
		EventType:     v.EventType,
		AggregateType: v.AggregateType,
		AggregateID:   v.AggregateID,
		Column6:       mustJSON(v.Payload),
		Status:        v.Status,
		RetryCount:    int32(v.RetryCount),
		LastError:     v.LastError,
		CreatedAt:     timestamptz(v.CreatedAt),
		ProcessedAt:   nullableTimestamptz(v.ProcessedAt),
	})
	return err
}

func (s *Store) ListOutboxEvents(execCtx context.Context, tenantID string) ([]domain.OutboxEvent, error) {
	items, err := s.q.ListOutboxEvents(tenantContext(execCtx, tenantID), tenantID)
	if err != nil {
		return nil, err
	}
	return mapSlice(items, fromOutboxEvent), nil
}

func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func timestamptz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func nullableTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil || t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return timestamptz(*t)
}

func nullableText(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func textArray(values []string) []string {
	out := utils.CopyStrings(values)
	if out == nil {
		return []string{}
	}
	return out
}

func textFrom(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func timeFrom(v pgtype.Timestamptz) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return v.Time.UTC()
}

func timePtrFrom(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time.UTC()
	return &t
}

func mustJSON(v any) []byte {
	return jsoncodec.Must(v)
}

func jsonMap(b []byte) map[string]any {
	return jsoncodec.Map(b)
}

func jsonEmployeeExperiences(b []byte) []domain.EmployeeExperience {
	if len(b) == 0 {
		return nil
	}
	var out []domain.EmployeeExperience
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

func jsonEmployeeImportRows(b []byte) []domain.EmployeeImportRow {
	if len(b) == 0 {
		return nil
	}
	var out []domain.EmployeeImportRow
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

func jsonPermissions(b []byte) []domain.Permission {
	return jsoncodec.Permissions(b)
}

func jsonRefs(b []byte) []domain.Reference {
	if len(b) == 0 {
		return nil
	}
	var out []domain.Reference
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

func mapSlice[S any, D any](items []S, convert func(S) D) []D {
	if len(items) == 0 {
		return []D{}
	}
	out := make([]D, 0, len(items))
	for _, item := range items {
		out = append(out, convert(item))
	}
	return out
}

func fromTenant(v sqlc.Tenant) domain.Tenant {
	return domain.Tenant{ID: v.ID, Name: v.Name, CreatedAt: timeFrom(v.CreatedAt)}
}

func fromAccount(v sqlc.Account) domain.Account {
	return domain.Account{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		DisplayName:            v.DisplayName,
		Email:                  v.Email,
		EmployeeID:             v.EmployeeID,
		Status:                 v.Status,
		UserGroupIDs:           utils.CopyStrings(v.UserGroupIds),
		DirectPermissionSetIDs: utils.CopyStrings(v.DirectPermissionSetIds),
		ActiveAssumableRoleID:  v.ActiveAssumableRoleID,
		CreatedAt:              timeFrom(v.CreatedAt),
	}
}

func fromUserIdentity(v sqlc.UserIdentity) domain.UserIdentity {
	return domain.UserIdentity{
		ID:        v.ID,
		TenantID:  v.TenantID,
		AccountID: v.AccountID,
		Provider:  v.Provider,
		Subject:   v.Subject,
		Email:     v.Email,
		CreatedAt: timeFrom(v.CreatedAt),
	}
}

func fromUserGroup(v sqlc.UserGroup) domain.UserGroup {
	return domain.UserGroup{
		ID:               v.ID,
		TenantID:         v.TenantID,
		Name:             v.Name,
		Description:      v.Description,
		MemberAccountIDs: utils.CopyStrings(v.MemberAccountIds),
		PermissionSetIDs: utils.CopyStrings(v.PermissionSetIds),
		CreatedAt:        timeFrom(v.CreatedAt),
	}
}

func fromPermissionSet(v sqlc.PermissionSet) domain.PermissionSet {
	return domain.PermissionSet{
		ID:          v.ID,
		TenantID:    v.TenantID,
		Name:        v.Name,
		Description: v.Description,
		Permissions: jsonPermissions(v.Permissions),
		CreatedAt:   timeFrom(v.CreatedAt),
	}
}

func fromPermissionSetAssignment(v sqlc.AuthzPermissionSetAssignment) domain.PermissionSetAssignment {
	return domain.PermissionSetAssignment{
		ID:              v.ID,
		TenantID:        v.TenantID,
		PrincipalType:   v.PrincipalType,
		PrincipalID:     v.PrincipalID,
		PermissionSetID: v.PermissionSetID,
		Effect:          v.Effect,
		DataScopeID:     v.DataScopeID,
		ConditionID:     v.ConditionID,
		StartsAt:        timePtrFrom(v.StartsAt),
		ExpiresAt:       timePtrFrom(v.ExpiresAt),
		CreatedAt:       timeFrom(v.CreatedAt),
	}
}

func fromDataScope(v sqlc.AuthzDataScope) domain.DataScope {
	return domain.DataScope{
		ID:        v.ID,
		TenantID:  v.TenantID,
		Code:      v.Code,
		Name:      v.Name,
		ScopeType: v.ScopeType,
		Params:    jsonMap(v.Params),
		CreatedAt: timeFrom(v.CreatedAt),
	}
}

func fromFieldPolicy(v sqlc.AuthzFieldPolicy) domain.FieldPolicy {
	return domain.FieldPolicy{
		ID:              v.ID,
		TenantID:        v.TenantID,
		ApplicationCode: v.ApplicationCode,
		ResourceType:    v.ResourceType,
		FieldName:       v.FieldName,
		Effect:          v.Effect,
		MaskStrategy:    v.MaskStrategy,
		PermissionID:    v.PermissionID,
		CreatedAt:       timeFrom(v.CreatedAt),
	}
}

func fromAssumableRole(v sqlc.AssumableRole) domain.AssumableRole {
	return domain.AssumableRole{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		Name:                   v.Name,
		Description:            v.Description,
		PermissionSetIDs:       utils.CopyStrings(v.PermissionSetIds),
		Trusted:                v.Trusted,
		TrustPolicy:            jsonMap(v.TrustPolicy),
		PermissionBoundary:     jsonMap(v.PermissionBoundary),
		SessionDurationSeconds: int(v.SessionDurationSeconds),
		CreatedAt:              timeFrom(v.CreatedAt),
	}
}

func fromAssumableRoleSession(v sqlc.AuthzAssumableRoleSession) domain.AssumableRoleSession {
	return domain.AssumableRoleSession{
		ID:              v.ID,
		TenantID:        v.TenantID,
		AccountID:       v.AccountID,
		AssumableRoleID: v.AssumableRoleID,
		SessionPolicy:   jsonMap(v.SessionPolicy),
		ExpiresAt:       timeFrom(v.ExpiresAt),
		RevokedAt:       timePtrFrom(v.RevokedAt),
		CreatedAt:       timeFrom(v.CreatedAt),
	}
}

func fromOrgUnit(v sqlc.OrgUnit) domain.OrgUnit {
	return domain.OrgUnit{
		ID:        v.ID,
		TenantID:  v.TenantID,
		Code:      v.Code,
		Name:      v.Name,
		ParentID:  v.ParentID,
		Path:      utils.CopyStrings(v.Path),
		CreatedAt: timeFrom(v.CreatedAt),
	}
}

func fromEmployee(v sqlc.Employee) domain.Employee {
	return domain.Employee{
		ID:                    v.ID,
		TenantID:              v.TenantID,
		EmployeeNo:            v.EmployeeNo,
		Name:                  v.Name,
		CompanyEmail:          v.CompanyEmail,
		PersonalEmail:         v.PersonalEmail,
		Phone:                 v.Phone,
		OrgUnitID:             v.OrgUnitID,
		AccountID:             v.AccountID,
		ManagerEmployeeID:     textFrom(v.ManagerEmployeeID),
		Position:              v.Position,
		Category:              v.Category,
		Status:                v.Status,
		EmploymentStatus:      v.EmploymentStatus,
		HireDate:              timePtrFrom(v.HireDate),
		ResignDate:            timePtrFrom(v.ResignDate),
		BasicInfo:             jsonMap(v.BasicInfo),
		EmploymentInfo:        jsonMap(v.EmploymentInfo),
		EducationMilitaryInfo: jsonMap(v.EducationMilitaryInfo),
		ContactInfo:           jsonMap(v.ContactInfo),
		InsuranceInfo:         jsonMap(v.InsuranceInfo),
		InternalExperiences:   jsonEmployeeExperiences(v.InternalExperiences),
		CreatedAt:             timeFrom(v.CreatedAt),
		UpdatedAt:             timeFrom(v.UpdatedAt),
	}
}

func fromEmployeeImportSession(v sqlc.EmployeeImportSession) domain.EmployeeImportSession {
	return domain.EmployeeImportSession{
		ID:                   v.ID,
		TenantID:             v.TenantID,
		Filename:             v.Filename,
		ObjectProvider:       v.ObjectProvider,
		ObjectBucket:         v.ObjectBucket,
		ObjectKey:            v.ObjectKey,
		ContentType:          v.ContentType,
		SizeBytes:            v.SizeBytes,
		SHA256:               v.Sha256,
		Status:               v.Status,
		Rows:                 jsonEmployeeImportRows(v.Rows),
		Summary:              jsonMap(v.Summary),
		CreatedByAccountID:   v.CreatedByAccountID,
		ConfirmedByAccountID: v.ConfirmedByAccountID,
		CreatedAt:            timeFrom(v.CreatedAt),
		ExpiresAt:            timeFrom(v.ExpiresAt),
		ConfirmedAt:          timePtrFrom(v.ConfirmedAt),
	}
}

func fromOutboxEvent(v sqlc.OutboxEvent) domain.OutboxEvent {
	return domain.OutboxEvent{
		ID:            v.ID,
		TenantID:      v.TenantID,
		EventType:     v.EventType,
		AggregateType: v.AggregateType,
		AggregateID:   v.AggregateID,
		Payload:       jsonMap(v.Payload),
		Status:        v.Status,
		RetryCount:    int(v.RetryCount),
		LastError:     v.LastError,
		CreatedAt:     timeFrom(v.CreatedAt),
		ProcessedAt:   timePtrFrom(v.ProcessedAt),
	}
}

func fromLeaveBalance(v sqlc.LeaveBalance) domain.LeaveBalance {
	return domain.LeaveBalance{
		ID:             v.ID,
		TenantID:       v.TenantID,
		EmployeeID:     v.EmployeeID,
		LeaveType:      v.LeaveType,
		RemainingHours: v.RemainingHours,
		UpdatedAt:      timeFrom(v.UpdatedAt),
	}
}

func fromLeaveRequest(v sqlc.LeaveRequest) domain.LeaveRequest {
	return domain.LeaveRequest{
		ID:             v.ID,
		TenantID:       v.TenantID,
		EmployeeID:     v.EmployeeID,
		LeaveType:      v.LeaveType,
		StartAt:        timeFrom(v.StartAt),
		EndAt:          timeFrom(v.EndAt),
		Hours:          v.Hours,
		Reason:         v.Reason,
		Status:         v.Status,
		FormInstanceID: v.FormInstanceID,
		CreatedAt:      timeFrom(v.CreatedAt),
	}
}

func fromAttendanceWorksite(v sqlc.AttendanceWorksite) domain.AttendanceWorksite {
	return domain.AttendanceWorksite{
		ID:           v.ID,
		TenantID:     v.TenantID,
		Name:         v.Name,
		Address:      v.Address,
		Latitude:     v.Latitude,
		Longitude:    v.Longitude,
		RadiusMeters: int(v.RadiusMeters),
		Status:       v.Status,
		CreatedAt:    timeFrom(v.CreatedAt),
		UpdatedAt:    timeFrom(v.UpdatedAt),
	}
}

func fromAttendanceShift(v sqlc.AttendanceShift) domain.AttendanceShift {
	return domain.AttendanceShift{
		ID:                     v.ID,
		TenantID:               v.TenantID,
		Name:                   v.Name,
		ClockInStart:           v.ClockInStart,
		ClockInEnd:             v.ClockInEnd,
		ClockOutStart:          v.ClockOutStart,
		ClockOutEnd:            v.ClockOutEnd,
		LateGraceMinutes:       int(v.LateGraceMinutes),
		EarlyLeaveGraceMinutes: int(v.EarlyLeaveGraceMinutes),
		Status:                 v.Status,
		CreatedAt:              timeFrom(v.CreatedAt),
		UpdatedAt:              timeFrom(v.UpdatedAt),
	}
}

func fromAttendanceShiftAssignment(v sqlc.AttendanceShiftAssignment) domain.AttendanceShiftAssignment {
	return domain.AttendanceShiftAssignment{
		ID:            v.ID,
		TenantID:      v.TenantID,
		EmployeeID:    v.EmployeeID,
		ShiftID:       v.ShiftID,
		WorksiteID:    v.WorksiteID,
		EffectiveFrom: timeFrom(v.EffectiveFrom),
		EffectiveTo:   timePtrFrom(v.EffectiveTo),
		Status:        v.Status,
		CreatedAt:     timeFrom(v.CreatedAt),
		UpdatedAt:     timeFrom(v.UpdatedAt),
	}
}

func fromAttendanceClockRecord(v sqlc.AttendanceClockRecord) domain.AttendanceClockRecord {
	return domain.AttendanceClockRecord{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		EmployeeID:          v.EmployeeID,
		ShiftAssignmentID:   v.ShiftAssignmentID,
		ShiftID:             v.ShiftID,
		WorksiteID:          v.WorksiteID,
		WorkDate:            v.WorkDate,
		Direction:           v.Direction,
		ClockedAt:           timeFrom(v.ClockedAt),
		Latitude:            v.Latitude,
		Longitude:           v.Longitude,
		AccuracyMeters:      v.AccuracyMeters,
		DistanceMeters:      v.DistanceMeters,
		RecordStatus:        v.RecordStatus,
		RejectionReason:     v.RejectionReason,
		Source:              v.Source,
		DeviceID:            v.DeviceID,
		DeviceInfo:          jsonMap(v.DeviceInfo),
		CorrectionRequestID: v.CorrectionRequestID,
		CreatedAt:           timeFrom(v.CreatedAt),
	}
}

func fromAttendanceCorrectionRequest(v sqlc.AttendanceCorrectionRequest) domain.AttendanceCorrectionRequest {
	return domain.AttendanceCorrectionRequest{
		ID:                  v.ID,
		TenantID:            v.TenantID,
		EmployeeID:          v.EmployeeID,
		Direction:           v.Direction,
		RequestedClockedAt:  timeFrom(v.RequestedClockedAt),
		WorkDate:            v.WorkDate,
		Reason:              v.Reason,
		Status:              v.Status,
		FormInstanceID:      v.FormInstanceID,
		ClockRecordID:       v.ClockRecordID,
		ReviewedByAccountID: v.ReviewedByAccountID,
		ReviewReason:        v.ReviewReason,
		ReviewedAt:          timePtrFrom(v.ReviewedAt),
		CreatedAt:           timeFrom(v.CreatedAt),
		UpdatedAt:           timeFrom(v.UpdatedAt),
	}
}

func fromFormTemplate(v sqlc.FormTemplate) domain.FormTemplate {
	return domain.FormTemplate{
		ID:          v.ID,
		TenantID:    v.TenantID,
		Key:         v.Key,
		Name:        v.Name,
		Description: v.Description,
		Schema:      jsonMap(v.Schema),
		CreatedAt:   timeFrom(v.CreatedAt),
	}
}

func fromFormInstance(v sqlc.FormInstance) domain.FormInstance {
	return domain.FormInstance{
		ID:                 v.ID,
		TenantID:           v.TenantID,
		TemplateID:         v.TemplateID,
		ApplicantAccountID: v.ApplicantAccountID,
		Status:             v.Status,
		Payload:            jsonMap(v.Payload),
		SubmittedAt:        timeFrom(v.SubmittedAt),
		ApprovedBy:         v.ApprovedBy,
		UpdatedAt:          timeFrom(v.UpdatedAt),
	}
}

func fromKnowledgeArticle(v sqlc.KnowledgeArticle) domain.KnowledgeArticle {
	return domain.KnowledgeArticle{
		ID:        v.ID,
		TenantID:  v.TenantID,
		Title:     v.Title,
		Content:   v.Content,
		Tags:      utils.CopyStrings(v.Tags),
		CreatedAt: timeFrom(v.CreatedAt),
	}
}

func fromAgentRun(v sqlc.AgentRun) domain.AgentRun {
	return domain.AgentRun{
		ID:         v.ID,
		TenantID:   v.TenantID,
		AccountID:  v.AccountID,
		Mode:       v.Mode,
		Prompt:     v.Prompt,
		Answer:     v.Answer,
		Status:     v.Status,
		References: jsonRefs(v.ReferenceItems),
		CreatedAt:  timeFrom(v.CreatedAt),
		UpdatedAt:  timeFrom(v.UpdatedAt),
	}
}

func fromAuditLog(v sqlc.AuditLog) domain.AuditLog {
	return domain.AuditLog{
		ID:             v.ID,
		TenantID:       v.TenantID,
		ActorAccountID: v.ActorAccountID,
		Action:         v.Action,
		Resource:       v.Resource,
		Target:         v.Target,
		Result:         v.Result,
		TraceID:        v.TraceID,
		Severity:       v.Severity,
		Details:        jsonMap(v.Details),
		CreatedAt:      timeFrom(v.CreatedAt),
	}
}

func fromAuthzOutboxEvent(v sqlc.AuthzOutboxEvent) domain.AuthzOutboxEvent {
	return domain.AuthzOutboxEvent{
		ID:          v.ID,
		TenantID:    v.TenantID,
		EventType:   v.EventType,
		Payload:     jsonMap(v.Payload),
		Status:      v.Status,
		RetryCount:  int(v.RetryCount),
		LastError:   v.LastError,
		CreatedAt:   timeFrom(v.CreatedAt),
		ProcessedAt: timePtrFrom(v.ProcessedAt),
	}
}

func fromAuthzRelationshipTuple(v sqlc.AuthzRelationshipTuple) domain.AuthzRelationshipTuple {
	return domain.AuthzRelationshipTuple{
		ID:          v.ID,
		TenantID:    v.TenantID,
		ObjectType:  v.ObjectType,
		ObjectID:    v.ObjectID,
		Relation:    v.Relation,
		SubjectType: v.SubjectType,
		SubjectID:   v.SubjectID,
		CreatedAt:   timeFrom(v.CreatedAt),
	}
}
