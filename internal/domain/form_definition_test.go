package domain

import "testing"

func TestValidateAndCompileFormDefinitionSchemaV2(t *testing.T) {
	schema := FormDefinitionSchemaV2{
		SchemaVersion: 2,
		Name:          "请假单",
		Fields: []FormFieldDefinitionV2{
			{ID: "leave_type", Label: "假别", DataType: "string", Widget: "select", Options: []FormFieldOptionV2{{Label: "年假", Value: "annual"}}},
			{ID: "reason", Label: "事由", DataType: "string", Widget: "textarea"},
		},
		Layout:   FormLayoutV2{Rows: []FormLayoutRowV2{{ID: "row-1", FieldIDs: []string{"leave_type", "reason"}}}},
		Workflow: FormWorkflowV2{Stages: []FormWorkflowStageV2{{ID: "manager", Type: "approver", Label: "直属主管", Config: map[string]any{"role": "manager"}}}},
	}
	compiled, result := CompileFormDefinitionSchemaV2(schema)
	if !result.Valid || len(result.Errors) != 0 {
		t.Fatalf("expected valid schema, got %#v", result)
	}
	design, ok := compiled["workspace_design"].(map[string]any)
	if !ok {
		t.Fatal("expected workspace_design runtime contract")
	}
	fields, ok := design["fields"].([]map[string]any)
	if !ok || len(fields) != 3 {
		t.Fatalf("expected layout plus two fields, got %#v", design["fields"])
	}
}

func TestValidateFormDefinitionSchemaV2RejectsUnsafeShape(t *testing.T) {
	result := ValidateFormDefinitionSchemaV2(FormDefinitionSchemaV2{SchemaVersion: 1, Name: "", Fields: []FormFieldDefinitionV2{{ID: "Bad ID", Label: "", DataType: "script", Widget: "html"}}})
	if result.Valid || len(result.Errors) < 4 {
		t.Fatalf("expected structured validation errors, got %#v", result)
	}
}

func TestValidateFormDefinitionSchemaV2RejectsUncontrolledBinding(t *testing.T) {
	result := ValidateFormDefinitionSchemaV2(FormDefinitionSchemaV2{
		SchemaVersion: 2,
		Name:          "员工选择",
		Fields: []FormFieldDefinitionV2{{
			ID: "employee", Label: "员工", DataType: "string", Widget: "input",
			Binding: &FormFieldBindingV2{SourceID: "sql://tenant.users", ValueField: "email", LabelField: "name"},
		}},
		Layout:   FormLayoutV2{Rows: []FormLayoutRowV2{{FieldIDs: []string{"employee"}}}},
		Workflow: FormWorkflowV2{Stages: []FormWorkflowStageV2{{ID: "manager", Type: "approver", Label: "主管", Config: map[string]any{"role": "manager"}}}},
	})
	if result.Valid {
		t.Fatalf("expected uncontrolled binding to be rejected, got %#v", result)
	}
}
