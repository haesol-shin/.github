package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

var nextLinkPattern = regexp.MustCompile(`<([^>]+)>; rel="next"`)

func (c *GitHubClient) Get(ctx context.Context, urlOrPath string, accept string) (any, error) {
	data, _, err := c.request(ctx, urlOrPath, accept)
	return data, err
}

func (c *GitHubClient) GetAll(ctx context.Context, urlOrPath string, field, accept string) ([]any, error) {
	url := urlOrPath
	var items []any
	for url != "" {
		data, link, err := c.request(ctx, url, accept)
		if err != nil {
			return nil, err
		}
		var page any
		if field != "" {
			obj, ok := data.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("paginated GitHub API response is not a list: %s", c.redact(c.resolve(url)))
			}
			page = obj[field]
			if page == nil {
				page = []any{}
			}
		} else {
			page = data
		}
		list, ok := page.([]any)
		if !ok {
			return nil, fmt.Errorf("paginated GitHub API response is not a list: %s", c.redact(c.resolve(url)))
		}
		items = append(items, list...)
		url = nextLink(link)
	}
	if items == nil {
		return []any{}, nil
	}
	return items, nil
}

func (c *GitHubClient) request(ctx context.Context, urlOrPath, accept string) (any, string, error) {
	if accept == "" {
		accept = "application/vnd.github+json"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	resolved := c.resolve(urlOrPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved, nil)
	if err != nil {
		return nil, "", fmt.Errorf("GitHub API request for %s: %w", c.redact(resolved), err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("GitHub API request for %s: %w", c.redact(resolved), err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("GitHub API request for %s: %w", c.redact(resolved), err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		detail := strings.ToValidUTF8(string(body), string(utf8.RuneError))
		return nil, "", fmt.Errorf("GitHub API %d for %s: %s", resp.StatusCode, c.redact(resolved), c.redact(detail))
	}
	data, err := decodeJSON(bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("GitHub API JSON for %s: %w", c.redact(resolved), err)
	}
	return data, resp.Header.Get("Link"), nil
}

func (c *GitHubClient) resolve(urlOrPath string) string {
	if strings.HasPrefix(urlOrPath, "http://") || strings.HasPrefix(urlOrPath, "https://") {
		return urlOrPath
	}
	return c.BaseURL + urlOrPath
}

func (c *GitHubClient) redact(s string) string {
	if c != nil && c.Token != "" && strings.Contains(s, c.Token) {
		return strings.ReplaceAll(s, c.Token, "REDACTED")
	}
	return s
}

func nextLink(header string) string {
	match := nextLinkPattern.FindStringSubmatch(header)
	if match == nil {
		return ""
	}
	return match[1]
}

func decodeJSON(r io.Reader) (any, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("JSON contains more than one value")
		}
		return nil, err
	}
	return convertJSON(v), nil
}

func convertJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = convertJSON(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = convertJSON(val)
		}
		return t
	case json.Number:
		if i, err := t.Int64(); err == nil {
			if int64(int(i)) == i {
				return int(i)
			}
			return i
		}
		f, err := t.Float64()
		if err != nil {
			return t.String()
		}
		return f
	default:
		return v
	}
}
