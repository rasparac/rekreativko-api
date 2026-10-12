package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// apiClient calls identity and account-profile through the gateway, the same
// way the apps do.
type apiClient struct {
	baseURL string
	http    *http.Client
}

func newAPIClient(baseURL string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// envelope is the shape of every response (shared/api.Response).
type envelope[T any] struct {
	Data  T `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Register creates the account and returns its ID.
func (c *apiClient) Register(ctx context.Context, email, password string) (string, error) {
	var accountID string
	err := c.do(ctx, http.MethodPost, "/identity/api/v1/register", "", map[string]string{
		"email":    email,
		"password": password,
	}, &accountID)
	return accountID, err
}

func (c *apiClient) VerifyAccount(ctx context.Context, code string) error {
	return c.do(ctx, http.MethodPost, "/identity/api/v1/verify-account", "", map[string]string{"code": code}, nil)
}

// Login returns an access token.
func (c *apiClient) Login(ctx context.Context, email, password string) (string, error) {
	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	err := c.do(ctx, http.MethodPost, "/identity/api/v1/login", "", map[string]string{
		"email":    email,
		"password": password,
	}, &tokens)
	return tokens.AccessToken, err
}

func (c *apiClient) UpdateProfile(ctx context.Context, token string, p Profile) error {
	return c.do(ctx, http.MethodPut, "/account-profile/api/v1/my/profile", token, profileRequest(p), nil)
}

func (c *apiClient) UpdateSettings(ctx context.Context, token string, settings map[string]any) error {
	return c.do(ctx, http.MethodPut, "/account-profile/api/v1/my/settings", token, map[string]any{"settings": settings}, nil)
}

// profileRequest builds the PUT /my/profile body (dtos.UpdateProfileRequest),
// leaving out what the YAML doesn't set so the service keeps its value.
func profileRequest(p Profile) map[string]any {
	body := map[string]any{}
	set := func(key, value string) {
		if value != "" {
			body[key] = value
		}
	}

	set("full_name", p.FullName)
	set("nickname", p.Nickname)
	set("bio", p.Bio)
	set("date_of_birth", p.DateOfBirth)
	set("profile_picture_url", p.ProfilePictureURL)

	if loc := p.Location; loc != nil {
		set("location_city", loc.City)
		set("location_country", loc.Country)
		if loc.Latitude != nil && loc.Longitude != nil {
			body["location_latitude"] = *loc.Latitude
			body["location_longitude"] = *loc.Longitude
		}
	}

	if len(p.ActivityInterests) > 0 {
		interests := make([]map[string]string, 0, len(p.ActivityInterests))
		for _, ai := range p.ActivityInterests {
			interests = append(interests, map[string]string{"name": ai.Name, "level": ai.Level})
		}
		body["activity_interests"] = interests
	}

	return body
}

func (c *apiClient) do(ctx context.Context, method, path, token string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s %s: read response: %w", method, path, err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		var env envelope[json.RawMessage]
		if json.Unmarshal(raw, &env) == nil && env.Error != nil {
			return fmt.Errorf("%s %s: HTTP %d %s: %s", method, path, resp.StatusCode, env.Error.Code, env.Error.Message)
		}
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, bytes.TrimSpace(raw))
	}

	if out == nil {
		return nil
	}

	env := envelope[any]{Data: out}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", method, path, err)
	}

	return nil
}
