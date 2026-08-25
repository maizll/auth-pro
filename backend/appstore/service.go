package appstore

import (
	"context"
	"strings"
)

type Service struct {
	repository TemplateRepository
}

func NewService(repository TemplateRepository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	templates, err := s.Templates(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	result := Dashboard{
		TemplateCount: len(templates),
		ModuleCount:   1,
		Modules:       []string{"home-templates"},
	}
	for _, template := range templates {
		if template.Enabled {
			result.EnabledTemplateCount++
		}
		if template.Available {
			result.AvailableTemplateCount++
		}
	}
	return result, nil
}

func (s *Service) Templates(ctx context.Context) ([]Template, error) {
	if s == nil || s.repository == nil {
		return nil, ServerError("应用商店模板仓储未配置", nil)
	}
	items, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []Template{}
	}
	return items, nil
}

func (s *Service) EnableTemplate(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ClientError("模板标识不能为空", nil)
	}
	if s == nil || s.repository == nil {
		return ServerError("应用商店模板仓储未配置", nil)
	}
	return s.repository.Enable(ctx, id)
}

func (s *Service) DisableTemplate(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ClientError("模板标识不能为空", nil)
	}
	if s == nil || s.repository == nil {
		return ServerError("应用商店模板仓储未配置", nil)
	}
	return s.repository.Disable(ctx, id)
}
