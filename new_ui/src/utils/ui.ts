import type { DialogApi, LoadingBarApi, MessageApi, NotificationApi } from 'naive-ui'

export const ui = {} as {
  message: MessageApi
  dialog: DialogApi
  loadingBar: LoadingBarApi
  notification: NotificationApi
}

// 删除/危险操作确认，返回是否确认
export function confirm(content: string, title = '确认操作', danger = true): Promise<boolean> {
  return new Promise(resolve => {
    ui.dialog.create({
      type: danger ? 'error' : 'info',
      showIcon: false,
      title,
      content,
      positiveText: danger ? '确认' : '确定',
      negativeText: '取消',
      // 按钮跟随设计：危险操作红色，其余黑色主按钮
      positiveButtonProps: { type: danger ? 'error' : 'primary' },
      negativeButtonProps: { type: 'default' },
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false),
    })
  })
}
