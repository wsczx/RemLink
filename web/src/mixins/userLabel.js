// 统一用户名展示：用户名后附昵称，格式与用户列表一致（用户名(昵称)）。
// 昵称为空时仅展示用户名。
export default {
  methods: {
    userLabel(username, nickname) {
      if (!username) return ''
      return nickname ? username + '(' + nickname + ')' : username
    },
  },
}
