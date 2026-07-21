package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/service"
)

func TestUploadFormInstanceFileStoresObjectAndMetadata(t *testing.T) {
	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	_, ctx, store := newWorkflowEngineFixture(t, now, "acct-admin")
	objects := service.NewMemoryObjectStore()
	svc := service.New(store, service.Options{ObjectStore: objects, Now: func() time.Time { return now }})

	draft, err := svc.Workflow().SaveFormDraft(ctx, domain.SaveFormDraftInput{
		TemplateKey: "leave-request",
		Payload:     map[string]any{"subject": "proof"},
	})
	if err != nil {
		t.Fatalf("SaveFormDraft() error = %v", err)
	}

	file, err := svc.Workflow().UploadFormInstanceFile(ctx, draft.ID, domain.UploadFormInstanceFileInput{
		FieldID: "attachment", Filename: "proof.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.4 mock"),
	})
	if err != nil {
		t.Fatalf("UploadFormInstanceFile() error = %v", err)
	}
	if file.ID == "" || file.FieldID != "attachment" || file.State != "draft" {
		t.Fatalf("unexpected file metadata: %+v", file)
	}
	if !strings.Contains(file.OriginalFilename, "proof.pdf") {
		t.Fatalf("original filename = %q", file.OriginalFilename)
	}
	got, err := objects.GetObject(context.Background(), "tenants/tenant-1/forms/"+draft.ID+"/attachment/"+file.ID)
	if err != nil {
		t.Fatalf("GetObject() error = %v", err)
	}
	if string(got) != "%PDF-1.4 mock" {
		t.Fatalf("stored object = %q", got)
	}

	listed, err := svc.Workflow().ListFormInstanceFiles(ctx, draft.ID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListFormInstanceFiles() = %+v err=%v", listed, err)
	}

	download, err := svc.Workflow().DownloadFormInstanceFile(ctx, draft.ID, file.ID)
	if err != nil {
		t.Fatalf("DownloadFormInstanceFile() error = %v", err)
	}
	if string(download.Content) != "%PDF-1.4 mock" {
		t.Fatalf("download content = %q", download.Content)
	}

	if _, err := svc.Workflow().DeleteFormInstanceFile(ctx, draft.ID, file.ID); err != nil {
		t.Fatalf("DeleteFormInstanceFile() error = %v", err)
	}
	listed, err = svc.Workflow().ListFormInstanceFiles(ctx, draft.ID)
	if err != nil || len(listed) != 0 {
		t.Fatalf("expected empty list after delete, got %+v err=%v", listed, err)
	}
}
