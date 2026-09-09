package appstore

import (
	"context"
	"errors"
)

type Author struct {
	Name  string `json:"name"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

type Template struct {
	UpdateAvailable bool   `json:"updateAvailable"`
	ID              string `json:"id"`
	CatalogID       string `json:"catalogId,omitempty"`
	TemplateID      string `json:"templateId"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	PreviewImage    string `json:"previewImage"`
	Version         string `json:"version"`
	Author          Author `json:"author"`
	Enabled         bool   `json:"enabled"`
	Source          string `json:"source,omitempty"`
	SourceURL       string `json:"sourceUrl,omitempty"`
	SourceType      string `json:"sourceType,omitempty"`
	Available       bool   `json:"available"`
	Installed       bool   `json:"installed"`
	Format          string `json:"format,omitempty"`
	SchemaVersion   int    `json:"schemaVersion,omitempty"`
	SHA256          string `json:"sha256,omitempty"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
}

type Dashboard struct {
	TemplateCount          int      `json:"templateCount"`
	EnabledTemplateCount   int      `json:"enabledTemplateCount"`
	AvailableTemplateCount int      `json:"availableTemplateCount"`
	ModuleCount            int      `json:"moduleCount"`
	Modules                []string `json:"modules"`
}

type TemplateRepository interface {
	List(ctx context.Context) ([]Template, error)
	Enable(ctx context.Context, id string) error
	Disable(ctx context.Context, id string) error
}

type Error struct {
	Code    int
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "应用商店操作失败"
}

func (e *Error) Unwrap() error {
	return e.Err
}

func ClientError(message string, err error) error {
	return &Error{Code: 400, Message: message, Err: err}
}

func ServerError(message string, err error) error {
	return &Error{Code: 500, Message: message, Err: err}
}

func UnavailableError(message string, err error) error {
	return &Error{Code: 503, Message: message, Err: err}
}

func ErrorResponse(err error) (int, string) {
	var appStoreError *Error
	if errors.As(err, &appStoreError) {
		return appStoreError.Code, appStoreError.Error()
	}
	return 500, "应用商店操作失败"
}
