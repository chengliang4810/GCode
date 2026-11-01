package contracts

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// RepoRefScope identifies one repository resource, including its host. Cloud
// branches use the reachable repository identity and default branch as well.
type RepoRefScope struct {
	ProjectID       string `json:"projectId"`
	TargetID        string `json:"targetId"`
	RepoPath        string `json:"repoPath"`
	CloudRepository string `json:"cloudRepository,omitempty"`
	DefaultBranch   string `json:"defaultBranch,omitempty"`
}

func (s RepoRefScope) Key() string {
	data, _ := json.Marshal(s)
	return string(data)
}

func (s RepoRefScope) Valid() bool {
	return s.ProjectID != "" && s.TargetID != "" && s.RepoPath != ""
}

type RepoRef struct {
	Name         string `json:"name"`
	Current      bool   `json:"current"`
	WorktreePath string `json:"worktreePath,omitempty"`
}

// RepoRefs retains rows during a forced refresh and after a failed refresh.
type RepoRefs struct {
	Scope       RepoRefScope `json:"scope"`
	State       LoadStatus   `json:"state"`
	Rows        []RepoRef    `json:"rows"`
	Error       string       `json:"error,omitempty"`
	Generation  uint64       `json:"generation"`
	Switching   string       `json:"switching,omitempty"`
	SwitchError string       `json:"switchError,omitempty"`
}

type CheckoutKind string

const (
	CheckoutCurrent CheckoutKind = "current"
	CheckoutReuse   CheckoutKind = "reuseWorktree"
	CheckoutNew     CheckoutKind = "newWorktree"
	CheckoutCloud   CheckoutKind = "cloud"
)

// CheckoutPlan is consumed only when creating a conversation. Empty Base for
// NewWorktree means HEAD, matching composer.rs's actual WorktreeSpec consumer.
type CheckoutPlan struct {
	Kind       CheckoutKind `json:"kind"`
	Branch     string       `json:"branch,omitempty"`
	Base       string       `json:"base,omitempty"`
	Path       string       `json:"path,omitempty"`
	WorktreeID string       `json:"worktreeId,omitempty"`
}

func (p CheckoutPlan) Validate() error {
	switch p.Kind {
	case CheckoutCurrent, CheckoutCloud:
		if p.Base != "" || p.Path != "" || p.WorktreeID != "" {
			return errors.New("unexpected checkout plan payload")
		}
	case CheckoutReuse:
		if p.Path == "" || p.Branch == "" || p.Base != "" {
			return errors.New("reuse checkout requires a path and branch")
		}
	case CheckoutNew:
		if p.Branch != "" || p.Path != "" || p.WorktreeID != "" {
			return errors.New("new checkout accepts only its base")
		}
	default:
		return errors.New("unknown checkout plan kind")
	}
	return nil
}

// ConversationSourceContext is captured from this conversation's actual cwd
// before a fresh host dispatch. Other conversations cannot rewrite it.
type ConversationSourceContext struct {
	CheckoutID string    `json:"checkoutId"`
	RepoRoot   string    `json:"repoRoot"`
	CWD        string    `json:"cwd"`
	Branch     string    `json:"branch"`
	HeadSHA    string    `json:"headSha,omitempty"`
	ObservedAt time.Time `json:"observedAt"`
}

type GitRequest struct {
	Scope RepoRefScope `json:"scope"`
	Ref   string       `json:"ref,omitempty"`
	Force bool         `json:"force"`
}

func IsComposerGitCommand(kind string) bool {
	return kind == "git.refs.load" || kind == "git.ref.switch"
}

func (r GitRequest) Validate(kind string) error {
	if !r.Scope.Valid() {
		return errors.New("repository scope is required")
	}
	if kind == "git.ref.switch" && (strings.TrimSpace(r.Ref) == "" || r.Scope.CloudRepository != "") {
		return errors.New("a local repository ref is required")
	}
	if kind == "git.refs.load" && r.Ref != "" {
		return errors.New("unexpected ref for list request")
	}
	return nil
}
