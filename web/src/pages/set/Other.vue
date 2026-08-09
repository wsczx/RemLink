<template>
  <div class="other-page">
    <el-card class="other-card" shadow="never">
      <el-tabs v-model="activeName" @tab-click="handleClick" class="other-tabs">
        <el-tab-pane name="dataSmtp">
          <span slot="label"><i class="el-icon-message"></i> 邮件配置</span>
          <div class="email-settings-wrap">
            <!-- SMTP 服务器 -->
            <div class="setting-card">
              <div class="setting-card-title"><i class="el-icon-connection"></i> SMTP 服务器连接</div>
              <el-form :model="dataSmtp" ref="dataSmtp" :rules="rules" label-width="110px" class="email-form">
                <el-form-item label="服务器地址" prop="host">
                  <el-input v-model="dataSmtp.host" placeholder="如 smtp.example.com"></el-input>
                </el-form-item>
                <el-form-item label="服务器端口" prop="port">
                  <el-input v-model.number="dataSmtp.port" placeholder="如 465 或 587"></el-input>
                  <div class="form-tip-new">常见端口：SSLTLS=465, STARTTLS=587, None=25</div>
                </el-form-item>
                <el-form-item label="加密类型" prop="encryption">
                  <el-radio-group v-model="dataSmtp.encryption" size="small">
                    <el-radio-button label="None">None</el-radio-button>
                    <el-radio-button label="SSLTLS">SSLTLS</el-radio-button>
                    <el-radio-button label="STARTTLS">STARTTLS</el-radio-button>
                  </el-radio-group>
                </el-form-item>
              </el-form>
            </div>

            <!-- 认证信息 -->
            <div class="setting-card">
              <div class="setting-card-title"><i class="el-icon-lock"></i> 认证信息</div>
              <el-form :model="dataSmtp" :rules="rules" label-width="110px" class="email-form">
                <el-form-item label="用户名" prop="username">
                  <el-input v-model="dataSmtp.username" placeholder="邮箱账号"></el-input>
                </el-form-item>
                <el-form-item label="密码" prop="password">
                  <el-input type="password" v-model="dataSmtp.password" placeholder="密码为空则不修改" show-password></el-input>
                </el-form-item>
              </el-form>
            </div>

            <!-- 发件设置 -->
            <div class="setting-card">
              <div class="setting-card-title"><i class="el-icon-s-promotion"></i> 发件设置</div>
              <el-form :model="dataSmtp" :rules="rules" label-width="110px" class="email-form">
                <el-form-item label="发件人地址" prop="from">
                  <el-input v-model="dataSmtp.from" placeholder="如 noreply@example.com"></el-input>
                  <div class="form-tip-new">发送邮件时显示的 From 地址，需与 SMTP 账号匹配</div>
                </el-form-item>
              </el-form>
            </div>

            <div class="setting-actions">
              <el-button type="primary" icon="el-icon-check" @click="submitForm('dataSmtp')">保存设置</el-button>
              <el-button icon="el-icon-refresh" @click="resetForm('dataSmtp')">重置</el-button>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane name="dataSms">
          <span slot="label"><i class="el-icon-mobile-phone"></i> 短信配置</span>
          <div class="sms-settings-wrap">
            <!-- 服务商选择 -->
            <div class="setting-card">
              <div class="setting-card-title"><i class="el-icon-s-operation"></i> 短信服务商</div>
              <el-form :model="dataSms" label-width="110px" class="sms-form">
                <el-form-item label="服务商" prop="provider">
                  <el-radio-group v-model="dataSms.provider" size="small" @change="onSmsProviderChange">
                    <el-radio-button label="">关闭</el-radio-button>
                    <el-radio-button label="aliyun">阿里云</el-radio-button>
                    <el-radio-button label="tencent">腾讯云</el-radio-button>
                  </el-radio-group>
                </el-form-item>
              </el-form>
            </div>

            <!-- 阿里云配置 -->
            <div class="setting-card" v-if="dataSms.provider === 'aliyun'">
              <div class="setting-card-title"><i class="el-icon-set-up"></i> 阿里云短信配置</div>
              <el-form :model="dataSms" ref="dataSms" :rules="smsRules" label-width="150px" class="sms-form">
                <el-form-item label="AccessKey ID" prop="ali_access_key_id">
                  <el-input v-model="dataSms.ali_access_key_id" placeholder="阿里云 RAM AccessKey ID"></el-input>
                </el-form-item>
                <el-form-item label="AccessKey Secret" prop="ali_access_key_secret">
                  <el-input type="password" v-model="dataSms.ali_access_key_secret" placeholder="请输入 AccessKey Secret"
                    show-password></el-input>
                </el-form-item>
                <el-form-item label="短信签名" prop="ali_sign_name">
                  <el-input v-model="dataSms.ali_sign_name" placeholder="审核通过的短信签名"></el-input>
                </el-form-item>
                <el-form-item label="模板CODE" prop="ali_template_code">
                  <el-input v-model="dataSms.ali_template_code" placeholder="如 SMS_123456789"></el-input>
                  <div class="form-tip-new">模板需包含变量 ${code}(验证码) 和 ${time}(有效分钟数)</div>
                </el-form-item>
              </el-form>
            </div>

            <!-- 腾讯云配置 -->
            <div class="setting-card" v-if="dataSms.provider === 'tencent'">
              <div class="setting-card-title"><i class="el-icon-set-up"></i> 腾讯云短信配置</div>
              <el-form :model="dataSms" ref="dataSms" :rules="smsRules" label-width="150px" class="sms-form">
                <el-form-item label="SecretId" prop="tencent_secret_id">
                  <el-input v-model="dataSms.tencent_secret_id" placeholder="腾讯云 API 密钥 SecretId"></el-input>
                </el-form-item>
                <el-form-item label="SecretKey" prop="tencent_secret_key">
                  <el-input type="password" v-model="dataSms.tencent_secret_key" placeholder="请输入 SecretKey"
                    show-password></el-input>
                </el-form-item>
                <el-form-item label="地域" prop="tencent_region">
                  <el-input v-model="dataSms.tencent_region" placeholder="ap-guangzhou"></el-input>
                  <div class="form-tip-new">常见地域：ap-guangzhou(华南)、ap-beijing(华北)、ap-shanghai(华东)</div>
                </el-form-item>
                <el-form-item label="SDKAppID" prop="tencent_app_id">
                  <el-input v-model="dataSms.tencent_app_id" placeholder="短信应用 SDKAppID（如 1400006666）"></el-input>
                </el-form-item>
                <el-form-item label="短信签名" prop="tencent_sign_name">
                  <el-input v-model="dataSms.tencent_sign_name" placeholder="审核通过的短信签名内容"></el-input>
                </el-form-item>
                <el-form-item label="模板ID" prop="tencent_template_id">
                  <el-input v-model="dataSms.tencent_template_id" placeholder="如 1234567"></el-input>
                  <div class="form-tip-new">模板需包含变量 {1}(验证码) 和 {2}(有效分钟数)，如"大猫{1}为您验证码，有效期{2}分钟"</div>
                </el-form-item>
              </el-form>
            </div>

            <!-- 测试发送 -->
            <div class="setting-card" v-if="dataSms.provider">
              <div class="setting-card-title"><i class="el-icon-s-promotion"></i> 测试发送</div>
              <el-form label-width="110px" class="sms-form">
                <el-form-item label="测试手机号">
                  <el-input v-model="smsTestPhone" placeholder="如 13800138000" style="width:240px"
                    size="small"></el-input>
                  <el-button size="small" type="primary" icon="el-icon-s-promotion" :loading="smsTestLoading"
                    @click="testSms" style="margin-left:10px">
                    发送测试
                  </el-button>
                </el-form-item>
              </el-form>
            </div>

            <div class="setting-actions" v-if="dataSms.provider">
              <el-button type="primary" icon="el-icon-check" @click="submitForm('dataSms')">保存设置</el-button>
              <el-button icon="el-icon-refresh" @click="resetForm('dataSms')">重置</el-button>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane name="dataAuditLog">
          <span slot="label"><i class="el-icon-document-copy"></i> 审计日志</span>
          <el-form :model="dataAuditLog" ref="dataAuditLog" :rules="rules" label-width="100px" class="tab-one">
            <el-alert title="审计去重间隔(audit_interval)请在「软件配置」页面修改，修改后即时生效无需重启" type="info" :closable="false" show-icon
              style="margin-bottom: 14px">
            </el-alert>
            <el-form-item label="存储时长" prop="life_day">
              <el-input-number v-model="dataAuditLog.life_day" :min="0" :max="365" size="small"
                label="天数"></el-input-number>
              天
              <p class="input_tip">
                范围: 0 ~ 365天 ,
                <strong style="color: #ea3323">0 代表永久保存</strong>
              </p>
            </el-form-item>
            <el-form-item label="清理时间" prop="clear_time">
              <el-time-select v-model="dataAuditLog.clear_time" :picker-options="{
                start: '00:00',
                step: '01:00',
                end: '23:00',
              }" :editable="false" size="small" placeholder="请选择" style="width: 130px">
              </el-time-select>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" icon="el-icon-check" @click="submitForm('dataAuditLog')">保存</el-button>
              <el-button icon="el-icon-refresh" @click="resetForm('dataAuditLog')">重置</el-button>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <el-tab-pane name="dataOther">
          <span slot="label"><i class="el-icon-s-tools"></i> 其他设置</span>
          <el-form :model="dataOther" ref="dataOther" :rules="rules" label-width="120px" class="other-settings-form">
            <div class="other-settings-wrap">
              <!-- Row 1: 访问地址 + Banner -->
              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-link"></i> 访问地址</div>
                <el-form-item label="VPN对外地址" prop="link_addr">
                  <el-input v-model="dataOther.link_addr"
                    placeholder="如 vpn.example.com 或 vpn.example.com:8443"></el-input>
                  <div class="form-tip-new">用于账号开通邮件中的登录地址及软件下载链接，无需填写 https://，需与证书域名一致。</div>
                </el-form-item>
              </div>

              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-notebook-2"></i> 登录 Banner</div>
                <el-form-item label="启用Banner" prop="banner_enable" class="form-item-switch">
                  <el-switch v-model="dataOther.banner_enable" active-color="#13ce66"></el-switch>
                  <span class="switch-label">开启后用户登录时展示下方自定义信息</span>
                </el-form-item>
                <el-form-item label="Banner内容" prop="banner" v-if="dataOther.banner_enable">
                  <el-input type="textarea" :rows="5" v-model="dataOther.banner"
                    placeholder="请输入登录页展示的Banner信息，支持 HTML"></el-input>
                </el-form-item>
              </div>

              <!-- Row 2: 自定义首页 -->
              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-monitor"></i> 自定义首页</div>
                <el-form-item label="首页状态码" prop="homecode">
                  <el-input-number v-model="dataOther.homecode" :min="0" :max="1000"></el-input-number>
                  <div class="form-tip-new">
                    <br>• 0：开启自定义首页（与门户共存），/ 直接展示下方「首页内容」，门户仍在 /ui/#/portal 可进（需填写首页内容）；
                    <br>• 200：门户关闭时 / 展示首页内容，门户开启则跳转到门户；
                    <br>• 其他值：门户关闭时 / 直接返回该 HTTP 状态码（如 503）。
                    <br>门户开启且未设为 0 时，/ 始终跳转到门户。
                  </div>
                </el-form-item>
                <el-form-item label="首页内容" prop="homeindex">
                  <el-input type="textarea" :rows="10" v-model="dataOther.homeindex"
                    placeholder="自定义 HTML 内容，参考 index_template 目录下的文件"></el-input>
                </el-form-item>
              </div>

              <!-- Row 3: 账户邮件模板 + 证书邮件模板 -->
              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-message"></i> 账户邮件模板</div>
                <el-form-item label="邮件模板" prop="account_mail">
                  <el-input type="textarea" :rows="10" v-model="dataOther.account_mail"
                    placeholder="HTML 邮件模板，支持变量 {{.Username}} {{.Password}} 等"></el-input>
                </el-form-item>
                <el-form-item label="预览效果">
                  <div class="mail-preview-wrap">
                    <div class="mail-preview-bar">
                      <span class="mail-preview-dot"></span>
                      <span class="mail-preview-dot"></span>
                      <span class="mail-preview-dot"></span>
                      <span class="mail-preview-title">邮件预览</span>
                    </div>
                    <iframe :srcdoc="dataOther.account_mail" class="mail-preview-iframe"></iframe>
                  </div>
                </el-form-item>
              </div>

              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-s-order"></i> 证书邮件模板</div>
                <div class="template-tip-bar">
                  <span>支持变量：.Username .Groupname .SerialNumber .NotAfter .Password .LinkAddr</span>
                  <el-button size="mini" type="text" @click="loadCertMailTemplate"
                    :loading="certMailTemplateLoading">加载默认模板</el-button>
                </div>
                <el-form-item label="邮件模板" prop="cert_mail">
                  <el-input type="textarea" :rows="10" v-model="dataOther.cert_mail"
                    placeholder="HTML 邮件模板，留空使用默认模板"></el-input>
                </el-form-item>
                <el-form-item label="预览效果">
                  <div class="mail-preview-wrap">
                    <div class="mail-preview-bar">
                      <span class="mail-preview-dot"></span>
                      <span class="mail-preview-dot"></span>
                      <span class="mail-preview-dot"></span>
                      <span class="mail-preview-title">邮件预览</span>
                    </div>
                    <iframe :srcdoc="dataOther.cert_mail" class="mail-preview-iframe"></iframe>
                  </div>
                </el-form-item>
              </div>

              <div class="setting-actions">
                <el-button type="primary" icon="el-icon-check" @click="submitForm('dataOther')">保存设置</el-button>
                <el-button icon="el-icon-refresh" @click="resetForm('dataOther')">重置</el-button>
              </div>
            </div>
          </el-form>
        </el-tab-pane>

        <el-tab-pane name="dataBrand">
          <span slot="label"><i class="el-icon-picture"></i> 品牌展示</span>
          <el-form :model="dataBrand" ref="dataBrand" label-width="120px" class="other-settings-form">
            <div class="other-settings-wrap">
              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-picture"></i> 登录页品牌</div>
                <el-form-item label="Logo 图片" prop="logo">
                  <el-input v-model="dataBrand.logo" placeholder="图片 URL 或 data:image/...;base64,..."></el-input>
                  <div class="brand-logo-row">
                    <img v-if="dataBrand.logo" :src="normalizeSrc(dataBrand.logo)" class="brand-logo-preview"
                      alt="logo 预览" />
                    <el-button size="small" icon="el-icon-upload" @click="$refs.brandFile.click()">上传图片</el-button>
                    <el-button size="small" icon="el-icon-document-copy" v-if="dataBrand.logo"
                      @click="copyText(dataBrand.logo, 'Logo 地址')">复制</el-button>
                    <el-button size="small" icon="el-icon-delete" v-if="dataBrand.logo"
                      @click="dataBrand.logo = ''">清除</el-button>
                    <input ref="brandFile" type="file" accept="image/*" style="display:none"
                      @change="uploadImage($event, 'logo')" />
                  </div>
                  <div class="form-tip-new">建议尺寸 32×32~48×48，支持 PNG/SVG；留空则显示默认图标。</div>
                </el-form-item>
                <el-form-item label="网站图标" prop="favicon">
                  <el-input v-model="dataBrand.favicon" placeholder="图片 URL 或 data:image/...;base64,..."></el-input>
                  <div class="brand-logo-row">
                    <img v-if="dataBrand.favicon" :src="normalizeSrc(dataBrand.favicon)" class="brand-favicon-preview"
                      alt="favicon 预览" />
                    <el-button size="small" icon="el-icon-upload" @click="$refs.faviconFile.click()">上传图片</el-button>
                    <el-button size="small" icon="el-icon-document-copy" v-if="dataBrand.favicon"
                      @click="copyText(dataBrand.favicon, 'Favicon 地址')">复制</el-button>
                    <el-button size="small" icon="el-icon-delete" v-if="dataBrand.favicon"
                      @click="dataBrand.favicon = ''">清除</el-button>
                    <input ref="faviconFile" type="file" accept="image/*" style="display:none"
                      @change="uploadImage($event, 'favicon')" />
                  </div>
                  <div class="form-tip-new">建议尺寸 16×16~32×32，支持 ICO/PNG/SVG；留空则使用默认 favicon（浏览器标签图标）。</div>
                </el-form-item>
                <el-form-item label="品牌名称" prop="title">
                  <el-input v-model="dataBrand.title" placeholder="如 某某公司 VPN，留空回退 RemLink"></el-input>
                  <div class="form-tip-new">门户、WebAuth、管理后台登录页统一展示该名称，留空则显示默认 RemLink。</div>
                </el-form-item>
                <el-form-item label="副标题" prop="desc">
                  <el-input v-model="dataBrand.desc" placeholder="如 企业级远程接入平台，留空回退默认副标题"></el-input>
                  <div class="form-tip-new">门户、WebAuth 登录页品牌名下方的灰色副标题，留空则显示默认文案。</div>
                </el-form-item>
                <el-form-item label="页脚内容" prop="footer">
                  <el-input type="textarea" :rows="3" v-model="dataBrand.footer"
                    placeholder="支持 HTML；可用 .github-badge 实现徽章式页脚，留空回退默认页脚"></el-input>
                </el-form-item>
              </div>
              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-menu"></i> 登录页功能卡片</div>
                <el-form-item label="显示功能卡片">
                  <el-switch v-model="brandFeaturesOn"></el-switch>
                  <div class="form-tip-new">关闭后门户登录页不展示副标题下方的功能亮点卡片。</div>
                </el-form-item>
                <template v-if="brandFeaturesOn">
                  <el-form-item v-for="(f, i) in brandFeatures" :key="i" :label="'卡片' + (i + 1)">
                    <el-input v-model="f.label" placeholder="标题" style="width:160px;margin-right:10px"></el-input>
                    <el-input v-model="f.desc" placeholder="描述" style="width:280px"></el-input>
                  </el-form-item>
                </template>
              </div>
              <div class="setting-actions">
                <el-button type="primary" icon="el-icon-check" @click="submitForm('dataBrand')">保存设置</el-button>
                <el-button icon="el-icon-refresh" @click="resetForm('dataBrand')">重置</el-button>
              </div>
            </div>
          </el-form>
        </el-tab-pane>

        <el-tab-pane name="dataDashboard">
          <span slot="label"><i class="el-icon-menu"></i> 门户首页</span>
          <el-form :model="dataDashboard" ref="dataDashboard" label-width="120px" class="other-settings-form">
            <div class="other-settings-wrap">

              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-bell"></i> 公告横幅</div>
                <el-form-item label="显示公告" class="form-item-switch">
                  <el-switch v-model="dashAnnouncementOn"></el-switch>
                  <span class="switch-label">开启后门户首页顶部展示公告</span>
                </el-form-item>
                <template v-if="dashAnnouncementOn">
                  <el-form-item label="公告样式">
                    <el-select v-model="dataDashboard.announcement_level" placeholder="选择样式" style="width:160px">
                      <el-option label="信息" value="info"></el-option>
                      <el-option label="成功" value="success"></el-option>
                      <el-option label="警告" value="warning"></el-option>
                      <el-option label="错误" value="error"></el-option>
                    </el-select>
                  </el-form-item>
                  <el-form-item label="公告内容">
                    <el-input type="textarea" :rows="4" v-model="dataDashboard.announcement"
                      placeholder="支持 HTML，如 &lt;strong&gt;系统维护通知&lt;/strong&gt;"></el-input>
                  </el-form-item>
                </template>
              </div>

              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-link"></i> 快捷链接</div>
                <el-form-item label="显示快捷链接" class="form-item-switch">
                  <el-switch v-model="dashQuickLinksOn"></el-switch>
                  <span class="switch-label">开启后门户首页展示自定义链接入口（如内部 Wiki、工单系统）</span>
                </el-form-item>
                <template v-if="dashQuickLinksOn">
                  <div class="form-tip-new ql-tip">
                    图标支持三种写法：① Element UI 图标类名，如
                    <code>el-icon-star-on</code>；② 图片地址（http(s):// 或
                    <code>data:image/...</code>）；③ 直接填 emoji 字符，如 🔗。留空则不显示图标。
                  </div>
                  <div class="ql-list">
                    <div class="ql-row ql-head">
                      <span class="ql-index"></span>
                      <span class="ql-col-label">标题</span>
                      <span class="ql-col-label ql-col-wide">链接地址</span>
                      <span class="ql-col-label">图标</span>
                      <span class="ql-col-btn"></span>
                    </div>
                    <div class="ql-row" v-for="(l, i) in dashQuickLinks" :key="i">
                      <span class="ql-index">{{ i + 1 }}</span>
                      <el-input v-model="l.label" placeholder="标题" class="ql-input"></el-input>
                      <el-input v-model="l.url" placeholder="链接地址 http(s)://" class="ql-input ql-input-wide"></el-input>
                      <el-input v-model="l.icon" placeholder="图标" class="ql-input"></el-input>
                      <el-button type="danger" size="small" icon="el-icon-delete" circle
                        @click="removeQuickLink(i)"></el-button>
                    </div>
                  </div>
                  <el-button type="primary" size="small" plain icon="el-icon-plus" @click="addQuickLink"
                    class="ql-add">新增链接</el-button>
                </template>
              </div>

              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-connection"></i> 客户端连接指引</div>
                <el-form-item label="显示指引" class="form-item-switch">
                  <el-switch v-model="dashClientGuideOn"></el-switch>
                  <span class="switch-label">开启后门户首页展示各平台客户端连接指引</span>
                </el-form-item>

                <template v-if="dashClientGuideOn">
                  <!-- 客户端下载内容 -->
                  <div class="cg-section-label">
                    <i class="el-icon-download"></i> 客户端下载内容
                  </div>
                  <div class="form-tip-new cg-tip">
                    指引卡片右上角「客户端下载」按钮弹窗展示的 HTML 内容（可放各平台下载链接）。留空则不显示下载按钮。
                  </div>
                  <el-input type="textarea" :rows="6" v-model="dataDashboard.client_download_html"
                    placeholder="支持 HTML，如 <a href='/files/remlink-windows-amd64.exe'>Windows 客户端</a>"
                    class="cg-download-input"></el-input>

                  <!-- 各平台连接步骤 -->
                  <div class="cg-section-label">
                    <i class="el-icon-guide"></i> 各平台连接步骤
                  </div>
                  <div class="form-tip-new cg-tip">
                    步骤内容支持 HTML，可用 <code v-pre>{{server_addr}}</code> 占位符（自动替换为用户实际服务器地址）。删除全部平台则门户展示默认指引。
                  </div>
                  <el-collapse v-model="cgActiveNames" class="cg-collapse">
                    <el-collapse-item v-for="(g, gi) in dashClientGuide" :key="gi" :name="gi">
                      <template slot="title">
                        <div class="cg-collapse-title">
                          <i class="el-icon-monitor"></i>
                          <span class="cg-collapse-name">{{ g.name || '未命名平台' }}</span>
                          <span class="cg-collapse-count">{{ (g.steps || []).length }} 步</span>
                        </div>
                      </template>
                      <div class="cg-group-head">
                        <el-input v-model="g.name" placeholder="平台名称，如 Windows" class="cg-name" size="small"></el-input>
                        <el-button type="danger" size="small" plain icon="el-icon-delete"
                          @click="removeClientGuideGroup(gi)">删除平台</el-button>
                      </div>
                      <div class="cg-steps">
                        <div class="cg-step" v-for="(s, si) in g.steps" :key="si">
                          <span class="cg-step-num">{{ si + 1 }}</span>
                          <el-input type="textarea" :rows="2" v-model="g.steps[si]"
                            placeholder="步骤说明，可用 {{server_addr}} 占位"></el-input>
                          <el-button type="danger" size="small" icon="el-icon-close" circle
                            @click="removeClientGuideStep(gi, si)"></el-button>
                        </div>
                        <el-button type="primary" size="small" plain icon="el-icon-plus"
                          @click="addClientGuideStep(gi)">新增步骤</el-button>
                      </div>
                    </el-collapse-item>
                  </el-collapse>
                  <el-button type="primary" size="small" plain icon="el-icon-plus" @click="addClientGuideGroup"
                    class="cg-add">新增平台</el-button>
                </template>
              </div>

              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-view"></i> 首页卡片显隐</div>
                <div class="card-visibility-grid">
                  <div class="cv-item" v-for="opt in dashCardOptions" :key="opt.key">
                    <span class="cv-label">{{ opt.label }}</span>
                    <el-switch v-model="dashCardsVisible[opt.key]"></el-switch>
                  </div>
                </div>
                <div class="form-tip-new">关闭后门户首页对应卡片对用户隐藏。</div>
              </div>

              <div class="setting-card">
                <div class="setting-card-title"><i class="el-icon-brush"></i> 主题与样式</div>
                <el-form-item label="主题主色">
                  <el-color-picker v-model="dataDashboard.theme_color"></el-color-picker>
                  <el-input v-model="dataDashboard.theme_color" placeholder="#2f7cff"
                    style="width:140px;margin-left:10px"></el-input>
                  <div class="form-tip-new">留空则使用默认主题色。</div>
                </el-form-item>
                <el-form-item label="自定义 CSS">
                  <el-input type="textarea" :rows="5" v-model="dataDashboard.custom_css"
                    placeholder="注入到门户页面的 &lt;style&gt; 中，如 .portal-page { background: var(--bg-page); }"></el-input>
                  <el-collapse class="css-vars-help">
                    <el-collapse-item title="可用 CSS 变量与选择器参考（点击展开）">
                      <p class="css-help-desc">
                        自定义 CSS 会注入到门户页面 &lt;head&gt;（<strong>全局作用域</strong>），不仅能改配色，也能改写门户的显示布局。
                        <strong>生效范围：仅登录后的「门户首页」生效，登录页不注入。</strong>
                      </p>
                      <p class="css-help-warn">
                        ⚠️ <strong>关键：</strong>门户组件样式带 scoped 属性选择器，特异性高于自定义 CSS。凡是要覆盖组件原有样式的规则，<strong>必须加
                          <code>!important</code></strong>，否则只有配色等少数规则生效、布局改不动（这是最常见的坑）。
                      </p>
                      <div class="css-vars-grid">
                        <div class="css-var-group">
                          <div class="css-var-group-title">主题色</div>
                          <code>--color-primary</code>
                          <code>--color-primary-bg</code>
                          <code>--color-primary-light</code>
                          <code>--color-primary-dark</code>
                          <code>--color-success</code>
                          <code>--color-warning</code>
                          <code>--color-danger</code>
                          <code>--color-info</code>
                        </div>
                        <div class="css-var-group">
                          <div class="css-var-group-title">背景</div>
                          <code>--bg-page</code>
                          <code>--bg-card</code>
                          <code>--bg-hover</code>
                          <code>--bg-header</code>
                          <code>--bg-stripe</code>
                          <code>--bg-overlay</code>
                          <code>--header-bg</code>
                          <code>--footer-bg</code>
                        </div>
                        <div class="css-var-group">
                          <div class="css-var-group-title">文字</div>
                          <code>--text-primary</code>
                          <code>--text-regular</code>
                          <code>--text-secondary</code>
                          <code>--text-placeholder</code>
                          <code>--text-inverse</code>
                        </div>
                        <div class="css-var-group">
                          <div class="css-var-group-title">边框 / 卡片</div>
                          <code>--border-base</code>
                          <code>--border-color</code>
                          <code>--border-color-light</code>
                          <code>--card-radius</code>
                          <code>--card-shadow</code>
                        </div>
                        <div class="css-var-group">
                          <div class="css-var-group-title">门户核心选择器（可直接写样式）</div>
                          <code>.portal-page</code>
                          <code>.portal-dash-header</code>
                          <code>.portal-dash-main</code>
                          <code>.welcome-banner</code>
                          <code>.stats-row / .stat-card / .stat-icon</code>
                          <code>.quicklinks-card / .quicklink-item</code>
                          <code>.devices-card / .device-item</code>
                          <code>.table-card / .card-title / .card-header</code>
                          <code>.client-step / .step-num</code>
                          <code>.group-item / .cert-item</code>
                          <code>.dash-grid / .dash-left / .dash-right</code>
                          <code>.portal-announcement</code>
                          <code>.portal-footer</code>
                          <code>.portal-login-card</code>
                        </div>
                      </div>
                      <p class="css-help-example">配色示例：<code>.portal-dash-main { --bg-card: #f7faff; }</code></p>
                      <p class="css-help-example">布局示例（主区两列，<strong>记得加 !important</strong>）：<code>.dash-grid { display: grid
                  !important; grid-template-columns: 1fr 360px !important; }</code></p>
                      <p class="css-help-example">悬浮窗示例（把快捷链接钉在左侧）：<code>.quicklinks-card { position: fixed !important; left:
                  20px; top: 90px; width: 220px; z-index: 50; }</code></p>
                      <p class="css-help-example">隐藏页脚：<code>.portal-footer { display: none !important; }</code></p>
                    </el-collapse-item>
                  </el-collapse>
                </el-form-item>
              </div>

              <div class="setting-actions">
                <el-button type="primary" icon="el-icon-check" @click="submitForm('dataDashboard')">保存设置</el-button>
                <el-button icon="el-icon-refresh" @click="resetForm('dataDashboard')">重置</el-button>
              </div>
            </div>
          </el-form>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script>
import axios from "axios"
import { defaultClientGuide } from "../../plugins/clientGuide";
import { normalizeImageSrc } from "../../plugins/brand";

export default {
  name: "Other",
  created() {
    this.$emit("update:route_path", this.$route.path);
    this.$emit("update:route_name", ["系统设置", "其他设置"]);
  },
  mounted() {
    this.getSmtp();
  },
  data() {
    return {
      activeName: "dataSmtp",
      dataSmtp: {},
      dataSms: {},
      dataAuditLog: {},
      dataOther: {},
      dataBrand: {},
      dataDashboard: {},
      dashQuickLinks: [],
      dashClientGuide: [],
      cgActiveNames: [],
      dashCardsVisible: {
        devices: true, groups: true,
        personal_policy: true, password: true, otp: true, certs: true,
        webvpn: true,
      },
      dashCardOptions: [
        { key: "devices", label: "在线设备" },
        { key: "groups", label: "分组权限详情" },
        { key: "personal_policy", label: "个人策略" },
        { key: "password", label: "修改密码" },
        { key: "otp", label: "二次验证(OTP)" },
        { key: "certs", label: "我的证书" },
        { key: "webvpn", label: "WebVPN 应用" },
      ],
      brandFeatures: [
        { label: "账号状态", desc: "查看在线设备与连接状态" },
        { label: "用户分组", desc: "查看所属用户分组" },
        { label: "安全设置", desc: "自主修改密码与验证方式" },
      ],
      smsTestPhone: "",
      smsTestLoading: false,
      certMailTemplateLoading: false,
      rules: {
        host: { required: true, message: "请输入服务器地址", trigger: "blur" },
        port: [
          { required: true, message: "请输入服务器端口", trigger: "blur" },
          {
            type: "number",
            message: "请输入正确的服务器端口",
            trigger: ["blur", "change"],
          },
        ],
        username: { required: true, message: "请输入用户名", trigger: "blur" },
        from: { required: true, message: "请输入发件人地址", trigger: "blur" },
        issuer: { required: true, message: "请输入系统名称", trigger: "blur" },
      },
      smsRules: {
        tencent_secret_id: { required: true, message: "请输入 SecretId", trigger: "blur" },
        tencent_secret_key: { required: true, message: "请输入 SecretKey", trigger: "blur" },
        tencent_region: { required: true, message: "请输入地域", trigger: "blur" },
        tencent_app_id: { required: true, message: "请输入 SDKAppID", trigger: "blur" },
        tencent_sign_name: { required: true, message: "请输入短信签名", trigger: "blur" },
        tencent_template_id: { required: true, message: "请输入模板ID", trigger: "blur" },
        ali_access_key_id: { required: true, message: "请输入 AccessKey ID", trigger: "blur" },
        ali_access_key_secret: { required: true, message: "请输入 AccessKey Secret", trigger: "blur" },
        ali_sign_name: { required: true, message: "请输入短信签名", trigger: "blur" },
        ali_template_code: { required: true, message: "请输入模板CODE", trigger: "blur" },
      },
    };
  },
  computed: {
    brandFeaturesOn: {
      get() {
        return this.dataBrand.features_enabled !== 2
      },
      set(v) {
        this.dataBrand.features_enabled = v ? 1 : 2
      },
    },
    dashAnnouncementOn: {
      get() {
        return this.dataDashboard.announcement_enabled !== 2
      },
      set(v) {
        this.dataDashboard.announcement_enabled = v ? 1 : 2
      },
    },
    dashQuickLinksOn: {
      get() {
        return this.dataDashboard.quick_links_enabled !== 2
      },
      set(v) {
        this.dataDashboard.quick_links_enabled = v ? 1 : 2
      },
    },
    dashClientGuideOn: {
      get() {
        return this.dataDashboard.client_guide_enabled === 1
      },
      set(v) {
        this.dataDashboard.client_guide_enabled = v ? 1 : 2
      },
    },
  },
  methods: {
    // 模板预览用：SVG 源码转 data URI，URL/data 原样
    normalizeSrc(src) { return normalizeImageSrc(src) },
    handleClick(tab) {
      switch (tab.name) {
        case "dataSmtp":
          this.getSmtp();
          break;
        case "dataSms":
          this.getSms();
          break;
        case "dataAuditLog":
          this.getAuditLog();
          break;
        case "dataOther":
          this.getOther();
          break;
        case "dataBrand":
          this.getBrand();
          break;
        case "dataDashboard":
          this.getDashboard();
          break;
      }
    },
    getSmtp() {
      axios
        .get("/set/other/smtp")
        .then((resp) => {
          let rdata = resp.data;
          if (rdata.code !== 0) {
            this.$message.error(rdata.msg);
            return;
          }
          this.dataSmtp = rdata.data;
        })
        .catch(() => {
          this.$message.error("哦，请求出错");
        });
    },
    getSms() {
      axios
        .get("/set/other/sms")
        .then((resp) => {
          let rdata = resp.data;
          if (rdata.code !== 0) {
            this.$message.error(rdata.msg);
            return;
          }
          this.dataSms = rdata.data || {};
        })
        .catch(() => {
          this.$message.error("哦，请求出错");
        });
    },
    testSms() {
      if (!this.smsTestPhone) {
        this.$message.warning("请输入测试手机号");
        return;
      }
      this.smsTestLoading = true;
      axios.post("/set/other/sms/test", { phone: this.smsTestPhone }).then((resp) => {
        var rdata = resp.data;
        if (rdata.code === 0) {
          this.$message.success("测试短信发送成功");
        } else {
          this.$message.error(rdata.msg);
        }
      }).catch(() => {
        this.$message.error("请求失败");
      }).finally(() => {
        this.smsTestLoading = false;
      });
    },
    getAuditLog() {
      axios
        .get("/set/other/audit_log")
        .then((resp) => {
          let rdata = resp.data;
          if (rdata.code !== 0) {
            this.$message.error(rdata.msg);
            return;
          }
          this.dataAuditLog = rdata.data;
        })
        .catch(() => {
          this.$message.error("哦，请求出错");
        });
    },
    getOther() {
      axios
        .get("/set/other")
        .then((resp) => {
          let rdata = resp.data;
          if (rdata.code !== 0) {
            this.$message.error(rdata.msg);
            return;
          }
          this.dataOther = rdata.data;
        })
        .catch(() => {
          this.$message.error("哦，请求出错");
        });
    },
    getBrand() {
      axios
        .get("/set/portal_brand")
        .then((resp) => {
          let rdata = resp.data;
          if (rdata.code !== 0) {
            this.$message.error(rdata.msg);
            return;
          }
          this.dataBrand = rdata.data || {};
          this.brandFeatures = this.parseBrandFeatures(this.dataBrand.features);
        })
        .catch(() => {
          this.$message.error("哦，请求出错");
        });
    },
    parseBrandFeatures(str) {
      let list = [];
      if (str) {
        try {
          list = JSON.parse(str);
        } catch (e) {
          list = [];
        }
      }
      if (!Array.isArray(list) || list.length === 0) {
        list = [
          { label: "账号状态", desc: "查看在线设备与连接状态" },
          { label: "用户分组", desc: "查看所属用户分组" },
          { label: "安全设置", desc: "自主修改密码与验证方式" },
        ];
      }
      return list;
    },
    getDashboard() {
      axios
        .get("/set/portal_dashboard")
        .then((resp) => {
          let rdata = resp.data;
          if (rdata.code !== 0) {
            this.$message.error(rdata.msg);
            return;
          }
          this.dataDashboard = rdata.data || {};
          this.dashQuickLinks = this.parseQuickLinks(this.dataDashboard.quick_links);
          this.dashClientGuide = this.parseClientGuide(this.dataDashboard.client_guide);
          this.dashCardsVisible = this.parseCardsVisible(this.dataDashboard.cards_visible);
        })
        .catch(() => {
          this.$message.error("哦，请求出错");
        });
    },
    parseQuickLinks(str) {
      let list = [];
      if (str) {
        try {
          list = JSON.parse(str);
        } catch (e) {
          list = [];
        }
      }
      return Array.isArray(list) ? list : [];
    },
    parseClientGuide(str) {
      let list = [];
      if (str) {
        try {
          list = JSON.parse(str);
        } catch (e) {
          list = [];
        }
      }
      if (!Array.isArray(list) || list.length === 0) {
        // 深拷贝默认，避免编辑时污染常量
        list = JSON.parse(JSON.stringify(defaultClientGuide));
      }
      return list;
    },
    parseCardsVisible(str) {
      let m = {};
      if (str) {
        try {
          m = JSON.parse(str);
        } catch (e) {
          m = {};
        }
      }
      const def = {
        devices: true, groups: true,
        personal_policy: true, password: true, otp: true, certs: true,
        webvpn: true,
      };
      return Object.assign(def, m);
    },
    addQuickLink() {
      this.dashQuickLinks.push({ label: "", url: "", icon: "" });
    },
    removeQuickLink(i) {
      this.dashQuickLinks.splice(i, 1);
    },
    addClientGuideGroup() {
      this.dashClientGuide.push({ name: "", steps: [""] });
      // 新增平台自动展开，方便立即编辑
      this.cgActiveNames = this.cgActiveNames.concat(this.dashClientGuide.length - 1);
    },
    removeClientGuideGroup(i) {
      this.dashClientGuide.splice(i, 1);
      this.cgActiveNames = [];
    },
    addClientGuideStep(gi) {
      if (!this.dashClientGuide[gi].steps) this.dashClientGuide[gi].steps = [];
      this.dashClientGuide[gi].steps.push("");
    },
    removeClientGuideStep(gi, si) {
      this.dashClientGuide[gi].steps.splice(si, 1);
    },
    uploadImage(e, field) {
      const file = e.target.files[0];
      if (!file) return;
      const maxBytes = 1 << 20; // 1MB 原始图片上限（base64 后约 1.33MB，远低于后端 2MB 限制）
      if (file.size > maxBytes) {
        this.$message.warning(`图片过大（${(file.size / 1024 / 1024).toFixed(1)}MB），请压缩到 1MB 以内再上传`);
        e.target.value = "";
        return;
      }
      const reader = new FileReader();
      reader.onload = () => {
        this.dataBrand[field] = reader.result;
      };
      reader.readAsDataURL(file);
      e.target.value = "";
    },
    copyText(text, label) {
      if (!text) {
        this.$message.warning('没有可复制的内容');
        return;
      }
      const done = () => this.$message.success(`${label || '内容'}已复制到剪贴板`);
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done).catch(() => this.fallbackCopy(text, done));
      } else {
        this.fallbackCopy(text, done);
      }
    },
    fallbackCopy(text, done) {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand('copy'); done(); } catch (e) { /* execCommand deprecated */ }
      document.body.removeChild(ta);
    },
    onSmsProviderChange(val) {
      if (val !== "") {
        return;
      }
      // 选择关闭则直接保存关闭状态
      axios.post("/set/other/sms/edit", { provider: "" }).then((resp) => {
        var rdata = resp.data;
        if (rdata.code === 0) {
          this.$message.success(rdata.msg || "短信功能已关闭");
        } else {
          this.$message.error(rdata.msg);
        }
      });
    },
    submitForm(formName) {
      this.$refs[formName].validate((valid) => {
        if (!valid) {
          this.$message.warning("请完善必填项后再保存");
          return;
        }

        switch (formName) {
          case "dataSmtp":
            axios.post("/set/other/smtp/edit", this.dataSmtp).then((resp) => {
              var rdata = resp.data;
              if (rdata.code === 0) {
                this.$message.success(rdata.msg);
              } else {
                this.$message.error(rdata.msg);
              }
            });
            break;
          case "dataSms":
            axios.post("/set/other/sms/edit", this.dataSms).then((resp) => {
              var rdata = resp.data;
              if (rdata.code === 0) {
                this.$message.success(rdata.msg);
              } else {
                this.$message.error(rdata.msg);
              }
            });
            break;
          case "dataAuditLog":
            axios
              .post("/set/other/audit_log/edit", this.dataAuditLog)
              .then((resp) => {
                var rdata = resp.data;
                if (rdata.code === 0) {
                  this.$message.success(rdata.msg);
                } else {
                  this.$message.error(rdata.msg);
                }
              });
            break;
          case "dataOther":
            axios.post("/set/other/edit", this.dataOther).then((resp) => {
              var rdata = resp.data;
              if (rdata.code === 0) {
                this.$message.success(rdata.msg);
              } else {
                this.$message.error(rdata.msg);
              }
            });
            break;
          case "dataBrand":
            this.dataBrand.features = JSON.stringify(this.brandFeatures);
            // SVG 源码转 data URI 再保存，使各展示页 <img :src="brand.logo"> 能直接渲染
            this.dataBrand.logo = normalizeImageSrc(this.dataBrand.logo)
            this.dataBrand.favicon = normalizeImageSrc(this.dataBrand.favicon)
            axios.post("/set/portal_brand/edit", this.dataBrand).then((resp) => {
              var rdata = resp.data;
              if (rdata.code === 0) {
                this.$message.success(rdata.msg);
                // 使 logo/title/favicon 立即生效
                window.dispatchEvent(new CustomEvent('remlink:brand-updated'))
              } else {
                this.$message.error(rdata.msg);
              }
            });
            break;
          case "dataDashboard":
            this.dataDashboard.quick_links = JSON.stringify(this.dashQuickLinks);
            this.dataDashboard.client_guide = JSON.stringify(this.dashClientGuide);
            this.dataDashboard.cards_visible = JSON.stringify(this.dashCardsVisible);
            axios.post("/set/portal_dashboard/edit", this.dataDashboard).then((resp) => {
              var rdata = resp.data;
              if (rdata.code === 0) {
                this.$message.success(rdata.msg);
              } else {
                this.$message.error(rdata.msg);
              }
            });
            break;
        }
      });
    },
    resetForm(formName) {
      this.$refs[formName].resetFields();
    },
    loadCertMailTemplate() {
      this.certMailTemplateLoading = true;
      axios.get('/set/client_cert/cert_mail_template').then(resp => {
        if (resp.data.code === 0) {
          this.dataOther.cert_mail = resp.data.data;
          this.$message.success('已加载默认证书邮件模板');
        } else {
          this.$message.error(resp.data.msg);
        }
      }).catch(() => {
        this.$message.error('加载失败');
      }).finally(() => {
        this.certMailTemplateLoading = false;
      });
    },
  },
};
</script>

<style scoped>
.other-page {
  padding: 4px 0;
}

.other-card {
  border-radius: var(--card-radius);
  overflow: hidden;
  border: 1px solid var(--border-color-light);
}

.other-tabs ::v-deep .el-tabs__content {
  padding: 20px 24px;
}

.tab-one {
  max-width: 760px;
}

.input_tip {
  line-height: 1.428;
  margin: 2px 0 0 0;
}

/* ========== 其他设置美化 ========== */
.other-settings-form {
  max-width: 860px;
}

.other-settings-wrap {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 设置卡片 */
.setting-card {
  background: var(--bg-card);
  border: 1px solid var(--border-color-light);
  border-radius: 10px;
  padding: 20px 24px 4px;
  transition: border-color 0.2s;
}

.setting-card:hover {
  border-color: #d4d9e1;
}

.setting-card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 16px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--border-color-light);
  display: flex;
  align-items: center;
  gap: 8px;
}

.setting-card-title i {
  color: var(--color-primary);
  font-size: 16px;
}

/* 模板加载提示栏 */
.template-tip-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
  background: var(--bg-hover);
  border-radius: 6px;
  padding: 8px 12px;
}

/* 表单项提示 */
.form-tip-new {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 6px;
  line-height: 1.6;
}

/* Switch 行 */
.form-item-switch ::v-deep .el-form-item__content {
  display: flex;
  align-items: center;
  gap: 10px;
}

.switch-label {
  font-size: 13px;
  color: var(--text-regular);
}

/* 邮件预览 */
.mail-preview-wrap {
  border: 1px solid var(--border-color);
  border-radius: 10px;
  overflow: hidden;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
  width: 100%;
  max-width: 560px;
}

.mail-preview-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 14px;
  background: var(--bg-hover);
  border-bottom: 1px solid var(--border-color);
}

.mail-preview-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--border-base);
}

.mail-preview-dot:nth-child(1) {
  background: var(--color-danger);
}

.mail-preview-dot:nth-child(2) {
  background: var(--color-warning);
}

.mail-preview-dot:nth-child(3) {
  background: var(--color-success);
}

.mail-preview-title {
  margin-left: 8px;
  font-size: 12px;
  color: var(--text-secondary);
  font-weight: 500;
}

.mail-preview-iframe {
  width: 100%;
  height: 320px;
  border: none;
  display: block;
  background: var(--bg-card);
}

/* 操作按钮 */
.setting-actions {
  display: flex;
  gap: 10px;
  padding-top: 4px;
}

/* 品牌 Logo 预览 */
.brand-logo-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 10px;
}

.brand-logo-preview {
  width: 40px;
  height: 40px;
  object-fit: contain;
  border: 1px solid var(--border-color-light);
  border-radius: 6px;
  background: var(--bg-hover);
  padding: 2px;
}

.brand-favicon-preview {
  width: 32px;
  height: 32px;
  object-fit: contain;
  border: 1px solid var(--border-color-light);
  border-radius: 4px;
  background: var(--bg-hover);
  padding: 2px;
}

/* ========== 邮件配置美化 ========== */
.email-settings-wrap {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 760px;
}

/* ========== 短信配置美化 ========== */
.sms-settings-wrap {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 760px;
}

/* ========== 门户首页配置 ========== */
.ql-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 12px;
}

.ql-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.ql-index {
  flex: 0 0 22px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--bg-hover);
  color: var(--text-secondary);
  font-size: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.ql-input {
  flex: 1 1 120px;
  min-width: 0;
}

.ql-input-wide {
  flex: 2 1 200px;
}

.ql-add {
  margin-top: 2px;
}

/* 快捷链接图标填写提示 */
.ql-tip {
  margin-bottom: 12px;
  background: var(--bg-hover);
  border: 1px solid var(--border-color-light);
  border-radius: 6px;
  padding: 8px 12px;
}

.ql-tip code {
  background: var(--bg-card);
  border: 1px solid var(--border-color-light);
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 12px;
  color: var(--color-primary-dark);
}

/* 快捷链接列头 */
.ql-head {
  margin-bottom: 2px;
}

.ql-col-label {
  flex: 1 1 120px;
  min-width: 0;
  font-size: 12px;
  color: var(--text-secondary);
  padding: 0 2px;
}

.ql-col-wide {
  flex: 2 1 200px;
}

/* 列头最后一列对齐删除按钮（固定宽度，不参与弹性分配） */
.ql-col-btn {
  flex: 0 0 32px;
  min-width: 0;
}

/* ========== 客户端连接指引编辑器 ========== */
.cg-tip {
  margin-bottom: 14px;
  background: var(--bg-hover);
  border: 1px solid var(--border-color-light);
  border-radius: 6px;
  padding: 8px 12px;
}

.cg-tip code {
  background: var(--bg-card);
  border: 1px solid var(--border-color-light);
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 12px;
  color: var(--color-primary-dark);
}

/* 卡片内分区标题 */
.cg-section-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 4px 0 8px;
}

.cg-section-label:not(:first-of-type) {
  margin-top: 20px;
}

.cg-section-label i {
  color: var(--color-primary);
}

.cg-download-input {
  margin-bottom: 4px;
}

/* 各平台折叠面板 */
.cg-collapse {
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  overflow: hidden;
  margin-bottom: 12px;
}

.cg-collapse ::v-deep .el-collapse-item__header {
  padding: 0 14px;
  height: 44px;
  line-height: 44px;
  background: var(--bg-hover);
}

.cg-collapse ::v-deep .el-collapse-item__content {
  padding: 14px;
}

.cg-collapse-title {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
}

.cg-collapse-title i {
  color: var(--color-primary);
}

.cg-collapse-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.cg-collapse-count {
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-card);
  border: 1px solid var(--border-color-light);
  border-radius: 10px;
  padding: 0 8px;
  line-height: 18px;
}

.cg-group-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}

.cg-name {
  flex: 1 1 auto;
  max-width: 280px;
}

.cg-steps {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.cg-step {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.cg-step-num {
  flex: 0 0 22px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--bg-hover);
  color: var(--text-secondary);
  font-size: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 6px;
}

.cg-step .el-textarea {
  flex: 1 1 auto;
}

.cg-add {
  margin-top: 2px;
}

.card-visibility-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px 24px;
  margin-bottom: 12px;
}

.cv-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  background: var(--bg-card);
}

.cv-label {
  font-size: 13px;
  color: var(--text-regular);
}

/* ========== 自定义 CSS 变量参考 ========== */
.css-vars-help {
  margin-top: 10px;
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  overflow: hidden;
}

.css-vars-help ::v-deep .el-collapse-item__header {
  padding-left: 14px;
  font-size: 13px;
  color: var(--text-regular);
  background: var(--bg-hover);
}

.css-vars-help ::v-deep .el-collapse-item__content {
  padding: 14px;
}

.css-help-desc {
  margin: 0 0 12px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--text-secondary);
}

.css-help-desc code,
.css-help-example code {
  background: var(--bg-hover);
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 12px;
  color: var(--color-primary-dark);
}

.css-vars-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 14px;
}

.css-var-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.css-var-group-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 2px;
}

.css-var-group code {
  display: block;
  background: var(--bg-hover);
  border: 1px solid var(--border-color-light);
  border-radius: 5px;
  padding: 4px 8px;
  font-size: 11.5px;
  color: var(--text-regular);
  word-break: break-all;
}

.css-help-example {
  margin: 14px 0 0;
  font-size: 12px;
  color: var(--text-secondary);
}

.css-help-warn {
  margin: 0 0 12px;
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.7;
  color: #8a5a00;
  background: #fff7e6;
  border: 1px solid #ffd591;
  border-radius: 8px;
}

.css-help-warn code {
  background: rgba(0, 0, 0, 0.06);
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 12px;
  color: #ad4e00;
}

/* 响应式 */
@media (max-width: 900px) {
  .other-tabs ::v-deep .el-tabs__content {
    padding: 14px 16px;
  }

  .email-settings-wrap,
  .sms-settings-wrap,
  .other-settings-form {
    max-width: 100%;
  }

  .tab-one {
    max-width: 100%;
  }

  .setting-card {
    padding: 16px 16px 4px;
  }

  .setting-actions {
    flex-wrap: wrap;
  }

  .mail-preview-wrap {
    max-width: 100%;
  }

  .card-visibility-grid {
    grid-template-columns: 1fr;
    gap: 10px;
  }

  .ql-row {
    flex-wrap: wrap;
  }

  .ql-index {
    display: none;
  }

  .ql-head {
    display: none;
  }

  .cg-group-head {
    flex-wrap: wrap;
  }

  .cg-name {
    max-width: 100%;
    flex: 1 1 100%;
  }

  .cg-step {
    flex-wrap: wrap;
  }

  .cg-step .el-textarea {
    flex: 1 1 100%;
  }

  .ql-input,
  .ql-input-wide {
    flex: 1 1 100%;
  }

  .css-vars-grid {
    grid-template-columns: 1fr;
    gap: 12px;
  }
}

@media (max-width: 640px) {
  .other-tabs ::v-deep .el-form-item {
    display: block;
  }

  .other-tabs ::v-deep .el-form-item__label {
    width: auto !important;
    text-align: left;
    padding-bottom: 4px;
    line-height: 1.4;
  }

  .other-tabs ::v-deep .el-form-item__content {
    margin-left: 0 !important;
    display: block;
  }

  .template-tip-bar {
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;
  }

  .other-tabs ::v-deep .el-tabs__item {
    font-size: 13px;
    padding: 0 12px;
  }

  .other-tabs ::v-deep .el-tabs__item i {
    margin-right: 3px;
  }

  .mail-preview-iframe {
    height: 240px;
  }
}
</style>
