package dbdata

// 按用户名批量查询昵称，返回 username -> nickname 的映射
func NicknameMap(usernames []string) map[string]string {
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
	if err := GetXdb().In("username", uniq).Cols("username", "nickname").Find(&users); err != nil {
		return m
	}
	for _, u := range users {
		if u.Username != "" {
			m[u.Username] = u.Nickname
		}
	}
	return m
}

// 从 nickMap 取昵称，缺失时回退为空串（调用方据空串决定仅显示 username）
func NicknameOf(nickMap map[string]string, username string) string {
	if nickMap == nil {
		return ""
	}
	return nickMap[username]
}
