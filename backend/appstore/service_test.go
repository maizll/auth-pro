package appstore

import (
	"context"
	"errors"
	"testing"
)

type fakeTemplateRepository struct {
	items      []Template
	listErr    error
	enableErr  error
	disableErr error
	enabledID  string
	disabledID string
}

func (r *fakeTemplateRepository) List(context.Context) ([]Template, error) {
	return r.items, r.listErr
}

func (r *fakeTemplateRepository) Enable(_ context.Context, id string) error {
	r.enabledID = id
	return r.enableErr
}

func (r *fakeTemplateRepository) Disable(_ context.Context, id string) error {
	r.disabledID = id
	return r.disableErr
}

func TestServiceDashboard(t *testing.T) {
	repository := &fakeTemplateRepository{items: []Template{
		{ID: "default", Enabled: false, Available: true},
		{ID: "12", Enabled: true, Available: true},
		{ID: "13", Enabled: false, Available: false},
	}}
	service := NewService(repository)

	dashboard, err := service.Dashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TemplateCount != 3 || dashboard.EnabledTemplateCount != 1 || dashboard.AvailableTemplateCount != 2 {
		t.Fatalf("unexpected dashboard: %+v", dashboard)
	}
	if dashboard.ModuleCount != 1 || len(dashboard.Modules) != 1 || dashboard.Modules[0] != "home-templates" {
		t.Fatalf("unexpected modules: %+v", dashboard)
	}
}

func TestServiceTemplateMutations(t *testing.T) {
	repository := &fakeTemplateRepository{}
	service := NewService(repository)

	if err := service.EnableTemplate(context.Background(), " 12 "); err != nil {
		t.Fatal(err)
	}
	if repository.enabledID != "12" {
		t.Fatalf("enabled id = %q", repository.enabledID)
	}
	if err := service.DisableTemplate(context.Background(), "12"); err != nil {
		t.Fatal(err)
	}
	if repository.disabledID != "12" {
		t.Fatalf("disabled id = %q", repository.disabledID)
	}

	code, message := ErrorResponse(service.EnableTemplate(context.Background(), " "))
	if code != 400 || message != "模板标识不能为空" {
		t.Fatalf("unexpected empty id error: code=%d message=%q", code, message)
	}
}

func TestServicePreservesRepositoryError(t *testing.T) {
	want := ClientError("模板不存在", errors.New("missing"))
	service := NewService(&fakeTemplateRepository{enableErr: want})
	if err := service.EnableTemplate(context.Background(), "99"); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
