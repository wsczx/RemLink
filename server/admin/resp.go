package admin

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wsczx/remlink/base"
)

type Resp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

func respHttp(w http.ResponseWriter, respCode int, data any, errS ...any) {
	resp := Resp{
		Code: respCode,
		Msg:  "success",
		Data: data,
	}

	if respCode != 0 {
		resp.Msg = ""
		if v, ok := RespMap[respCode]; ok {
			resp.Msg += v
		}

		if len(errS) > 0 {
			resp.Msg += fmt.Sprint(errS...)
		}
	}

	b, err := json.Marshal(resp)
	if err != nil {
		base.Error(err, resp)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(b)
	if err != nil {
		base.Error(err)
	}
}

func RespSucess(w http.ResponseWriter, data any) {
	respHttp(w, 0, data, "")
}

func RespError(w http.ResponseWriter, respCode int, errS ...any) {
	respHttp(w, respCode, nil, errS...)
}
