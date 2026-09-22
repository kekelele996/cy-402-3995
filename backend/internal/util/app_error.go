package util

import "fmt"

// AppError 业务错误，携带统一错误码。
type AppError struct {
	Code    int
	Message string
	Detail  any // 可选的结构化错误明细（如律师利益冲突的冲突案号列表）
}

// Error 实现 error 接口。
func (e *AppError) Error() string {
	return fmt.Sprintf("code=%d message=%s", e.Code, e.Message)
}

// NewAppError 构造业务错误。
func NewAppError(code int, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// NewAppErrorWithDetail 构造带结构化明细的业务错误。
func NewAppErrorWithDetail(code int, message string, detail any) *AppError {
	return &AppError{Code: code, Message: message, Detail: detail}
}

// Wrap 包装错误并附带上下文。
func Wrap(err error, format string, args ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}
