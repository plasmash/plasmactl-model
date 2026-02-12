package compose

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestGetVersionedMap(t *testing.T) {
	repoDir := t.TempDir()

	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	files := []string{"file1.txt", "dir/file2.txt"}
	for _, f := range files {
		p := filepath.Join(repoDir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(p, []byte("content"), 0600); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}
	for _, f := range files {
		if _, err := wt.Add(f); err != nil {
			t.Fatalf("failed to add file: %v", err)
		}
	}
	_, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	versionedMap, err := getVersionedMap(repoDir)
	if err != nil {
		t.Fatalf("getVersionedMap failed: %v", err)
	}

	for _, f := range files {
		if !versionedMap[f] {
			t.Errorf("expected %q in versioned map", f)
		}
	}

	if !versionedMap["dir"] {
		t.Error("expected 'dir' in versioned map")
	}
}

func TestGetVersionedMapWorktree(t *testing.T) {
	repoDir := t.TempDir()

	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	testFile := "file.txt"
	if err := os.WriteFile(filepath.Join(repoDir, testFile), []byte("main content"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}
	if _, err := wt.Add(testFile); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}
	headHash, err := wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	worktreeDir := t.TempDir()
	cmd := testGitCommand(t, repoDir, "worktree", "add", worktreeDir, "-b", "test-branch", headHash.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create worktree: %v\n%s", err, out)
	}

	fi, err := os.Lstat(filepath.Join(worktreeDir, ".git"))
	if err != nil {
		t.Fatalf("failed to stat .git: %v", err)
	}
	if fi.IsDir() {
		t.Fatal("expected .git to be a file in worktree, got directory")
	}

	versionedMap, err := getVersionedMap(worktreeDir)
	if err != nil {
		t.Fatalf("getVersionedMap failed on worktree: %v", err)
	}

	if !versionedMap[testFile] {
		t.Errorf("expected %q in versioned map from worktree", testFile)
	}
}
