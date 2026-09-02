package admin

import (
	"archive/zip"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/notify"
	mail "github.com/xhit/go-simple-mail/v2"
)

func CustomCert(w http.ResponseWriter, r *http.Request) {
	cert, _, err := r.FormFile("cert")
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	key, _, err := r.FormFile("key")
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	certBytes, err := io.ReadAll(cert)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	keyBytes, err := io.ReadAll(key)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}

	// 校验证书和密钥是否合法
	if _, err := tls.X509KeyPair(certBytes, keyBytes); err != nil {
		RespError(w, RespParamErr, fmt.Sprintf("证书或密钥不合法，请重新上传: %v", err))
		return
	}

	// slot=wild 写入 WebVPN 泛域名证书，否则主证书
	wild := r.FormValue("slot") == "wild"
	if err := applyUploadedCert(w, wild, certBytes, keyBytes, r); err != nil {
		return
	}
	RespSucess(w, "上传成功")
}

// 保存上传证书并重载到 SNI 表。wild=true 写 WebVPN 泛域名证书，false 写主证书。
func applyUploadedCert(w http.ResponseWriter, wild bool, certBytes, keyBytes []byte, r *http.Request) error {
	certStr, keyStr := string(certBytes), string(keyBytes)
	if wild {
		if err := dbdata.SettingSaveTLSCertWild(certStr, keyStr); err != nil {
			RespError(w, RespInternalErr, err)
			return err
		}
		wildCert, _, err := dbdata.ParseCertWild()
		if err != nil {
			RespError(w, RespInternalErr, fmt.Sprintf("泛域名证书加载失败:%v", err))
			return err
		} else if wildCert != nil {
			dbdata.LoadCertificates([]*tls.Certificate{wildCert})
		}
		dbdata.AdminLog("证书管理", "WebVPN泛域名证书", "上传了自定义泛域名TLS证书", r.RemoteAddr)
		return nil
	}
	if err := dbdata.SettingSaveTLSCert(certStr, keyStr); err != nil {
		RespError(w, RespInternalErr, err)
		return err
	}
	tlscert, _, err := dbdata.ParseCert()
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("证书加载失败:%v", err))
		return err
	}
	dbdata.LoadCertificate(tlscert)
	dbdata.AdminLog("证书管理", "TLS证书", "上传了自定义TLS证书", r.RemoteAddr)
	return nil
}
func GetCertSetting(w http.ResponseWriter, r *http.Request) {
	data := &dbdata.SettingLetsEncrypt{}
	if err := dbdata.SettingGet(data); err != nil {
		if dbdata.CheckErrNotFound(err) {
			// 记录不存在时创建默认记录
			if saveErr := dbdata.SettingSave(data); saveErr != nil {
				base.Warn("创建默认LetsEncrypt配置失败:", saveErr)
			}
		} else {
			RespError(w, RespInternalErr, err)
			return
		}
	}
	data.DNSProvider.AliYun.SecretKey = data.DNSProvider.AliYun.SecretKey.Masked()
	data.DNSProvider.TXCloud.SecretKey = data.DNSProvider.TXCloud.SecretKey.Masked()
	data.DNSProvider.CfCloud.AuthToken = data.DNSProvider.CfCloud.AuthToken.Masked()
	RespSucess(w, data)
}
func CreatCert(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	defer r.Body.Close()
	config := &dbdata.SettingLetsEncrypt{}
	if err := json.Unmarshal(body, config); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	// 证书类型：wild=WebVPN 泛域名证书
	certType := r.FormValue("cert_type")
	if certType == "" {
		var certTypeBody struct {
			CertType string `json:"certType"`
		}
		if err := json.Unmarshal(body, &certTypeBody); err == nil {
			certType = certTypeBody.CertType
		}
	}
	// 保留未修改的 DNS 密钥
	old := &dbdata.SettingLetsEncrypt{}
	if dbdata.SettingGet(old) == nil {
		if config.DNSProvider.AliYun.SecretKey.IsPlaceholder() {
			config.DNSProvider.AliYun.SecretKey = old.DNSProvider.AliYun.SecretKey
		}
		if config.DNSProvider.TXCloud.SecretKey.IsPlaceholder() {
			config.DNSProvider.TXCloud.SecretKey = old.DNSProvider.TXCloud.SecretKey
		}
		if config.DNSProvider.CfCloud.AuthToken.IsPlaceholder() {
			config.DNSProvider.CfCloud.AuthToken = old.DNSProvider.CfCloud.AuthToken
		}
	}
	if err := dbdata.SettingSave(config); err != nil {
		RespError(w, RespInternalErr, err)
		return
	}
	client := dbdata.LeGoClient{}
	if err := client.NewClient(config); err != nil {
		base.Error(err)
		RespError(w, RespInternalErr, fmt.Sprintf("获取证书失败:%v", err))
		return
	}
	// cert_type=wild 时申请 WebVPN 泛域名证书（*.WebVpnDomain）
	domain := config.Domain
	wild := false
	if certType == "wild" {
		wild = true
		domain = base.GetCfg().WebVpnDomain
		if domain == "" {
			RespError(w, RespParamErr, "未配置 WebVPN 域名（WebVpnDomain），无法申请泛域名证书")
			return
		}
	}
	if err := client.GetCert(domain, wild); err != nil {
		base.Error(err)
		RespError(w, RespInternalErr, fmt.Sprintf("获取证书失败:%v", err))
		return
	}
	if wild {
		dbdata.AdminLog("证书管理", "*."+domain, "通过LetsEncrypt生成了WebVPN泛域名证书", r.RemoteAddr)
	} else {
		dbdata.AdminLog("证书管理", config.Domain, "通过LetsEncrypt生成了证书", r.RemoteAddr)
	}
	RespSucess(w, "生成证书成功")
}

// 查询客户端 CA 是否已初始化
func CheckCAStatus(w http.ResponseWriter, r *http.Request) {
	ca := dbdata.SettingClientCA{}
	err := dbdata.SettingGet(&ca)
	initialized := err == nil && ca.CertContent != "" && ca.KeyContent != ""
	RespSucess(w, map[string]bool{"initialized": initialized})
}

// 初始化客户端 CA
// force=true 可强制重置已有的 CA
func InitClientCA(w http.ResponseWriter, r *http.Request) {
	force := r.FormValue("force") == "true"
	if !force {
		ca := dbdata.SettingClientCA{}
		if err := dbdata.SettingGet(&ca); err == nil && ca.CertContent != "" && ca.KeyContent != "" {
			RespError(w, RespInternalErr, "客户端 CA 已存在，如需强制重置请使用 force=true 参数")
			return
		}
	}
	err := dbdata.GenerateClientCA()
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("客户端 CA 生成失败: %v", err))
		return
	}
	if force {
		dbdata.AdminLog("证书管理", "客户端CA", "强制重置了客户端CA（旧证书已失效）", r.RemoteAddr)
		RespSucess(w, "客户端 CA 强制重置成功（旧客户端证书已失效）")
	} else {
		dbdata.AdminLog("证书管理", "客户端CA", "初始化了客户端CA", r.RemoteAddr)
		RespSucess(w, "客户端 CA 初始化成功")
	}
}

// 生成客户端证书
func GenerateClientCert(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	if username == "" {
		RespError(w, RespInternalErr, "用户名不能为空")
		return
	}
	groupname := r.FormValue("group_name")
	if groupname == "" {
		RespError(w, RespInternalErr, "用户组不能为空")
		return
	}
	csrData := r.FormValue("csr")

	deviceBindingEnabled := false
	if deviceBindingStr := r.FormValue("device_binding_enabled"); deviceBindingStr == "true" {
		deviceBindingEnabled = true
	}

	// 获取最大设备数
	maxDevicesStr := r.FormValue("max_devices")
	if maxDevicesStr == "" {
		RespError(w, RespInternalErr, "最大设备数不能为空")
		return
	}

	maxDevices, err := strconv.Atoi(maxDevicesStr)
	if err != nil || maxDevices < 1 {
		RespError(w, RespInternalErr, "最大设备数必须为正整数")
		return
	}

	// 检查用户是否存在
	user := &dbdata.User{}
	if err := dbdata.One("Username", username, user); err != nil {
		RespError(w, RespInternalErr, "用户不存在")
		return
	}

	// 生成客户端证书
	var certData *dbdata.ClientCertData
	if csrData != "" {
		certData, err = dbdata.GenerateClientCert(username, groupname, deviceBindingEnabled, maxDevices, csrData)
	} else {
		certData, err = dbdata.GenerateClientCert(username, groupname, deviceBindingEnabled, maxDevices)
	}
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("证书生成失败: %v", err))
		return
	}

	dbdata.AdminLog("证书管理", username, "为用户生成客户端证书(组:"+groupname+")", r.RemoteAddr)
	certData.PrivateKey = certData.PrivateKey.Masked()
	RespSucess(w, certData)
}

// 批量生成客户端证书
func BatchGenerateClientCert(w http.ResponseWriter, r *http.Request) {
	raw := r.FormValue("usernames")
	if strings.TrimSpace(raw) == "" {
		RespError(w, RespParamErr, "用户名列表不能为空")
		return
	}
	groupname := r.FormValue("group_name")
	if groupname == "" {
		RespError(w, RespParamErr, "用户组不能为空")
		return
	}
	// 支持换行、逗号、空格分隔
	usernames := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == ',' || r == ' ' || r == '\r' || r == '\t'
	})

	deviceBindingEnabled := false
	if deviceBindingStr := r.FormValue("device_binding_enabled"); deviceBindingStr == "true" {
		deviceBindingEnabled = true
	}
	maxDevicesStr := r.FormValue("max_devices")
	if maxDevicesStr == "" {
		maxDevicesStr = "3"
	}
	maxDevices, err := strconv.Atoi(maxDevicesStr)
	if err != nil || maxDevices < 1 {
		RespError(w, RespParamErr, "最大设备数必须为正整数")
		return
	}

	success, failed := dbdata.BatchGenerateClientCert(usernames, groupname, deviceBindingEnabled, maxDevices)
	dbdata.AdminLog("证书管理", "批量", fmt.Sprintf("批量生成客户端证书 %d 个(组:%s)", success, groupname), r.RemoteAddr)
	RespSucess(w, map[string]any{
		"success": success,
		"failed":  failed,
	})
}

// 续期客户端证书：删除旧证书并以相同参数重新签发
func RenewClientCert(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	if username == "" {
		RespError(w, RespParamErr, "用户名不能为空")
		return
	}
	groupname := r.FormValue("group_name")
	if groupname == "" {
		RespError(w, RespParamErr, "用户组不能为空")
		return
	}
	csrData := r.FormValue("csr")

	var certData *dbdata.ClientCertData
	var err error
	if csrData != "" {
		certData, err = dbdata.RenewClientCert(username, groupname, csrData)
	} else {
		certData, err = dbdata.RenewClientCert(username, groupname)
	}
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("证书续期失败: %v", err))
		return
	}
	dbdata.AdminLog("证书管理", username, "续期了客户端证书(组:"+groupname+")", r.RemoteAddr)
	certData.PrivateKey = certData.PrivateKey.Masked()
	RespSucess(w, certData)
}

// 更新客户端证书的最大设备数
func UpdateClientCertMaxDevices(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	if username == "" {
		RespError(w, RespInternalErr, "用户名不能为空")
		return
	}

	groupname := r.FormValue("groupname")
	if groupname == "" {
		RespError(w, RespInternalErr, "用户组不能为空")
		return
	}

	maxDevicesStr := r.FormValue("max_devices")
	if maxDevicesStr == "" {
		RespError(w, RespInternalErr, "最大设备数不能为空")
		return
	}

	maxDevices, err := strconv.Atoi(maxDevicesStr)
	if err != nil || maxDevices < 1 {
		RespError(w, RespInternalErr, "最大设备数必须为正整数")
		return
	}

	// 获取证书记录
	certData, err := dbdata.GetClientCert(username, groupname)
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("获取证书失败: %v", err))
		return
	}

	// 检查当前绑定设备数是否超过新设置的最大值
	if len(certData.DeviceId) > maxDevices {
		RespError(w, RespInternalErr, fmt.Sprintf("当前已绑定 %d 台设备，不能少于当前绑定数", len(certData.DeviceId)))
		return
	}

	// 更新最大设备数
	certData.MaxDevices = maxDevices
	err = certData.Save()
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("更新证书失败: %v", err))
		return
	}

	dbdata.AdminLog("证书管理", username, "更新了证书最大设备数(组:"+groupname+",最大设备:"+maxDevicesStr+")", r.RemoteAddr)
	certData.PrivateKey = certData.PrivateKey.Masked()
	RespSucess(w, certData)
}

// 下载客户端 P12 证书
func DownloadClientP12(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	groupname := r.FormValue("groupname")
	password := r.FormValue("password")

	if username == "" {
		RespError(w, RespInternalErr, "用户名不能为空")
		return
	}
	if groupname == "" {
		RespError(w, RespInternalErr, "用户组不能为空")
		return
	}

	// if password == "" {
	// 	password = "123456" // 默认密码
	// }

	// 下载CSR模式的证书
	clientCert, err := dbdata.GetClientCert(username, groupname)
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("获取证书失败: %v", err))
		return
	}

	if clientCert.IsCSRBased {
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.cer", username))
		dbdata.AdminLog("证书管理", username, "下载了客户端证书(组:"+groupname+")", r.RemoteAddr)
		w.Write([]byte(clientCert.Certificate))
		return
	}

	// 生成 P12 证书
	p12Data, err := dbdata.GenerateClientP12FromDB(username, groupname, password)
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("证书下载失败: %v", err))
		return
	}

	// 设置下载响应头
	w.Header().Set("Content-Type", "application/x-pkcs12")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.p12", username))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(p12Data)))
	dbdata.AdminLog("证书管理", username, "下载了客户端P12证书(组:"+groupname+")", r.RemoteAddr)
	w.Write(p12Data)
}

// 切换客户端证书状态（禁用/启用）
func ChangeClientCertStatus(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	if username == "" {
		RespError(w, RespInternalErr, "用户名不能为空")
		return
	}
	groupname := r.FormValue("groupname")
	if groupname == "" {
		RespError(w, RespInternalErr, "用户组不能为空")
		return
	}

	clientCert, err := dbdata.GetClientCert(username, groupname)
	if err != nil {
		RespError(w, RespInternalErr, "证书不存在")
		return
	}

	err = clientCert.ChangeStatus()
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("证书状态切换失败: %v", err))
		return
	}

	statusText := "启用"
	if clientCert.Status == dbdata.CertStatusDisabled {
		statusText = "禁用"
	}

	dbdata.AdminLog("证书管理", username, statusText+"客户端证书(组:"+groupname+")", r.RemoteAddr)
	RespSucess(w, fmt.Sprintf("证书%s成功", statusText))
}

// 删除客户端证书
func DeleteClientCert(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	if username == "" {
		RespError(w, RespInternalErr, "用户名不能为空")
		return
	}
	groupname := r.FormValue("groupname")
	if groupname == "" {
		RespError(w, RespInternalErr, "用户组不能为空")
		return
	}

	clientCert, err := dbdata.GetClientCert(username, groupname)
	if err != nil {
		RespError(w, RespInternalErr, "证书不存在")
		return
	}

	err = clientCert.Delete()
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("证书删除失败: %v", err))
		return
	}

	dbdata.AdminLog("证书管理", username, "删除了客户端证书(组:"+groupname+")", r.RemoteAddr)
	RespSucess(w, "证书删除成功")
}

// 批量删除客户端证书
type batchCertReq struct {
	Certs []certMailItem `json:"certs"`
}

func BatchDeleteClientCert(w http.ResponseWriter, r *http.Request) {
	var req batchCertReq
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespError(w, RespInternalErr, "请求参数解析失败")
		return
	}
	if len(req.Certs) == 0 {
		RespError(w, RespParamErr, "证书记录不能为空")
		return
	}

	var successCount, failCount int
	var failDetails []string

	for _, item := range req.Certs {
		clientCert, err := dbdata.GetClientCert(item.Username, item.Groupname)
		if err != nil {
			failCount++
			failDetails = append(failDetails, fmt.Sprintf("%s/%s: 证书不存在", item.Username, item.Groupname))
			continue
		}
		if err := clientCert.Delete(); err != nil {
			failCount++
			failDetails = append(failDetails, fmt.Sprintf("%s/%s: 删除失败: %v", item.Username, item.Groupname, err))
			continue
		}
		successCount++
	}

	msg := fmt.Sprintf("批量删除完成，成功：%d，失败：%d", successCount, failCount)
	dbdata.AdminLog("证书管理", "批量删除", "批量删除了"+strconv.Itoa(successCount)+"个客户端证书", r.RemoteAddr)
	if len(failDetails) > 0 {
		msg += "；失败详情: " + strings.Join(failDetails, "；")
	}
	RespSucess(w, msg)
}

// 批量下载客户端证书（打包为 ZIP）
func BatchDownloadClientP12(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Certs    []certMailItem `json:"certs"`
		Password string         `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespError(w, RespInternalErr, "请求参数解析失败")
		return
	}
	if len(req.Certs) == 0 {
		RespError(w, RespParamErr, "证书记录不能为空")
		return
	}

	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	for _, item := range req.Certs {
		cert, err := dbdata.GetClientCert(item.Username, item.Groupname)
		if err != nil || cert.IsCSRBased {
			// CSR 模式跳过
			continue
		}
		p12Data, err := dbdata.GenerateClientP12FromDB(item.Username, item.Groupname, req.Password)
		if err != nil {
			continue
		}
		f, err := zipWriter.Create(item.Username + ".p12")
		if err != nil {
			continue
		}
		if _, err := f.Write(p12Data); err != nil {
			continue
		}
	}

	zipWriter.Close()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=certificates.zip")
	dbdata.AdminLog("证书管理", "批量下载", "批量下载了"+strconv.Itoa(len(req.Certs))+"个客户端P12证书", r.RemoteAddr)
	w.Write(buf.Bytes())
}

// 获取默认证书邮件模板
func GetCertMailTemplate(w http.ResponseWriter, r *http.Request) {
	RespSucess(w, dbdata.CertMailTemplate())
}

// 获取客户端证书列表
func GetClientCertList(w http.ResponseWriter, r *http.Request) {
	pageSize := 10
	pageIndex := 1

	if r.FormValue("page_size") != "" {
		if ps, err := strconv.Atoi(r.FormValue("page_size")); err == nil {
			pageSize = ps
		}
	}

	if r.FormValue("page_index") != "" {
		if pi, err := strconv.Atoi(r.FormValue("page_index")); err == nil {
			pageIndex = pi
		}
	}

	// 添加搜索参数
	username := r.FormValue("username")
	groupname := r.FormValue("groupname")
	status := r.FormValue("status")

	certs, total, err := dbdata.GetClientCertList(pageSize, pageIndex, username, groupname, status)
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("获取证书列表失败: %v", err))
		return
	}

	data := map[string]any{
		"list":  certs,
		"total": total,
	}

	for i := range certs {
		certs[i].PrivateKey = certs[i].PrivateKey.Masked()
	}
	RespSucess(w, data)
}

// 获取用户证书生成所需信息
func UserCertInfo(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()

	// 获取所有启用的用户
	var users []dbdata.User
	err := dbdata.Find(&users, 1000, 1)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}

	// 获取所有启用的组
	var groups []dbdata.Group
	err = dbdata.Find(&groups, 1000, 1)
	if err != nil && !dbdata.CheckErrNotFound(err) {
		RespError(w, RespInternalErr, err)
		return
	}

	// 过滤启用的用户和组
	activeUsers := make([]dbdata.User, 0)
	for _, user := range users {
		if user.Status == 1 {
			activeUsers = append(activeUsers, user)
		}
	}

	activeGroups := make([]dbdata.Group, 0)
	for _, group := range groups {
		if group.Status == 1 {
			activeGroups = append(activeGroups, group)
		}
	}

	data := map[string]any{
		"users":  activeUsers,
		"groups": activeGroups,
	}

	RespSucess(w, data)
}

// 解绑特定设备
func UnbindDevice(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	groupname := r.FormValue("groupname")
	deviceId := r.FormValue("device_id")

	if username == "" || groupname == "" || deviceId == "" {
		RespError(w, RespParamErr, "用户名、用户组和设备ID不能为空")
		return
	}

	// 获取证书信息
	cert, err := dbdata.GetClientCert(username, groupname)
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("获取证书失败: %v", err))
		return
	}

	// 解绑设备
	if err := cert.UnbindDevice(deviceId); err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("解绑设备失败: %v", err))
		return
	}

	dbdata.AdminLog("证书管理", username, "解绑了证书设备(组:"+groupname+",设备:"+deviceId+")", r.RemoteAddr)
	RespSucess(w, nil)
}

// 更新客户端证书的设备绑定开关
func UpdateClientCertDeviceBinding(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	if username == "" {
		RespError(w, RespInternalErr, "用户名不能为空")
		return
	}

	groupname := r.FormValue("groupname")
	if groupname == "" {
		RespError(w, RespInternalErr, "用户组不能为空")
		return
	}

	// 获取设备绑定开关
	deviceBindingEnabled := false
	if deviceBindingStr := r.FormValue("device_binding_enabled"); deviceBindingStr == "true" {
		deviceBindingEnabled = true
	}

	// 获取证书记录
	certData, err := dbdata.GetClientCert(username, groupname)
	if err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("获取证书失败: %v", err))
		return
	}

	// 如果关闭设备绑定，清空已绑定的设备ID
	if !deviceBindingEnabled && certData.DeviceBindingEnabled {
		certData.DeviceId = []string{}
	}

	// 更新设备绑定开关
	certData.DeviceBindingEnabled = deviceBindingEnabled
	if err := certData.Save(); err != nil {
		RespError(w, RespInternalErr, fmt.Sprintf("更新证书失败: %v", err))
		return
	}

	dbdata.AdminLog("证书管理", username, "更新了证书设备绑定(组:"+groupname+")", r.RemoteAddr)
	certData.PrivateKey = certData.PrivateKey.Masked()
	RespSucess(w, certData)
}

// 发送证书邮件的请求体
type sendCertMailReq struct {
	Certs    []certMailItem `json:"certs"`
	Password string         `json:"password"`
}

type certMailItem struct {
	Username  string `json:"username"`
	Groupname string `json:"groupname"`
}

// 发送证书邮件（支持批量）
func SendClientCertMail(w http.ResponseWriter, r *http.Request) {
	var req sendCertMailReq
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespError(w, RespInternalErr, "请求参数解析失败")
		return
	}

	if len(req.Certs) == 0 {
		RespError(w, RespParamErr, "证书记录不能为空")
		return
	}

	// P12 密码（允许空密码）
	password := req.Password

	// 获取 LinkAddr 与证书邮件模板
	dataOther := &dbdata.SettingOther{}
	if err := dbdata.SettingGet(dataOther); err != nil {
		base.Error("获取设置失败:", err)
	}

	certMailContent := dataOther.CertMail
	if certMailContent == "" {
		certMailContent = dbdata.CertMailTemplate()
	}
	certMailHTML := `<html><body>` + certMailContent + `</body></html>`

	// 检查 SMTP 是否配置
	smtpCfg := &dbdata.SettingSmtp{}
	if err := dbdata.SettingGet(smtpCfg); err != nil || smtpCfg.Host == "" {
		RespError(w, RespInternalErr, "邮件服务未配置，请先在系统设置中配置SMTP")
		return
	}

	var (
		wg             sync.WaitGroup
		mu             sync.Mutex
		successCount   int
		failCount      int
		failDetails    []string
		maxConcurrency = min(len(req.Certs), 10)
		concurrency    = make(chan struct{}, maxConcurrency)
	)

	for _, item := range req.Certs {
		wg.Add(1)
		go func(username, groupname string) {
			defer wg.Done()
			concurrency <- struct{}{}
			defer func() { <-concurrency }()

			// 获取证书记录
			cert, err := dbdata.GetClientCert(username, groupname)
			if err != nil {
				mu.Lock()
				failCount++
				failDetails = append(failDetails, fmt.Sprintf("%s/%s: 证书记录不存在", username, groupname))
				mu.Unlock()
				return
			}

			// CSR模式证书不支持P12
			if cert.IsCSRBased {
				mu.Lock()
				failCount++
				failDetails = append(failDetails, fmt.Sprintf("%s/%s: CSR模式证书无法生成P12文件", username, groupname))
				mu.Unlock()
				return
			}

			// 查用户邮箱
			user := &dbdata.User{}
			if err := dbdata.One("Username", username, user); err != nil {
				mu.Lock()
				failCount++
				failDetails = append(failDetails, fmt.Sprintf("%s/%s: 用户不存在", username, groupname))
				mu.Unlock()
				return
			}
			if user.Email == "" {
				mu.Lock()
				failCount++
				failDetails = append(failDetails, fmt.Sprintf("%s/%s: 未设置邮箱地址", username, groupname))
				mu.Unlock()
				return
			}

			// 生成 P12 文件
			p12Data, err := dbdata.GenerateClientP12FromDB(username, groupname, password)
			if err != nil {
				mu.Lock()
				failCount++
				failDetails = append(failDetails, fmt.Sprintf("%s/%s: 生成P12失败: %v", username, groupname, err))
				mu.Unlock()
				return
			}

			// 拼装邮件正文
			body := certMailHTML
			body = strings.ReplaceAll(body, "{{.Username}}", username)
			body = strings.ReplaceAll(body, "{{.Groupname}}", groupname)
			body = strings.ReplaceAll(body, "{{.SerialNumber}}", cert.SerialNumber)
			body = strings.ReplaceAll(body, "{{.NotAfter}}", cert.NotAfter.Local().Format("2006-01-02 15:04"))
			body = strings.ReplaceAll(body, "{{.Password}}", password)
			body = strings.ReplaceAll(body, "{{.LinkAddr}}", dataOther.LinkAddr)

			// 发送邮件
			attach := &mail.File{
				MimeType: "application/x-pkcs12",
				Name:     fmt.Sprintf("%s.p12", username),
				Data:     p12Data,
				Inline:   false,
			}

			err = notify.GetNotify().SendEmail(notify.Message{
				Subject:    fmt.Sprintf("[%s] 客户端证书", base.GetCfg().Issuer),
				To:         user.Email,
				Body:       body,
				Attachment: attach,
			})
			mu.Lock()
			if err != nil {
				failCount++
				failDetails = append(failDetails, fmt.Sprintf("%s/%s: 邮件发送失败: %v", username, groupname, err))
			} else {
				successCount++
			}
			mu.Unlock()
		}(item.Username, item.Groupname)
	}

	wg.Wait()

	dbdata.AdminLog("证书管理", "批量发送邮件", fmt.Sprintf("批量发送证书邮件(成功:%d,失败:%d)", successCount, failCount), r.RemoteAddr)

	msg := fmt.Sprintf("批量发送完成，成功：%d，失败：%d", successCount, failCount)
	if len(failDetails) > 0 {
		msg += "；失败详情: " + strings.Join(failDetails, "；")
	}

	if successCount > 0 || (successCount == 0 && failCount == 0) {
		RespSucess(w, msg)
	} else {
		RespError(w, RespInternalErr, msg)
	}
}
