<template>
  <div>
    <el-form :model="searchForm" ref="searchForm" :inline="true" class="search-form">
      <el-form-item label="操作类型:">
        <el-select size="mini" v-model="searchForm.op_type" clearable placeholder="全部" style="width: 130px">
          <el-option v-for="(item, index) in opTypes" :key="index" :label="item" :value="item"></el-option>
        </el-select>
      </el-form-item>
      <el-form-item label="日期范围:">
        <el-date-picker v-model="searchForm.dateRange" type="daterange" size="mini" range-separator="至"
          start-placeholder="开始日期" end-placeholder="结束日期" format="yyyy-MM-dd" value-format="yyyy-MM-dd"
          style="width: 240px">
        </el-date-picker>
      </el-form-item>
      <el-form-item label="关键字:">
        <el-input size="mini" v-model="searchForm.keyword" clearable placeholder="操作目标/详情" style="width: 180px"
          @keydown.enter.native="searchEnterFun"></el-input>
      </el-form-item>
      <el-form-item>
        <el-button size="mini" type="primary" icon="el-icon-search" @click="handleSearch">搜索</el-button>
        <el-button size="mini" icon="el-icon-refresh" @click="rest">重置</el-button>
      </el-form-item>
    </el-form>

    <el-table ref="multipleTable" :data="tableData" :default-sort="{ prop: 'id', order: 'descending' }"
      @sort-change="sortChange" :header-cell-style="{ backgroundColor: 'var(--bg-header)' }" border>

      <el-table-column prop="id" label="ID" sortable="custom" width="80"></el-table-column>
      <el-table-column prop="admin_user" label="管理员" width="120" sortable></el-table-column>
      <el-table-column prop="op_type" label="操作类型" width="110" sortable>
        <template slot-scope="{ row }">
          <el-tag size="small" type="primary" disable-transitions>{{ row.op_type }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="op_target" label="操作目标" min-width="150" sortable>
        <template slot-scope="{ row }">
          <span v-if="row.op_target">{{ row.op_target }}</span>
          <span v-else style="color:var(--text-placeholder)">-</span>
        </template>
      </el-table-column>
      <el-table-column prop="detail" label="详情" min-width="180" sortable></el-table-column>
      <el-table-column prop="client_ip" label="操作IP" width="130" sortable></el-table-column>
      <el-table-column prop="created_at" label="操作时间" width="155" :formatter="tableDateFormat"
        sortable></el-table-column>
    </el-table>
    <div class="sh-20"></div>
    <el-pagination background layout="prev, pager, next" :pager-count="11" @current-change="pageChange"
      :current-page="page" :total="count">
    </el-pagination>
  </div>
</template>

<script>
import axios from "axios";

export default {
  name: "AdminOpLog",
  data() {
    return {
      page: 1,
      tableData: [],
      idSort: 1,
      count: 10,
      searchForm: { op_type: '', dateRange: null, keyword: '' },
      opTypes: [],
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
    handleSearch() {
      this.getData(1)
    },
    pageChange(p) {
      this.getData(p)
    },
    searchEnterFun(e) {
      var keyCode = window.event ? e.keyCode : e.which;
      if (keyCode == 13) {
        this.handleSearch()
      }
    },
    getData(page) {
      this.page = page
      let sdate = ''
      let edate = ''
      if (this.searchForm.dateRange && this.searchForm.dateRange.length === 2) {
        sdate = this.searchForm.dateRange[0] || ''
        edate = this.searchForm.dateRange[1] || ''
      }
      axios.get('/set/audit/admin_op_log_list', {
        params: {
          page: page,
          op_type: this.searchForm.op_type || '',
          sdate: sdate,
          edate: edate,
          keyword: this.searchForm.keyword || '',
          sort: this.idSort,
        }
      }).then(resp => {
        var data = resp.data.data
        this.tableData = data.datas;
        this.count = data.count
        this.opTypes = data.opTypes || []
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    rest() {
      this.searchForm.op_type = "";
      this.searchForm.dateRange = null;
      this.searchForm.keyword = "";
      this.handleSearch();
    },
    sortChange(column) {
      let { order } = column;
      if (order === 'ascending') {
        this.idSort = 2;
      } else {
        this.idSort = 1;
      }
    },
  }
}
</script>

<style scoped>
.el-form-item {
  margin-bottom: 8px;
}

.el-table {
  font-size: 12px;
}

.search-form>>>.el-form-item__label {
  font-size: 12px;
}

::v-deep .el-table th {
  padding: 5px 0;
}

::v-deep .el-table td {
  padding: 5px 0;
}
</style>
