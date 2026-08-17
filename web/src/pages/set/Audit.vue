<template>
  <div class="audit-page">
    <el-card class="audit-card" shadow="never">
      <el-tabs v-model="activeName" @tab-click="handleClick" class="audit-tabs">
        <el-tab-pane name="act_log">
          <span slot="label"><i class="el-icon-s-order"></i> 用户活动日志</span>
          <AuditActLog ref="auditActLog"></AuditActLog>
        </el-tab-pane>
        <el-tab-pane name="access_audit">
          <span slot="label"><i class="el-icon-document-copy"></i> 用户访问日志</span>
          <AuditAccess ref="auditAccess"></AuditAccess>
        </el-tab-pane>
        <el-tab-pane name="admin_op_log">
          <span slot="label"><i class="el-icon-s-check"></i> 管理员操作日志</span>
          <AuditAdminOpLog ref="auditAdminOpLog"></AuditAdminOpLog>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script>
import AuditAccess from "../../components/audit/Access";
import AuditActLog from "../../components/audit/ActLog";
import AuditAdminOpLog from "../../components/audit/AdminOpLog";

export default {
  name: "Audit",
  components: {
    AuditAccess,
    AuditActLog,
    AuditAdminOpLog
  },
  mounted() {
    this.upTab();
  },
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['日志审计', '安全审计'])
  },
  data() {
    return {
      activeName: "act_log",
    }
  },
  methods: {
    upTab() {
      var tabname = this.$route.query.tabname
      if (tabname) {
        this.activeName = tabname
      }
      this.handleClick({ name: this.activeName })
    },
    handleClick(tab) {
      switch (tab ? tab.name : this.activeName) {
        case "access_audit":
          this.$refs.auditAccess.setSearchData()
          this.$refs.auditAccess.getData(1)
          break
        case "act_log":
          this.$refs.auditActLog.getData(1)
          break
        case "admin_op_log":
          this.$refs.auditAdminOpLog.getData(1)
          break
      }
      this.$router.push({ path: this.$route.path, query: { tabname: this.activeName } })
    },
  },
}
</script>

<style scoped>
.audit-page {
  padding: 4px 0;
  position: relative;
}

.audit-card {
  border-radius: var(--card-radius);
  overflow: hidden;
  border: 1px solid var(--border-color-light);
}

.audit-tabs ::v-deep .el-tabs__content {
  padding: 16px 20px;
}
</style>
