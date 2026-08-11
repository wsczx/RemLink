<template>
  <div class="provider-page">
    <!-- 统计卡片 -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon stat-icon-total">
          <i class="el-icon-s-platform"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statTotal }}</div>
          <div class="stat-label">认证源总数</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon stat-icon-active">
          <i class="el-icon-circle-check"></i>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ statActive }}</div>
          <div class="stat-label">已启用</div>
        </div>
      </div>
    </div>

    <!-- 认证源表格 -->
    <el-card class="table-card" shadow="never" v-loading="loading">
      <div slot="header" class="card-header">
        <span class="card-title"><i class="el-icon-s-platform"></i> 认证源列表</span>
        <div class="card-actions">
          <el-button size="small" type="primary" icon="el-icon-plus" @click="handleEdit('')">
            添加认证源
          </el-button>
        </div>
      </div>

      <div class="provider-table-wrap">
        <el-table :data="tableData" stripe highlight-current-row style="width:100%" border
          :header-cell-style="{ background: 'var(--bg-header)', color: 'var(--text-primary)', fontWeight: '600', fontSize: '13px' }">
          <el-table-column prop="id" label="ID" width="65" align="center" sortable></el-table-column>
          <el-table-column prop="name" label="名称" min-width="150" sortable>
            <template slot-scope="scope">
              <span class="provider-name">{{ scope.row.name }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="type" label="类型" width="100" align="center" sortable>
            <template slot-scope="scope">
              <el-tag :type="getTypeTagType(scope.row.type)" size="small" effect="plain">
                {{ getTypeLabel(scope.row.type) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="80" align="center" sortable>
            <template slot-scope="scope">
              <span :class="scope.row.status === 1 ? 'status-dot-online' : 'status-dot-offline'"
                class="status-dot"></span>
              <span class="status-text" :class="scope.row.status === 1 ? 'text-success' : 'text-danger'">
                {{ scope.row.status === 1 ? '启用' : '停用' }}
              </span>
            </template>
          </el-table-column>
          <el-table-column prop="updated_at" label="更新时间" :formatter="tableDateFormat" width="165"
            sortable></el-table-column>
          <el-table-column label="操作" width="120" class-name="col-ops" min-width="120" align="center">
            <template slot-scope="scope">
              <el-dropdown trigger="click" @command="(cmd) => handleRowCmd(scope.row, cmd)">
                <el-button size="mini" class="action-more-btn">
                  操作<i class="el-icon-arrow-down el-icon--right"></i>
                </el-button>
                <el-dropdown-menu slot="dropdown">
                  <el-dropdown-item command="edit" icon="el-icon-edit">编辑认证源</el-dropdown-item>
                  <el-dropdown-item command="test" icon="el-icon-connection"
                    v-if="scope.row.status === 1 && isTestable(scope.row.type)">测试登录</el-dropdown-item>
                  <el-dropdown-item command="sync" icon="el-icon-refresh"
                    v-if="isSyncable(scope.row.type)">同步用户</el-dropdown-item>
                  <el-dropdown-item command="delete" icon="el-icon-delete" divided
                    class="dropdown-danger">删除认证源</el-dropdown-item>
                </el-dropdown-menu>
              </el-dropdown>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <div class="pagination-wrap">
        <el-pagination background layout="prev,pager,next" :pager-count="9" @current-change="pageChange"
          :current-page="page" :total="count" />
      </div>
    </el-card>

    <!-- 编辑弹窗 -->
    <el-dialog :close-on-click-modal="false" :title="ruleForm.id ? '编辑认证源' : '添加认证源'" :visible.sync="editDialog"
      width="800px" top="3vh" @close="closeDialog" class="provider-edit-dialog">
      <el-form :model="ruleForm" :rules="rules" ref="ruleForm" label-width="90px" size="small">
        <div class="edit-basic-row">
          <el-form-item label="名称" prop="name" class="form-item-compact">
            <el-input v-model="ruleForm.name" :disabled="ruleForm.id > 0" placeholder="唯一名称，如 北京LDAP"></el-input>
          </el-form-item>
          <el-form-item label="类型" label-width="60px" class="form-item-compact">
            <el-select v-model="ruleForm.type" :disabled="ruleForm.id > 0" placeholder="选择认证类型" style="width:100%"
              @change="onTypeChange">
              <el-option label="LDAP" value="ldap"></el-option>
              <el-option label="RADIUS" value="radius"></el-option>
              <el-option label="企微" value="wxwork"></el-option>
              <el-option label="飞书" value="feishu"></el-option>
              <el-option label="钉钉" value="dingtalk"></el-option>
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="状态" class="form-item-status">
          <el-radio-group v-model="ruleForm.status" size="small">
            <el-radio-button :label="1"><i class="el-icon-circle-check"></i> 启用</el-radio-button>
            <el-radio-button :label="0"><i class="el-icon-remove"></i> 停用</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <el-divider></el-divider>

        <!-- 配置面板 -->
        <div class="config-panel">
          <!-- LDAP -->
          <template v-if="ruleForm.type === 'ldap'">
            <div class="section-title"><i class="el-icon-connection"></i> LDAP 连接参数</div>
            <el-form-item label="服务器地址">
              <el-input v-model="configForm.addr" placeholder="如 192.168.1.10:389"></el-input>
            </el-form-item>
            <el-row type="flex" justify="start" :gutter="24" class="switch-row">
              <el-col :span="5">
                <el-form-item class="inline-switch-item" label-width="64px">
                  <template slot="label">
                    TLS
                    <el-tooltip
                      content="在明文 LDAP 连接上启动 StartTLS 加密升级（如 OpenLDAP / 旧版 AD 的 389 端口）。Windows Server 2025 的 AD 已不支持此方式，需改用 LDAPS"
                      placement="top">
                      <i class="el-icon-question help-icon"></i>
                    </el-tooltip>
                  </template>
                  <el-switch v-model="configForm.tls" @change="onTlsChange"></el-switch>
                </el-form-item>
              </el-col>
              <el-col :span="5">
                <el-form-item class="inline-switch-item" label-width="74px">
                  <template slot="label">
                    LDAPS
                    <el-tooltip content="直连 TLS（如 AD 的 636 端口）。Windows Server 2025 的 AD 已移除 StartTLS，必须开启此项；端口按实际填写"
                      placement="top">
                      <i class="el-icon-question help-icon"></i>
                    </el-tooltip>
                  </template>
                  <el-switch v-model="configForm.ldaps" @change="onLdapsChange"></el-switch>
                </el-form-item>
              </el-col>
              <el-col :span="6">
                <el-form-item class="inline-switch-item" label-width="92px">
                  <template slot="label">
                    校验证书
                    <el-tooltip content="开启后校验服务端证书（防中间人）；自签证书须先在系统部署 CA，默认关闭=兼容自签证书" placement="top">
                      <i class="el-icon-question help-icon"></i>
                    </el-tooltip>
                  </template>
                  <el-switch v-model="configForm.tls_verify"></el-switch>
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="16">
              <el-col :span="14">
                <el-form-item label="管理员DN">
                  <el-input v-model="configForm.bind_name" placeholder="CN=admin,DC=abc,DC=com"></el-input>
                </el-form-item>
              </el-col>
              <el-col :span="10">
                <el-form-item label="密码" label-width="50px">
                  <el-input type="password" v-model="configForm.bind_pwd" placeholder="管理员密码" show-password></el-input>
                </el-form-item>
              </el-col>
            </el-row>
            <div class="section-title"><i class="el-icon-search"></i> 用户搜索参数</div>
            <el-form-item label="Base DN">
              <el-input v-model="configForm.base_dn" placeholder="DC=abc,DC=com"></el-input>
            </el-form-item>
            <el-row :gutter="16">
              <el-col :span="12">
                <el-form-item label="用户对象类">
                  <el-input v-model="configForm.object_class" placeholder="person / user"></el-input>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="唯一ID">
                  <el-input v-model="configForm.search_attr" placeholder="sAMAccountName / uid"></el-input>
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="16">
              <el-col :span="12">
                <el-form-item>
                  <template slot="label">
                    受限用户组
                    <el-tooltip content="选填, 只允许指定组登入" placement="top">
                      <i class="el-icon-question help-icon"></i>
                    </el-tooltip>
                  </template>
                  <el-input v-model="configForm.member_of" placeholder="选填"></el-input>
                </el-form-item>
              </el-col>
            </el-row>
            <div class="section-title"><i class="el-icon-mobile-phone"></i> OTP 动态验证</div>
            <el-form-item>
              <template slot="label">
                启用OTP
                <el-tooltip content="开启后同步用户时自动生成OTP秘钥" placement="top">
                  <i class="el-icon-question help-icon"></i>
                </el-tooltip>
              </template>
              <el-switch v-model="configForm.enable_otp"></el-switch>
            </el-form-item>
            <div class="section-title"><i class="el-icon-refresh"></i> 用户同步</div>
            <el-row :gutter="16">
              <el-col :span="12">
                <el-form-item>
                  <template slot="label">
                    自动同步用户
                    <el-tooltip content="开启后定时从本 LDAP 认证源同步用户到本地" placement="top">
                      <i class="el-icon-question help-icon"></i>
                    </el-tooltip>
                  </template>
                  <el-switch v-model="configForm.sync_users"></el-switch>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item>
                  <template slot="label">
                    状态过滤
                    <el-tooltip content="开启后仅同步状态正常的用户" placement="top">
                      <i class="el-icon-question help-icon"></i>
                    </el-tooltip>
                  </template>
                  <el-switch v-model="configForm.sync_user_status"></el-switch>
                </el-form-item>
              </el-col>
            </el-row>
          </template>

          <!-- RADIUS -->
          <template v-if="ruleForm.type === 'radius'">
            <div class="section-title"><i class="el-icon-connection"></i> RADIUS 连接参数</div>
            <el-row :gutter="16">
              <el-col :span="15">
                <el-form-item label="服务器地址">
                  <el-input v-model="configForm.addr" placeholder="192.168.1.10:1812"></el-input>
                </el-form-item>
              </el-col>
              <el-col :span="9">
                <el-form-item label="NAS IP" label-width="60px">
                  <el-input v-model="configForm.nasip" placeholder="可选"></el-input>
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item label="共享密钥">
              <el-input type="password" v-model="configForm.secret" placeholder="RADIUS 共享密钥" show-password></el-input>
            </el-form-item>
          </template>

          <!-- 飞书 -->
          <template v-if="ruleForm.type === 'feishu'">
            <div class="section-title">
              <i class="el-icon-mobile-phone"></i> 飞书应用参数
              <span class="section-warn"><i class="el-icon-warning"></i> 仅支持 PC 端 AnyConnect 客户端</span>
            </div>
            <div class="form-tip form-tip-info">
              飞书开放平台后台「安全设置 - 重定向 URL」需填写：<code>{{ feishuCallback }}</code>
            </div>
            <div class="form-tip form-tip-warn">
              若需使用 Web 认证（浏览器弹窗扫码）登录，还需额外在飞书「安全设置 - 重定向 URL」中补加：<code>{{ webAuthCallback }}</code>
              <br>飞书按完整路径精确匹配回调，门户/原生与 Web 认证路径不同，两条都需配置。
            </div>
            <el-row :gutter="16">
              <el-col :span="12">
                <el-form-item label="App ID">
                  <el-input v-model="configForm.app_id" placeholder="cli_xxx"></el-input>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="App Secret">
                  <el-input type="password" v-model="configForm.app_secret" placeholder="应用密钥" show-password></el-input>
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="16">
              <el-col :span="16">
                <el-form-item>
                  <template slot="label">
                    允许的部门
                    <el-tooltip content="部门ID在飞书管理后台查看，逗号分隔，留空不限制" placement="top">
                      <i class="el-icon-question help-icon"></i>
                    </el-tooltip>
                  </template>
                  <el-input v-model="configForm.allowed_departments" placeholder="逗号分隔，留空不限制"></el-input>
                </el-form-item>
              </el-col>
              <el-col :span="8">
                <el-form-item label="浏览器">
                  <el-radio-group v-model="configForm.use_default_browser" size="mini">
                    <el-radio-button :label="false">内置</el-radio-button>
                    <el-radio-button :label="true">系统</el-radio-button>
                  </el-radio-group>
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item prop="blocked_userids">
              <template slot="label">拒绝的用户
                <el-tooltip content="填写后，列表中的用户即使通过部门限制也会被拒绝登录" placement="top">
                  <i class="el-icon-question"></i>
                </el-tooltip>
              </template>
              <el-input v-model="configForm.blocked_userids" placeholder="逗号分隔，留空不限制"></el-input>
            </el-form-item>
            <div class="section-title"><i class="el-icon-refresh"></i> 用户同步</div>
            <el-form-item>
              <template slot="label">
                自动同步用户
                <el-tooltip content="开启后定时从本飞书认证源同步用户到本地" placement="top">
                  <i class="el-icon-question help-icon"></i>
                </el-tooltip>
              </template>
              <el-switch v-model="configForm.sync_users"></el-switch>
            </el-form-item>
          </template>

          <!-- 钉钉 -->
          <template v-if="ruleForm.type === 'dingtalk'">
            <div class="section-title">
              <i class="el-icon-mobile-phone"></i> 钉钉应用参数
              <span class="section-warn"><i class="el-icon-warning"></i> 仅支持 PC 端 AnyConnect 客户端</span>
            </div>
            <div class="form-tip form-tip-info">
              钉钉开放平台后台「登录与分享 - 回调地址」需填写：<code>{{ dingtalkCallback }}</code>
            </div>
            <el-row :gutter="16">
              <el-col :span="12">
                <el-form-item label="AppKey">
                  <el-input v-model="configForm.client_id" placeholder="钉钉应用 AppKey"></el-input>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="AppSecret">
                  <el-input type="password" v-model="configForm.client_secret" placeholder="应用密钥"
                    show-password></el-input>
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="16">
              <el-col :span="16">
                <el-form-item>
                  <template slot="label">
                    允许的部门
                    <el-tooltip
                      content="部门ID在钉钉管理后台查看，逗号分隔，留空不限制。注意：请给钉钉应用授予「通讯录只读」权限，否则部门限制与拒绝名单无法按工号精确匹配（将自动回退用 unionid 登录）"
                      placement="top">
                      <i class="el-icon-question help-icon"></i>
                    </el-tooltip>
                  </template>
                  <el-input v-model="configForm.allowed_departments" placeholder="逗号分隔，留空不限制"></el-input>
                </el-form-item>
              </el-col>
              <el-col :span="8">
                <el-form-item label="浏览器">
                  <el-radio-group v-model="configForm.use_default_browser" size="mini">
                    <el-radio-button :label="false">内置</el-radio-button>
                    <el-radio-button :label="true">系统</el-radio-button>
                  </el-radio-group>
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item prop="blocked_userids">
              <template slot="label">拒绝的用户
                <el-tooltip content="填写后，列表中的用户即使通过部门限制也会被拒绝登录" placement="top">
                  <i class="el-icon-question"></i>
                </el-tooltip>
              </template>
              <el-input v-model="configForm.blocked_userids" placeholder="逗号分隔，留空不限制"></el-input>
            </el-form-item>
            <div class="section-title"><i class="el-icon-refresh"></i> 用户同步</div>
            <el-form-item>
              <template slot="label">
                自动同步用户
                <el-tooltip content="开启后定时从本钉钉认证源同步用户到本地" placement="top">
                  <i class="el-icon-question help-icon"></i>
                </el-tooltip>
              </template>
              <el-switch v-model="configForm.sync_users"></el-switch>
            </el-form-item>
          </template>

          <!-- 企业微信 -->
          <template v-if="ruleForm.type === 'wxwork'">
            <div class="section-title">
              <i class="el-icon-mobile-phone"></i> 企业微信应用参数
              <span class="section-warn"><i class="el-icon-warning"></i> 仅支持 PC 端 AnyConnect 客户端</span>
            </div>
            <div class="form-tip form-tip-info">
              企业微信后台「企业微信登录 - 可信域名/回调」需填写：<code>{{ wxworkCallback }}</code>
            </div>
            <el-row :gutter="16">
              <el-col :span="12">
                <el-form-item label="企业ID">
                  <el-input v-model="configForm.corp_id" placeholder="ww7164hdf7kc84073"></el-input>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="应用ID">
                  <el-input v-model="configForm.agent_id" placeholder="1000001"></el-input>
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item label="应用Secret">
              <el-input type="password" v-model="configForm.secret" placeholder="应用密钥" show-password></el-input>
            </el-form-item>
            <el-row :gutter="16">
              <el-col :span="16">
                <el-form-item>
                  <template slot="label">
                    允许的部门
                    <el-tooltip content="部门ID在企业微信管理后台查看，逗号分隔，留空不限制" placement="top">
                      <i class="el-icon-question help-icon"></i>
                    </el-tooltip>
                  </template>
                  <el-input v-model="configForm.allowed_departments" placeholder="逗号分隔，留空不限制"></el-input>
                </el-form-item>
              </el-col>
              <el-col :span="8">
                <el-form-item label="浏览器">
                  <el-radio-group v-model="configForm.use_default_browser" size="mini">
                    <el-radio-button :label="false">内置</el-radio-button>
                    <el-radio-button :label="true">系统</el-radio-button>
                  </el-radio-group>
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item>
              <template slot="label">
                拒绝的用户
                <el-tooltip content="拒绝登录的企业微信用户ID（userid），逗号分隔，留空不限制。与允许的部门叠加生效：需先属于允许部门、且不在拒绝列表中" placement="top">
                  <i class="el-icon-question help-icon"></i>
                </el-tooltip>
              </template>
              <el-input v-model="configForm.blocked_userids" placeholder="逗号分隔，留空不限制"></el-input>
            </el-form-item>
            <div class="section-title"><i class="el-icon-refresh"></i> 用户同步</div>
            <el-form-item>
              <template slot="label">
                自动同步用户
                <el-tooltip content="开启后定时从本企微认证源同步用户到本地" placement="top">
                  <i class="el-icon-question help-icon"></i>
                </el-tooltip>
              </template>
              <el-switch v-model="configForm.sync_users"></el-switch>
            </el-form-item>
            <div class="section-title"><i class="el-icon-document"></i> 回调域名验证</div>
            <el-form-item label="验证文件名">
              <el-input v-model="configForm.verify_file_name" placeholder="如 WW_verify_abc.txt"></el-input>
            </el-form-item>
            <el-form-item label="验证文件内容">
              <el-input v-model="configForm.verify_file_content" placeholder="企微后台要求的校验文件内容"></el-input>
            </el-form-item>
          </template>
        </div>
      </el-form>

      <div slot="footer" class="dialog-footer">
        <template v-if="ruleForm.id && isLdapOrRadius && isTestEnabled">
          <el-button @click="openTestLoginDialog()" style="margin-right:10px">测试登录</el-button>
        </template>
        <template v-if="ruleForm.id && isSyncEnabled">
          <el-button type="success" @click="openSyncUsersDialog()" :loading="syncLoading"
            style="margin-right:10px">同步用户</el-button>
        </template>
        <el-button @click="closeDialog">取消</el-button>
        <el-button type="primary" icon="el-icon-check" @click="submitForm">保存认证源</el-button>
      </div>
    </el-dialog>

    <!-- 测试登录弹窗 -->
    <el-dialog :close-on-click-modal="false" title="测试用户登录" :visible.sync="testLoginDialog" width="420px"
      custom-class="test-login-dialog" :append-to-body="true" center @opened="focusTestName">
      <div class="test-login-body">
        <el-form :model="testLoginForm" :rules="testLoginRules" ref="testLoginForm" label-width="60px">
          <el-form-item label="账号" prop="name">
            <el-input v-model="testLoginForm.name" ref="testLoginName" placeholder="LDAP/RADIUS 用户名"
              @keydown.enter.native="testAuthLogin"></el-input>
          </el-form-item>
          <el-form-item label="密码" prop="pwd">
            <el-input type="password" v-model="testLoginForm.pwd" placeholder="用户密码"
              @keydown.enter.native="testAuthLogin"></el-input>
          </el-form-item>
        </el-form>
        <div class="test-login-actions">
          <el-button type="primary" size="small" @click="testAuthLogin()" :loading="testLoginLoading">登录</el-button>
          <el-button size="small" @click="testLoginDialog = false">取消</el-button>
        </div>
      </div>
    </el-dialog>

    <!-- 同步用户弹窗 -->
    <el-dialog :close-on-click-modal="false" :title="'同步 ' + getSyncTypeLabel() + ' 用户'" :visible.sync="syncUsersDialog"
      width="450px" :append-to-body="true" center>
      <el-form :model="syncForm" :rules="syncRules" ref="syncForm" label-width="80px">
        <el-form-item label="用户组" prop="group_name">
          <el-select v-model="syncForm.group_name" placeholder="选择目标用户组" style="width:100%"
            @visible-change="(v) => v && loadGroupNames()">
            <el-option v-for="g in groupNames" :key="g" :label="g" :value="g"></el-option>
          </el-select>
          <div class="form-tip form-tip-info">同步的用户将加入该用户组</div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="syncProviderUsers()" :loading="syncDoLoading">同步</el-button>
          <el-button @click="syncUsersDialog = false">取消</el-button>
        </el-form-item>
      </el-form>
    </el-dialog>
  </div>
</template>

<script>
import axios from "axios";

const configDefaults = {
  ldap: {
    addr: "", tls: false, tls_verify: false, ldaps: false, bind_name: "", bind_pwd: "", base_dn: "",
    object_class: "person", search_attr: "sAMAccountName",
    member_of: "", sync_user_status: false, enable_otp: false, sync_users: false
  },
  radius: { addr: "", secret: "", nasip: "" },
  wxwork: { corp_id: "", agent_id: "", secret: "", use_default_browser: false, allowed_departments: "", blocked_userids: "", sync_users: false, verify_file_name: "", verify_file_content: "" },
  feishu: { app_id: "", app_secret: "", use_default_browser: false, allowed_departments: "", blocked_userids: "", sync_users: false },
  dingtalk: { client_id: "", client_secret: "", use_default_browser: false, allowed_departments: "", blocked_userids: "", sync_users: false },
};

const TYPE_LABELS = { ldap: 'LDAP', radius: 'RADIUS', wxwork: '企微', feishu: '飞书', dingtalk: '钉钉' };

export default {
  name: "ProviderList",
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['认证管理'])
  },
  mounted() { this.getData(1); },
  data() {
    return {
      loading: false,
      page: 1, tableData: [], count: 0,
      editDialog: false,
      detailCallbackBase: "",
      ruleForm: { id: 0, name: "", type: "ldap", status: 1 },
      configForm: Object.assign({}, configDefaults.ldap),
      rules: {
        name: [
          { required: true, message: '请输入名称', trigger: 'blur' },
          { max: 60, message: '长度不超过 60 个字符', trigger: 'blur' },
        ],
        type: [{ required: true, message: '请选择类型', trigger: 'change' }],
      },
      testLoginDialog: false, testLoginLoading: false,
      testLoginForm: { name: "", pwd: "" },
      testLoginRules: {
        name: [{ required: true, message: '请输入账号', trigger: 'blur' }],
        pwd: [{ required: true, message: '请输入密码', trigger: 'blur' }, { min: 6, message: '至少6个字符', trigger: 'blur' }],
      },
      syncUsersDialog: false, syncLoading: false, syncDoLoading: false,
      syncForm: { group_name: "" },
      syncRules: { group_name: [{ required: true, message: '请选择用户组', trigger: 'change' }] },
      groupNames: [],
    };
  },
  computed: {
    isLdap() { return this.ruleForm.type === 'ldap'; },
    isWxwork() { return this.ruleForm.type === 'wxwork'; },
    isFeishu() { return this.ruleForm.type === 'feishu'; },
    isDingtalk() { return this.ruleForm.type === 'dingtalk'; },
    // 各第三方认证源在对应开放平台后台需填写的回调地址（路径固定，基础地址由后端下发，
    // 使用 VPN 服务端口；新增态未加载详情时回退到当前页 origin）
    callbackBase() {
      return this.detailCallbackBase || (typeof window !== 'undefined' && window.location.origin) || '';
    },
    wxworkCallback() { return this.callbackBase + '/WXAuth/callback'; },
    feishuCallback() { return this.callbackBase + '/FeishuAuth/callback'; },
    dingtalkCallback() { return this.callbackBase + '/DingtalkAuth/callback'; },
    webAuthCallback() { return this.callbackBase + '/web-auth/sso-callback'; },
    isLdapOrRadius() { return this.ruleForm.type === 'ldap' || this.ruleForm.type === 'radius'; },
    isTestEnabled() { return this.ruleForm.status === 1; },
    isSyncEnabled() { return this.ruleForm.id && (this.isLdap || this.isWxwork || this.isFeishu || this.isDingtalk); },
    statTotal() { return this.count; },
    statActive() { return this.tableData.filter(r => r.status === 1).length; },
  },
  methods: {
    getTypeLabel(t) { return TYPE_LABELS[t] || t; },
    getTypeTagType(t) {
      return { ldap: 'primary', radius: 'warning', wxwork: 'success', feishu: '', dingtalk: 'primary' }[t] || 'info';
    },
    getSyncTypeLabel() {
      if (this.isWxwork) return '企微';
      if (this.isFeishu) return '飞书';
      if (this.isDingtalk) return '钉钉';
      return 'LDAP';
    },
    isTestable(t) { return t === 'ldap' || t === 'radius'; },
    isSyncable(t) { return t === 'ldap' || t === 'wxwork' || t === 'feishu' || t === 'dingtalk'; },
    tableDateFormat(row, col, val) {
      if (!val) return '';
      return new Date(val).toLocaleString();
    },
    handleRowCmd(row, cmd) {
      this.ruleForm = { id: row.id, name: row.name, type: row.type, status: row.status };
      this.configForm = Object.assign({}, configDefaults[row.type] || {});
      switch (cmd) {
        case 'edit': this.handleEdit(row); break;
        case 'test': this.openTestLoginDialog(); break;
        case 'sync':
          this.syncLoading = true;
          this.openSyncUsersDialog();
          this.syncLoading = false;
          break;
        case 'delete':
          this.$confirm('确定要删除该认证源吗？', '删除确认', {
            confirmButtonText: '确定删除', cancelButtonText: '取消',
            type: 'warning', confirmButtonClass: 'el-button--danger',
          }).then(() => this.handleDel(row)).catch(() => { });
          break;
      }
    },
    getData(page) {
      this.page = page;
      this.loading = true;
      axios.get('/provider/list', { params: { page } }).then(resp => {
        const rdata = resp.data.data;
        this.tableData = rdata.datas || [];
        this.count = rdata.count || 0;
        // 列表接口一并下发回调基础地址（基于 VPN 服务端口），
        // 新增态无需加载详情即可正确显示，避免回退到后台 8800 地址
        if (rdata.callbackBase) {
          this.detailCallbackBase = rdata.callbackBase;
        }
        this.loading = false;
      }).catch(() => { this.loading = false; });
    },
    pageChange(p) { this.getData(p); },
    onTypeChange() {
      this.configForm = Object.assign({}, configDefaults[this.ruleForm.type] || {});
    },
    // TLS 与 LDAPS 互斥：两者加密方式不同，不可能同时生效
    onLdapsChange(val) {
      if (val) this.configForm.tls = false;
    },
    onTlsChange(val) {
      if (val) this.configForm.ldaps = false;
    },
    handleEdit(row) {
      this.$refs['ruleForm'] && this.$refs['ruleForm'].resetFields();
      this.editDialog = true;
      if (!row) {
        this.ruleForm = { id: 0, name: "", type: "ldap", status: 1 };
        this.configForm = Object.assign({}, configDefaults.ldap);
        return;
      }
      axios.get('/provider/detail', { params: { id: row.id } }).then(resp => {
        const data = resp.data.data;
        const p = data.provider || data;
        this.ruleForm = {
          id: p.id || 0, name: p.name || "",
          type: p.type || "ldap",
          status: p.status !== undefined ? p.status : 1,
        };
        let cfg = {};
        try { cfg = typeof p.config === 'string' ? JSON.parse(p.config) : (p.config || {}); }
        catch (e) { cfg = {}; }
        this.configForm = Object.assign({}, configDefaults[this.ruleForm.type] || {}, cfg);
        // 后端下发的回调地址基础（VPN 服务端口），用于引导在开放平台填写回调 URL
        this.detailCallbackBase = data.callbackBase || "";
      }).catch(() => {
        this.$message.error('获取详情失败');
      });
    },
    submitForm() {
      this.$refs['ruleForm'].validate(valid => {
        if (!valid) return;
        axios.post('/provider/set', {
          id: this.ruleForm.id, name: this.ruleForm.name,
          type: this.ruleForm.type, status: this.ruleForm.status,
          config: this.configForm,
        }).then(resp => {
          if (resp.data.code === 0) {
            this.$message.success(resp.data.msg || '保存成功');
            this.getData(this.page);
            this.editDialog = false;
          } else { this.$message.error(resp.data.msg); }
        }).catch(() => this.$message.error('请求出错'));
      });
    },
    handleDel(row) {
      axios.post('/provider/del', null, { params: { id: row.id } }).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success(resp.data.msg || '删除成功');
          this.getData(this.page);
        } else { this.$message.error(resp.data.msg); }
      }).catch(() => this.$message.error('请求出错'));
    },
    openTestLoginDialog() { this.testLoginForm = { name: "", pwd: "" }; this.testLoginDialog = true; },
    focusTestName() { this.$nextTick(() => { const el = this.$refs['testLoginName']; if (el) el.focus(); }); },
    testAuthLogin() {
      this.$refs['testLoginForm'].validate(valid => {
        if (!valid) return;
        this.testLoginLoading = true;
        axios.post('/provider/test_login', {
          id: this.ruleForm.id,
          name: this.testLoginForm.name,
          pwd: this.testLoginForm.pwd,
        }).then(resp => {
          if (resp.data.code === 0) { this.$message.success("登录成功"); }
          else { this.$message.error(resp.data.msg); }
        }).catch(() => this.$message.error('请求出错'))
          .finally(() => { this.testLoginLoading = false; });
      });
    },
    openSyncUsersDialog() { this.syncForm.group_name = ''; this.syncUsersDialog = true; this.loadGroupNames(); },
    loadGroupNames() {
      if (this.groupNames.length > 0) return;
      axios.get('/group/names').then(resp => {
        if (resp.data.code === 0 && resp.data.data && resp.data.data.datas) {
          this.groupNames = resp.data.data.datas;
        }
      }).catch(() => { });
    },
    syncProviderUsers() {
      this.$refs['syncForm'].validate(valid => {
        if (!valid) return;
        this.syncDoLoading = true;
        axios.post('/provider/sync_users', {
          id: this.ruleForm.id, group_name: this.syncForm.group_name,
        }).then(resp => {
          if (resp.data.code === 0) { this.$message.success(resp.data.data || '正在后台同步，请查看后台日志确认同步结果'); this.syncUsersDialog = false; }
          else { this.$message.error(resp.data.msg); }
        }).catch(() => this.$message.error('同步失败'))
          .finally(() => { this.syncDoLoading = false; });
      });
    },
    closeDialog() { this.editDialog = false; },
  },
};
</script>

<style scoped>
/* ========== 页面整体 ========== */
.provider-page {
  padding: 4px 0;
}

/* Provider 页面统计卡片为 2 列 */
.provider-page .stats-row {
  grid-template-columns: repeat(2, 1fr);
}

/* ========== 统计卡片 ========== */
.stat-icon-total {
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.stat-icon-active {
  background: var(--success-bg);
  color: var(--color-success);
}

/* 表格内 */
.provider-name {
  font-weight: 600;
  color: var(--text-primary);
  font-size: 13px;
}

.status-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 5px;
  vertical-align: middle;
}

.status-dot-online {
  background: var(--color-success);
  box-shadow: 0 0 0 2px rgba(103, 194, 58, 0.2);
}

.status-dot-offline {
  background: var(--color-danger);
  box-shadow: 0 0 0 2px rgba(245, 108, 108, 0.2);
}

.status-text {
  font-size: 12px;
  vertical-align: middle;
}

.text-success {
  color: var(--color-success);
  font-weight: 500;
}

.text-danger {
  color: var(--color-danger);
  font-weight: 500;
}

.action-more-btn {
  padding: 5px 10px;
  border-radius: 6px;
  font-size: 12px;
  border: 1px solid var(--border-base);
  background: var(--bg-card);
  color: var(--text-regular);
  transition: all 0.2s;
}

.action-more-btn:hover {
  color: var(--color-primary);
  border-color: var(--color-primary-light);
  background: var(--color-primary-bg);
}

.dropdown-danger {
  color: var(--color-danger) !important;
}

.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  padding-top: 16px;
}

/* 表格滚动容器 */
.provider-table-wrap {
  overflow-x: auto;
  width: 100%;
}

/* ========== 编辑弹窗 ========== */
.provider-edit-dialog ::v-deep .el-dialog__body {
  padding: 16px 24px 10px;
}

.edit-basic-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 20px;
}

.edit-basic-row .form-item-compact {
  margin-bottom: 14px;
}

.form-item-status {
  margin-bottom: 8px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.form-tip-info,
.form-tip {
  font-size: 12px;
  color: var(--text-secondary);
}

.form-tip-info {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  padding: 6px 10px;
  background: var(--info-bg);
  border-radius: 6px;
}

.form-tip-info code {
  font-family: monospace;
  background: var(--bg-hover);
  padding: 1px 6px;
  border-radius: 4px;
  color: var(--text-primary);
  word-break: break-all;
}

.form-tip-warn {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  margin-top: 6px;
  padding: 6px 10px;
  background: var(--warning-bg);
  border-left: 3px solid var(--color-warning);
  border-radius: 4px;
  font-size: 12px;
  color: var(--text-regular);
}

.form-tip-warn code {
  font-family: monospace;
  background: var(--bg-hover);
  padding: 1px 6px;
  border-radius: 4px;
  color: var(--text-primary);
  word-break: break-all;
}

/* 配置面板 */
.config-panel {
  margin-top: 4px;
  padding: 8px 16px 4px;
  background: var(--bg-hover);
  border: 1px solid var(--border-color);
  border-radius: 8px;
}

.config-panel ::v-deep .el-form-item {
  margin-bottom: 12px;
}

.config-panel ::v-deep .el-form-item__label {
  white-space: nowrap;
}

.section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 16px 0 10px;
  padding-left: 4px;
  border-left: 3px solid var(--color-primary);
  overflow: hidden;
}

.section-title i {
  margin-right: 6px;
  color: var(--color-primary);
  font-size: 14px;
}

.section-title:first-child {
  margin-top: 0;
}

.section-warn {
  font-size: 12px;
  font-weight: 500;
  color: var(--color-warning);
  float: right;
}

.section-warn .el-icon-warning {
  margin-right: 3px;
}

.help-icon {
  color: var(--text-secondary);
  margin-left: 4px;
  cursor: pointer;
  font-size: 14px;
  vertical-align: -1px;
}

.help-icon:hover {
  color: var(--color-primary);
}

.switch-row .inline-switch-item .el-form-item__label {
  display: flex;
  align-items: center;
  white-space: nowrap;
}

.switch-row .inline-switch-item .el-form-item__content {
  line-height: 32px;
}

.help-icon {
  margin-left: 4px;
  font-size: 13px;
  color: var(--text-secondary);
  cursor: help;
}

.help-icon:hover {
  color: var(--color-primary);
}

/* 响应式 */
@media (max-width: 1200px) {
  .edit-basic-row {
    grid-template-columns: 1fr 1fr;
  }
}

@media (max-width: 768px) {
  .edit-basic-row {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 880px) {
  .provider-table-wrap ::v-deep .col-ops {
    min-width: 180px;
  }
}

@media (max-width: 600px) {
  .provider-table-wrap ::v-deep .col-ops {
    min-width: 140px;
  }
}
</style>

<!-- 非 scoped：针对 append-to-body 的弹窗样式 -->
<style>
.test-login-dialog {
  min-width: 360px;
}

.test-login-dialog .el-dialog__body {
  padding: 20px 24px 24px;
}

.test-login-dialog .el-form-item {
  margin-bottom: 16px;
}

.test-login-dialog .el-form-item:last-child {
  margin-bottom: 0;
}

.test-login-dialog .test-login-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 4px;
}
</style>
