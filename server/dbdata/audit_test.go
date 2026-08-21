package dbdata

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSearchAudit(t *testing.T) {
	ast := assert.New(t)

	preIpData(t)
	defer closeIpdata()

	currDateVal := "2022-07-24 00:00:00"
	CreatedAt, _ := time.ParseInLocation("2006-01-02 15:04:05", currDateVal, time.Local)

	user := User{Username: "Test", Nickname: "测试用户"}
	ast.Nil(Add(&user))

	dataTest := AccessAudit{
		Username:    user.Username,
		Protocol:    6,
		Src:         "10.10.1.5",
		Dst:         "172.217.160.68",
		DstPort:     80,
		AccessProto: 4,
		Info:        "www.google.com",
		CreatedAt:   CreatedAt,
	}
	err := Add(dataTest)
	ast.Nil(err)

	var datas []AccessAudit
	values := url.Values{}
	values.Set("search[username]", user.Nickname)
	values.Set("search[src]", dataTest.Src)
	values.Set("search[dst]", dataTest.Dst)
	values.Set("search[dst_port]", fmt.Sprintf("%d", dataTest.DstPort))
	values.Set("search[access_proto]", fmt.Sprintf("%d", dataTest.AccessProto))
	values.Set("search[info]", dataTest.Info)
	values.Set("search[date][0]", currDateVal)
	values.Set("search[date][1]", currDateVal)

	session := GetAuditSession(values)
	count, _ := FindAndCount(session, &datas, PageSize, 0)
	ast.Equal(count, int64(1))
	ast.Equal(datas[0].Username, dataTest.Username)
	ast.Equal(datas[0].Dst, dataTest.Dst)
}
