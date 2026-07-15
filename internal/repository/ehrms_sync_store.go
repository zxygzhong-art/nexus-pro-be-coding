package repository

import (
	"context"
)

// EHRMSSyncLocker 防止同租戶的 eHRMS 同步重疊執行，不保存同步運行資料。
type EHRMSSyncLocker interface {
	WithEHRMSSyncLock(context.Context, string, string, func() error) (bool, error)
}
