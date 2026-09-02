package dbdata

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wsczx/remlink/base"
)

func TestEscapeLike(t *testing.T) {
	tests := map[string]string{
		"plain":      "plain",
		"user_name":  `user\_name`,
		"100%_user":  `100\%\_user`,
		`path\value`: `path\\value`,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, want, EscapeLike(input))
		})
	}
}

func TestGenerateClientCert(t *testing.T) {
	base.Test()
	ast := assert.New(t)
	req := require.New(t)

	preIpData(t)
	defer closeIpdata()

	// 使用 GenerateClientCA 生成 CA
	err := GenerateClientCA()
	req.NoError(err, "生成客户端 CA 失败")

	dns := []ValData{{Val: "8.8.8.8"}}
	p := Policy{Name: "cert-test-policy", Status: 1, ClientDns: dns}
	err = SetPolicy(&p)
	req.NoError(err)
	group := "cert-test-group"
	g := Group{Name: group, Status: 1, PolicyId: p.Id}
	err = SetGroup(&g)
	req.NoError(err)

	username := "cert-test-user"
	u := User{Username: username, Groups: []string{group}, Status: 1}
	err = SetUser(&u)
	req.NoError(err)

	certData, err := GenerateClientCert(username, group, true, 3)
	req.NoError(err)
	req.NotNil(certData)
	ast.Equal(username, certData.Username)
	ast.Equal(group, certData.Groupname)
	ast.Equal(CertStatusActive, certData.Status)
	ast.NotEmpty(certData.Certificate)
	ast.NotEmpty(certData.PrivateKey)
	ast.NotEmpty(certData.SerialNumber)

	_, err = GenerateClientCert(username, group, true, 3)
	ast.NotNil(err)
	ast.Contains(err.Error(), "已存在证书")

	_, err = GenerateClientCert(username, "nonexistent-group", true, 3)
	ast.NotNil(err)
	ast.Contains(err.Error(), "不属于组")

	_, err = GenerateClientCert("nonexistent-user", group, true, 3)
	ast.NotNil(err)
	ast.Contains(err.Error(), "用户不存在")
}
func TestCertificateAuthFlow(t *testing.T) {
	base.Test()
	ast := assert.New(t)
	req := require.New(t)

	preIpData(t)
	defer closeIpdata()

	group := "auth-test-group"
	username := "auth-test-user"

	dns := []ValData{{Val: "8.8.8.8"}}
	pt := Policy{Name: "auth-test-policy", Status: 1, ClientDns: dns}
	err := SetPolicy(&pt)
	req.NoError(err)
	g := Group{Name: group, Status: 1, PolicyId: pt.Id}
	err = SetGroup(&g)
	req.NoError(err)

	u := User{Username: username, Groups: []string{group}, Status: 1}
	err = SetUser(&u)
	req.NoError(err)

	certData, err := GenerateClientCert(username, group, true, 3)
	req.NoError(err)
	req.NotNil(certData)

	cert, err := parseCertFromPEM(certData.Certificate)
	req.NoError(err)

	// 证书验证
	valid := ValidateClientCert(cert, "test-ID")
	ast.True(valid)

	certData.Status = CertStatusDisabled
	err = certData.UpdateStatus(CertStatusDisabled)
	ast.Nil(err)
	deviceId := "test-device-id"
	valid = ValidateClientCert(cert, deviceId)
	ast.False(valid)
}

func TestValidateClientCert(t *testing.T) {
	base.Test()
	ast := assert.New(t)
	req := require.New(t)

	preIpData(t)
	defer closeIpdata()

	err := GenerateClientCA()
	req.NoError(err, "初始化客户端 CA 失败")

	dns := []ValData{{Val: "8.8.8.8"}}
	pt := Policy{Name: "cert-gen-test-policy", Status: 1, ClientDns: dns}
	err = SetPolicy(&pt)
	req.NoError(err)
	group := "test-group"
	g := Group{Name: group, Status: 1, PolicyId: pt.Id}
	err = SetGroup(&g)
	req.NoError(err)

	username := "test-user"
	u := User{Username: username, Groups: []string{group}, Status: 1}
	err = SetUser(&u)
	req.NoError(err)

	certData, err := GenerateClientCert(username, group, true, 3)
	req.NoError(err)
	req.NotNil(certData)
	ast.Equal(username, certData.Username)
	ast.Equal(group, certData.Groupname)

	cert, err := parseCertFromPEM(certData.Certificate)
	req.NoError(err)
	ast.Equal(username, cert.Subject.CommonName)
	ast.Equal(group, cert.Subject.OrganizationalUnit[0])

	deviceId := "test-device-id"
	valid := ValidateClientCert(cert, deviceId)
	ast.True(valid)

	cert.Subject.CommonName = "nonexistent-user"
	valid = ValidateClientCert(cert, deviceId)
	ast.False(valid)

	cert.Subject.CommonName = username
	u.Status = 0
	err = SetUser(&u)
	ast.Nil(err)
	valid = ValidateClientCert(cert, deviceId)
	ast.False(valid)

	u.Status = 1
	err = SetUser(&u)
	ast.Nil(err)

	cert.Subject.OrganizationalUnit[0] = "wrong-group"
	valid = ValidateClientCert(cert, deviceId)
	ast.False(valid)

	cert.Subject.OrganizationalUnit[0] = group
	certData.Status = CertStatusDisabled
	err = certData.Save()
	ast.Nil(err)
	valid = ValidateClientCert(cert, deviceId)
	ast.False(valid)
}

func parseCertFromPEM(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

func TestClientCertConcurrency(t *testing.T) {
	base.Test()
	ast := assert.New(t)
	req := require.New(t)

	preIpData(t)
	defer closeIpdata()

	req.NoError(GenerateClientCA())

	group := "test-group"
	username := "test-user"
	pt := Policy{Name: "maxdev-test-policy", Status: 1, ClientDns: []ValData{{Val: "8.8.8.8"}}}
	err := SetPolicy(&pt)
	req.NoError(err)
	req.NoError(SetGroup(&Group{Name: group, Status: 1, PolicyId: pt.Id}))
	req.NoError(SetUser(&User{Username: username, Groups: []string{group}, Status: 1}))

	maxDevices := 3
	certData, err := GenerateClientCert(username, group, true, maxDevices)
	req.NoError(err)
	req.NotNil(certData)
	cert, err := parseCertFromPEM(certData.Certificate)
	req.NoError(err)

	concurrentCount := 10
	var wg sync.WaitGroup
	successCount := int32(0)
	failCount := int32(0)

	for i := range concurrentCount {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			deviceId := fmt.Sprintf("device-%d", id)
			if ValidateClientCert(cert, deviceId) {
				atomic.AddInt32(&successCount, 1)
			} else {
				atomic.AddInt32(&failCount, 1)
			}
		}(i)
	}
	wg.Wait()

	ast.Condition(func() bool {
		return successCount <= int32(maxDevices)
	}, fmt.Sprintf("并发绑定成功的数量不应超过 MaxDevices，期望<= %d，实际 %d", maxDevices, successCount))

	latestCert, _ := GetClientCert(username, group)
	ast.Condition(func() bool {
		return len(latestCert.DeviceId) <= maxDevices
	},
		fmt.Sprintf("数据库中绑定的设备数量不应超过最大值，期望<= %d，实际 %d", maxDevices, len(latestCert.DeviceId)))

	stopChan := make(chan bool)
	wg.Add(2)

	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopChan:
				return
			default:
				ValidateClientCert(cert, "device-0")
			}
		}
	}()

	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		currentCert, err := GetClientCert(username, group)
		if err != nil {
			t.Errorf("获取证书失败: %v", err)
			return
		}
		_ = currentCert.Disable()
		stopChan <- true
	}()

	wg.Wait()

	finalCert, _ := GetClientCert(username, group)
	ast.Equal(CertStatusDisabled, finalCert.GetStatus(), "证书应该已被禁用")
	ast.False(ValidateClientCert(cert, "device-0"), "证书禁用后，验证不应通过")
}
