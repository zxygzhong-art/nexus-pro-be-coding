package v1

import (
	"io"
	"net/http"
	"strings"

	"nexus-pro-be/internal/domain"
)

const pathParamDocumentID = "document_id"

const maxKnowledgeSourceUploadBytes = 20 << 20

// listKnowledgeBases 列出租戶知識庫。
func (c WorkspaceAgentCtrl) listKnowledgeBases(w http.ResponseWriter, _ *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ListKnowledgeBases(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, domain.KnowledgeBaseListResponse{Items: items, Total: len(items)})
	return nil
}

// getKnowledgeBase 取得租戶知識庫。
func (c WorkspaceAgentCtrl) getKnowledgeBase(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.GetKnowledgeBase(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// createKnowledgeBase 建立租戶知識庫。
func (c WorkspaceAgentCtrl) createKnowledgeBase(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.CreateKnowledgeBaseInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateKnowledgeBase(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateKnowledgeBase 更新租戶知識庫。
func (c WorkspaceAgentCtrl) updateKnowledgeBase(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateKnowledgeBaseInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateKnowledgeBase(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteKnowledgeBase 刪除租戶知識庫。
func (c WorkspaceAgentCtrl) deleteKnowledgeBase(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteKnowledgeBase(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// listKnowledgeDocuments lists manual and uploaded knowledge sources.
func (c WorkspaceAgentCtrl) listKnowledgeDocuments(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	items, err := c.svc.ListKnowledgeDocuments(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, domain.KnowledgeDocumentListResponse{Items: items, Total: len(items)})
	return nil
}

// createKnowledgeDocument creates manual text or uploads a text/PDF source.
func (c WorkspaceAgentCtrl) createKnowledgeDocument(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, maxKnowledgeSourceUploadBytes+(1<<20))
		if err := r.ParseMultipartForm(maxKnowledgeSourceUploadBytes + (1 << 20)); err != nil {
			return domain.BadRequest("invalid multipart form: " + err.Error())
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			return domain.BadRequest("file is required")
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, maxKnowledgeSourceUploadBytes+1))
		if err != nil {
			return domain.BadRequest("read knowledge source: " + err.Error())
		}
		item, err := c.svc.UploadKnowledgeDocument(ctx, r.PathValue(PathParamID), domain.UploadKnowledgeDocumentInput{
			Title: r.FormValue("title"), Filename: header.Filename,
			ContentType: header.Header.Get("Content-Type"), Content: content,
		})
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusCreated, item)
		return nil
	}
	var input domain.CreateKnowledgeDocumentInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.CreateKnowledgeDocument(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, item)
	return nil
}

// updateKnowledgeDocument 更新手動文字文件。
func (c WorkspaceAgentCtrl) updateKnowledgeDocument(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.UpdateKnowledgeDocumentInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	item, err := c.svc.UpdateKnowledgeDocument(ctx, r.PathValue(PathParamID), r.PathValue(pathParamDocumentID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// deleteKnowledgeDocument deletes a manual or uploaded knowledge source.
func (c WorkspaceAgentCtrl) deleteKnowledgeDocument(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	item, err := c.svc.DeleteKnowledgeDocument(ctx, r.PathValue(PathParamID), r.PathValue(pathParamDocumentID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, item)
	return nil
}

// searchKnowledgeBase tests semantic vector retrieval within one knowledge base.
func (c WorkspaceAgentCtrl) searchKnowledgeBase(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	if _, err := c.svc.GetKnowledgeBase(ctx, r.PathValue(PathParamID)); err != nil {
		return err
	}
	var input domain.KnowledgeSearchInput
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	input.KnowledgeBaseIDs = []string{r.PathValue(PathParamID)}
	result, err := c.svc.SearchKnowledge(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, domain.KnowledgeSearchResponse{
		Items: result.Hits, Total: result.Total, Query: result.Query, Semantics: result.Semantics,
	})
	return nil
}
