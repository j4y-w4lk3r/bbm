package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// EnableB2BucketEncryption turns on default SSE-B2 on a bucket via the B2
// Native API. S3 CreateBucket does not accept encryption settings on B2.
func EnableB2BucketEncryption(ctx context.Context, keyID, appKey, bucketName string) error {
	auth, err := b2Authorize(ctx, keyID, appKey)
	if err != nil {
		return err
	}

	bucketID, bucketType, err := b2FindBucket(ctx, auth, bucketName)
	if err != nil {
		return err
	}

	body, _ := json.Marshal(map[string]any{
		"accountId": auth.AccountID,
		"bucketId":  bucketID,
		"bucketType": bucketType,
		"defaultServerSideEncryption": map[string]string{
			"mode":      "SSE-B2",
			"algorithm": "AES256",
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, auth.APIURL+"/b2api/v4/b2_update_bucket", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", auth.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("b2_update_bucket: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("b2_update_bucket: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

type b2Auth struct {
	APIURL    string
	Token     string
	AccountID string
}

func b2Authorize(ctx context.Context, keyID, appKey string) (b2Auth, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.backblazeb2.com/b2api/v4/b2_authorize_account", nil)
	if err != nil {
		return b2Auth{}, err
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(keyID+":"+appKey)))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return b2Auth{}, fmt.Errorf("b2_authorize_account: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return b2Auth{}, fmt.Errorf("b2_authorize_account: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		APIURL            string `json:"apiUrl"`
		AuthorizationToken string `json:"authorizationToken"`
		AccountID         string `json:"accountId"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return b2Auth{}, fmt.Errorf("b2_authorize_account parse: %w", err)
	}
	return b2Auth{APIURL: out.APIURL, Token: out.AuthorizationToken, AccountID: out.AccountID}, nil
}

func b2FindBucket(ctx context.Context, auth b2Auth, name string) (bucketID, bucketType string, err error) {
	body, _ := json.Marshal(map[string]string{
		"accountId":  auth.AccountID,
		"bucketName": name,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, auth.APIURL+"/b2api/v4/b2_list_buckets", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", auth.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("b2_list_buckets: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("b2_list_buckets: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Buckets []struct {
			BucketID   string `json:"bucketId"`
			BucketName string `json:"bucketName"`
			BucketType string `json:"bucketType"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", fmt.Errorf("b2_list_buckets parse: %w", err)
	}
	for _, b := range out.Buckets {
		if b.BucketName == name {
			return b.BucketID, b.BucketType, nil
		}
	}
	return "", "", fmt.Errorf("bucket %q not found after create", name)
}
