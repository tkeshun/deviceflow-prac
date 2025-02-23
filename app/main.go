package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	clientID        = "YOUR_CLIENT_ID" // GitHub AppのClient ID
	deviceAuthURL   = "https://github.com/login/device/code"
	accessTokenURL  = "https://github.com/login/oauth/access_token"
	githubUserURL   = "https://api.github.com/user"
	pollingInterval = 5 * time.Second // intervalより長くしない、とりあえず5秒
)

// デバイスコード
type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// アクセストークン
type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error,omitempty"`
}

func main() {

	// 1. デバイスコードを取得
	deviceCodeRes, err := requestDeviceFlow()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("verification_uri: %s\n", deviceCodeRes.VerificationURI)
	fmt.Printf("user_code: %s\n", deviceCodeRes.UserCode)

	// 2. アクセストークンを取得（ポーリング）
	timeout := 3 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var accessTokenResponse accessTokenResponse
loop:
	for {
		select {
		case <-ctx.Done():
		default:
			accessTokenRes, err := requestAccessToken(deviceCodeRes.DeviceCode)
			if err != nil {
				log.Fatal(err)
			}
			if accessTokenRes.AccessToken != "" {
				break loop
			}

			switch accessTokenRes.Error {
			case "authorization_pending":
				fmt.Println("ユーザー認証待ち")
				continue
			case "slow_down":
				fmt.Println("ポーリング感覚短すぎ")
			case "access_denied":
				log.Fatal("認証失敗")
			case "expired_token":
				log.Fatal("デバイスコードの有効期限が切れた")
			default:
				log.Fatal(accessTokenResponse.Error)
			}
			// time.Sleep(pollingInterval)
			time.Sleep(time.Duration(deviceCodeRes.Interval) * time.Second)
		}

	}

	userInfo, err := getGitHubUserInfo(accessTokenResponse.AccessToken)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(userInfo)
}

func requestDeviceFlow() (*deviceCodeResponse, error) {
	client := http.Client{}

	sendData := url.Values{}
	sendData.Set("client_id", clientID)
	sendData.Set("scope", "repo,user")

	req, err := http.NewRequest("POST", deviceAuthURL, strings.NewReader(sendData.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	dCodeResp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer dCodeResp.Body.Close()

	var deviceCodeRes deviceCodeResponse
	err = json.NewDecoder(dCodeResp.Body).Decode(&deviceCodeRes)
	if err != nil {
		return nil, err
	}
	return &deviceCodeRes, nil
}

func requestAccessToken(deviceCode string) (*accessTokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("device_code", deviceCode)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	resp, err := http.PostForm(accessTokenURL, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenRes accessTokenResponse
	err = json.NewDecoder(resp.Body).Decode(&tokenRes)
	if err != nil {
		return nil, err
	}

	return &tokenRes, nil
}

func getGitHubUserInfo(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", githubUserURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
