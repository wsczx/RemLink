<template>
  <div>
    <el-form :model="searchForm" ref="searchForm" :inline="true" class="search-form">
      <el-form-item prop="username">
        <el-input size="mini" v-model="searchForm.username" clearable placeholder="姓名或用户名" style="width: 130px"
          @keydown.enter.native="searchEnterFun"></el-input>
      </el-form-item>
      <el-form-item prop="group_name">
        <el-input size="mini" v-model="searchForm.group_name" clearable placeholder="用户组" style="width: 100px"
          @keydown.enter.native="searchEnterFun"></el-input>
      </el-form-item>
      <el-form-item prop="src">
        <el-input size="mini" v-model="searchForm.src" clearable placeholder="源IP" style="width: 130px"
          @keydown.enter.native="searchEnterFun"></el-input>
      </el-form-item>
      <el-form-item prop="dst">
        <el-input size="mini" v-model="searchForm.dst" clearable placeholder="目的IP" style="width: 130px"
          @keydown.enter.native="searchEnterFun"></el-input>
      </el-form-item>
      <el-form-item prop="dst_port">
        <el-input size="mini" v-model="searchForm.dst_port" clearable placeholder="目的端口" style="width: 80px"
          @keydown.enter.native="searchEnterFun"></el-input>
      </el-form-item>
      <el-form-item prop="access_proto">
        <el-select size="mini" v-model="searchForm.access_proto" clearable placeholder="访问协议" style="width: 100px">
          <el-option v-for="(item, index) in access_proto" :key="index" :label="item.text" :value="item.value">
          </el-option>
        </el-select>
      </el-form-item>
      <el-form-item prop="date">
        <el-date-picker v-model="searchForm.date" type="datetimerange" value-format="yyyy-MM-dd HH:mm:ss" size="mini"
          align="left" range-separator="至" start-placeholder="开始日期" end-placeholder="结束日期"
          :default-time="['00:00:00', '23:59:59']">
        </el-date-picker>
      </el-form-item>
      <el-form-item prop="info">
        <el-input size="mini" v-model="searchForm.info" placeholder="详情关键字" clearable style="width: 200px"
          @keydown.enter.native="searchEnterFun"></el-input>
      </el-form-item>
      <el-form-item>
        <el-switch size="mini" v-model="autoRefresh" active-text="自动刷新" @change="toggleAutoRefresh"></el-switch>
      </el-form-item>
      <el-form-item v-if="autoRefresh">
        <el-select size="mini" v-model="refreshInterval" @change="toggleAutoRefresh" placeholder="刷新间隔"
          style="width: 90px">
          <el-option label="5 秒" :value="5"></el-option>
          <el-option label="10 秒" :value="10"></el-option>
          <el-option label="30 秒" :value="30"></el-option>
        </el-select>
      </el-form-item>
      <el-form-item>
        <el-button size="mini" type="primary" icon="el-icon-search" @click="handleSearch">搜索</el-button>
        <el-button size="mini" icon="el-icon-refresh" @click="rest">重置搜索</el-button>
        <el-button size="mini" icon="el-icon-download" @click="handleExport">导出</el-button>
      </el-form-item>
    </el-form>

    <div class="audit-table-wrap">
      <el-table ref="multipleTable" :data="tableData" :default-sort="{ prop: 'id', order: 'descending' }"
        v-loading="loading" element-loading-text="玩命加载中" element-loading-spinner="el-icon-loading"
        @sort-change="sortChange" :header-cell-style="{ backgroundColor: 'var(--bg-header)' }" border>

        <el-table-column prop="id" label="ID" sortable="custom" width="100">
        </el-table-column>

        <el-table-column prop="username" label="用户名" min-width="130" show-overflow-tooltip>
          <template slot-scope="scope">
            {{ userLabel(scope.row.username, scope.row.nickname) }}
          </template>
        </el-table-column>

        <el-table-column prop="group_name" label="用户组" min-width="110" show-overflow-tooltip>
        </el-table-column>

        <el-table-column prop="src" label="源IP地址" width="180" show-overflow-tooltip>
        </el-table-column>

        <el-table-column prop="dst" label="目的IP地址" width="180" show-overflow-tooltip>
        </el-table-column>

        <el-table-column prop="dst_port" label="目的端口" width="85">
        </el-table-column>

        <el-table-column prop="access_proto" label="访问协议" width="80" :formatter="protoFormat">
        </el-table-column>

        <el-table-column label="访问目标" min-width="240" show-overflow-tooltip :formatter="targetFormat">
        </el-table-column>

        <el-table-column prop="created_at" label="创建时间" width="160" :formatter="tableDateFormat">
        </el-table-column>
      </el-table>
    </div>

    <div class="sh-20"></div>

    <el-pagination background layout="prev, pager, next" :pager-count="11" :current-page.sync="currentPage"
      @current-change="pageChange" :total="count">
    </el-pagination>
  </div>
</template>

<script>
import axios from "axios";
import userLabel from "@/mixins/userLabel";

export default {
  name: "auditAccess",
  mixins: [userLabel],
  data() {
    return {
      tableData: [],
      count: 10,
      currentPage: 1,
      idSort: 1,
      activeName: "first",
      accessProtoArr: ["", "UDP", "TCP", "HTTPS", "HTTP", "DNS"],
      defSearchForm: { username: '', group_name: '', src: '', dst: '', dst_port: '', access_proto: '', info: '', date: ["", ""] },
      searchForm: {},
      access_proto: [
        { text: 'UDP', value: '1' },
        { text: 'TCP', value: '2' },
        { text: 'HTTPS', value: '3' },
        { text: 'HTTP', value: '4' },
        { text: 'DNS', value: '5' },
      ],
      maxExportNum: 1000000,
      loading: false,
      autoRefresh: false,
      refreshInterval: 10,
      timer: null,
      rules: {
        username: [
          { max: 30, message: '长度小于 30 个字符', trigger: 'blur' }
        ],
        src: [
          { message: '请输入正确的IP地址', validator: this.validateIP, trigger: 'blur' },
        ],
        dst: [
          { message: '请输入正确的IP地址', validator: this.validateIP, trigger: 'blur' },
        ],
      },
    }
  },
  watch: {
    idSort: {
      handler(newValue, oldValue) {
        if (newValue != oldValue) {
          this.getData(1);
        }
      },
    },
  },
  methods: {
    setSearchData() {
      this.searchForm = JSON.parse(JSON.stringify(this.defSearchForm));
    },
    handleSearch() {
      this.$refs["searchForm"].validate((valid) => {
        if (!valid) {
          return false;
        }
        this.getData(1)
      })
    },
    searchEnterFun(e) {
      var keyCode = window.event ? e.keyCode : e.which;
      if (keyCode == 13) {
        this.handleSearch()
      }
    },
    getData(p) {
      this.loading = true
      if (!this.searchForm.date) {
        this.searchForm.date = ["", ""];
      }
      this.searchForm.sort = this.idSort
      axios.get('/set/audit/list', {
        params: {
          page: p,
          search: this.searchForm,
        }
      }).then(resp => {
        var data = resp.data.data
        this.tableData = data.datas;
        this.count = data.count
        this.currentPage = p;
      }).catch(() => {
        this.$message.error('哦，请求出错');
      }).finally(() => {
        this.loading = false
      });
    },
    pageChange(p) {
      this.getData(p)
    },
    toggleAutoRefresh(val) {
      if (this.timer) {
        clearInterval(this.timer)
        this.timer = null
      }
      if (val) {
        this.getData(this.currentPage)
        this.timer = setInterval(() => {
          this.getData(this.currentPage)
        }, this.refreshInterval * 1000)
      }
    },
    handleExport() {
      if (this.count > this.maxExportNum) {
        var formatNum = (this.maxExportNum + "").replace(/\d{1,3}(?=(\d{3})+$)/g, function (s) {
          return s + ','
        })
        this.$message.error("你导出的数据量超过" + formatNum + "条，请调整搜索条件，再导出");
        return;
      }
      if (!this.searchForm.date) {
        this.searchForm.date = ["", ""];
      }
      const exporting = this.$loading({
        lock: true,
        text: '玩命导出中，请稍等片刻...',
        spinner: 'el-icon-loading',
        background: 'rgba(0, 0, 0, 0.7)'
      });
      axios.get('/set/audit/export', {
        params: {
          search: this.searchForm,
        }
      }).then(resp => {
        var rdata = resp.data
        if (rdata.code && rdata.code != 0) {
          exporting.close();
          this.$message.error(rdata.msg);
          return;
        }
        exporting.close();
        this.$message.success("成功导出CSV文件")
        let csvData = 'data:text/csv;charset=utf-8,\uFEFF' + rdata
        this.createDownLoadClick(csvData, `remlink_audit_log_` + Date.parse(new Date()) + `.csv`)
      }).catch(() => {
        exporting.close();
        this.$message.error('哦，请求出错');
      });
    },
    createDownLoadClick(content, fileName) {
      const link = document.createElement('a')
      link.href = encodeURI(content)
      link.download = fileName
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
    },
    protoFormat(row) {
      var access_proto = row.access_proto
      if (row.access_proto == 0) {
        switch (row.protocol) {
          case 6: access_proto = 2; break;
          case 17: access_proto = 1; break;
        }
      }
      return this.accessProtoArr[access_proto]
    },
    targetFormat(row) {
      // 域名优先；无域名时回退为 IP:端口
      if (row.info && row.info !== "") {
        return row.info
      }
      return row.dst + (row.dst_port ? (":" + row.dst_port) : "")
    },
    rest() {
      this.setSearchData();
      this.handleSearch();
    },
    validateIP(rule, value, callback) {
      if (value === '' || typeof value === 'undefined' || value == null) {
        callback()
      } else {
        const reg = /^(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])\.(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])\.(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])\.(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])$/
        if ((!reg.test(value)) && value !== '') {
          callback(new Error('请输入正确的IP地址'))
        } else {
          callback()
        }
      }
    },
    sortChange(column) {
      let { order } = column;
      if (order === 'ascending') {
        this.idSort = 2;
      } else {
        this.idSort = 1;
      }
    },
  },
  beforeDestroy() {
    if (this.timer) {
      clearInterval(this.timer)
      this.timer = null
    }
  },
}
</script>

<style scoped>
.el-form-item {
  margin-bottom: 5px;
}

.el-table {
  font-size: 12px;
}

.search-form>>>.el-form-item__label {
  font-size: 12px;
}

.search-form>>>.el-table th {
  padding: 5px 0;
}

.sh-20 {
  height: 20px;
}

/* 表格滚动容器 */
.audit-table-wrap {
  overflow-x: auto;
  width: 100%;
}

/* 响应式：搜索表单换行 */
@media (max-width: 900px) {
  .search-form ::v-deep .el-form-item {
    display: block;
    margin-bottom: 8px;
    margin-right: 0;
  }

  .search-form ::v-deep .el-form-item__label {
    width: 80px;
    float: none;
    display: inline-block;
    text-align: left;
    line-height: 32px;
  }

  .search-form ::v-deep .el-form-item__content {
    margin-left: 0 !important;
    display: inline-block;
  }
}

@media (max-width: 600px) {
  .search-form ::v-deep .el-date-editor--datetimerange {
    width: 100%;
  }

  .search-form ::v-deep .el-date-editor .el-range-input {
    width: 40%;
  }

  .search-form ::v-deep .el-form-item__content .el-input {
    width: 100% !important;
  }
}
</style>
