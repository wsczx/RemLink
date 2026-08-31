package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

type tencentSender struct {
	secretId   string
	secretKey  string
	sdkAppId   string
	signName   string
	templateId string
	region     string
}

func newTencentSender(cfg *dbdata.SettingSms) *tencentSender {
	region := cfg.TencentRegion
	if region == "" {
		region = "ap-guangzhou"
	}
	return &tencentSender{
		secretId:   cfg.TencentSecretId,
		secretKey:  string(cfg.TencentSecretKey),
		sdkAppId:   cfg.TencentSdkAppId,
		signName:   cfg.TencentSignName,
		templateId: cfg.TencentTemplateId,
		region:     region,
	}
}

// buildTemplateParams 按模板变量序号排序
func getTemplateParams(params map[string]string) []string {
	if len(params) == 0 {
		return []string{}
	}
	ids := make([]int, 0, len(params))
	for k := range params {
		n, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		ids = append(ids, n)
	}
	sort.Ints(ids)
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		result = append(result, params[strconv.Itoa(id)])
	}
	return result
}

// 将腾讯云错误码映射为中文提示
func tencentErrMsg(code, msg string) string {
	switch code {
	case "FailedOperation.PhoneNumberInBlacklist":
		return "手机号在黑名单中"
	case "FailedOperation.TemplateIncorrectOrUnapproved":
		return "模板未审批或不存在，请检查模板ID"
	case "FailedOperation.SignatureIncorrectOrUnapproved":
		return "签名未审批或不存在，请检查签名"
	case "FailedOperation.InsufficientBalanceInSmsPackage":
		return "短信套餐包余量不足，请充值"
	case "FailedOperation.TemplateParamSetNotMatchApprovedTemplate":
		return "模板参数与审批内容不匹配"
	case "FailedOperation.TemplateParameterFormatError":
		return "模板参数格式错误"
	case "FailedOperation.SdkAppIdIsDisabled":
		return "SdkAppId 已被禁用"
	case "InvalidParameterValue.IncorrectPhoneNumber":
		return "手机号码格式错误"
	case "LimitExceeded.PhoneNumberDailyLimit":
		return "该手机号已达日发送上限"
	default:
		return msg
	}
}

const tencentSmsHost = "sms.tencentcloudapi.com"
const tencentSmsAction = "SendSms"
const tencentSmsVersion = "2021-01-11"

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func hmacSha256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func (s *tencentSender) Send(phone string, params map[string]string) error {
	if s.secretId == "" || s.secretKey == "" {
		return fmt.Errorf("腾讯云短信 SecretId/SecretKey 未配置")
	}

	// 手机号转 E.164 格式
	e164phone := phone
	if !strings.HasPrefix(phone, "+") {
		e164phone = "+86" + phone
	}

	templateParams := getTemplateParams(params)

	body := map[string]any{
		"PhoneNumberSet":   []string{e164phone},
		"SmsSdkAppId":      s.sdkAppId,
		"SignName":         s.signName,
		"TemplateId":       s.templateId,
		"TemplateParamSet": templateParams,
	}
	payloadBytes, _ := json.Marshal(body)
	payload := string(payloadBytes)

	// TC3-HMAC-SHA256 签名
	now := time.Now().UTC()
	timestamp := now.Unix()
	date := now.Format("2006-01-02")

	// 1. CanonicalRequest
	httpMethod := "POST"
	canonicalURI := "/"
	canonicalQuery := ""
	canonicalHeaders := fmt.Sprintf("content-type:application/json; charset=utf-8\nhost:%s\nx-tc-action:%s\n",
		tencentSmsHost, strings.ToLower(tencentSmsAction))
	signedHeaders := "content-type;host;x-tc-action"
	hashedPayload := sha256Hex(payload)
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpMethod, canonicalURI, canonicalQuery, canonicalHeaders, signedHeaders, hashedPayload)

	// 2. StringToSign
	algorithm := "TC3-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/sms/tc3_request", date)
	hashedCanonicalRequest := sha256Hex(canonicalRequest)
	stringToSign := fmt.Sprintf("%s\n%d\n%s\n%s",
		algorithm, timestamp, credentialScope, hashedCanonicalRequest)

	// 3. Signature
	secretDate := hmacSha256([]byte("TC3"+s.secretKey), date)
	secretService := hmacSha256(secretDate, "sms")
	secretSigning := hmacSha256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSha256(secretSigning, stringToSign))

	// 4. Authorization header
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, s.secretId, credentialScope, signedHeaders, signature)

	req, err := http.NewRequest("POST", "https://"+tencentSmsHost, strings.NewReader(payload))
	if err != nil {
		base.Error("腾讯云短信请求构建失败:", err)
		return fmt.Errorf("腾讯云短信请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", tencentSmsHost)
	req.Header.Set("X-TC-Action", tencentSmsAction)
	req.Header.Set("X-TC-Version", tencentSmsVersion)
	req.Header.Set("X-TC-Region", s.region)
	req.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("Authorization", authorization)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		base.Error("腾讯云短信请求失败:", err)
		return fmt.Errorf("腾讯云短信请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Response struct {
			SendStatusSet []struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"SendStatusSet"`
			Error struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
			RequestId string `json:"RequestId"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		base.Error("腾讯云短信响应解析失败:", string(respBody))
		return fmt.Errorf("腾讯云短信响应异常")
	}

	if result.Response.Error.Code != "" {
		errMsg := tencentErrMsg(result.Response.Error.Code, result.Response.Error.Message)
		base.Error("腾讯云短信发送失败:", result.Response.Error.Code, errMsg)
		return fmt.Errorf("腾讯云短信发送失败: %s", errMsg)
	}

	if len(result.Response.SendStatusSet) > 0 && result.Response.SendStatusSet[0].Code != "Ok" {
		code := result.Response.SendStatusSet[0].Code
		errMsg := tencentErrMsg(code, result.Response.SendStatusSet[0].Message)
		base.Error("腾讯云短信发送失败:", code, errMsg)
		return fmt.Errorf("腾讯云短信发送失败: %s", errMsg)
	}

	return nil
}
