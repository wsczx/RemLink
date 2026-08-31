package notify

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

type aliyunSender struct {
	accessKeyId     string
	accessKeySecret string
	signName        string
	templateCode    string
}

func newAliyunSender(cfg *dbdata.SettingSms) *aliyunSender {
	return &aliyunSender{
		accessKeyId:     cfg.AliAccessKeyId,
		accessKeySecret: string(cfg.AliAccessKeySecret),
		signName:        cfg.AliSignName,
		templateCode:    cfg.AliTemplateCode,
	}
}

func (s *aliyunSender) Send(phone string, params map[string]string) error {
	// 模板变量映射：params["1"]→code(验证码), params["2"]→time(有效分钟)
	templateParam := make(map[string]string)
	if v, ok := params["1"]; ok {
		templateParam["code"] = v
	}
	if v, ok := params["2"]; ok {
		templateParam["time"] = v
	}
	paramBytes, _ := json.Marshal(templateParam)

	query := url.Values{}
	query.Set("AccessKeyId", s.accessKeyId)
	query.Set("Action", "SendSms")
	query.Set("Format", "JSON")
	query.Set("PhoneNumbers", phone)
	query.Set("SignName", s.signName)
	query.Set("TemplateCode", s.templateCode)
	query.Set("TemplateParam", string(paramBytes))
	query.Set("SignatureMethod", "HMAC-SHA1")
	query.Set("SignatureNonce", fmt.Sprintf("%d", time.Now().UnixNano()))
	query.Set("SignatureVersion", "1.0")
	query.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	query.Set("Version", "2017-05-25")

	// HMAC-SHA1 签名
	stringToSign := "GET&" + url.QueryEscape("/") + "&" + url.QueryEscape(query.Encode())
	mac := hmac.New(sha1.New, []byte(s.accessKeySecret+"&"))
	mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	query.Set("Signature", signature)

	reqURL := "https://dysmsapi.aliyuncs.com/?" + query.Encode()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		base.Error("阿里云短信请求失败:", err)
		return fmt.Errorf("阿里云短信请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		base.Error("阿里云短信响应解析失败:", string(body))
		return fmt.Errorf("阿里云短信响应异常")
	}

	if result.Code != "OK" {
		errMsg := result.Message
		switch result.Code {
		case "isv.BUSINESS_LIMIT_CONTROL":
			errMsg = "触发流控限制，请稍后重试"
		case "isv.MOBILE_NUMBER_ILLEGAL":
			errMsg = "手机号码格式错误"
		case "isv.TEMPLATE_MISSING_PARAMETERS":
			errMsg = "模板变量缺失"
		case "isp.RAM_PERMISSION_DENY":
			errMsg = "RAM权限不足，请检查AccessKey权限"
		}
		base.Error("阿里云短信发送失败:", result.Code, errMsg)
		return fmt.Errorf("阿里云短信发送失败: %s", errMsg)
	}

	return nil
}
