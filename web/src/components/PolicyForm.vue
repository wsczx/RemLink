<template>
  <div class="policy-form-wrap">
    <el-tabs v-model="activeTab" :before-leave="beforeTabLeave" class="policy-tabs">
      <!-- ========== 通用 ========== -->
      <el-tab-pane name="general">
        <span slot="label"><i class="el-icon-setting"></i> 通用</span>
        <div class="tab-section">
          <div class="section-title">基本设置</div>
          <el-form-item label="下行带宽" prop="bandwidth_format" class="form-item-row">
            <el-input v-model="form.bandwidth_format"
              oninput="value= value.match(/\d+(\.\d{0,2})?/) ? value.match(/\d+(\.\d{0,2})?/)[0] : ''"
              class="bandwidth-input">
              <template slot="append">Mbps</template>
            </el-input>
            <span class="form-tip">0 表示不限制（服务器→客户端）</span>
          </el-form-item>

          <el-form-item label="上行带宽" prop="bandwidth_up_format" class="form-item-row">
            <el-input v-model="form.bandwidth_up_format"
              oninput="value= value.match(/\d+(\.\d{0,2})?/) ? value.match(/\d+(\.\d{0,2})?/)[0] : ''"
              class="bandwidth-input">
              <template slot="append">Mbps</template>
            </el-input>
            <span class="form-tip">0 表示不限制（客户端→服务器）</span>
          </el-form-item>

          <el-form-item label="流量配额" prop="traffic_quota_format" class="form-item-row">
            <el-input v-model="form.traffic_quota_format"
              oninput="value= value.match(/\d+(\.\d{0,2})?/) ? value.match(/\d+(\.\d{0,2})?/)[0] : ''"
              class="bandwidth-input">
              <template slot="append">GB</template>
            </el-input>
            <el-select v-model="form.traffic_reset" placeholder="重置周期" size="small" class="traffic-reset-select"
              :disabled="!form.traffic_quota_format || parseFloat(form.traffic_quota_format) <= 0">
              <el-option label="不限" value=""></el-option>
              <el-option label="按日" value="daily"></el-option>
              <el-option label="按周" value="weekly"></el-option>
              <el-option label="按月" value="monthly"></el-option>
            </el-select>
            <span class="form-tip">上下行累计流量超出后断开</span>
          </el-form-item>

          <el-form-item label="排除本地网络" prop="allow_lan" class="form-item-row">
            <el-switch v-model="form.allow_lan" active-color="#13ce66" inactive-color="#dcdfe6">
            </el-switch>
            <span class="form-tip">开启后，AnyConnect 客户端需勾选 Allow Local LAN 才能生效</span>
          </el-form-item>
        </div>

        <div class="tab-section">
          <div class="section-title">客户端 DNS</div>
          <el-form-item label="DNS 服务器" prop="client_dns" class="form-item-block">
            <div class="dynamic-list">
              <div class="dynamic-list-header">
                <span class="dynamic-col-val">DNS 地址</span>
                <span class="dynamic-col-note">备注</span>
                <span class="dynamic-col-ops"></span>
              </div>
              <div v-for="(item, index) in form.client_dns" :key="index" class="dynamic-item">
                <div class="dynamic-col-val">
                  <el-input v-model="item.val" placeholder="如 8.8.8.8" size="small"></el-input>
                </div>
                <div class="dynamic-col-note">
                  <el-input v-model="item.note" placeholder="备注" size="small"></el-input>
                </div>
                <div class="dynamic-col-ops">
                  <el-button size="mini" type="danger" icon="el-icon-delete" circle
                    @click.prevent="removeItem(form.client_dns, index)"></el-button>
                </div>
              </div>
              <el-button size="small" type="primary" plain icon="el-icon-plus" @click.prevent="addItem(form.client_dns)"
                class="dynamic-add-btn">
                添加 DNS
              </el-button>
            </div>
          </el-form-item>
        </div>
      </el-tab-pane>

      <!-- ========== 路由设置 ========== -->
      <el-tab-pane name="route">
        <span slot="label"><i class="el-icon-share"></i> 路由设置</span>
        <div class="tab-section">
          <div class="section-title">
            包含路由
            <div class="section-title-actions">
              <el-dropdown size="mini" trigger="click" @command="(id) => copyRoutesFrom(id)">
                <el-button size="mini" type="text" icon="el-icon-document-copy">
                  从策略复制
                </el-button>
                <el-dropdown-menu slot="dropdown">
                  <el-dropdown-item v-for="p in otherPolicies" :key="p.id" :command="p.id">
                    {{ p.name }}
                  </el-dropdown-item>
                  <el-dropdown-item v-if="otherPolicies.length === 0" disabled>暂无其他策略</el-dropdown-item>
                </el-dropdown-menu>
              </el-dropdown>
              <el-button size="mini" type="text" icon="el-icon-edit"
                @click.prevent="$emit('open-ip-editor', 'route_include')">
                批量编辑
              </el-button>
            </div>
          </div>
          <el-form-item label="路由规则" prop="route_include" class="form-item-block">
            <div v-if="form.route_include.length > collapseThreshold && !expanded.route_include" class="route-compact">
              <i class="el-icon-info"></i>
              共 <b>{{ form.route_include.length }}</b> 条包含路由，建议使用上方「批量编辑」管理，或
              <el-button type="text" @click="expanded.route_include = true">展开逐条编辑</el-button>
            </div>
            <div v-show="!(form.route_include.length > collapseThreshold && !expanded.route_include)"
              class="dynamic-list" :class="{ 'route-list-scroll': form.route_include.length > collapseThreshold }">
              <div class="dynamic-list-header">
                <span class="dynamic-col-val">CIDR 地址</span>
                <span class="dynamic-col-note">备注</span>
                <span class="dynamic-col-ops"></span>
              </div>
              <div v-for="(item, index) in form.route_include" :key="index" class="dynamic-item">
                <div class="dynamic-col-val">
                  <el-input v-model="item.val" placeholder="如 192.168.1.0/24 或 all" size="small"></el-input>
                </div>
                <div class="dynamic-col-note">
                  <el-input v-model="item.note" placeholder="备注" size="small"></el-input>
                </div>
                <div class="dynamic-col-ops">
                  <el-button size="mini" type="danger" icon="el-icon-delete" circle
                    @click.prevent="removeItem(form.route_include, index)"></el-button>
                </div>
              </div>
              <el-button size="small" type="primary" plain icon="el-icon-plus"
                @click.prevent="addItem(form.route_include)" class="dynamic-add-btn">
                添加包含路由
              </el-button>
            </div>
          </el-form-item>
        </div>

        <div class="tab-section">
          <div class="section-title">
            排除路由
            <div class="section-title-actions">
              <el-button size="mini" type="text" icon="el-icon-edit"
                @click.prevent="$emit('open-ip-editor', 'route_exclude')">
                批量编辑
              </el-button>
            </div>
          </div>
          <el-form-item label="路由规则" prop="route_exclude" class="form-item-block">
            <div v-if="form.route_exclude.length > collapseThreshold && !expanded.route_exclude" class="route-compact">
              <i class="el-icon-info"></i>
              共 <b>{{ form.route_exclude.length }}</b> 条排除路由，建议使用上方「批量编辑」管理，或
              <el-button type="text" @click="expanded.route_exclude = true">展开逐条编辑</el-button>
            </div>
            <div v-show="!(form.route_exclude.length > collapseThreshold && !expanded.route_exclude)"
              class="dynamic-list" :class="{ 'route-list-scroll': form.route_exclude.length > collapseThreshold }">
              <div class="dynamic-list-header">
                <span class="dynamic-col-val">CIDR 地址</span>
                <span class="dynamic-col-note">备注</span>
                <span class="dynamic-col-ops"></span>
              </div>
              <div v-for="(item, index) in form.route_exclude" :key="index" class="dynamic-item">
                <div class="dynamic-col-val">
                  <el-input v-model="item.val" placeholder="如 10.0.0.0/8" size="small"></el-input>
                </div>
                <div class="dynamic-col-note">
                  <el-input v-model="item.note" placeholder="备注" size="small"></el-input>
                </div>
                <div class="dynamic-col-ops">
                  <el-button size="mini" type="danger" icon="el-icon-delete" circle
                    @click.prevent="removeItem(form.route_exclude, index)"></el-button>
                </div>
              </div>
              <el-button size="small" type="primary" plain icon="el-icon-plus"
                @click.prevent="addItem(form.route_exclude)" class="dynamic-add-btn">
                添加排除路由
              </el-button>
            </div>
          </el-form-item>
        </div>
      </el-tab-pane>

      <!-- ========== 权限控制 ========== -->
      <el-tab-pane name="link_acl">
        <span slot="label"><i class="el-icon-lock"></i> 权限控制</span>
        <div class="tab-section">
          <div class="section-title">
            访问控制规则 (ACL)
            <div class="section-title-actions">
              <el-dropdown size="mini" trigger="click" @command="(id) => copyAclFrom(id)">
                <el-button size="mini" type="text" icon="el-icon-document-copy">
                  从策略复制
                </el-button>
                <el-dropdown-menu slot="dropdown">
                  <el-dropdown-item v-for="p in otherPolicies" :key="p.id" :command="p.id">
                    {{ p.name }}
                  </el-dropdown-item>
                  <el-dropdown-item v-if="otherPolicies.length === 0" disabled>暂无其他策略</el-dropdown-item>
                </el-dropdown-menu>
              </el-dropdown>
              <el-button size="mini" type="text" icon="el-icon-edit" @click.prevent="$emit('open-acl-editor')">
                批量编辑
              </el-button>
            </div>
          </div>
          <el-form-item label="ACL 规则" prop="link_acl" class="form-item-block">
            <div class="msg-info acl-info">
              <i class="el-icon-info"></i>
              规则自上而下匹配，未匹配的流量默认拒绝。支持 all / tcp / udp / icmp 协议，端口 0 表示所有端口。多个端口逗号分隔：80,443，连续端口：8000-9000
            </div>
            <draggable v-model="form.link_acl" handle=".drag-handle" class="acl-list"
              :class="{ 'acl-list-scroll': form.link_acl.length > collapseThreshold }">
              <div v-for="(item, index) in form.link_acl" :key="index" class="acl-item">
                <div class="acl-drag drag-handle" title="拖拽排序">
                  <i class="el-icon-rank"></i>
                </div>
                <div class="acl-action">
                  <el-select v-model="item.action" size="small" class="acl-action-select">
                    <el-option label="允许" value="allow">
                      <span class="acl-option-allow"><i class="el-icon-check"></i> 允许</span>
                    </el-option>
                    <el-option label="禁止" value="deny">
                      <span class="acl-option-deny"><i class="el-icon-close"></i> 禁止</span>
                    </el-option>
                  </el-select>
                </div>
                <div class="acl-cidr">
                  <el-input v-model="item.val" placeholder="CIDR 地址" size="small"></el-input>
                </div>
                <div class="acl-proto">
                  <el-input v-model="item.protocol" placeholder="协议" size="small"></el-input>
                </div>
                <div class="acl-port">
                  <el-input v-model="item.port" placeholder="端口" size="small"></el-input>
                </div>
                <div class="acl-note">
                  <el-input v-model="item.note" placeholder="备注" size="small"></el-input>
                </div>
                <div class="acl-del">
                  <el-button size="mini" type="danger" icon="el-icon-delete" circle
                    @click.prevent="removeItem(form.link_acl, index)"></el-button>
                </div>
              </div>
            </draggable>
            <el-button size="small" type="primary" plain icon="el-icon-plus" @click.prevent="addItem(form.link_acl)"
              class="dynamic-add-btn">
              添加 ACL 规则
            </el-button>
          </el-form-item>
        </div>
      </el-tab-pane>

      <!-- ========== 域名拆分隧道 ========== -->
      <el-tab-pane name="ds_domains">
        <span slot="label"><i class="el-icon-connection"></i> 域名拆分隧道</span>
        <div class="tab-section">
          <div class="section-title">包含域名</div>
          <el-form-item label="域名列表" prop="ds_include_domains" class="form-item-block">
            <el-input type="textarea" :rows="4" v-model="form.ds_include_domains"
              placeholder="多个域名逗号分隔，如 baidu.com,163.com（默认匹配所有子域名）"></el-input>
          </el-form-item>
        </div>

        <div class="tab-section">
          <div class="section-title">排除域名</div>
          <el-form-item label="域名列表" prop="ds_exclude_domains" class="form-item-block">
            <el-input type="textarea" :rows="4" v-model="form.ds_exclude_domains"
              placeholder="多个域名逗号分隔，如 baidu.com,163.com（默认匹配所有子域名）"></el-input>
          </el-form-item>
        </div>

        <div class="msg-info">
          <i class="el-icon-warning"></i>
          域名拆分隧道仅支持 AnyConnect 的 Windows 和 macOS 桌面客户端，不支持移动端。
        </div>
      </el-tab-pane>

      <!-- ========== FakeDNS ========== -->
      <el-tab-pane name="fakedns" v-if="showFakeDNS">
        <span slot="label"><i class="el-icon-magic-stick"></i> FakeDNS</span>

        <div class="tab-section">
          <div class="section-title">基本开关</div>
          <el-form-item label="FakeDNS" prop="enable_fakedns" class="form-item-row">
            <el-switch v-model="form.enable_fakedns" active-color="#409eff" inactive-color="#dcdfe6">
            </el-switch>
            <span class="form-tip">通过服务端拦截 DNS 请求实现域名分流，支持所有客户端（含移动端）</span>
          </el-form-item>
        </div>

        <div class="tab-section" :class="{ 'tab-section-disabled': !form.enable_fakedns }">
          <div class="section-title">上游 DNS</div>
          <el-form-item label="DNS 地址" prop="fake_dns_upstream" class="form-item-block">
            <el-input v-model="form.fake_dns_upstream" placeholder="如 8.8.8.8（留空则使用客户端 DNS）"
              :disabled="!form.enable_fakedns"></el-input>
            <div class="form-tip form-tip-warning" v-if="form.enable_fakedns">
              <i class="el-icon-warning"></i>
              若未配置客户端 DNS，需手动配置上游 DNS 地址，否则 FakeDNS 无法生效
            </div>
          </el-form-item>
          <el-form-item label="IPv6 优先" prop="prefer_ipv6" class="form-item-block">
            <el-switch v-model="form.prefer_ipv6" active-color="#409eff" inactive-color="#dcdfe6"
              :disabled="!form.enable_fakedns">
            </el-switch>
            <div class="form-tip">开启后，命中 FakeDNS 规则的域名优先回 AAAA（v6 fakeIP），对 A 查询返回 NODATA 引导双栈应用走 v6（仅双栈开启时有效）</div>
          </el-form-item>
        </div>

        <div class="tab-section" :class="{ 'tab-section-disabled': !form.enable_fakedns }">
          <div class="section-title">包含域名（白名单模式）</div>
          <el-form-item label="域名列表" prop="fake_dns_include" class="form-item-block">
            <el-input type="textarea" :rows="4" v-model="form.fake_dns_include" :disabled="!form.enable_fakedns"
              placeholder="如 google.com,youtube.com（仅拦截列表中的域名，其余正常解析）"></el-input>
            <div class="form-tip">
              仅拦截列表中的域名。与"排除域名"二选一，不能同时填写。
            </div>
          </el-form-item>
        </div>

        <div class="tab-section" :class="{ 'tab-section-disabled': !form.enable_fakedns }">
          <div class="section-title">排除域名（黑名单模式）</div>
          <el-form-item label="域名列表" prop="fake_dns_exclude" class="form-item-block">
            <el-input type="textarea" :rows="4" v-model="form.fake_dns_exclude" :disabled="!form.enable_fakedns"
              placeholder="如 baidu.com,163.com（拦截所有域名，仅排除列表中的域名正常解析）"></el-input>
            <div class="form-tip">
              拦截所有域名，仅排除列表中的域名。与"包含域名"二选一，不能同时填写。
            </div>
          </el-form-item>
        </div>

      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script>
import draggable from 'vuedraggable'
import axios from 'axios'

export default {
  name: 'PolicyForm',
  components: { draggable },
  props: {
    value: { type: Object, required: true },
    showSaveBtn: { type: Boolean, default: true },
  },
  data() {
    return {
      activeTab: 'general',
      otherPolicies: [],
      // 路由/ACL 条目超过该阈值时，内联列表默认折叠为摘要，避免编辑页过长
      collapseThreshold: 10,
      expanded: { route_include: false, route_exclude: false },
    }
  },
  computed: {
    form: {
      get() { return this.value },
      set(val) { this.$emit('input', val) }
    }
  },
  methods: {
    addItem(arr) {
      arr.push({ protocol: "all", val: "", action: "allow", port: "0", note: "" });
    },
    removeItem(arr, index) {
      if (index >= 0 && index < arr.length) {
        arr.splice(index, 1)
      }
    },
    beforeTabLeave(activeName) {
      // 切换到路由或ACL tab时加载其他策略列表
      if (activeName === 'route' || activeName === 'link_acl') {
        this.loadOtherPolicies()
      }
      return true
    },
    loadOtherPolicies() {
      if (this.otherPolicies.length > 0) return
      axios.get('/policy/list', { params: { page: 1, page_size: 9999 } }).then(resp => {
        const datas = (resp.data.data && resp.data.data.datas) || []
        this.otherPolicies = datas
          .filter(p => p.id !== this.form.id)
          .map(p => ({ id: p.id, name: p.name }))
      }).catch(() => { })
    },
    copyRoutesFrom(policyId) {
      axios.get('/policy/detail', { params: { id: policyId } }).then(resp => {
        const d = resp.data.data
        if (d.route_include && d.route_include.length) {
          this.form.route_include = JSON.parse(JSON.stringify(d.route_include))
        }
        if (d.route_exclude && d.route_exclude.length) {
          this.form.route_exclude = JSON.parse(JSON.stringify(d.route_exclude))
        }
        this.$message.success('路由已从策略复制')
      }).catch(() => this.$message.error('复制失败'))
    },
    copyAclFrom(policyId) {
      axios.get('/policy/detail', { params: { id: policyId } }).then(resp => {
        const d = resp.data.data
        if (d.link_acl && d.link_acl.length) {
          this.form.link_acl = JSON.parse(JSON.stringify(d.link_acl))
        } else {
          this.form.link_acl = []
        }
        this.$message.success('ACL规则已从策略复制')
      }).catch(() => this.$message.error('复制失败'))
    },
  }
}
</script>

<style scoped>
/* ========== Tabs 美化 ========== */
.policy-tabs ::v-deep .el-tabs__header {
  margin-bottom: 0;
  padding: 0 0 0 4px;
}

.policy-tabs ::v-deep .el-tabs__nav-wrap::after {
  height: 1px;
  background: var(--border-color);
}

.policy-tabs ::v-deep .el-tabs__item {
  font-size: 13px;
  font-weight: 500;
  padding: 0 18px;
  height: 42px;
  line-height: 42px;
}

.policy-tabs ::v-deep .el-tabs__item i {
  margin-right: 4px;
}

/* ========== Tab 内容区 ========== */
.tab-section {
  background: var(--bg-card);
  border: 1px solid var(--border-color-light);
  border-radius: 8px;
  padding: 16px 20px 4px;
  margin-bottom: 14px;
}

.tab-section-disabled {
  opacity: 0.5;
  pointer-events: none;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border-color-light);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-title .el-button--text {
  font-size: 12px;
  padding: 4px 8px;
  font-weight: 400;
}

.section-title-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

/* ========== 表单项 ========== */
.form-item-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.form-item-row ::v-deep .el-form-item__content {
  display: flex;
  align-items: center;
  gap: 10px;
}

.form-item-block ::v-deep .el-form-item__content {
  display: block;
}

.bandwidth-input {
  width: 180px;
}

.traffic-reset-select {
  width: 110px;
}

.form-tip {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
}

.form-tip-warning {
  margin-top: 8px;
  padding: 8px 12px;
  background: var(--warning-bg);
  border: 1px solid var(--warning-bg);
  border-radius: 6px;
  color: var(--color-warning);
  display: flex;
  align-items: center;
  gap: 6px;
}

.form-tip-warning i {
  font-size: 14px;
  flex-shrink: 0;
}

/* ========== 动态列表 ========== */
.dynamic-list {
  width: 100%;
}

.dynamic-list-header {
  display: flex;
  gap: 10px;
  margin-bottom: 6px;
  padding: 0 40px 0 0;
  font-size: 12px;
  color: var(--text-secondary);
  font-weight: 500;
}

.dynamic-col-val {
  flex: 1;
}

.dynamic-col-note {
  flex: 1;
}

.dynamic-col-ops {
  width: 32px;
  flex-shrink: 0;
}

.dynamic-item {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  background: var(--bg-header);
  border: 1px solid transparent;
  transition: all 0.15s;
}

.dynamic-item:hover {
  background: var(--color-primary-bg);
  border-color: var(--color-primary-light);
}

.dynamic-item .dynamic-col-val {
  flex: 1;
}

.dynamic-item .dynamic-col-note {
  flex: 1;
}

.dynamic-item .dynamic-col-ops {
  width: 32px;
  flex-shrink: 0;
  display: flex;
  justify-content: center;
}

.dynamic-add-btn {
  margin-top: 4px;
}

/* 路由条目过多时的折叠摘要 */
.route-compact {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 10px 14px;
  margin-bottom: 10px;
  background: var(--info-bg);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  font-size: 13px;
  color: var(--text-regular);
  line-height: 1.6;
}

.route-compact i {
  color: var(--text-secondary);
  font-size: 15px;
}

.route-compact b {
  color: var(--color-primary);
}

/* 条目过多时内联列表限制高度并滚动 */
.route-list-scroll {
  max-height: 360px;
  overflow-y: auto;
  padding-right: 4px;
}

.acl-list-scroll {
  max-height: 400px;
  overflow-y: auto;
  padding-right: 4px;
}

/* ========== ACL 规则列表 ========== */
.acl-info {
  margin-bottom: 12px !important;
  display: flex;
  align-items: center;
  gap: 6px;
}

.acl-list {
  width: 100%;
}

.acl-item {
  display: flex;
  gap: 6px;
  align-items: center;
  margin-bottom: 8px;
  padding: 8px;
  border-radius: 8px;
  background: var(--bg-header);
  border: 1px solid var(--border-color-light);
  transition: all 0.15s;
}

.acl-item:hover {
  background: var(--color-primary-bg);
  border-color: var(--color-primary-light);
}

.acl-drag {
  flex-shrink: 0;
  cursor: grab;
  color: var(--text-placeholder);
  font-size: 16px;
  padding: 0 4px;
  display: flex;
  align-items: center;
  transition: color 0.15s;
}

.acl-drag:hover {
  color: var(--color-primary);
}

.acl-drag:active {
  cursor: grabbing;
}

.acl-action {
  width: 85px;
  flex-shrink: 0;
}

.acl-cidr {
  flex: 1.5;
  min-width: 100px;
}

.acl-proto {
  width: 80px;
  flex-shrink: 0;
}

.acl-port {
  width: 100px;
  flex-shrink: 0;
}

.acl-note {
  flex: 1;
  min-width: 60px;
}

.acl-del {
  width: 32px;
  flex-shrink: 0;
  display: flex;
  justify-content: center;
}

.acl-action-select {
  width: 100%;
}

.acl-option-allow {
  color: var(--color-success);
  display: flex;
  align-items: center;
  gap: 4px;
  font-weight: 500;
}

.acl-option-deny {
  color: var(--color-danger);
  display: flex;
  align-items: center;
  gap: 4px;
  font-weight: 500;
}

/* ========== 通用信息提示 ========== */
.msg-info {
  background-color: var(--info-bg);
  color: var(--text-secondary);
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 12px;
  display: flex;
  align-items: center;
  gap: 6px;
  line-height: 1.5;
}

.msg-info i {
  flex-shrink: 0;
  font-size: 14px;
}

/* ========== 响应式 ========== */
/* 窄屏：表单标签上置 + 表单项纵向排列 */
@media (max-width: 900px) {
  .tab-section {
    padding: 12px 12px 4px;
  }

  .section-title {
    flex-wrap: wrap;
    gap: 6px;
  }

  .section-title-actions {
    flex-wrap: wrap;
  }

  .bandwidth-input {
    width: 100%;
    max-width: 220px;
  }

  .traffic-reset-select {
    width: 100%;
    max-width: 160px;
  }
}

@media (max-width: 720px) {
  .policy-form-wrap ::v-deep .el-form-item {
    display: block;
    margin-bottom: 16px;
  }

  .policy-form-wrap ::v-deep .el-form-item__label {
    width: auto !important;
    text-align: left;
    padding-bottom: 4px;
    line-height: 1.4;
    float: none;
  }

  .policy-form-wrap ::v-deep .el-form-item__content {
    margin-left: 0 !important;
    display: block;
    line-height: 1.5;
  }

  /* form-item-row 提示换行 */
  .form-item-row {
    flex-wrap: wrap;
  }

  .form-item-row ::v-deep .el-form-item__content {
    flex-wrap: wrap;
  }

  .form-item-row .form-tip {
    width: 100%;
    flex-basis: 100%;
  }

  .form-item-row.label-nowrap ::v-deep .el-form-item__label {
    white-space: nowrap;
    flex-shrink: 0;
  }

  /* 动态列表列堆叠 */
  .dynamic-list-header {
    display: none;
  }

  .dynamic-item {
    flex-wrap: wrap;
    padding: 8px;
    gap: 6px;
  }

  .dynamic-item .dynamic-col-val {
    flex: 1 1 calc(50% - 24px);
    min-width: 100px;
  }

  .dynamic-item .dynamic-col-note {
    flex: 1 1 calc(50% - 24px);
    min-width: 100px;
  }

  .dynamic-item .dynamic-col-ops {
    width: auto;
    flex-shrink: 0;
  }

  /* ACL 项折行 */
  .acl-item {
    flex-wrap: wrap;
    gap: 4px;
  }

  .acl-action {
    width: 70px;
  }

  .acl-proto {
    width: 70px;
  }

  .acl-port {
    width: 80px;
  }

  .acl-cidr {
    flex: 1 1 120px;
    min-width: 100px;
  }

  .acl-note {
    flex: 1 1 100px;
    min-width: 60px;
  }

  .tab-section {
    padding: 10px 8px 2px;
  }
}

@media (max-width: 520px) {
  .dynamic-item .dynamic-col-val {
    flex: 1 1 100%;
  }

  .dynamic-item .dynamic-col-note {
    flex: 1 1 100%;
  }

  .acl-item .acl-cidr {
    flex: 1 1 100%;
  }

  .acl-item .acl-note {
    flex: 1 1 100%;
  }
}
</style>
