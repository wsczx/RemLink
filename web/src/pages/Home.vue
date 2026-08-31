<template>
  <div class="home">

    <!-- 统计卡片行 -->
    <el-row :gutter="24" class="stat-cards">
      <el-col :span="6">
        <div class="stat-card stat-card--online" @click="jump('/admin/user/online')">
          <div class="stat-card-icon">
            <i class="el-icon-user-solid"></i>
          </div>
          <div class="stat-card-body">
            <div class="stat-card-label">当前在线</div>
            <countTo :startVal="0" :endVal="counts.online" :duration="1500" class="stat-card-value" />
          </div>
        </div>
      </el-col>

      <el-col :span="6">
        <div class="stat-card stat-card--users" @click="jump('/admin/user/list')">
          <div class="stat-card-icon">
            <i class="el-icon-s-custom"></i>
          </div>
          <div class="stat-card-body">
            <div class="stat-card-label">用户总数</div>
            <countTo :startVal="0" :endVal="counts.user" :duration="1500" class="stat-card-value" />
          </div>
        </div>
      </el-col>

      <el-col :span="6">
        <div class="stat-card stat-card--groups" @click="jump('/admin/group/list')">
          <div class="stat-card-icon">
            <i class="el-icon-s-grid"></i>
          </div>
          <div class="stat-card-body">
            <div class="stat-card-label">用户组数</div>
            <countTo :startVal="0" :endVal="counts.group" :duration="1500" class="stat-card-value" />
          </div>
        </div>
      </el-col>

      <el-col :span="6">
        <div class="stat-card stat-card--ipmap" @click="jump('/admin/user/ip_map')">
          <div class="stat-card-icon">
            <i class="el-icon-connection"></i>
          </div>
          <div class="stat-card-body">
            <div class="stat-card-label">IP 映射数</div>
            <countTo :startVal="0" :endVal="counts.ip_map" :duration="1500" class="stat-card-value" />
          </div>
        </div>
      </el-col>
    </el-row>

    <!-- 图表区域 -->
    <el-row :gutter="20" class="chart-row">
      <el-col :span="12">
        <div class="chart-card">
          <div class="chart-card-header">
            <span class="chart-card-title">用户在线数</span>
            <el-select size="small" v-model="lineChartGroup.online" @change="lineChartGroupChange('online')" class="chart-group-select">
              <el-option v-for="(item,index) in groupNames" :key="index" :label="item.text" :value="item.value" />
            </el-select>
          </div>
          <div class="chart-card-toolbar">
            <el-radio-group v-model="lineChartScope.online" size="mini" @change="(l)=>lineChartScopeChange('online',l)">
              <el-radio-button label="rt">实时</el-radio-button>
              <el-radio-button label="1h">1小时</el-radio-button>
              <el-radio-button label="24h">24小时</el-radio-button>
              <el-radio-button label="7d">7天</el-radio-button>
              <el-radio-button label="30d">30天</el-radio-button>
            </el-radio-group>
          </div>
          <LineChart :chart-data="lineChart.online" />
        </div>
      </el-col>

      <el-col :span="12">
        <div class="chart-card">
          <div class="chart-card-header">
            <span class="chart-card-title">网络吞吐量</span>
            <el-select size="small" v-model="lineChartGroup.network" @change="lineChartGroupChange('network')" class="chart-group-select">
              <el-option v-for="(item,index) in groupNames" :key="index" :label="item.text" :value="item.value" />
            </el-select>
          </div>
          <div class="chart-card-toolbar">
            <el-radio-group v-model="lineChartScope.network" size="mini" @change="(l)=>lineChartScopeChange('network',l)">
              <el-radio-button label="rt">实时</el-radio-button>
              <el-radio-button label="1h">1小时</el-radio-button>
              <el-radio-button label="24h">24小时</el-radio-button>
              <el-radio-button label="7d">7天</el-radio-button>
              <el-radio-button label="30d">30天</el-radio-button>
            </el-radio-group>
          </div>
          <LineChart :chart-data="lineChart.network" />
        </div>
      </el-col>
    </el-row>

    <el-row :gutter="20" class="chart-row">
      <el-col :span="12">
        <div class="chart-card">
          <div class="chart-card-header">
            <span class="chart-card-title">CPU 使用率</span>
          </div>
          <div class="chart-card-toolbar">
            <el-radio-group v-model="lineChartScope.cpu" size="mini" @change="(l)=>lineChartScopeChange('cpu',l)">
              <el-radio-button label="rt">实时</el-radio-button>
              <el-radio-button label="1h">1小时</el-radio-button>
              <el-radio-button label="24h">24小时</el-radio-button>
              <el-radio-button label="7d">7天</el-radio-button>
              <el-radio-button label="30d">30天</el-radio-button>
            </el-radio-group>
          </div>
          <LineChart :chart-data="lineChart.cpu" />
        </div>
      </el-col>

      <el-col :span="12">
        <div class="chart-card">
          <div class="chart-card-header">
            <span class="chart-card-title">内存使用率</span>
          </div>
          <div class="chart-card-toolbar">
            <el-radio-group v-model="lineChartScope.mem" size="mini" @change="(l)=>lineChartScopeChange('mem',l)">
              <el-radio-button label="rt">实时</el-radio-button>
              <el-radio-button label="1h">1小时</el-radio-button>
              <el-radio-button label="24h">24小时</el-radio-button>
              <el-radio-button label="7d">7天</el-radio-button>
              <el-radio-button label="30d">30天</el-radio-button>
            </el-radio-group>
          </div>
          <LineChart :chart-data="lineChart.mem" />
        </div>
      </el-col>
    </el-row>

  </div>
</template>

<script>

import countTo from 'vue-count-to';
import LineChart from "@/components/LineChart";
import axios from "axios";

export default {
  name: "Home",
  components: {
    LineChart,
    countTo,
  },
  data() {
    return {
      counts: {
        online: 0,
        user: 0,
        group: 0,
        ip_map: 0,
      },
      groupNames:[],
      // 每张图各自维护最新请求序号，避免并发拉取时相互顶掉响应
      statsReqId: {
        online: 0,
        network: 0,
        cpu: 0,
        mem: 0,
      },
      lineChart: {
        online: {
            title: '用户在线数',
            xname: [],
            xdata: {
                '在线人数': [],
            },
            yminInterval: 1,
            yname:"人数"
        },
        network: {
            title: '网络吞吐量',
            xname: [],
            xdata: {
                '下行流量': [],
                '上行流量': [],
            },
            yname:"Mbps"
        },
        cpu: {
            title: 'CPU占用比例',
            xname: [],
            xdata: {
                'CPU': [],
            },
            yname:"%"
        },
        mem: {
                title: '内存占用比例',
                xname: [],
                xdata: {
                    '内存': [],
                },
                yname:"%"
        }
      },
      lineChartScope : {
            online: "rt",
            network : "rt",
            cpu : "rt",
            mem : "rt"
      },
      lineChartGroup : {
            online: "",
            network: "",
      }
    }
  },
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['仪表盘'])
  },
  mounted() {
    this.getData()
    this.getGroups()
    this.getAllStats()
    const chartsTimer = setInterval(() => {
        this.getAllStats()
    }, 10000);
    this.$once('hook:beforeDestroy', () => {
      clearInterval(chartsTimer);
    })
  },
  methods: {
    getData() {
      axios.get('/set/home').then(resp => {
        var data = resp.data.data
        this.counts = data.counts
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    getAllStats() {
        for (var action in this.lineChartScope){
           if (this.lineChartScope[action] == "rt") {
               this.getStatsData(action);
           }
        }
    },
    getStatsData(action, scope) {
        if (!scope) {
            scope = "rt"
        }
        const reqId = ++this.statsReqId[action]
        let getData = {params:{"action": action, "scope": scope}}
        axios.get('/statsinfo/list', getData).then(resp => {
            if (reqId !== this.statsReqId[action]) return;
            var data = resp.data.data
            if (! data.datas) return ;
            data.action = action
            data.scope = scope
            switch(action) {
                case "online": this.formatOnline(data); break;
                case "network": this.formatNetwork(data); break;
                case "cpu": this.formatCpu(data); break;
                case "mem": this.formatMem(data); break;
            }
        }).catch((error) => {
            if (error.response && error.response.status === 401) {
               return ;
            }
            this.$message.error('请求出错');
        });
    },
    formatOnline(data) {
        let timeFormat = this.getTimeFormat(data.scope)
        let chartData = this.lineChart[data.action]
        let chooseGroup = this.lineChartGroup[data.action]
        let datas = data.datas
        let xnum = 0
        chartData.xname = []
        chartData.xdata["在线人数"] = []
        for(var i=0; i<datas.length;i++){
            chartData.xname[i] = this.dateFormat(datas[i].created_at, timeFormat)
            xnum = datas[i].num
            if (chooseGroup != "" && xnum > 0) {
                let num_groups = JSON.parse(datas[i].num_groups)
                xnum = ! num_groups[chooseGroup] ? 0 : num_groups[chooseGroup]
            }
            chartData.xdata["在线人数"][i] = xnum
        }
        if (data.scope == "rt" && chooseGroup == "" && datas.length > 0) {
            this.counts.online = datas[datas.length - 1].num
        }
        this.lineChart[data.action] = chartData
    },
    formatNetwork(data) {
        let timeFormat = this.getTimeFormat(data.scope)
        let chartData = this.lineChart[data.action]
        let chooseGroup = this.lineChartGroup[data.action]
        let datas = data.datas
        let xnumUp = 0, xnumDown = 0
        chartData.xname = []
        chartData.xdata["上行流量"] = []
        chartData.xdata["下行流量"] = []
        for(var i=0; i<datas.length;i++){
            chartData.xname[i] = this.dateFormat(datas[i].created_at, timeFormat)
            xnumUp = datas[i].up
            xnumDown = datas[i].down
            if (chooseGroup != "") {
                if (xnumUp > 0) {
                    let upGroups = JSON.parse(datas[i].up_groups)
                    xnumUp = ! upGroups[chooseGroup] ? 0 : upGroups[chooseGroup]
                }
                if (xnumDown > 0) {
                    let downGroups = JSON.parse(datas[i].down_groups)
                    xnumDown = ! downGroups[chooseGroup] ? 0 : downGroups[chooseGroup]
                }
            }
            chartData.xdata["上行流量"][i] = this.toMbps(xnumUp)
            chartData.xdata["下行流量"][i] = this.toMbps(xnumDown)
        }
        this.lineChart[data.action] = chartData
    },
    formatCpu(data) {
        let timeFormat = this.getTimeFormat(data.scope)
        let chartData = this.lineChart[data.action]
        let datas = data.datas
        chartData.xname = []
        chartData.xdata["CPU"] = []
        for(var i=0; i<datas.length;i++){
            chartData.xname[i] = this.dateFormat(datas[i].created_at, timeFormat)
            chartData.xdata["CPU"][i] = this.toDecimal(datas[i].percent)
        }
        this.lineChart[data.action] = chartData
    },
    formatMem(data) {
        let timeFormat = this.getTimeFormat(data.scope)
        let chartData = this.lineChart[data.action]
        let datas = data.datas
        chartData.xname = []
        chartData.xdata["内存"] = []
        for(var i=0; i<datas.length;i++){
            chartData.xname[i] = this.dateFormat(datas[i].created_at, timeFormat)
            chartData.xdata["内存"][i] = this.toDecimal(datas[i].percent)
        }
        this.lineChart[data.action] = chartData
    },
    getTimeFormat(scope) {
        return (scope == "rt" || scope == "1h" || scope == "24h") ? "h:i:s" : "m/d h:i:s"
    },
    toMbps(bytes) {
        if (bytes == 0) return 0
        return (bytes / Math.pow(1024, 2) * 8).toFixed(2) * 1
    },
    toDecimal(f) {
        return Math.floor(f * 100) / 100
    },
    lineChartScopeChange(action, label) {
        this.lineChartScope[action] = label;
        this.getStatsData(action, label);
    },
    dateFormat(p, format) {
        var da = new Date(p);
        var year = da.getFullYear();
        var month = da.getMonth() + 1;
        var dt = da.getDate();
        var h = ('0'+da.getHours()).slice(-2);
        var m = ('0'+da.getMinutes()).slice(-2)
        var s = ('0'+da.getSeconds()).slice(-2);
        switch (format) {
            case "h:i:s":  return h + ':' + m + ':' + s;
            case "m/d h:i:s":  return month + '/' + dt + ' ' + h + ':' + m + ':' + s;
        }
        return year + '-' + month + '-' + dt + ' ' + h + ':' + m + ':' + s;
    },
    jump(path) {
        this.$router.push(path);
    },
    getGroups() {
      axios.get('/group/names_ids', {}).then(resp => {
        var data = resp.data.data
        var groupNames = []
        groupNames[0] = {text:"全部", value:""}
        for(var i=0; i<data.datas.length;i++){
            groupNames[i+1] = {text:data.datas[i].name, value:data.datas[i].id}
        }
        this.groupNames = groupNames
      }).catch(() => {
        this.$message.error('请求出错');
      });
    },
    lineChartGroupChange(action) {
        this.getStatsData(action, this.lineChartScope[action]);
    }
  },
}
</script>

<style scoped>
/* ========== 统计卡片 ========== */
.stat-cards {
  margin-bottom: 20px;
}

.stat-card {
  display: flex;
  align-items: center;
  padding: 20px 24px;
  border-radius: 12px;
  cursor: pointer;
  transition: all var(--transition-normal);
  position: relative;
  overflow: hidden;
}
.stat-card::after {
  content: '';
  position: absolute;
  right: -20px;
  top: -20px;
  width: 100px;
  height: 100px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
  transition: all var(--transition-normal);
}
.stat-card:hover {
  transform: translateY(-3px);
  box-shadow: 0 8px 25px rgba(0, 0, 0, 0.12);
}
.stat-card:hover::after {
  transform: scale(1.2);
}

.stat-card--online {
  background: linear-gradient(135deg, #f4516c, #ff7a95);
  color: var(--text-inverse);
}
.stat-card--users {
  background: linear-gradient(135deg, #36a3f7, #66b8ff);
  color: var(--text-inverse);
}
.stat-card--groups {
  background: linear-gradient(135deg, #34bfa3, #5dd6bd);
  color: var(--text-inverse);
}
.stat-card--ipmap {
  background: linear-gradient(135deg, #40c9c6, #6bdfdd);
  color: var(--text-inverse);
}

.stat-card-icon {
  flex-shrink: 0;
  margin-right: 16px;
  z-index: 1;
}
.stat-card-icon i {
  font-size: 38px;
  opacity: 0.85;
}
.stat-card-body {
  z-index: 1;
}
.stat-card-label {
  font-size: 13px;
  opacity: 0.85;
  margin-bottom: 6px;
}
.stat-card-value {
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 1px;
}

/* ========== 图表卡片 ========== */
.chart-row {
  margin-bottom: 20px;
}

.chart-card {
  background: var(--bg-card);
  border-radius: var(--card-radius);
  box-shadow: var(--card-shadow);
  padding: 16px 20px;
  position: relative;
  transition: box-shadow var(--transition-normal);
}
.chart-card:hover {
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.08);
}

.chart-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}
.chart-card-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
}
.chart-group-select {
  width: 130px;
}

.chart-card-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 8px;
}

/* ========== 移动端适配 ========== */
@media (max-width: 768px) {
  .stat-cards .el-col {
    width: 50% !important;
    margin-bottom: 12px;
  }

  .stat-card {
    padding: 14px 16px;
  }

  .stat-card-icon i {
    font-size: 30px;
  }

  .stat-card-value {
    font-size: 22px;
  }

  .stat-card-label {
    font-size: 11px;
  }

  .chart-row .el-col {
    width: 100% !important;
    margin-bottom: 16px;
  }

  .chart-card {
    padding: 12px 14px;
  }

  .chart-card-title {
    font-size: 14px;
  }

  .chart-group-select {
    width: 100px;
  }

  .chart-card-toolbar {
    overflow-x: auto;
  }
}

@media (max-width: 480px) {
  .stat-cards .el-col {
    width: 100% !important;
  }
}
</style>
