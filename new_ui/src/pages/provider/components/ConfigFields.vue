<script setup lang="ts">
// 各类型认证源的连接配置：按主题分成若干面板，每个字段一行 SettingRow
import { computed } from 'vue'
import { NInput, NRadioButton, NRadioGroup, NSwitch } from 'naive-ui'
import Warning from '~icons/fluent/warning-20-regular'
import RlPanel from '@/components/RlPanel.vue'
import SettingRow from '@/components/SettingRow.vue'
import { typeMeta } from './types'

const props = defineProps<{ type: string }>()
const cfg = defineModel<Record<string, any>>('cfg', { required: true })

const meta = computed(() => typeMeta(props.type))
const platform = computed(() => ({ wxwork: '企业微信', feishu: '飞书', dingtalk: '钉钉' } as Record<string, string>)[props.type] || '')

// LDAP 加密方式：StartTLS 与 LDAPS 互斥，合并成一个单选
const ldapEnc = computed({
  get: () => (cfg.value.ldaps ? 'ldaps' : cfg.value.tls ? 'starttls' : 'none'),
  set: (v: string) => {
    cfg.value.tls = v === 'starttls'
    cfg.value.ldaps = v === 'ldaps'
  },
})

const deptTip = computed(() => props.type === 'dingtalk'
  ? '部门 ID 在钉钉管理后台查看，逗号分隔，留空不限制。请给钉钉应用授予「通讯录只读」权限，否则部门限制与拒绝名单无法按工号精确匹配（将自动回退用 unionid 登录）。'
  : `部门 ID 在${platform.value}管理后台查看，逗号分隔，留空不限制。`)
const blockTip = computed(() => props.type === 'wxwork'
  ? '企业微信用户 ID（userid），逗号分隔。与允许的部门叠加生效：需先属于允许部门，且不在此列表中。'
  : '列表中的用户即使通过部门限制也会被拒绝登录，逗号分隔。')
</script>

<template>
  <div class="grid gap-16px">
    <!-- LDAP -->
    <template v-if="type === 'ldap'">
      <RlPanel title="目录服务器" subtitle="RemLink 用一个只读管理员账号连接目录，查找并校验用户" :padded="false">
        <SettingRow label="服务器地址" description="主机:端口，AD 明文通常为 389，LDAPS 通常为 636" required>
          <NInput v-model:value="cfg.addr" placeholder="192.168.1.10:389" />
        </SettingRow>
        <SettingRow label="加密方式">
          <template #description>
            StartTLS 在明文连接上升级加密（OpenLDAP / 旧版 AD）；Windows Server 2025 的 AD 已移除 StartTLS，必须使用 LDAPS。
          </template>
          <NRadioGroup v-model:value="ldapEnc">
            <NRadioButton value="none">不加密</NRadioButton>
            <NRadioButton value="starttls">StartTLS</NRadioButton>
            <NRadioButton value="ldaps">LDAPS</NRadioButton>
          </NRadioGroup>
        </SettingRow>
        <SettingRow label="校验服务端证书" description="防中间人攻击。自签证书需先在系统中部署 CA；关闭时兼容自签证书">
          <NSwitch v-model:value="cfg.tls_verify" />
        </SettingRow>
        <SettingRow label="管理员 DN" description="用于查询目录的绑定账号" required>
          <NInput v-model:value="cfg.bind_name" placeholder="CN=admin,DC=abc,DC=com" />
        </SettingRow>
        <SettingRow label="管理员密码" required>
          <NInput v-model:value="cfg.bind_pwd" type="password" show-password-on="click" placeholder="管理员密码" />
        </SettingRow>
      </RlPanel>

      <RlPanel title="用户查找" subtitle="决定在目录的哪个位置、用哪个属性匹配登录名" :padded="false">
        <SettingRow label="Base DN" description="从该节点开始向下搜索用户" required>
          <NInput v-model:value="cfg.base_dn" placeholder="DC=abc,DC=com" />
        </SettingRow>
        <SettingRow label="用户对象类" description="AD 一般为 person 或 user，OpenLDAP 常见 inetOrgPerson" required>
          <NInput v-model:value="cfg.object_class" placeholder="person" />
        </SettingRow>
        <SettingRow label="登录名属性" description="用户登录时输入的账号对应的属性" required>
          <NInput v-model:value="cfg.search_attr" placeholder="sAMAccountName / uid" />
        </SettingRow>
        <SettingRow label="仅允许指定组" description="选填。填写组 DN 后，只有该组成员可以登录">
          <NInput v-model:value="cfg.member_of" placeholder="CN=VPN,OU=Groups,DC=abc,DC=com" />
        </SettingRow>
      </RlPanel>

      <RlPanel title="OTP 与用户同步" :padded="false">
        <SettingRow label="启用 OTP" description="同步用户时自动生成 OTP 密钥">
          <NSwitch v-model:value="cfg.enable_otp" />
        </SettingRow>
        <SettingRow label="自动同步用户" description="定时从该目录同步用户到本地">
          <NSwitch v-model:value="cfg.sync_users" />
        </SettingRow>
        <SettingRow label="只同步正常状态的用户" description="跳过目录中已禁用的账号">
          <NSwitch v-model:value="cfg.sync_user_status" />
        </SettingRow>
      </RlPanel>
    </template>

    <!-- RADIUS -->
    <RlPanel v-else-if="type === 'radius'" title="RADIUS 服务器" subtitle="用户输入的账号密码将转发给该服务器校验" :padded="false">
      <SettingRow label="服务器地址" description="主机:端口，认证端口通常为 1812" required>
        <NInput v-model:value="cfg.addr" placeholder="192.168.1.10:1812" />
      </SettingRow>
      <SettingRow label="共享密钥" description="与 RADIUS 服务器上为本客户端配置的密钥一致，长度 8～200" required>
        <NInput v-model:value="cfg.secret" type="password" show-password-on="click" placeholder="RADIUS 共享密钥" />
      </SettingRow>
      <SettingRow label="NAS IP" description="选填。RADIUS 按 NAS IP 识别客户端时填写">
        <NInput v-model:value="cfg.nasip" placeholder="可选" />
      </SettingRow>
    </RlPanel>

    <!-- 企微 / 飞书 / 钉钉 -->
    <template v-else>
      <RlPanel :title="`${meta?.full || ''}应用`" :subtitle="`在${platform}开放平台创建自建应用后，填入应用凭据`" :padded="false">
        <template #extra>
          <span class="ax-badge gap-4px text-warning"><Warning />仅支持 PC 端 AnyConnect 客户端</span>
        </template>
        <template v-if="type === 'wxwork'">
          <SettingRow label="企业 ID" description="企业微信管理后台「我的企业」中查看" required>
            <NInput v-model:value="cfg.corp_id" placeholder="ww7164hdf7kc84073" />
          </SettingRow>
          <SettingRow label="应用 AgentId" required>
            <NInput v-model:value="cfg.agent_id" placeholder="1000001" />
          </SettingRow>
          <SettingRow label="应用 Secret" required>
            <NInput v-model:value="cfg.secret" type="password" show-password-on="click" placeholder="应用密钥" />
          </SettingRow>
        </template>
        <template v-if="type === 'feishu'">
          <SettingRow label="App ID" required>
            <NInput v-model:value="cfg.app_id" placeholder="cli_xxx" />
          </SettingRow>
          <SettingRow label="App Secret" required>
            <NInput v-model:value="cfg.app_secret" type="password" show-password-on="click" placeholder="应用密钥" />
          </SettingRow>
        </template>
        <template v-if="type === 'dingtalk'">
          <SettingRow label="AppKey" required>
            <NInput v-model:value="cfg.client_id" placeholder="钉钉应用 AppKey" />
          </SettingRow>
          <SettingRow label="AppSecret" required>
            <NInput v-model:value="cfg.client_secret" type="password" show-password-on="click" placeholder="应用密钥" />
          </SettingRow>
        </template>
        <SettingRow label="扫码页面打开方式" description="客户端用内置浏览器弹窗，或调用系统默认浏览器">
          <NRadioGroup v-model:value="cfg.use_default_browser">
            <NRadioButton :value="false">内置浏览器</NRadioButton>
            <NRadioButton :value="true">系统浏览器</NRadioButton>
          </NRadioGroup>
        </SettingRow>
      </RlPanel>

      <RlPanel title="谁可以登录" subtitle="留空表示所有成员都可扫码登录" :padded="false">
        <SettingRow label="允许的部门" :description="deptTip">
          <NInput v-model:value="cfg.allowed_departments" placeholder="部门 ID，逗号分隔" />
        </SettingRow>
        <SettingRow label="拒绝的用户" :description="blockTip">
          <NInput v-model:value="cfg.blocked_userids" placeholder="用户 ID，逗号分隔" />
        </SettingRow>
      </RlPanel>

      <RlPanel title="通讯录同步" :padded="false">
        <SettingRow label="自动同步用户" :description="`定时从${platform}通讯录同步用户到本地`">
          <NSwitch v-model:value="cfg.sync_users" />
        </SettingRow>
      </RlPanel>

      <RlPanel v-if="type === 'wxwork'" title="可信域名验证" subtitle="企业微信校验可信域名时，要求在域名根路径放置的验证文件" :padded="false">
        <SettingRow label="验证文件名">
          <NInput v-model:value="cfg.verify_file_name" placeholder="WW_verify_abc.txt" />
        </SettingRow>
        <SettingRow label="验证文件内容">
          <NInput v-model:value="cfg.verify_file_content" placeholder="企业微信后台提供的校验内容" />
        </SettingRow>
      </RlPanel>
    </template>
  </div>
</template>
