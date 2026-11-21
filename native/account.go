package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

type chatLogin struct {
	URL             string `json:"authUrl"`
	LoginID         string `json:"loginId"`
	VerificationURL string `json:"verificationUrl"`
	UserCode        string `json:"userCode"`
}

func (c *Codex) beginChatLogin(ctx context.Context) (chatLogin, error) {
	var start chatLogin
	err := c.Call(ctx, "account/login/start", map[string]string{"type": "chatgpt"}, &start)
	if err != nil {
		// Windows can reserve the loopback port without a listening process.
		// The official device flow also works when another app owns the port.
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "login server") && !strings.Contains(msg, "address already in use") && !strings.Contains(msg, "port 1455") {
			return chatLogin{}, err
		}
		start = chatLogin{}
		if err = c.Call(ctx, "account/login/start", map[string]string{"type": "chatgptDeviceCode"}, &start); err != nil {
			return chatLogin{}, errors.New("回调端口不可用，且未能取得 OpenAI 设备码，请检查网络及账户的设备码登录设置")
		}
		start.URL = start.VerificationURL
		if start.UserCode == "" || len(start.UserCode) > 32 {
			return chatLogin{}, errors.New("OpenAI 未返回有效的设备码")
		}
	}
	u, err := url.Parse(start.URL)
	if err != nil || u.Scheme != "https" || u.Host != "auth.openai.com" || u.User != nil || start.LoginID == "" {
		return chatLogin{}, errors.New("Codex 未返回有效的 OpenAI 登录页面")
	}
	return start, nil
}

func (c *Codex) loginChatGPT(ctx context.Context, open func(chatLogin) error) error {
	start, err := c.beginChatLogin(ctx)
	if err != nil {
		return err
	}
	if err = open(start); err != nil {
		return errors.New("无法打开系统浏览器，请检查默认浏览器设置")
	}
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return errors.New("已取消登录")
			}
			return errors.New("登录等待超时，可重新登录")
		case <-c.done:
			return errors.New("登录过程中 Codex 已断开")
		case n := <-c.notifications:
			if n.Method != "account/login/completed" {
				continue
			}
			var result struct {
				LoginID string
				Success bool
			}
			if json.Unmarshal(n.Params, &result) != nil || result.LoginID != start.LoginID {
				continue
			}
			if !result.Success {
				return errors.New("ChatGPT 授权未完成，请重新登录")
			}
			return nil
		}
	}
}
