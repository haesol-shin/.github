package collect

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

func APIContent(ctx context.Context, c *GitHubClient, repository, path, ref string) (string, error) {
	encodedPath := quotePreservingSlash(path)
	result, err := c.Get(ctx, "/repos/"+repository+"/contents/"+encodedPath+"?ref="+quotePreservingSlash(ref), "")
	if err != nil {
		return "", err
	}
	obj, ok := result.(map[string]any)
	if !ok {
		return "", fmt.Errorf("unsupported content encoding for %s", path)
	}
	encoding, _ := obj["encoding"].(string)
	if encoding != "base64" {
		return "", fmt.Errorf("unsupported content encoding for %s", path)
	}
	raw, ok := obj["content"].(string)
	if !ok {
		return "", fmt.Errorf("unsupported content encoding for %s", path)
	}
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, raw)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return "", fmt.Errorf("invalid base64 content for %s: %w", path, err)
	}
	if !utf8.Valid(decoded) {
		return "", fmt.Errorf("invalid UTF-8 content for %s", path)
	}
	return string(decoded), nil
}

func collaboratorPermission(ctx context.Context, c *GitHubClient, repository, login string, cache map[string]*string) *string {
	if login == "" {
		return nil
	}
	if cached, ok := cache[login]; ok {
		return cached
	}
	result, err := c.Get(ctx, "/repos/"+repository+"/collaborators/"+url.PathEscape(login)+"/permission", "")
	if err != nil {
		cache[login] = nil
		return nil
	}
	obj, _ := result.(map[string]any)
	permission, ok := obj["permission"].(string)
	if !ok {
		cache[login] = nil
		return nil
	}
	cache[login] = &permission
	return cache[login]
}

func quotePreservingSlash(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func entryFromAPI(item map[string]any, permission *string) map[string]any {
	body, _ := item["body"].(string)
	if item["body"] == nil {
		body = ""
	}
	var author any
	if user, ok := item["user"].(map[string]any); ok {
		author = user["login"]
	}
	var perm any
	if permission != nil {
		perm = *permission
	}
	return map[string]any{
		"body":        body,
		"author":      author,
		"association": item["author_association"],
		"permission":  perm,
	}
}
