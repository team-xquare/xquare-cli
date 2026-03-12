package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	base       string
	token      string
	httpClient *http.Client
}

func New(base, token string) *Client {
	return &Client{
		base:  base,
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	resp, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

func (c *Client) put(ctx context.Context, path string, body, out any) error {
	resp, err := c.do(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

func (c *Client) patch(ctx context.Context, path string, body, out any) error {
	resp, err := c.do(ctx, http.MethodPatch, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

func (c *Client) delete(ctx context.Context, path string) error {
	resp, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return parseError(resp)
	}
	return nil
}

func decode(resp *http.Response, out any) error {
	if resp.StatusCode >= 400 {
		return parseError(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func parseError(resp *http.Response) error {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if e.Error != "" {
		return fmt.Errorf("server error %d: %s", resp.StatusCode, e.Error)
	}
	return fmt.Errorf("server error %d", resp.StatusCode)
}

// Auth

type AuthResponse struct {
	Token     string `json:"token"`
	GithubID  int64  `json:"github_id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

func (c *Client) AuthGitHub(ctx context.Context, code string) (*AuthResponse, error) {
	var out AuthResponse
	return &out, c.post(ctx, "/auth/github/callback", map[string]string{"code": code}, &out)
}

// Projects

func (c *Client) ListProjects(ctx context.Context) ([]string, error) {
	var out struct {
		Projects []string `json:"projects"`
	}
	return out.Projects, c.get(ctx, "/projects", &out)
}

func (c *Client) GetProject(ctx context.Context, project string) (map[string]any, error) {
	var out map[string]any
	return out, c.get(ctx, "/projects/"+project, &out)
}

func (c *Client) CreateProject(ctx context.Context, name string) error {
	return c.post(ctx, "/projects", map[string]string{"name": name}, nil)
}

func (c *Client) DeleteProject(ctx context.Context, project string) error {
	return c.delete(ctx, "/projects/"+project)
}

func (c *Client) ListMembers(ctx context.Context, project string) ([]string, error) {
	var out struct {
		Owners []string `json:"owners"`
	}
	return out.Owners, c.get(ctx, "/projects/"+project+"/members", &out)
}

func (c *Client) AddMember(ctx context.Context, project, username string) error {
	return c.post(ctx, "/projects/"+project+"/members", map[string]string{"username": username}, nil)
}

func (c *Client) RemoveMember(ctx context.Context, project, username string) error {
	return c.delete(ctx, "/projects/"+project+"/members/"+username)
}

// Apps

func (c *Client) ListApps(ctx context.Context, project string) ([]map[string]any, error) {
	var out struct {
		Applications []map[string]any `json:"applications"`
	}
	return out.Applications, c.get(ctx, "/projects/"+project+"/apps", &out)
}

func (c *Client) GetApp(ctx context.Context, project, app string) (map[string]any, error) {
	var out map[string]any
	return out, c.get(ctx, "/projects/"+project+"/apps/"+app, &out)
}

func (c *Client) GetAppStatus(ctx context.Context, project, app string) (map[string]any, error) {
	var out map[string]any
	return out, c.get(ctx, "/projects/"+project+"/apps/"+app+"/status", &out)
}

func (c *Client) CreateApp(ctx context.Context, project string, body any) (map[string]any, error) {
	var out map[string]any
	return out, c.post(ctx, "/projects/"+project+"/apps", body, &out)
}

func (c *Client) UpdateApp(ctx context.Context, project, app string, body any) (map[string]any, error) {
	var out map[string]any
	return out, c.put(ctx, "/projects/"+project+"/apps/"+app, body, &out)
}

func (c *Client) DeleteApp(ctx context.Context, project, app string) error {
	return c.delete(ctx, "/projects/"+project+"/apps/"+app)
}

func (c *Client) RedeployApp(ctx context.Context, project, app, _ string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/projects/"+project+"/apps/"+app+"/redeploy", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	return out, decode(resp, &out)
}

// Env

func (c *Client) GetEnv(ctx context.Context, project, app string) (map[string]string, error) {
	var out map[string]string
	return out, c.get(ctx, "/projects/"+project+"/apps/"+app+"/env", &out)
}

func (c *Client) SetEnv(ctx context.Context, project, app string, envs map[string]string) (map[string]string, error) {
	var out map[string]string
	return out, c.put(ctx, "/projects/"+project+"/apps/"+app+"/env", envs, &out)
}

func (c *Client) PatchEnv(ctx context.Context, project, app string, envs map[string]string) (map[string]string, error) {
	var out map[string]string
	return out, c.patch(ctx, "/projects/"+project+"/apps/"+app+"/env", envs, &out)
}

func (c *Client) DeleteEnvKey(ctx context.Context, project, app, key string) error {
	return c.delete(ctx, "/projects/"+project+"/apps/"+app+"/env/"+key)
}

// Addons

func (c *Client) ListAddons(ctx context.Context, project string) ([]map[string]any, error) {
	var out struct {
		Addons []map[string]any `json:"addons"`
	}
	return out.Addons, c.get(ctx, "/projects/"+project+"/addons", &out)
}

func (c *Client) CreateAddon(ctx context.Context, project string, body any) (map[string]any, error) {
	var out map[string]any
	return out, c.post(ctx, "/projects/"+project+"/addons", body, &out)
}

func (c *Client) DeleteAddon(ctx context.Context, project, addon string) error {
	return c.delete(ctx, "/projects/"+project+"/addons/"+addon)
}

func (c *Client) GetAddonConnection(ctx context.Context, project, addon string) (map[string]any, error) {
	var out map[string]any
	return out, c.get(ctx, "/projects/"+project+"/addons/"+addon+"/connection", &out)
}

// Logs — returns the raw response for streaming
func (c *Client) StreamLogs(ctx context.Context, project, app string, tail int64, follow bool, since string) (*http.Response, error) {
	url := fmt.Sprintf("%s/projects/%s/apps/%s/logs?tail=%d", c.base, project, app, tail)
	if !follow {
		url += "&follow=false"
	}
	if since != "" {
		url += "&since=" + since
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return (&http.Client{}).Do(req)
}

// Builds

func (c *Client) ListBuilds(ctx context.Context, project, app string) ([]map[string]any, error) {
	var out struct {
		Builds []map[string]any `json:"builds"`
	}
	return out.Builds, c.get(ctx, "/projects/"+project+"/apps/"+app+"/builds", &out)
}

func (c *Client) StreamBuildLogs(ctx context.Context, project, app, workflow string, follow bool) (*http.Response, error) {
	url := fmt.Sprintf("%s/projects/%s/apps/%s/builds/%s/logs", c.base, project, app, workflow)
	if !follow {
		url += "?follow=false"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return (&http.Client{}).Do(req)
}
