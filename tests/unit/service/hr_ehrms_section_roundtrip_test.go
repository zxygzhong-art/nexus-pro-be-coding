package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

// TestSyncEHRMSEmployeesSectionKeysRoundTrip 驗證 eHRMS 寫入的 section 鍵與 typed 視圖讀端一致：
// 同步寫入 → 讀回 → typed 視圖逐欄位斷言，並檢查 sections JSON 線上格式。
func TestSyncEHRMSEmployeesSectionKeysRoundTrip(t *testing.T) {
	rows := []domain.EHRMSEmployeeRecord{{
		"emp_id":                  "E9001",
		"name_zh":                 "測試員工",
		"name_en":                 "Test Employee",
		"gender":                  "男",
		"birthday":                "1990/1/2",
		"nationality":             "台灣",
		"national_id":             "A123456789",
		"passport_no":             "P123456",
		"passport_name":           "TEST EMPLOYEE",
		"entry_date":              "2024/3/6",
		"arc_no":                  "A800000001",
		"arc_expiry_date":         "2027/3/6",
		"tax_id":                  "TW-001",
		"work_permit_no":          "WP-001",
		"work_permit_expiry_date": "2027/3/5",
		"contract_expiry_date":    "2028/3/5",
		"broker":                  "Nexus Agency",
		"emergency_phone":         "0912-345-678",
		"emergency_contact":       "王小明",
		"emergency_relation":      "配偶",
		"work_status":             "在職",
		"hire_date":               "2024/3/5",
		"dept_code":               "C01",
		"dept_name_zh":            "總部",
		"job_code":                "0704",
		"job_title_zh":            "工程師",
		"shift_name":              "早班",
		"shift_attr":              "常日班",
	}}
	store, svc, ctx := newEmployeeFeatureFixture(t, []domain.Permission{
		{Resource: "hr.employee", Action: "import", Scope: "all"},
		{Resource: "hr.employee", Action: "read", Scope: "all"},
	}, service.Options{EHRMSClient: fakeEHRMSClient{rows: rows}})
	seedOrgUnitCodes(t, store, "tenant-1", "C01")

	result, err := svc.HR().SyncEHRMSEmployees(ctx, domain.EHRMSEmployeeSyncInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.Failed != 0 {
		t.Fatalf("unexpected eHRMS sync result: %+v", result)
	}

	employee, ok, err := store.GetEmployeeByEmployeeNo(context.Background(), "tenant-1", "E9001")
	if err != nil || !ok {
		t.Fatalf("expected synced employee, ok=%v err=%v", ok, err)
	}

	// 寫端：canonical 鍵落地；nationality 保留 legacy 鍵供前端讀取；shift_name 改寫 canonical shift。
	basicRaw := map[string]string{
		domain.EmployeeBasicInfoKeyName:                 "測試員工",
		domain.EmployeeBasicInfoKeyNameEN:               "Test Employee",
		domain.EmployeeBasicInfoKeyGender:               "男",
		domain.EmployeeBasicInfoKeyBirthDate:            "1990-01-02",
		domain.EmployeeBasicInfoKeyNationalityType:      "台灣",
		domain.EmployeeBasicInfoKeyNationality:          "台灣",
		domain.EmployeeBasicInfoKeyNationalID:           "A123456789",
		domain.EmployeeBasicInfoKeyPassportNo:           "P123456",
		domain.EmployeeBasicInfoKeyPassportName:         "TEST EMPLOYEE",
		domain.EmployeeBasicInfoKeyEntryDate:            "2024-03-06",
		domain.EmployeeBasicInfoKeyARCNo:                "A800000001",
		domain.EmployeeBasicInfoKeyARCExpiryDate:        "2027-03-06",
		domain.EmployeeBasicInfoKeyTaxID:                "TW-001",
		domain.EmployeeBasicInfoKeyWorkPermitNo:         "WP-001",
		domain.EmployeeBasicInfoKeyWorkPermitExpiryDate: "2027-03-05",
		domain.EmployeeBasicInfoKeyContractExpiryDate:   "2028-03-05",
		domain.EmployeeBasicInfoKeyBroker:               "Nexus Agency",
	}
	for key, want := range basicRaw {
		if got, _ := employee.BasicInfo[key].(string); got != want {
			t.Fatalf("basic_info[%q] = %q, want %q (map=%v)", key, got, want, employee.BasicInfo)
		}
	}
	contactRaw := map[string]string{
		"emergency_contact_phone":    "0912-345-678",
		"emergency_contact_name":     "王小明",
		"emergency_contact_relation": "配偶",
	}
	for key, want := range contactRaw {
		if got, _ := employee.ContactInfo[key].(string); got != want {
			t.Fatalf("contact_info[%q] = %q, want %q (map=%v)", key, got, want, employee.ContactInfo)
		}
	}
	employmentRaw := map[string]string{
		domain.EmployeeEmploymentInfoKeyShift:     "早班",
		domain.EmployeeEmploymentInfoKeyShiftType: "常日班",
	}
	for key, want := range employmentRaw {
		if got, _ := employee.EmploymentInfo[key].(string); got != want {
			t.Fatalf("employment_info[%q] = %q, want %q (map=%v)", key, got, want, employee.EmploymentInfo)
		}
	}
	if _, found := employee.EmploymentInfo["shift_name"]; found {
		t.Fatalf("expected legacy shift_name to be replaced by canonical shift, got %v", employee.EmploymentInfo)
	}

	// 讀端：typed 視圖逐欄位讀回。
	sections := domain.EmployeeSectionsFromEmployee(employee)
	if sections.BasicInfo.Name != "測試員工" ||
		sections.BasicInfo.NameEN != "Test Employee" ||
		sections.BasicInfo.Gender != "男" ||
		sections.BasicInfo.BirthDate != "1990-01-02" ||
		sections.BasicInfo.NationalityType != "台灣" ||
		sections.BasicInfo.NationalID != "A123456789" ||
		sections.BasicInfo.PassportNo != "P123456" ||
		sections.BasicInfo.PassportName != "TEST EMPLOYEE" ||
		sections.BasicInfo.EntryDate != "2024-03-06" ||
		sections.BasicInfo.ARCNo != "A800000001" ||
		sections.BasicInfo.ARCExpiryDate != "2027-03-06" ||
		sections.BasicInfo.TaxID != "TW-001" ||
		sections.BasicInfo.WorkPermitNo != "WP-001" ||
		sections.BasicInfo.WorkPermitExpiryDate != "2027-03-05" ||
		sections.BasicInfo.ContractExpiryDate != "2028-03-05" ||
		sections.BasicInfo.Broker != "Nexus Agency" {
		t.Fatalf("typed basic info mismatch: %+v", sections.BasicInfo)
	}
	if sections.EmploymentInfo.Shift != "早班" ||
		sections.EmploymentInfo.OrgUnitID == "" ||
		sections.EmploymentInfo.Position != "工程師" ||
		sections.EmploymentInfo.HireDate != "2024-03-05" {
		t.Fatalf("typed employment info mismatch: %+v", sections.EmploymentInfo)
	}
	if sections.ContactInfo.EmergencyContactPhone != "0912-345-678" ||
		sections.ContactInfo.EmergencyContactName != "王小明" ||
		sections.ContactInfo.EmergencyContactRelation != "配偶" {
		t.Fatalf("typed contact info mismatch: %+v", sections.ContactInfo)
	}

	// 線上格式：typed 欄位與 Additional 透出的 legacy nationality 同時存在。
	basicJSON, err := json.Marshal(sections.BasicInfo)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"nationality_type":"台灣"`, `"nationality":"台灣"`, `"gender":"男"`, `"birth_date":"1990-01-02"`, `"name_en":"Test Employee"`} {
		if !strings.Contains(string(basicJSON), fragment) {
			t.Fatalf("expected basic info JSON to contain %s, got %s", fragment, basicJSON)
		}
	}
	employmentJSON, err := json.Marshal(sections.EmploymentInfo)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"shift":"早班"`, `"shift_type":"常日班"`} {
		if !strings.Contains(string(employmentJSON), fragment) {
			t.Fatalf("expected employment info JSON to contain %s, got %s", fragment, employmentJSON)
		}
	}
	if strings.Contains(string(employmentJSON), `"shift_name"`) {
		t.Fatalf("expected legacy shift_name to be consumed by typed shift, got %s", employmentJSON)
	}
}

// TestEHRMSMergeEmployeeUpdatesEmergencyContact verifies the sync button's upsert path
// updates emergency contact fields while preserving unrelated contact data.
func TestEHRMSMergeEmployeeUpdatesEmergencyContact(t *testing.T) {
	existing := domain.Employee{
		BasicInfo: map[string]any{},
		ContactInfo: map[string]any{
			"mobile_phone":               "0900-000-000",
			"emergency_contact_phone":    "old-phone",
			"emergency_contact_name":     "舊聯絡人",
			"emergency_contact_relation": "其他",
		},
	}
	candidate := domain.Employee{
		BasicInfo: map[string]any{},
		ContactInfo: map[string]any{
			"emergency_contact_phone":    "0912-345-678",
			"emergency_contact_name":     "王小明",
			"emergency_contact_relation": "配偶",
		},
	}

	merged := service.EHRMSMergeEmployee(existing, candidate)
	if merged.ContactInfo["mobile_phone"] != "0900-000-000" ||
		merged.ContactInfo["emergency_contact_phone"] != "0912-345-678" ||
		merged.ContactInfo["emergency_contact_name"] != "王小明" ||
		merged.ContactInfo["emergency_contact_relation"] != "配偶" {
		t.Fatalf("unexpected merged contact info: %+v", merged.ContactInfo)
	}
}

// TestEmployeeSectionsFromEmployeeLegacyEHRMSKeyAliases 驗證資料庫中舊鍵（nationality、shift_name）
// 仍被 typed 視圖以别名讀出，且 nationality 原始值繼續透過 Additional 保留給前端。
func TestEmployeeSectionsFromEmployeeLegacyEHRMSKeyAliases(t *testing.T) {
	employee := domain.Employee{
		ID:             "emp-legacy",
		TenantID:       "tenant-1",
		EmployeeNo:     "E9002",
		Name:           "舊鍵員工",
		Status:         "active",
		BasicInfo:      map[string]any{"nationality": "日本", "gender": "女", "birth_date": "1988-12-31"},
		EmploymentInfo: map[string]any{"shift_name": "晚班"},
	}

	sections := domain.EmployeeSectionsFromEmployee(employee)
	if sections.BasicInfo.NationalityType != "日本" {
		t.Fatalf("expected legacy nationality alias to populate nationality_type, got %+v", sections.BasicInfo)
	}
	if sections.BasicInfo.Gender != "女" || sections.BasicInfo.BirthDate != "1988-12-31" {
		t.Fatalf("expected gender/birth_date typed fields, got %+v", sections.BasicInfo)
	}
	if sections.EmploymentInfo.Shift != "晚班" {
		t.Fatalf("expected legacy shift_name alias to populate shift, got %+v", sections.EmploymentInfo)
	}

	basicJSON, err := json.Marshal(sections.BasicInfo)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"nationality_type":"日本"`, `"nationality":"日本"`} {
		if !strings.Contains(string(basicJSON), fragment) {
			t.Fatalf("expected basic info JSON to contain %s, got %s", fragment, basicJSON)
		}
	}
	employmentJSON, err := json.Marshal(sections.EmploymentInfo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(employmentJSON), `"shift":"晚班"`) || strings.Contains(string(employmentJSON), `"shift_name"`) {
		t.Fatalf("expected legacy shift_name consumed into typed shift only, got %s", employmentJSON)
	}
}
