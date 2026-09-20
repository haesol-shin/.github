package collect

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const gitReadSize = 1024 * 1024

func (g *GitRunner) Metadata(ctx context.Context, pullRequest map[string]any, token, mergeBase string) (string, string, error) {
	return GitMetadata(ctx, pullRequest, token, mergeBase)
}

func GitMetadata(ctx context.Context, pullRequest map[string]any, token, mergeBase string) (string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	number, ok := lookupInt(pullRequest["number"])
	if !ok {
		return "", "", fmt.Errorf("pull request number is required")
	}
	headSHA, ok := nestedString(pullRequest, "head", "sha")
	if !ok || headSHA == "" {
		return "", "", fmt.Errorf("pull request head SHA is required")
	}
	cloneURL, ok := nestedString(pullRequest, "base", "repo", "clone_url")
	if !ok || cloneURL == "" {
		return "", "", fmt.Errorf("pull request clone URL is required")
	}

	directory, err := os.MkdirTemp("", "repo-ops-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(directory)

	env := gitEnv(os.Environ(), token)
	if err := runGit(ctx, "", env, "init", "--quiet", directory); err != nil {
		return "", "", err
	}
	refspec := "refs/pull/" + strconv.Itoa(number) + "/head:refs/repo-ops/head"
	if err := runGit(ctx, directory, env, "fetch", "--quiet", "--no-tags", cloneURL, refspec); err != nil {
		return "", "", err
	}
	fetched, err := gitOutput(ctx, directory, env, "rev-parse", "refs/repo-ops/head")
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(fetched) != headSHA {
		return "", "", fmt.Errorf("fetched pull request head does not match the GitHub API")
	}
	if err := runGit(ctx, directory, env, "cat-file", "-e", mergeBase+"^{commit}"); err != nil {
		return "", "", err
	}
	if err := runGit(ctx, directory, env, "update-ref", "refs/repo-ops/base", mergeBase); err != nil {
		return "", "", err
	}

	args := []string{
		"-c", "core.quotePath=true",
		"diff",
		"--binary",
		"--full-index",
		"--no-color",
		"--no-ext-diff",
		"--no-textconv",
		"--no-renames",
		"refs/repo-ops/base",
		"refs/repo-ops/head",
		"--",
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = directory
	cmd.Env = env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	if err := cmd.Start(); err != nil {
		return "", "", err
	}
	digest := sha256.New()
	buf := make([]byte, gitReadSize)
	if _, err := io.CopyBuffer(digest, stdout, buf); err != nil {
		_ = cmd.Wait()
		return "", "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", "", fmt.Errorf("git diff: %w", err)
	}
	return mergeBase, "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func gitEnv(parent []string, token string) []string {
	env := append([]string{}, parent...)
	if token == "" {
		return env
	}
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic "+basic,
	)
}

func runGit(ctx context.Context, dir string, env []string, args ...string) error {
	_, err := gitOutput(ctx, dir, env, args...)
	return err
}

func gitOutput(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}
