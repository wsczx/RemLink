package dbdata

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wsczx/remlink/pkg/utils"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/security"
	"software.sslmate.com/src/go-pkcs12"
)

// 客户端证书的存储入口
type CertStore struct{ *table[ClientCertData] }

// 客户端证书的存储入口实例，由 SetDB 注入持久化实现
var Certs = &CertStore{}

// 客户端证书数据结构
type ClientCertData struct {
	Id                   int                      `json:"id" xorm:"pk autoincr not null"`
	Username             string                   `json:"username" xorm:"varchar(60)"`
	Groupname            string                   `json:"groupname" xorm:"varchar(60)"`
	Status               int                      `json:"status" xorm:"int"`
	IsCSRBased           bool                     `json:"is_csr_based" xorm:"bool"`
	Certificate          string                   `json:"certificate" xorm:"text"`
	PrivateKey           security.EncryptedString `json:"private_key" xorm:"text"`
	DeviceId             []string                 `json:"device_id" xorm:"text"`
	DeviceBindingEnabled bool                     `json:"device_binding_enabled" xorm:"bool"`
	MaxDevices           int                      `json:"max_devices" xorm:"int"`
	SerialNumber         string                   `json:"serial_number" xorm:"varchar(100)"`
	NotAfter             time.Time                `json:"not_after" xorm:"datetime"`
	CreatedAt            time.Time                `json:"created_at" xorm:"datetime created"`
	certMux              sync.Mutex
}

var (
	clientCACert *x509.Certificate
	clientCAKey  *rsa.PrivateKey
	// clientCAKey *ecdsa.PrivateKey
	caLoaded  bool
	caLoadErr error
	caMu      sync.Mutex // 保护 caLoaded 和证书变量
	certLock  [2048]sync.Mutex

	// 证书序列号生成器，防止高并发下 time.Now().UnixNano() 重复
	serialMu      sync.Mutex
	serialCounter int64
)

func getCertLock(certId int) *sync.Mutex {
	index := certId % len(certLock)
	if index < 0 {
		index = -index
	}
	return &certLock[index]
}

// 生成唯一证书序列号，防止高并发下 time.Now().UnixNano() 重复
func nextSerialNumber() *big.Int {
	serialMu.Lock()
	defer serialMu.Unlock()
	serialCounter++
	return big.NewInt(time.Now().UnixNano() + serialCounter)
}

func getCertLockByName(name string) *sync.Mutex {
	var hash uint32 = 2166136261
	for i := 0; i < len(name); i++ {
		hash *= 16777619
		hash ^= uint32(name[i])
	}

	index := hash % 2048
	return &certLock[index]
}

// 证书状态
const (
	CertStatusActive   = 0 // 有效
	CertStatusDisabled = 1 // 禁用
	CertStatusExpired  = 2 // 过期
)

// 获取证书状态描述
func (c *ClientCertData) GetStatusText() string {
	switch c.GetStatus() {
	case CertStatusActive:
		return "有效"
	case CertStatusDisabled:
		return "禁用"
	case CertStatusExpired:
		return "过期"
	default:
		return "未知"
	}
}

// 获取证书状态
func (c *ClientCertData) GetStatus() int {
	c.certMux.Lock()
	defer c.certMux.Unlock()

	return c.Status
}

// 保存客户端证书
func (c *ClientCertData) Save() error {
	c.certMux.Lock()
	defer c.certMux.Unlock()

	if c.Id > 0 {
		return GetDB().Save(c) // 更新现有记录
	}
	return GetDB().Insert(c)
}

// 禁用证书
func (c *ClientCertData) Disable() error {
	return c.UpdateStatus(CertStatusDisabled)
}

// 启用证书
func (c *ClientCertData) Enable() error {
	return c.UpdateStatus(CertStatusActive)
}

// 删除证书记录
func (c *ClientCertData) Delete() error {
	c.certMux.Lock()
	defer c.certMux.Unlock()

	return GetDB().Delete(c)
}

// 切换证书状态
func (c *ClientCertData) ChangeStatus() error {
	switch c.Status {
	case CertStatusActive:
		return c.Disable()
	case CertStatusDisabled:
		return c.Enable()
	}
	return fmt.Errorf("证书已过期，无法切换状态")
}

// 更新客户端证书状态
func (c *ClientCertData) UpdateStatus(status int) error {
	c.certMux.Lock()
	c.Status = status
	c.certMux.Unlock()

	if _, err := GetDB().Engine().ID(c.Id).Cols("status").Update(c); err != nil {
		return fmt.Errorf("更新客户端证书状态失败: %w", err)
	}
	return nil
}

// 检查并更新证书状态为过期
func (c *ClientCertData) CheckAndUpdateStatus() error {
	c.certMux.Lock()
	isExpired := c.Status != CertStatusExpired && time.Now().After(c.NotAfter)
	c.certMux.Unlock()

	if isExpired {
		return c.UpdateStatus(CertStatusExpired)
	}
	return nil
}

// 绑定设备ID
func (c *ClientCertData) BindDevice(deviceID string) error {
	if !c.DeviceBindingEnabled {
		return nil
	}
	if deviceID == "" {
		return fmt.Errorf("设备绑定功能仅支持Cisco AnyConnect客户端")
	}

	mu := getCertLock(c.Id)
	mu.Lock()
	defer mu.Unlock()

	cert := &ClientCertData{}
	has, err := GetDB().Engine().ID(c.Id).Get(cert)
	if err != nil || !has {
		return fmt.Errorf("查询证书失败")
	}

	if slices.Contains(cert.DeviceId, deviceID) {
		return nil
	}
	if len(cert.DeviceId) >= cert.MaxDevices {
		return fmt.Errorf("设备数量已达上限")
	}

	cert.DeviceId = append(cert.DeviceId, deviceID)
	if _, err := GetDB().Engine().ID(c.Id).Cols("device_id").Update(cert); err != nil {
		return fmt.Errorf("绑定设备ID失败: %w", err)
	}

	c.certMux.Lock()
	c.DeviceId = cert.DeviceId
	c.certMux.Unlock()

	return nil
}

// 解绑设备ID
func (c *ClientCertData) UnbindDevice(deviceID string) error {
	mu := getCertLock(c.Id)
	mu.Lock()
	defer mu.Unlock()

	cert := &ClientCertData{}
	has, err := GetDB().Engine().ID(c.Id).Get(cert)
	if err != nil {
		return fmt.Errorf("查询证书失败: %w", err)
	}
	if !has {
		return nil
	}

	found := false
	for i, id := range cert.DeviceId {
		if id == deviceID {
			cert.DeviceId = append(cert.DeviceId[:i], cert.DeviceId[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("未绑定")
	}

	if _, err := GetDB().Engine().ID(c.Id).Cols("device_id").Update(cert); err != nil {
		return fmt.Errorf("解绑设备ID失败: %w", err)
	}

	c.certMux.Lock()
	c.DeviceId = cert.DeviceId
	c.certMux.Unlock()

	return nil
}

// 检查是否绑定了设备ID
func (c *ClientCertData) CheckDevice(deviceID string) bool {
	c.certMux.Lock()
	defer c.certMux.Unlock()

	if !c.DeviceBindingEnabled {
		return true // 不启用设备绑定
	}
	if deviceID == "" {
		return false
	}
	return slices.Contains(c.DeviceId, deviceID)
}

// 获取客户端证书列表
func (st *CertStore) GetClientCertList(pageSize, pageIndex int, username, groupname, status string) ([]ClientCertData, int64, error) {
	var certs []ClientCertData
	session := GetDB().Engine().NewSession()
	defer session.Close()

	session = session.Where("1=1")
	session.OrderBy("id desc")
	// 添加搜索条件
	if username != "" {
		session.And("username LIKE ? ESCAPE '\\'", "%"+utils.EscapeLike(username)+"%")
	}
	if groupname != "" {
		session.And("groupname LIKE ? ESCAPE '\\'", "%"+utils.EscapeLike(groupname)+"%")
	}
	if status != "" {
		if statusInt, err := strconv.Atoi(status); err == nil {
			session.And("status = ?", statusInt)
		}
	}
	total, err := FindAndCount(session, &certs, pageSize, pageIndex)
	if err != nil {
		return nil, 0, fmt.Errorf("获取客户端证书列表失败: %w", err)
	}
	return certs, total, nil
}

// 统计指定组的有效客户端证书数量（仅统计 status=0 的有效证书）
func (st *CertStore) CountByGroupName(groupname string) (int64, error) {
	return GetDB().Engine().Where("groupname = ? AND status = 0", groupname).Count(&ClientCertData{})
}

// 获取客户端证书
func (st *CertStore) GetClientCert(username, groupname string) (*ClientCertData, error) {
	clientCert := &ClientCertData{}
	has, err := GetDB().Engine().Where("username = ? AND groupname = ?", username, groupname).Get(clientCert)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, utils.ErrNotFound
	}
	return clientCert, err
}

// 获取用户名下所有组的客户端证书
func (st *CertStore) GetClientCertsByUsername(username string) ([]ClientCertData, error) {
	var certs []ClientCertData
	err := GetDB().FindWhere(&certs, 0, 0, "username=?", username)
	return certs, err
}

// 生成客户端 CA 证书
func (st *CertStore) GenerateClientCA() error {

	priv, err := rsa.GenerateKey(rand.Reader, 2048)

	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: base.GetCfg().Issuer},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour * 24 * 365 * 10),
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,

		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	// 重置 CA 加载状态，后续 LoadClientCA() 会重新解析新证书
	resetClientCA()

	if err := Settings.SaveClientCA(string(certPEM), string(keyPEM)); err != nil {
		return fmt.Errorf("保存客户端CA到数据库失败: %w", err)
	}
	return nil
}

// 生成客户端证书并保存到数据库
func (st *CertStore) GenerateClientCert(username, groupname string, deviceBindingEnabled bool, maxDevices int, csrData ...string) (*ClientCertData, error) {
	mu := getCertLockByName(username)
	mu.Lock()
	defer mu.Unlock()

	user := &User{}
	err := GetDB().Get("Username", username, user)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return nil, fmt.Errorf("用户不存在: %s", username)
		}
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}

	if !slices.Contains(user.Groups, groupname) {
		return nil, fmt.Errorf("用户 %s 不属于组 %s", username, groupname)
	}

	_, err = st.GetClientCert(username, groupname)
	if err != nil {
		if !errors.Is(err, utils.ErrNotFound) {
			return nil, fmt.Errorf("获取用户证书失败: %w", err)
		}
	} else {
		return nil, fmt.Errorf("用户 %s 已存在证书,所在组：%s，请先删除现有证书", username, groupname)
	}

	if err := st.LoadClientCA(); err != nil {
		return nil, fmt.Errorf("无法加载客户端 CA: %w", err)
	}

	var publicKey crypto.PublicKey
	var certPEM, keyPEM []byte
	var csr string
	if len(csrData) > 0 {
		csr = csrData[0]
	}
	isCSRBased := csr != ""

	if isCSRBased {
		block, _ := pem.Decode([]byte(csrData[0]))
		if block == nil {
			return nil, fmt.Errorf("无法解析CSR PEM数据")
		}
		csr, err := x509.ParseCertificateRequest(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("解析CSR失败: %w", err)
		}
		if err := csr.CheckSignature(); err != nil {
			return nil, fmt.Errorf("CSR签名验证失败: %w", err)
		}
		publicKey = csr.PublicKey
		keyPEM = nil
	} else {
		clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}
		publicKey = &clientKey.PublicKey
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)})
	}

	template := x509.Certificate{
		SerialNumber: nextSerialNumber(),
		Subject: pkix.Name{
			CommonName:         username,
			OrganizationalUnit: []string{groupname},
			Organization:       []string{base.GetCfg().Issuer},
			// Country:            []string{"CN"},
			// Province:           []string{"Beijing"},
			// Locality:           []string{"Beijing"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 365),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:              []string{username},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// 签发客户端证书
	certDER, err := x509.CreateCertificate(rand.Reader, &template, clientCACert, publicKey, clientCAKey)
	if err != nil {
		return nil, err
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	clientCertData := &ClientCertData{
		Username:             username,
		Groupname:            groupname,
		IsCSRBased:           isCSRBased,
		Certificate:          string(certPEM),
		PrivateKey:           security.EncryptedString(keyPEM),
		SerialNumber:         template.SerialNumber.String(),
		NotAfter:             template.NotAfter,
		CreatedAt:            time.Now(),
		Status:               CertStatusActive,
		DeviceId:             []string{},
		DeviceBindingEnabled: deviceBindingEnabled,
		MaxDevices:           maxDevices,
	}

	if err := clientCertData.Save(); err != nil {
		return nil, fmt.Errorf("保存客户端证书失败: %w", err)
	}

	return clientCertData, nil
}

// 批量生成客户端证书
func (st *CertStore) BatchGenerateClientCert(usernames []string, groupname string, deviceBindingEnabled bool, maxDevices int) (success int, failed []string) {
	for _, username := range usernames {
		username = strings.TrimSpace(username)
		if username == "" {
			continue
		}
		if _, err := st.GenerateClientCert(username, groupname, deviceBindingEnabled, maxDevices); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", username, err))
			continue
		}
		success++
	}
	return success, failed
}

// 为指定用户续期客户端证书：保留原设备绑定/最大设备数等参数
// 删除旧证书后以相同用户名与组重新签发（有效期顺延 1 年）
// CSR 模式证书因不持有私钥，续期需由调用方重新上传 CSR（csrData 非空）
func (st *CertStore) RenewClientCert(username, groupname string, csrData ...string) (*ClientCertData, error) {
	old, err := st.GetClientCert(username, groupname)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return nil, fmt.Errorf("用户 %s 在组 %s 下没有可续期的证书", username, groupname)
		}
		return nil, fmt.Errorf("获取旧证书失败: %w", err)
	}

	if old.IsCSRBased && len(csrData) == 0 {
		return nil, fmt.Errorf("CSR 模式证书续期需重新上传 CSR")
	}
	if err := old.Delete(); err != nil {
		return nil, fmt.Errorf("删除旧证书失败: %w", err)
	}
	newCert, err := st.GenerateClientCert(username, groupname, old.DeviceBindingEnabled, old.MaxDevices, csrData...)
	if err != nil {
		return nil, err
	}
	newNotAfter := old.NotAfter.AddDate(0, 0, 365)
	if newNotAfter.Before(time.Now()) {
		newNotAfter = time.Now().AddDate(0, 0, 365)
	}
	newCert.NotAfter = newNotAfter
	if err := newCert.Save(); err != nil {
		return nil, fmt.Errorf("更新续期后过期时间失败: %w", err)
	}
	return newCert, nil
}

// 生成 PKCS#12 格式证书文件
func (st *CertStore) GenerateClientP12FromDB(username, groupname, password string) ([]byte, error) {
	clientCert, err := st.GetClientCert(username, groupname)
	if err != nil {
		return nil, err
	}
	if err := clientCert.CheckAndUpdateStatus(); err != nil {
		base.Error("检查并更新证书状态失败:", err)
	}
	if clientCert.GetStatus() != CertStatusActive {
		return nil, fmt.Errorf("用户 %s 的证书状态为：%s", username, clientCert.GetStatusText())
	}
	if clientCert.IsCSRBased {
		return []byte(clientCert.Certificate), nil
	}

	if err := st.LoadClientCA(); err != nil {
		return nil, fmt.Errorf("无法加载客户端 CA: %w", err)
	}

	certBlock, _ := pem.Decode([]byte(clientCert.Certificate))
	if certBlock == nil {
		return nil, fmt.Errorf("无法解析客户端证书 PEM 数据")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}

	keyBlock, _ := pem.Decode([]byte(string(clientCert.PrivateKey)))
	if keyBlock == nil {
		return nil, fmt.Errorf("无法解析客户端私钥 PEM 数据")
	}
	// RSA私钥
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	// 打包为 .p12 格式
	p12Data, err := pkcs12.LegacyDES.Encode(key, cert, []*x509.Certificate{clientCACert}, password)
	if err != nil {
		return nil, err
	}

	return p12Data, nil
}

// 验证客户端证书
func (st *CertStore) ValidateClientCert(cert *x509.Certificate, deviceid string) bool {
	user := &User{
		Username: cert.Subject.CommonName,
	}
	err := GetDB().Get("Username", user.Username, user)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			base.Error("证书验证失败：用户不存在", cert.Subject.CommonName)
		} else {
			base.Error("证书验证失败：查询用户失败:", err)
		}
		return false
	}

	if user.Status != 1 {
		base.Error("证书验证失败：用户已禁用:", user.Username)
		return false
	}

	if len(cert.Subject.OrganizationalUnit) == 0 {
		base.Error("证书验证失败：证书缺少组织单位(OU)信息")
		return false
	}
	ou := cert.Subject.OrganizationalUnit[0]

	clientCertData, err := st.GetClientCert(user.Username, ou)
	if err != nil {
		base.Error("证书验证失败：获取客户端证书失败:", err)
		return false
	}

	if clientCertData.Groupname != ou {
		base.Error("证书验证失败：证书组名与用户组名不匹配")
		return false
	}
	if time.Now().After(cert.NotAfter) {
		base.Error("证书验证失败：证书已过期:", cert.NotAfter)
		return false
	}

	// 验证证书链
	verifyOptions := x509.VerifyOptions{
		Roots:     st.LoadClientCAPool(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	if _, err := cert.Verify(verifyOptions); err != nil {
		base.Error("证书验证失败：证书链验证失败:", err)
		return false
	}
	// 检查扩展密钥用途
	hasClientAuth := slices.Contains(cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
	if !hasClientAuth {
		base.Error("证书验证失败：证书缺少客户端认证扩展")
		return false
	}

	// 验证证书指纹
	storedCertBlock, _ := pem.Decode([]byte(clientCertData.Certificate))
	if storedCertBlock == nil {
		base.Error("证书验证失败：无法解析存储的证书 PEM 数据")
		return false
	}
	storedCert, err := x509.ParseCertificate(storedCertBlock.Bytes)
	if err != nil {
		base.Error("证书验证失败：解析存储证书失败:", err)
		return false
	}

	// 比较证书的完整内容
	if !bytes.Equal(cert.Raw, storedCert.Raw) {
		base.Error("证书验证失败：证书内容不匹配")
		return false
	}

	// 检查证书状态
	if clientCertData.GetStatus() != CertStatusActive {
		base.Error("证书验证失败：", user.Username, "证书状态为", clientCertData.GetStatusText())
		return false
	}

	if !clientCertData.CheckDevice(deviceid) {
		if err := clientCertData.BindDevice(deviceid); err != nil {
			base.Error("证书验证失败：设备绑定失败:", err)
			return false
		} else {
			base.Info("设备绑定成功", user.Username, "用户组:", cert.Subject.OrganizationalUnit[0], "设备ID:", deviceid)
		}
	}
	return true
}

// 加载客户端 CA 证书池
func (st *CertStore) LoadClientCAPool() *x509.CertPool {
	if err := st.LoadClientCA(); err != nil {
		return nil
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(clientCACert)
	return caCertPool
}

// 加载客户端 CA 证书和私钥
func (st *CertStore) LoadClientCA() error {
	caMu.Lock()
	defer caMu.Unlock()
	if caLoaded {
		return caLoadErr
	}
	caLoadErr = loadClientCAInternal()
	caLoaded = true
	return caLoadErr
}

// 重置 CA 加载状态，允许重新加载
// 在 GenerateClientCA() 或 SettingLoadClientCA() 更新证书后调用
func resetClientCA() {
	caMu.Lock()
	defer caMu.Unlock()
	caLoaded = false
	clientCACert = nil
	clientCAKey = nil
	caLoadErr = nil
}

func loadClientCAInternal() error {
	ca := SettingClientCA{}
	err := Settings.Get(&ca)
	if err != nil {
		if utils.CheckErrNotFound(err) {
			base.Warn("无法读取客户端 CA 证书（可能尚未初始化）:", err)
			return fmt.Errorf("无法读取客户端 CA 证书,请初始化CA")
		}
		return fmt.Errorf("读取客户端 CA 证书失败: %w", err)
	}

	certContent := ca.CertContent
	keyContent := string(ca.KeyContent)

	if certContent == "" || keyContent == "" {
		base.Warn("客户端 CA 证书内容为空（可能尚未初始化）")
		return fmt.Errorf("无法读取客户端 CA 证书,请初始化CA")
	}

	caCertBlock, _ := pem.Decode([]byte(certContent))
	if caCertBlock == nil {
		return errors.New("无法解析客户端 CA 证书 PEM 块")
	}

	var parseErr error
	clientCACert, parseErr = x509.ParseCertificate(caCertBlock.Bytes)
	if parseErr != nil {
		return fmt.Errorf("无法解析客户端 CA 证书: %w", parseErr)
	}

	caKeyBlock, _ := pem.Decode([]byte(keyContent))
	if caKeyBlock == nil {
		return errors.New("无法解析客户端 CA 私钥 PEM 块")
	}
	// 解析RSA私钥
	var parseKeyErr error
	clientCAKey, parseKeyErr = x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if parseKeyErr != nil {
		// 解析为PKCS8
		pkcs8Key, pkcs8Err := x509.ParsePKCS8PrivateKey(caKeyBlock.Bytes)
		if pkcs8Err != nil {
			return fmt.Errorf("无法解析客户端 CA 私钥 (PKCS1 or PKCS8): %w", parseKeyErr)
		}
		var ok bool
		clientCAKey, ok = pkcs8Key.(*rsa.PrivateKey)
		if !ok {
			return errors.New("解析私钥成功，但不是 RSA 类型")
		}
	}
	return nil
}
