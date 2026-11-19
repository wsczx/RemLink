package dbdata

// 返回「账号(昵称)」格式。nickname 为空时仅返回 username
func (st *UserStore) Label(username, nickname string) string {
	if username == "" {
		return ""
	}
	if nickname == "" {
		return username
	}
	return username + "(" + nickname + ")"
}

// 按用户名批量查询昵称，返回 username -> nickname 的映射
func (st *UserStore) NicknameMap(usernames []string) map[string]string {
	m := make(map[string]string)
	if len(usernames) == 0 {
		return m
	}
	seen := make(map[string]struct{})
	uniq := make([]string, 0, len(usernames))
	for _, u := range usernames {
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		uniq = append(uniq, u)
	}
	if len(uniq) == 0 {
		return m
	}
	var users []User
	if err := GetDB().Engine().In("username", uniq).Cols("username", "nickname").Find(&users); err != nil {
		return m
	}
	for _, u := range users {
		if u.Username != "" {
			m[u.Username] = u.Nickname
		}
	}
	return m
}
