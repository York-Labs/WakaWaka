package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/gin-gonic/gin"
)

type QRCodeResponse struct {
	Data QRCodeData `json:"data"`
}

type QRCodeData struct {
	Sid string `json:"sid"`
}

type StatusResponse struct {
	Status    string `json:"status"`
	AuthCode  string `json:"authCode"`
}

type TokenResponse struct {
	Data TokenData `json:"data"`
}

type TokenData struct {
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func main() {
	// 请求生成二维码（不再需要，但保留逻辑）
	resp, err := http.Post("http://api.extscreen.com/aliyundrive/qrcode", "application/x-www-form-urlencoded", strings.NewReader("scopes=user:base,file:all:read,file:all:write&width=500&height=500"))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	var qrCodeResp QRCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&qrCodeResp); err != nil {
		panic(err)
	}
	sid := qrCodeResp.Data.Sid

	// 打印授权链接
	fmt.Printf("点击此链接授权阿里云盘TV：https://www.aliyundrive.com/o/oauth/authorize?sid=%s\n", sid)

	// 轮询检查登录状态
	var refreshToken string
	for {
		time.Sleep(3 * time.Second)
		statusResp, err := http.Get(fmt.Sprintf("https://openapi.alipan.com/oauth/qrcode/%s/status", sid))
		if err != nil {
			panic(err)
		}
		defer statusResp.Body.Close()

		var statusData StatusResponse
		if err := json.NewDecoder(statusResp.Body).Decode(&statusData); err != nil {
			panic(err)
		}

		if statusData.Status == "LoginSuccess" {
			authCode := statusData.AuthCode
			tokenResp, err := http.Post("http://api.extscreen.com/aliyundrive/token", "application/x-www-form-urlencoded", strings.NewReader(fmt.Sprintf("code=%s", authCode)))
			if err != nil {
				panic(err)
			}
			defer tokenResp.Body.Close()

			var tokenData TokenResponse
			if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil {
				panic(err)
			}

			refreshToken = tokenData.Data.RefreshToken
			fmt.Printf("refresh_token: %s\n", refreshToken)

			saveToken(tokenData.Data)
			break
		}
	}

	// 启动HTTP服务器
	router := gin.Default()
	router.POST("/refresh", func(c *gin.Context) {
		var req RefreshRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		resp, err := http.Post("http://api.extscreen.com/aliyundrive/token", "application/x-www-form-urlencoded", strings.NewReader(fmt.Sprintf("refresh_token=%s", req.RefreshToken)))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer resp.Body.Close()

		var tokenResp TokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token_type":   "Bearer",
			"access_token": tokenResp.Data.AccessToken,
			"refresh_token": tokenResp.Data.RefreshToken,
			"expires_in":    tokenResp.Data.ExpiresIn,
		})

		// 保存刷新后的token
		saveToken(tokenResp.Data)
	})

	port := 5001
	fmt.Printf("Server listening on port %d\n", port)
	router.Run(fmt.Sprintf(":%d", port))
}

func saveToken(data TokenData) {
	tokenFile, err := os.Create("token.yml")
	if err != nil {
		panic(err)
	}
	defer tokenFile.Close()

	tokenData := map[string]interface{}{
		"refresh_token": data.RefreshToken,
		"access_token":  data.AccessToken,
		"expires_in":    data.ExpiresIn,
	}

	encoder := yaml.NewEncoder(tokenFile)
	if err := encoder.Encode(tokenData); err != nil {
		panic(err)
	}
}
