package repohost

import "testing"

func TestPullRequestsURL(t *testing.T) {
	for _, test := range []struct{ remote, want string }{
		{"https://github.com/chengliang4810/GCode.git", "https://github.com/chengliang4810/GCode/pulls"},
		{"git@github.com:owner/repo.git", "https://github.com/owner/repo/pulls"},
		{"ssh://git@github.com:22/owner/repo.git", "https://github.com/owner/repo/pulls"},
		{"https://user:secret@github.com/owner/repo.git", "https://github.com/owner/repo/pulls"},
		{"https://gitlab.com/group/subgroup/repo.git", "https://gitlab.com/group/subgroup/repo/-/merge_requests"},
		{"git@bitbucket.org:owner/repo.git", "https://bitbucket.org/owner/repo/pull-requests"},
		{"file:///tmp/repo.git", ""},
		{"https://github.com/owner/../repo.git", ""},
		{"https://github.com/owner/repo.git?token=secret", ""},
		{"https://unconfigured.example/owner/repo.git", ""},
		{"https://github.com/owner", ""},
		{"javascript:alert(1)", ""},
		{"C:/repositories/project", ""},
		{"", ""},
	} {
		t.Run(test.remote, func(t *testing.T) {
			if got := PullRequestsURL(test.remote); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
