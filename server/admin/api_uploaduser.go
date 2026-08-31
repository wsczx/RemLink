package admin

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/wsczx/remlink/dbdata"
	"github.com/wsczx/remlink/pkg/utils"
	"github.com/xuri/excelize/v2"
)

func UserUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(8 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		RespError(w, RespInternalErr, "文件解析失败:", err)
		return
	}
	defer file.Close()

	// 校验文件扩展名
	fn := strings.ToLower(header.Filename)
	if !strings.HasSuffix(fn, ".xlsx") && !strings.HasSuffix(fn, ".xls") {
		RespError(w, RespInternalErr, "文件解析失败:仅支持xlsx或xls文件")
		return
	}

	// 上传到临时目录，不直接写入 FilesPath
	fileName := path.Join(os.TempDir(), utils.RandomRunes(10))
	newFile, err := os.Create(fileName)
	if err != nil {
		RespError(w, RespInternalErr, "创建文件失败:", err)
		return
	}
	defer newFile.Close()

	if _, err := io.Copy(newFile, file); err != nil {
		RespError(w, RespInternalErr, "写入文件失败:", err)
		os.Remove(fileName)
		return
	}
	if err = UploadUser(newFile.Name()); err != nil {
		RespError(w, RespInternalErr, err)
		os.Remove(fileName)
		return
	}
	os.Remove(fileName)
	dbdata.AdminLog("用户管理", header.Filename, "批量导入了用户", r.RemoteAddr)
	RespSucess(w, "批量添加成功")
}

func UploadUser(file string) error {
	f, err := excelize.OpenFile(file)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			return
		}
	}()
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		return err
	}
	if len(rows) == 0 || len(rows[0]) < 12 {
		return fmt.Errorf("批量添加失败，表格为空或列数不足")
	}
	if rows[0][0] != "id" || rows[0][1] != "username" || rows[0][2] != "nickname" || rows[0][3] != "email" || rows[0][4] != "pin_code" || rows[0][5] != "limittime" || rows[0][6] != "otp_secret" || rows[0][7] != "disable_otp" || rows[0][8] != "groups" || rows[0][9] != "status" || rows[0][10] != "send_email" || rows[0][11] != "change_pwd" {
		return fmt.Errorf("批量添加失败，表格格式不正确")
	}
	groupSet := make(map[string]struct{})
	for _, v := range dbdata.GetGroupNames() {
		groupSet[v] = struct{}{}
	}
	for index, row := range rows {
		if index == 0 {
			continue
		}
		id, _ := strconv.Atoi(row[0])
		if len(row[4]) < 6 {
			row[4] = utils.RandomRunes(8)
		}
		limittime, _ := time.ParseInLocation("2006-01-02 15:04:05", row[5], time.Local)
		disableOtp, _ := strconv.ParseBool(row[7])
		var group []string
		if row[8] == "" {
			return fmt.Errorf("第%d行数据错误，用户组不允许为空", index)
		}
		for v := range strings.SplitSeq(row[8], ",") {
			if _, ok := groupSet[v]; ok {
				group = append(group, v)
			} else {
				return fmt.Errorf("用户组【%s】不存在,请检查第%d行数据", v, index)
			}
		}
		statusVal, _ := strconv.ParseInt(row[9], 10, 8)
		status := int8(statusVal)
		sendmail, _ := strconv.ParseBool(row[10])
		changePwd, _ := strconv.ParseBool(row[11])
		// createdAt, _ := time.ParseInLocation("2006-01-02 15:04:05", row[12], time.Local)
		// updatedAt, _ := time.ParseInLocation("2006-01-02 15:04:05", row[13], time.Local)
		user := &dbdata.User{
			Id:         id,
			Type:       "local",
			Username:   row[1],
			Nickname:   row[2],
			Email:      row[3],
			PinCode:    row[4],
			LimitTime:  &limittime,
			OtpSecret:  row[6],
			DisableOtp: disableOtp,
			Groups:     group,
			Status:     status,
			SendEmail:  sendmail,
			ForcePwd:   changePwd,
			// CreatedAt:  createdAt,
			// UpdatedAt:  updatedAt,
		}
		if err := dbdata.AddBatch(user); err != nil {
			return fmt.Errorf("请检查第%d行数据是否导入有重复用户", index)
		}
		user.PinCode = row[4]
		if user.SendEmail {
			if err := userAccountMail(user); err != nil {
				return err
			}
		}
	}
	return nil
}

// 动态生成用户批量导入模版文件并下载
func UserUploadTemplate(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	// 表头与 UploadUser() 校验字段严格一致
	headers := []string{"id", "username", "nickname", "email", "pin_code", "limittime", "otp_secret", "disable_otp", "groups", "status", "send_email", "change_pwd"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue("Sheet1", cell, h)
	}

	// 示例行
	example := []string{"0", "zhangsan", "张三", "zhangsan@example.com", "123456", "2030-12-31 23:59:59", "", "false", "默认组", "1", "true", "true"}
	for i, v := range example {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue("Sheet1", cell, v)
	}

	buf := new(bytes.Buffer)
	if err := f.Write(buf); err != nil {
		RespError(w, RespInternalErr, "生成模版文件失败:", err)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="批量添加用户模版.xlsx"`)
	w.Write(buf.Bytes())
}
