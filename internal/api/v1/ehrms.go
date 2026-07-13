package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/service"
)

// EHRMSCtrl 提供同步運行的查詢與人工恢復入口。
type EHRMSCtrl struct {
	routes routeBinder
	svc    service.EHRMSFacade
}

// RegisterRoutes 註冊 eHRMS 運維路由。
func (c EHRMSCtrl) RegisterRoutes(router *gin.RouterGroup) {
	ehrms := router.Group("/ehrms")
	ehrms.POST("/sync", c.routes.Handle("hr.employee", "import", c.startSync))
	ehrms.GET("/sync-runs", c.routes.Handle("hr.employee", "read", c.listSyncRuns))
	ehrms.GET("/sync-runs/:id", c.routes.Handle("hr.employee", "read", c.getSyncRun, PathParam(PathParamID)))
	ehrms.POST("/sync-runs/:id/retry", c.routes.Handle("hr.employee", "import", c.retrySyncRun, PathParam(PathParamID)))
}

// startSync 啟動可追蹤的手動同步。
func (c EHRMSCtrl) startSync(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.StartEHRMSSyncInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.StartSync(ctx, input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// listSyncRuns 列出同步運行。
func (c EHRMSCtrl) listSyncRuns(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	page, err := pageRequestFromRequest(r)
	if err != nil {
		return err
	}
	result, err := c.svc.ListSyncRunPage(ctx, page)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// getSyncRun 取得同步運行詳情。
func (c EHRMSCtrl) getSyncRun(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	result, err := c.svc.GetSyncRun(ctx, r.PathValue(PathParamID))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

// retrySyncRun 人工重試失敗或部分成功的同步運行。
func (c EHRMSCtrl) retrySyncRun(w http.ResponseWriter, r *http.Request, ctx domain.RequestContext) error {
	var input domain.RetryEHRMSSyncRunInput
	if _, err := readOptionalJSON(w, r, &input); err != nil {
		return err
	}
	result, err := c.svc.RetrySyncRun(ctx, r.PathValue(PathParamID), input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}
