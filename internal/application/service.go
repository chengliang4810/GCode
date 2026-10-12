package application

import (
	"context"
	"path/filepath"

	"gcode/internal/appmodel"
	"gcode/internal/controller"
	"gcode/internal/runtime"
	"gcode/internal/worktrees"
)

// Service connects the desktop to the controller without a transport protocol.
type Service struct {
	controller *controller.Controller
}

func New(dataDir string) (*Service, error) {
	engine, err := runtime.New(runtime.Options{DataDir: dataDir})
	if err != nil {
		return nil, err
	}
	resolvedDataDir := engine.DataDir()
	manager := worktrees.NewManager(worktrees.ManagerOptions{BaseDir: filepath.Join(resolvedDataDir, "worktrees")})
	control, err := controller.New(controller.Options{DataDir: resolvedDataDir, Runtime: engine, WorktreeManager: manager})
	if err != nil {
		_ = engine.Close()
		return nil, err
	}
	return &Service{controller: control}, nil
}

func (s *Service) Snapshot(ctx context.Context) (appmodel.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return appmodel.Snapshot{}, err
	}
	return s.controller.Snapshot(), nil
}

func (s *Service) Subscribe(callback func(appmodel.Snapshot)) func() {
	return s.controller.Subscribe(callback)
}

func (s *Service) AddProject(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.AddProject(path)
}

func (s *Service) ListBranches(ctx context.Context, projectID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.controller.ListBranches(projectID)
}

func (s *Service) MergeWorktree(ctx context.Context, sessionID, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.MergeWorktree(sessionID, target)
}

func (s *Service) RemoveWorktree(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.RemoveWorktree(sessionID)
}

func (s *Service) NewSession(ctx context.Context, options appmodel.NewSessionOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := s.controller.NewSession(options)
	return err
}

func (s *Service) Send(ctx context.Context, id, text string, mode appmodel.SendMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.Send(id, text, mode)
}

func (s *Service) Stop(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.Stop(id)
}

func (s *Service) Withdraw(ctx context.Context, id, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.Withdraw(id, message)
}

func (s *Service) ResumeQueue(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.ResumeQueue(id)
}

func (s *Service) SetProvider(ctx context.Context, id, provider string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.SetProvider(id, provider)
}

func (s *Service) SetFullAccess(ctx context.Context, id string, value bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.SetFullAccess(id, value)
}

func (s *Service) ReplyPermission(ctx context.Context, id, request, choice string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.ReplyPermission(id, request, choice)
}

func (s *Service) ReplyAsk(ctx context.Context, id, request string, answers []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.ReplyAsk(id, request, answers)
}

func (s *Service) SaveSettings(ctx context.Context, settings appmodel.Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.SaveSettings(settings)
}

func (s *Service) SaveProvider(ctx context.Context, provider appmodel.Provider, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.SaveProvider(provider, key)
}

func (s *Service) RemoveProvider(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.controller.RemoveProvider(id)
}

func (s *Service) Close(context.Context) error {
	return s.controller.Close()
}
