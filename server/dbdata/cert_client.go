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
	"sync"
	"time"

	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/pkg/security"
	"software.sslmate.com/src/go-pkcs12"
)

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
		return Set(c) // 更新现有记录
	}
	return Add(c)
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

	return Del(c)
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

	if _, err := GetXdb().ID(c.Id).Cols("status").Update(c); err != nil {
		return fmt.Errorf("更新客户端证书状态失败: %v", err)
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
func (c *ClientCertData) BindDevice(deviceId string) error {
	if !c.DeviceBindingEnabled {
		return nil
	}
	if deviceId == "" {
		return fmt.Errorf("设备绑定功能仅支持Cisco AnyConnect客户端")
	}

	mu := getCertLock(c.Id)
	mu.Lock()
	defer mu.Unlock()

	cert := &ClientCertData{}
	has, err := GetXdb().ID(c.Id).Get(cert)
	if err != nil || !has {
		return fmt.Errorf("查询证书失败")
	}

	if slices.Contains(cert.DeviceId, deviceId) {
		return nil
	}
	if len(cert.DeviceId) >= cert.MaxDevices {
		return fmt.Errorf("设备数量已达上限")
	}

	cert.DeviceId = append(cert.DeviceId, deviceId)
	if _, err := GetXdb().ID(c.Id).Cols("device_id").Update(cert); err != nil {
		return fmt.Errorf("绑定设备ID失败: %v", err)
	}

	c.certMux.Lock()
	c.DeviceId = cert.DeviceId
	c.certMux.Unlock()

	return nil
}

// 解绑设备ID
func (c *ClientCertData) UnbindDevice(deviceId string) error {
	mu := getCertLock(c.Id)
	mu.Lock()
	defer mu.Unlock()

	cert := &ClientCertData{}
	has, err := GetXdb().ID(c.Id).Get(cert)
	if err != nil {
		return fmt.Errorf("查询证书失败: %w", err)
	}
	if !has {
		return nil
	}

	found := false
	for i, id := range cert.DeviceId {
		if id == deviceId {
			cert.DeviceId = append(cert.DeviceId[:i], cert.DeviceId[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("未绑定")
	}

	if _, err := GetXdb().ID(c.Id).Cols("device_id").Update(cert); err != nil {
		return fmt.Errorf("解绑设备ID失败: %v", err)
	}

	c.certMux.Lock()
	c.DeviceId = cert.DeviceId
	c.certMux.Unlock()

	return nil
}

// 检查是否绑定了设备ID
func (c *ClientCertData) CheckDevice(deviceId string) bool {
	c.certMux.Lock()
	defer c.certMux.Unlock()

	if !c.DeviceBindingEnabled {
		return true // 不启用设备绑定
	}
	if deviceId == "" {
		return false
	}
	return slices.Contains(c.DeviceId, deviceId)
}

// 获取客户端证书列表
func GetClientCertList(pageSize, pageIndex int, username, groupname, status string) ([]ClientCertData, int64, error) {
	var certs []ClientCertData
	session := GetXdb().NewSession()
	defer session.Close()

	session = session.Where("1=1")
	session.OrderBy("id desc")
	// 添加搜索条件
	if username != "" {
		session.And("username LIKE ? ESCAPE '\\'", "%"+EscapeLike(username)+"%")
	}
	if groupname != "" {
		session.And("groupname LIKE ? ESCAPE '\\'", "%"+EscapeLike(groupname)+"%")
	}
	if status != "" {
		if statusInt, err := strconv.Atoi(status); err == nil {
			session.And("status = ?", statusInt)
		}
	}
	total, err := FindAndCount(session, &certs, pageSize, pageIndex)
	if err != nil {
		return nil, 0, fmt.Errorf("获取客户端证书列表失败: %v", err)
	}
	return certs, total, nil
}

// 统计指定组的有效客户端证书数量（仅统计 status=0 的有效证书）
func CountCertsByGroupName(groupname string) (int64, error) {
	return GetXdb().Where("groupname = ? AND status = 0", groupname).Count(&ClientCertData{})
}

// 获取客户端证书
func GetClientCert(username, groupname string) (*ClientCertData, error) {
	clientCert := &ClientCertData{}
	has, err := GetXdb().Where("username = ? AND groupname = ?", username, groupname).Get(clientCert)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrNotFound
	}
	return clientCert, err
}

// 获取用户名下所有组的客户端证书
func GetClientCertsByUsername(username string) ([]ClientCertData, error) {
	var certs []ClientCertData
	err := FindWhere(&certs, 0, 0, "username=?", username)
	return certs, err
}

// 生成客户端 CA 证书
func GenerateClientCA() error {
	// 生成RSA密钥
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	// 生成ECC密钥
	// priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: base.GetCfg().Issuer},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour * 24 * 365 * 10), // 10年有效期
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		// ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	// 编码为 PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	// RSA 私钥
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	// ECC 私钥
	// keyBytes, err := x509.MarshalECPrivateKey(priv)
	// if err != nil {
	// 	return err
	// }
	// keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	// 重置 CA 加载状态，后续 LoadClientCA() 会重新解析新证书
	resetClientCA()

	// 证书 PEM 独立存储
	if err := SettingSaveClientCA(string(certPEM), string(keyPEM)); err != nil {
		return fmt.Errorf("保存客户端CA到数据库失败: %w", err)
	}
	return nil
}

// 生成客户端证书并保存到数据库
func GenerateClientCert(username, groupname string, deviceBindingEnabled bool, maxDevices int, csrData ...string) (*ClientCertData, error) {
	mu := getCertLockByName(username)
	mu.Lock()
	defer mu.Unlock()

	// 检查用户是否存在并验证组成员资格
	user := &User{}
	err := One("Username", username, user)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("用户不存在: %s", username)
		}
		return nil, fmt.Errorf("获取用户信息失败: %v", err)
	}

	// 检查用户是否属于指定组
	if !slices.Contains(user.Groups, groupname) {
		return nil, fmt.Errorf("用户 %s 不属于组 %s", username, groupname)
	}
	// 检查是否已存在证书记录
	_, err = GetClientCert(username, groupname)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("获取用户证书失败: %v", err)
		}
	} else {
		// 用户已有证书记录，不允许重复生成
		return nil, fmt.Errorf("用户 %s 已存在证书,所在组：%s，请先删除现有证书", username, groupname)
	}

	// 确保客户端 CA 已加载
	if err := LoadClientCA(); err != nil {
		return nil, fmt.Errorf("无法加载客户端 CA: %v", err)
	}

	var publicKey crypto.PublicKey
	var certPEM, keyPEM []byte
	var csr string
	if len(csrData) > 0 {
		csr = csrData[0]
	}
	isCSRBased := csr != ""

	if isCSRBased {
		// CSR模式:解析CSR获取公钥
		block, _ := pem.Decode([]byte(csrData[0]))
		if block == nil {
			return nil, fmt.Errorf("无法解析CSR PEM数据")
		}
		csr, err := x509.ParseCertificateRequest(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("解析CSR失败: %w", err)
		}
		// 验证CSR签名
		if err := csr.CheckSignature(); err != nil {
			return nil, fmt.Errorf("CSR签名验证失败: %w", err)
		}
		publicKey = csr.PublicKey
		keyPEM = nil // CSR模式不存储私钥
	} else {
		// 服务端生成模式:生成客户端私钥
		// RSA 私钥
		clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
		// ECC 私钥
		// clientKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			return nil, err
		}
		publicKey = &clientKey.PublicKey
		// ECC私钥编码
		// keyBytes, err := x509.MarshalECPrivateKey(clientKey)
		// keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)})
	}

	// 创建客户端证书模板
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
		NotAfter:              time.Now().Add(time.Hour * 24 * 365), // 1年有效期
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
	// 保存到数据库
	clientCertData := &ClientCertData{
		Username:             username,
		Groupname:            groupname,
		IsCSRBased:           isCSRBased,
		Certificate:          string(certPEM),
		PrivateKey:           security.EncryptedString(keyPEM),
		SerialNumber:         template.SerialNumber.String(),
		NotAfter:             template.NotAfter,
		CreatedAt:            time.Now(),
		Status:               CertStatusActive, // 初始状态为有效
		DeviceId:             []string{},
		DeviceBindingEnabled: deviceBindingEnabled,
		MaxDevices:           maxDevices,
	}

	if err := clientCertData.Save(); err != nil {
		return nil, fmt.Errorf("保存客户端证书失败: %v", err)
	}

	return clientCertData, nil
}

// 生成 PKCS#12 格式证书文件
func GenerateClientP12FromDB(username, groupname, password string) ([]byte, error) {
	// 从数据库获取证书
	clientCert, err := GetClientCert(username, groupname)
	if err != nil {
		return nil, err
	}
	// 检查并更新证书状态
	if err := clientCert.CheckAndUpdateStatus(); err != nil {
		base.Error("检查并更新证书状态失败:", err)
	}
	// 检查证书状态
	if clientCert.GetStatus() != CertStatusActive {
		return nil, fmt.Errorf("用户 %s 的证书状态为：%s", username, clientCert.GetStatusText())
	}
	if clientCert.IsCSRBased {
		return []byte(clientCert.Certificate), nil
	}

	// 确保客户端 CA 已加载
	if err := LoadClientCA(); err != nil {
		return nil, fmt.Errorf("无法加载客户端 CA: %v", err)
	}

	// 解析证书和私钥
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
	// ECC私钥
	// key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
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
func ValidateClientCert(cert *x509.Certificate, deviceid string) bool {
	// 获取用户和证书信息
	user := &User{
		Username: cert.Subject.CommonName,
	}
	err := One("Username", user.Username, user)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			base.Error("证书验证失败：用户不存在", cert.Subject.CommonName)
		} else {
			base.Error("证书验证失败：查询用户失败:", err)
		}
		return false
	}

	// 检查用户状态是否启用
	if user.Status != 1 {
		base.Error("证书验证失败：用户已禁用:", user.Username)
		return false
	}

	// 检查证书是否包含组织单位信息
	if len(cert.Subject.OrganizationalUnit) == 0 {
		base.Error("证书验证失败：证书缺少组织单位(OU)信息")
		return false
	}
	ou := cert.Subject.OrganizationalUnit[0]

	// 获取客户端证书记录
	clientCertData, err := GetClientCert(user.Username, ou)
	if err != nil {
		base.Error("证书验证失败：获取客户端证书失败:", err)
		return false
	}

	if clientCertData.Groupname != ou {
		base.Error("证书验证失败：证书组名与用户组名不匹配")
		return false
	}
	// 检查证书是否过期
	if time.Now().After(cert.NotAfter) {
		base.Error("证书验证失败：证书已过期:", cert.NotAfter)
		return false
	}

	// 验证证书链
	verifyOptions := x509.VerifyOptions{
		Roots:     LoadClientCAPool(),
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
func LoadClientCAPool() *x509.CertPool {
	if err := LoadClientCA(); err != nil {
		return nil
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(clientCACert)
	return caCertPool
}

// 加载客户端 CA 证书和私钥
func LoadClientCA() error {
	caMu.Lock()
	defer caMu.Unlock()
	if caLoaded {
		return caLoadErr
	}
	caLoadErr = loadClientCAInternal()
	caLoaded = true
	return caLoadErr
}

// 重置 CA 加载状态，允许重新加载。
// 在 GenerateClientCA() 或 SettingLoadClientCA() 更新证书后调用。
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
	err := SettingGet(&ca)
	if err != nil {
		if CheckErrNotFound(err) {
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
