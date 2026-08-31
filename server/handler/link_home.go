package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/wsczx/remlink/admin"
	"github.com/wsczx/remlink/base"
	"github.com/wsczx/remlink/dbdata"
)

func LinkHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Del("X-Aggregate-Auth")

	connection := strings.ToLower(r.Header.Get("Connection"))
	userAgent := strings.ToLower(r.UserAgent())
	if connection == "close" && (strings.Contains(userAgent, "anyconnect") || strings.Contains(userAgent, "openconnect")) {
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	index := &dbdata.SettingOther{}
	if err := dbdata.SettingGet(index); err != nil {
		return
	}

	// Homecode == 0：开启自定义首页（与门户共存）
	if index.Homecode == 0 && index.Homeindex != "" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, index.Homeindex)
		return
	}

	if base.GetCfg().EnableUserPortal {
		http.Redirect(w, r, "/ui/#/portal", http.StatusFound)
		return
	}

	if index.Homecode != http.StatusOK {
		w.WriteHeader(index.Homecode)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, index.Homeindex)
}

func LinkOtpQr(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")

	_ = r.ParseForm()
	idS := r.FormValue("id")
	jwtToken := r.FormValue("jwt")
	data, err := admin.GetJwtData(jwtToken)
	if err != nil || idS != fmt.Sprint(data["id"]) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	admin.UserOtpQr(w, r)
}
