/**
 * CacheMonitorPage - 缓存监控页面
 * 从 Alpine.js cache_monitor.html 移植到 Vue 3
 */
window.VuePages = window.VuePages || {};

(function () {
  "use strict";

  var ref = Vue.ref;
  var reactive = Vue.reactive;
  var onMounted = Vue.onMounted;
  var onUnmounted = Vue.onUnmounted;
  var inject = Vue.inject;
  var nextTick = Vue.nextTick;

  window.VuePages.CacheMonitorPage = {
    name: "CacheMonitorPage",
    setup: function () {
      var toastStore = inject("toastStore");
      var confirmStore = inject("confirmStore");
      var headerActionsStore = inject("headerActionsStore");

      // Chart.js 实例存储在闭包变量中，不进入响应式系统
      var chartInstance = null;

      // 状态
      var loading = ref(true);
      var loadingEntries = ref(true);
      var hitRateChart = ref(null);

      // 统计数据
      var stats = reactive({
        overall_hit_rate: 0,
        total_requests: 0,
        total_hits: 0,
        llm_calls: 0,
        l1: { hit_rate: 0, hits: 0, misses: 0, size: 0 },
        l2: { hit_rate: 0, hits: 0, misses: 0 },
        l3: { hit_rate: 0, hits: 0, misses: 0 },
        llm: { calls: 0, errors: 0, error_rate: 0, avg_latency_ms: 0 },
        efficiency: { llm_calls_saved: 0, estimated_time_saved_ms: 0 },
      });

      // 图表控制
      var chartPeriod = ref("24h");
      var chartInterval = ref("15m");
      var chartHasData = ref(false);
      var periodOpen = ref(false);
      var intervalOpen = ref(false);
      var periodOptions = [
        { value: "1h", label: "1 小时" },
        { value: "6h", label: "6 小时" },
        { value: "24h", label: "24 小时" },
        { value: "7d", label: "7 天" },
      ];
      var intervalOptions = [
        { value: "1m", label: "1 分钟" },
        { value: "5m", label: "5 分钟" },
        { value: "15m", label: "15 分钟" },
        { value: "1h", label: "1 小时" },
      ];

      // 缓存条目
      var entries = ref([]);
      var totalEntries = ref(0);
      var entriesSort = ref("hit_count");
      var sortOpen = ref(false);
      var sortOptions = [
        { value: "hit_count", label: "按命中次数" },
        { value: "created_at", label: "按创建时间" },
        { value: "last_hit_at", label: "按最后命中" },
      ];

      // 清除缓存模态框
      var showClearModal = ref(false);
      var clearLayer = ref("memory");
      var clearStatsOption = ref(false);

      // 自动刷新
      var refreshTimer = null;

      // --- 工具函数 ---
      function formatTimestamp(ts) {
        if (!ts) return "-";
        var date = new Date(ts);
        return date.toLocaleString("zh-CN", {
          month: "2-digit",
          day: "2-digit",
          hour: "2-digit",
          minute: "2-digit",
        });
      }

      function formatTime(ms) {
        if (!ms || ms === 0) return "0 ms";
        if (ms < 1000) return ms.toFixed(0) + " ms";
        if (ms < 60000) return (ms / 1000).toFixed(1) + " s";
        return (ms / 60000).toFixed(1) + " min";
      }

      function truncate(str, len) {
        if (!str) return "";
        return str.length > len ? str.substring(0, len) + "..." : str;
      }

      // 下拉选项标签查找
      function periodLabel() {
        var found = periodOptions.find(function (o) {
          return o.value === chartPeriod.value;
        });
        return found ? found.label : "请选择";
      }
      function intervalLabel() {
        var found = intervalOptions.find(function (o) {
          return o.value === chartInterval.value;
        });
        return found ? found.label : "请选择";
      }
      function sortLabel() {
        var found = sortOptions.find(function (o) {
          return o.value === entriesSort.value;
        });
        return found ? found.label : "请选择";
      }

      // --- 初始化图表 ---
      function initChart() {
        var canvas = hitRateChart.value;
        if (!canvas) return;
        var ctx = canvas.getContext("2d");
        chartInstance = new Chart(ctx, {
          type: "line",
          data: {
            labels: [],
            datasets: [
              {
                label: "L1 内存缓存",
                data: [],
                borderColor: "#4f46e5",
                backgroundColor: "rgba(79, 70, 229, 0.1)",
                tension: 0.3,
                fill: true,
              },
              {
                label: "L2 精确匹配",
                data: [],
                borderColor: "#10b981",
                backgroundColor: "rgba(16, 185, 129, 0.1)",
                tension: 0.3,
                fill: true,
              },
              {
                label: "L3 语义缓存",
                data: [],
                borderColor: "#f59e0b",
                backgroundColor: "rgba(245, 158, 11, 0.1)",
                tension: 0.3,
                fill: true,
              },
            ],
          },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: { intersect: false, mode: "index" },
            plugins: {
              legend: { position: "top" },
              tooltip: {
                callbacks: {
                  label: function (context) {
                    return (
                      context.dataset.label +
                      ": " +
                      context.parsed.y.toFixed(1) +
                      "%"
                    );
                  },
                },
              },
            },
            scales: {
              y: {
                beginAtZero: true,
                max: 100,
                ticks: {
                  callback: function (value) {
                    return value + "%";
                  },
                },
              },
              x: {
                ticks: { maxTicksLimit: 12 },
              },
            },
          },
        });
      }

      // --- 数据加载 ---
      async function loadStats() {
        try {
          var response = await VueApi.get("/api/cache/stats");
          if (!response.ok) throw new Error("获取统计数据失败");
          var data = await response.json();
          stats.overall_hit_rate =
            (data.summary && data.summary.overall_hit_rate) || 0;
          stats.total_requests =
            (data.summary && data.summary.total_requests) || 0;
          stats.total_hits = (data.summary && data.summary.total_hits) || 0;
          stats.llm_calls = (data.llm && data.llm.calls) || 0;
          var bl = data.by_layer || {};
          stats.l1.hit_rate = (bl.l1 && bl.l1.hit_rate) || 0;
          stats.l1.hits = (bl.l1 && bl.l1.hits) || 0;
          stats.l1.misses = (bl.l1 && bl.l1.misses) || 0;
          stats.l1.size = (bl.l1 && bl.l1.size) || 0;
          stats.l2.hit_rate = (bl.l2 && bl.l2.hit_rate) || 0;
          stats.l2.hits = (bl.l2 && bl.l2.hits) || 0;
          stats.l2.misses = (bl.l2 && bl.l2.misses) || 0;
          stats.l3.hit_rate = (bl.l3 && bl.l3.hit_rate) || 0;
          stats.l3.hits = (bl.l3 && bl.l3.hits) || 0;
          stats.l3.misses = (bl.l3 && bl.l3.misses) || 0;
          var llm = data.llm || {};
          stats.llm.calls = llm.calls || 0;
          stats.llm.errors = llm.errors || 0;
          stats.llm.error_rate = llm.error_rate || 0;
          stats.llm.avg_latency_ms = llm.avg_latency_ms || 0;
          var eff = data.cache_efficiency || {};
          stats.efficiency.llm_calls_saved = eff.llm_calls_saved || 0;
          stats.efficiency.estimated_time_saved_ms =
            eff.estimated_time_saved_ms || 0;
        } catch (error) {
          console.error("加载统计数据失败:", error);
        }
      }

      async function updateChart() {
        if (!chartInstance) return;
        try {
          var response = await VueApi.get(
            "/api/cache/stats/timeseries?period=" +
              chartPeriod.value +
              "&interval=" +
              chartInterval.value,
          );
          if (!response.ok) throw new Error("获取时间序列数据失败");
          var data = await response.json();
          var points = data.data_points || [];
          if (points.length === 0) {
            chartHasData.value = false;
            return;
          }
          chartHasData.value = true;
          chartInstance.data.labels = points.map(function (p) {
            return formatTimestamp(p.timestamp);
          });
          chartInstance.data.datasets[0].data = points.map(function (p) {
            return p.l1_hit_rate;
          });
          chartInstance.data.datasets[1].data = points.map(function (p) {
            return p.l2_hit_rate;
          });
          chartInstance.data.datasets[2].data = points.map(function (p) {
            return p.l3_hit_rate;
          });
          chartInstance.update();
        } catch (error) {
          console.error("更新图表失败:", error);
        }
      }

      async function loadCacheEntries() {
        loadingEntries.value = true;
        try {
          var response = await VueApi.get(
            "/api/cache/entries?limit=20&sort_by=" + entriesSort.value,
          );
          if (!response.ok) throw new Error("获取缓存条目失败");
          var data = await response.json();
          totalEntries.value = data.total || 0;
          entries.value = data.entries || [];
        } catch (error) {
          console.error("加载缓存条目失败:", error);
        } finally {
          loadingEntries.value = false;
        }
      }

      // --- 操作 ---
      async function refreshData() {
        await Promise.all([loadStats(), updateChart(), loadCacheEntries()]);
      }

      async function resetStats() {
        var confirmed = await confirmStore.show({
          title: "重置统计",
          message: "确定要重置统计计数器吗？",
          detail: "历史数据不会受影响。",
          confirmText: "确认重置",
          type: "warning",
        });
        if (!confirmed) return;
        try {
          var response = await VueApi.post("/api/cache/stats/reset");
          if (!response.ok) throw new Error("重置失败");
          toastStore.success("统计计数器已重置");
          await loadStats();
        } catch (error) {
          toastStore.error("重置失败: " + error.message);
        }
      }

      async function clearCache() {
        try {
          var response = await VueApi.post(
            "/api/cache/clear?layer=" +
              clearLayer.value +
              "&clear_stats=" +
              clearStatsOption.value,
          );
          if (!response.ok) throw new Error("清除失败");
          var data = await response.json();
          toastStore.success(data.message);
          showClearModal.value = false;
          await refreshData();
        } catch (error) {
          toastStore.error("清除失败: " + error.message);
        }
      }

      // 选择周期后刷新图表
      function selectPeriod(val) {
        chartPeriod.value = val;
        periodOpen.value = false;
        updateChart();
      }
      function selectInterval(val) {
        chartInterval.value = val;
        intervalOpen.value = false;
        updateChart();
      }
      function selectSort(val) {
        entriesSort.value = val;
        sortOpen.value = false;
        loadCacheEntries();
      }

      // header 操作按钮组件
      var CacheHeaderActions = {
        name: "CacheHeaderActions",
        setup: function () {
          return {
            resetStats: resetStats,
            showClearModal: showClearModal,
            refreshData: refreshData,
          };
        },
        template:
          '<button class="btn" @click="resetStats()">重置统计</button>\
          <button class="btn btn-warning" @click="showClearModal = true">清除缓存</button>\
          <button class="btn btn-primary" @click="refreshData()">刷新数据</button>',
      };

      // --- 生命周期 ---
      onMounted(async function () {
        headerActionsStore.value = CacheHeaderActions;
        await nextTick();
        initChart();
        await refreshData();
        loading.value = false;
        refreshTimer = setInterval(function () {
          refreshData();
        }, 30000);
      });

      onUnmounted(function () {
        headerActionsStore.value = null;
        if (refreshTimer) {
          clearInterval(refreshTimer);
          refreshTimer = null;
        }
        if (chartInstance) {
          chartInstance.destroy();
          chartInstance = null;
        }
      });

      return {
        loading: loading,
        loadingEntries: loadingEntries,
        hitRateChart: hitRateChart,
        stats: stats,
        chartPeriod: chartPeriod,
        chartInterval: chartInterval,
        chartHasData: chartHasData,
        periodOpen: periodOpen,
        intervalOpen: intervalOpen,
        periodOptions: periodOptions,
        intervalOptions: intervalOptions,
        periodLabel: periodLabel,
        intervalLabel: intervalLabel,
        entries: entries,
        totalEntries: totalEntries,
        entriesSort: entriesSort,
        sortOpen: sortOpen,
        sortOptions: sortOptions,
        sortLabel: sortLabel,
        showClearModal: showClearModal,
        clearLayer: clearLayer,
        clearStatsOption: clearStatsOption,
        refreshData: refreshData,
        resetStats: resetStats,
        clearCache: clearCache,
        selectPeriod: selectPeriod,
        selectInterval: selectInterval,
        selectSort: selectSort,
        formatTimestamp: formatTimestamp,
        formatTime: formatTime,
        truncate: truncate,
      };
    },
    template:
      '\
<div class="cache-monitor-page">\
    <!-- 统计卡片区 -->\
    <div class="stats-grid">\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21.21 15.89A10 10 0 1 1 8 2.83"/><path d="M22 12A10 10 0 0 0 12 2v10z"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">{{ stats.overall_hit_rate }}%</span>\
                <span class="stat-label">总体命中率</span>\
            </div>\
        </div>\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">{{ stats.total_requests }}</span>\
                <span class="stat-label">总请求数</span>\
            </div>\
        </div>\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">{{ stats.total_hits }}</span>\
                <span class="stat-label">缓存命中</span>\
            </div>\
        </div>\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16" rx="2" ry="2"/><rect x="9" y="9" width="6" height="6"/><line x1="9" y1="1" x2="9" y2="4"/><line x1="15" y1="1" x2="15" y2="4"/><line x1="9" y1="20" x2="9" y2="23"/><line x1="15" y1="20" x2="15" y2="23"/><line x1="20" y1="9" x2="23" y2="9"/><line x1="20" y1="14" x2="23" y2="14"/><line x1="1" y1="9" x2="4" y2="9"/><line x1="1" y1="14" x2="4" y2="14"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">{{ stats.llm_calls }}</span>\
                <span class="stat-label">LLM 调用</span>\
            </div>\
        </div>\
    </div>\
    <!-- 两列布局：图表 + 缓存层统计 -->\
    <div class="cache-main-grid">\
        <!-- 左侧：命中率曲线图 -->\
        <div class="section">\
            <div class="section-header">\
                <h3>命中率趋势</h3>\
                <div class="chart-controls">\
                    <div class="custom-select">\
                        <button type="button" class="custom-select-trigger" :class="{ open: periodOpen }" @click="periodOpen = !periodOpen">\
                            <span>{{ periodLabel() }}</span>\
                            <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"></polyline></svg>\
                        </button>\
                        <div class="custom-select-dropdown" v-show="periodOpen" v-cloak>\
                            <button type="button" class="custom-select-option" v-for="opt in periodOptions" :key="opt.value" :class="{ selected: chartPeriod === opt.value }" @click="selectPeriod(opt.value)">{{ opt.label }}</button>\
                        </div>\
                    </div>\
                    <div class="custom-select">\
                        <button type="button" class="custom-select-trigger" :class="{ open: intervalOpen }" @click="intervalOpen = !intervalOpen">\
                            <span>{{ intervalLabel() }}</span>\
                            <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"></polyline></svg>\
                        </button>\
                        <div class="custom-select-dropdown" v-show="intervalOpen" v-cloak>\
                            <button type="button" class="custom-select-option" v-for="opt in intervalOptions" :key="opt.value" :class="{ selected: chartInterval === opt.value }" @click="selectInterval(opt.value)">{{ opt.label }}</button>\
                        </div>\
                    </div>\
                </div>\
            </div>\
            <div class="chart-container" v-show="chartHasData">\
                <canvas ref="hitRateChart"></canvas>\
            </div>\
            <div class="empty" v-show="!chartHasData" v-cloak>\
                <p>暂无历史数据，统计数据每分钟自动保存一次</p>\
            </div>\
        </div>\
        <!-- 右侧：分层缓存统计 -->\
        <div class="cache-layers-column">\
            <!-- L1: 内存缓存 -->\
            <div class="cache-layer-card">\
                <div class="layer-header">\
                    <div class="layer-title">\
                        <span class="layer-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/></svg></span>\
                        <h4>L1: 内存缓存</h4>\
                    </div>\
                    <span class="badge badge-primary">{{ stats.l1.hit_rate }}%</span>\
                </div>\
                <div class="layer-stats">\
                    <div class="layer-stat"><span class="value">{{ stats.l1.hits }}</span><span class="label">命中</span></div>\
                    <div class="layer-stat"><span class="value">{{ stats.l1.misses }}</span><span class="label">未命中</span></div>\
                    <div class="layer-stat"><span class="value">{{ stats.l1.size }}</span><span class="label">缓存数</span></div>\
                </div>\
                <div class="layer-bar"><div class="layer-bar-fill" :style="{ width: stats.l1.hit_rate + \'%\' }"></div></div>\
            </div>\
            <!-- L2: 精确匹配 -->\
            <div class="cache-layer-card">\
                <div class="layer-header">\
                    <div class="layer-title">\
                        <span class="layer-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="2"/></svg></span>\
                        <h4>L2: 精确匹配</h4>\
                    </div>\
                    <span class="badge badge-success">{{ stats.l2.hit_rate }}%</span>\
                </div>\
                <div class="layer-stats">\
                    <div class="layer-stat"><span class="value">{{ stats.l2.hits }}</span><span class="label">命中</span></div>\
                    <div class="layer-stat"><span class="value">{{ stats.l2.misses }}</span><span class="label">未命中</span></div>\
                    <div class="layer-stat"><span class="value">-</span><span class="label">-</span></div>\
                </div>\
                <div class="layer-bar"><div class="layer-bar-fill layer-bar-l2" :style="{ width: stats.l2.hit_rate + \'%\' }"></div></div>\
            </div>\
            <!-- L3: 语义缓存 -->\
            <div class="cache-layer-card">\
                <div class="layer-header">\
                    <div class="layer-title">\
                        <span class="layer-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2a10 10 0 0 1 7.07 17.07l-2.12-2.12a7 7 0 1 0-9.9 0l-2.12 2.12A10 10 0 0 1 12 2z"/><circle cx="12" cy="12" r="3"/><path d="M12 2v7m0 6v7M2 12h7m6 0h7"/></svg></span>\
                        <h4>L3: 语义缓存</h4>\
                    </div>\
                    <span class="badge badge-warning">{{ stats.l3.hit_rate }}%</span>\
                </div>\
                <div class="layer-stats">\
                    <div class="layer-stat"><span class="value">{{ stats.l3.hits }}</span><span class="label">命中</span></div>\
                    <div class="layer-stat"><span class="value">{{ stats.l3.misses }}</span><span class="label">未命中</span></div>\
                    <div class="layer-stat"><span class="value">-</span><span class="label">-</span></div>\
                </div>\
                <div class="layer-bar"><div class="layer-bar-fill layer-bar-l3" :style="{ width: stats.l3.hit_rate + \'%\' }"></div></div>\
            </div>\
        </div>\
    </div>\
    <!-- LLM 调用统计 -->\
    <div class="section">\
        <h3>LLM 调用统计</h3>\
        <div class="llm-stats-grid">\
            <div class="llm-stat-item"><span class="llm-stat-value">{{ stats.llm.calls }}</span><span class="llm-stat-label">总调用次数</span></div>\
            <div class="llm-stat-item"><span class="llm-stat-value text-danger">{{ stats.llm.errors }}</span><span class="llm-stat-label">错误次数</span></div>\
            <div class="llm-stat-item"><span class="llm-stat-value">{{ stats.llm.error_rate }}%</span><span class="llm-stat-label">错误率</span></div>\
            <div class="llm-stat-item"><span class="llm-stat-value">{{ (stats.llm.avg_latency_ms || 0).toFixed(1) }} ms</span><span class="llm-stat-label">平均延迟</span></div>\
            <div class="llm-stat-item"><span class="llm-stat-value text-success">{{ stats.efficiency.llm_calls_saved }}</span><span class="llm-stat-label">节省调用</span></div>\
            <div class="llm-stat-item"><span class="llm-stat-value text-success">{{ formatTime(stats.efficiency.estimated_time_saved_ms) }}</span><span class="llm-stat-label">节省时间</span></div>\
        </div>\
    </div>\
    <!-- 热门缓存条目 -->\
    <div class="section">\
        <div class="section-header">\
            <div class="section-header-text"><h3>热门缓存</h3></div>\
            <div class="section-header-action">\
                <div class="custom-select">\
                    <button type="button" class="custom-select-trigger" :class="{ open: sortOpen }" @click="sortOpen = !sortOpen">\
                        <span>{{ sortLabel() }}</span>\
                        <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"></polyline></svg>\
                    </button>\
                    <div class="custom-select-dropdown" v-show="sortOpen" v-cloak>\
                        <button type="button" class="custom-select-option" v-for="opt in sortOptions" :key="opt.value" :class="{ selected: entriesSort === opt.value }" @click="selectSort(opt.value)">{{ opt.label }}</button>\
                    </div>\
                </div>\
            </div>\
        </div>\
        <div class="table-container">\
            <table class="table">\
                <thead><tr><th>内容预览</th><th>任务类型</th><th>命中次数</th><th>创建时间</th><th>最后命中</th></tr></thead>\
                <tbody>\
                    <tr v-show="loadingEntries"><td colspan="5" class="loading">加载中...</td></tr>\
                    <tr v-show="!loadingEntries && entries.length === 0" v-cloak><td colspan="5" class="empty">暂无缓存数据</td></tr>\
                    <tr v-for="entry in entries" :key="entry.id || entry.content_preview">\
                        <td class="content-preview" :title="entry.content_preview">{{ truncate(entry.content_preview, 50) }}</td>\
                        <td><span :class="\'role-\' + entry.task_type">{{ entry.task_type }}</span></td>\
                        <td>{{ entry.hit_count }}</td>\
                        <td>{{ formatTimestamp(entry.created_at) }}</td>\
                        <td>{{ entry.last_hit_at ? formatTimestamp(entry.last_hit_at) : \'-\' }}</td>\
                    </tr>\
                </tbody>\
            </table>\
        </div>\
        <div class="text-muted text-sm mt-2">共 {{ totalEntries }} 条缓存记录</div>\
    </div>\
    <!-- 清除缓存模态框 -->\
    <div class="modal" v-show="showClearModal" v-cloak @keydown.escape="showClearModal = false">\
        <div class="modal-content" @click.self="showClearModal = false">\
            <div class="modal-header">\
                <h3>清除缓存</h3>\
                <button class="modal-close" @click="showClearModal = false">&times;</button>\
            </div>\
            <div class="modal-body">\
                <p class="mb-4">选择要清除的缓存层级：</p>\
                <div class="radio-group">\
                    <label class="radio-label"><input type="radio" v-model="clearLayer" value="memory"> 仅内存缓存 (L1)</label>\
                    <label class="radio-label"><input type="radio" v-model="clearLayer" value="persistent"> 仅持久化缓存 (L2/L3)</label>\
                    <label class="radio-label"><input type="radio" v-model="clearLayer" value="all"> 所有缓存</label>\
                </div>\
                <div class="checkbox-section">\
                    <label class="checkbox-label"><input type="checkbox" v-model="clearStatsOption"> 同时清除统计数据（命中趋势图）</label>\
                </div>\
                <div class="help-text warning">注意：清除缓存后，后续请求将需要重新调用 LLM 进行路由决策</div>\
            </div>\
            <div class="modal-footer">\
                <button class="btn btn-secondary" @click="showClearModal = false">取消</button>\
                <button class="btn btn-danger" @click="clearCache()">确认清除</button>\
            </div>\
        </div>\
    </div>\
</div>',
  };
})();
