/**
 * DashboardPage - 仪表盘页面
 * 从 Alpine.js index.html 移植到 Vue 3
 */
window.VuePages = window.VuePages || {};

(function () {
  "use strict";

  var ref = Vue.ref;
  var reactive = Vue.reactive;
  var computed = Vue.computed;
  var onMounted = Vue.onMounted;
  var onUnmounted = Vue.onUnmounted;
  var inject = Vue.inject;

  window.VuePages.DashboardPage = {
    name: "DashboardPage",
    setup: function () {
      var toastStore = inject("toastStore");
      var headerActionsStore = inject("headerActionsStore");
      var formatUptime = VueUtils.formatUptime;

      // 状态
      var loading = ref(true);
      var refreshing = ref(false);
      var checkingHealth = ref(false);
      var refreshSeconds = ref(30);
      var refreshInterval = ref(null);

      var stats = reactive({
        uptime_seconds: 0,
        total_requests: 0,
        total_errors: 0,
      });
      var models = ref([]);
      var endpoints = ref([]);

      // 计算属性：健康端点文本
      var healthyEndpointsText = computed(function () {
        var healthy = endpoints.value.filter(function (e) {
          return e.status === "healthy";
        }).length;
        return healthy + "/" + endpoints.value.length;
      });

      // 获取模型对应的端点服务商信息
      function getModelEndpoints(modelName) {
        return endpoints.value
          .filter(function (ep) {
            return ep.name.endsWith("/" + modelName);
          })
          .map(function (ep) {
            return {
              name: ep.name,
              provider: ep.name.split("/")[0],
              status: ep.status || "unknown",
            };
          });
      }

      // 加载 UI 配置（获取刷新间隔）
      async function loadUIConfig() {
        try {
          var response = await VueApi.get("/api/config/ui");
          if (response.ok) {
            var config = await response.json();
            refreshSeconds.value = config.dashboard_refresh_seconds || 30;
          }
        } catch (error) {
          console.error("加载 UI 配置失败:", error);
        }
      }

      // 刷新状态数据
      async function refreshStatus() {
        refreshing.value = true;
        try {
          var response = await VueApi.get("/api/status");
          var data = await response.json();
          stats.uptime_seconds = data.uptime_seconds;
          stats.total_requests = data.total_requests;
          stats.total_errors = data.total_errors;
          models.value = data.models || [];
          endpoints.value = data.endpoints || [];
          if (!loading.value) {
            toastStore.success("状态已刷新");
          }
        } catch (error) {
          toastStore.error("获取状态失败: " + error.message);
        } finally {
          loading.value = false;
          refreshing.value = false;
        }
      }

      // 手动触发健康检查
      async function checkHealthNow() {
        checkingHealth.value = true;
        try {
          var response = await VueApi.post("/api/health/check-now");
          if (!response.ok) {
            var data = await response.json();
            throw new Error(data.detail || "检查失败");
          }
          toastStore.success("健康检查已完成");
          await refreshStatus();
        } catch (error) {
          toastStore.error("健康检查失败: " + error.message);
        } finally {
          checkingHealth.value = false;
        }
      }

      // 格式化最后检查时间
      function formatLastCheck(timeStr) {
        if (!timeStr) return "-";
        return new Date(timeStr).toLocaleString();
      }

      // 格式化响应时间
      function formatResponseTime(ms) {
        return (ms || 0).toFixed(0) + " ms";
      }

      // header 操作按钮组件（通过 store 注入 AppHeader，替代 Teleport）
      var DashboardHeaderActions = {
        name: "DashboardHeaderActions",
        setup: function () {
          return {
            refreshing: refreshing,
            checkingHealth: checkingHealth,
            refreshStatus: refreshStatus,
            checkHealthNow: checkHealthNow,
          };
        },
        template:
          '<button class="btn btn-primary" :disabled="refreshing" @click="refreshStatus()">\
              <svg v-show="refreshing" class="btn-spinner" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>\
              <span>{{ refreshing ? "刷新中..." : "刷新状态" }}</span>\
          </button>\
          <button class="btn" :disabled="checkingHealth" @click="checkHealthNow()">\
              <svg v-show="checkingHealth" class="btn-spinner" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>\
              <span>{{ checkingHealth ? "检查中..." : "检查健康状态" }}</span>\
          </button>',
      };

      onMounted(async function () {
        headerActionsStore.value = DashboardHeaderActions;
        await loadUIConfig();
        await refreshStatus();
        refreshInterval.value = setInterval(function () {
          refreshStatus();
        }, refreshSeconds.value * 1000);
      });

      onUnmounted(function () {
        headerActionsStore.value = null;
        if (refreshInterval.value) {
          clearInterval(refreshInterval.value);
        }
      });

      return {
        loading: loading,
        refreshing: refreshing,
        checkingHealth: checkingHealth,
        stats: stats,
        models: models,
        endpoints: endpoints,
        healthyEndpointsText: healthyEndpointsText,
        getModelEndpoints: getModelEndpoints,
        formatUptime: formatUptime,
        refreshStatus: refreshStatus,
        checkHealthNow: checkHealthNow,
        formatLastCheck: formatLastCheck,
        formatResponseTime: formatResponseTime,
      };
    },
    template:
      '\
<div class="dashboard">\
    <!-- 统计卡片 -->\
    <div class="stats-grid">\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">{{ formatUptime(stats.uptime_seconds) }}</span>\
                <span class="stat-label">运行时间</span>\
            </div>\
        </div>\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">{{ stats.total_requests.toLocaleString() }}</span>\
                <span class="stat-label">总请求数</span>\
            </div>\
        </div>\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">{{ stats.total_errors.toLocaleString() }}</span>\
                <span class="stat-label">错误数</span>\
            </div>\
        </div>\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">{{ healthyEndpointsText }}</span>\
                <span class="stat-label">健康端点</span>\
            </div>\
        </div>\
    </div>\
    <!-- 模型状态（网格布局） -->\
    <div class="section">\
        <h3>模型状态</h3>\
        <div v-show="loading" class="loading">加载中...</div>\
        <div v-show="!loading && models.length === 0" v-cloak class="empty">暂无模型</div>\
        <div v-show="!loading && models.length > 0" class="models-grid">\
            <div v-for="model in models" :key="model.name" :class="\'model-card model-card--\' + model.role">\
                <div class="model-card__header">\
                    <span class="model-card__name">{{ model.name }}</span>\
                    <span class="model-card__role">{{ model.role }}</span>\
                </div>\
                <div class="model-card__providers">\
                    <span v-for="ep in getModelEndpoints(model.name)" :key="ep.name"\
                          :class="\'model-card__provider model-card__provider--\' + ep.status"\
                          :title="ep.name + \' (\' + ep.status + \')\'">{{ ep.provider }}</span>\
                    <span v-show="getModelEndpoints(model.name).length === 0"\
                          class="model-card__provider model-card__provider--none">无服务商</span>\
                </div>\
            </div>\
        </div>\
    </div>\
    <!-- 端点详情 -->\
    <div class="section">\
        <h3>端点详情</h3>\
        <div class="table-container">\
            <table class="table">\
                <thead>\
                    <tr><th>端点</th><th>状态</th><th>当前连接</th><th>总请求</th><th>错误数</th><th>平均响应时间</th><th>最后检查</th></tr>\
                </thead>\
                <tbody>\
                    <tr v-show="loading"><td colspan="7" class="loading">加载中...</td></tr>\
                    <tr v-show="!loading && endpoints.length === 0" v-cloak><td colspan="7" class="empty">暂无端点</td></tr>\
                    <tr v-for="endpoint in endpoints" :key="endpoint.name">\
                        <td><strong>{{ endpoint.name }}</strong></td>\
                        <td><span :class="\'status-badge status-\' + endpoint.status">{{ endpoint.status }}</span></td>\
                        <td>{{ endpoint.current_connections }}</td>\
                        <td>{{ endpoint.total_requests.toLocaleString() }}</td>\
                        <td>{{ endpoint.total_errors }}</td>\
                        <td>{{ formatResponseTime(endpoint.avg_response_time_ms) }}</td>\
                        <td>{{ formatLastCheck(endpoint.last_check_time) }}</td>\
                    </tr>\
                </tbody>\
            </table>\
        </div>\
        <!-- 移动端卡片 -->\
        <div class="endpoints-mobile">\
            <div v-show="loading" class="loading">加载中...</div>\
            <div v-show="!loading && endpoints.length === 0" v-cloak class="empty">暂无端点</div>\
            <div v-for="endpoint in endpoints" :key="endpoint.name" class="endpoint-card">\
                <div class="endpoint-card-header">\
                    <div class="endpoint-card-title">{{ endpoint.name }}</div>\
                    <span :class="\'status-badge status-\' + endpoint.status">{{ endpoint.status }}</span>\
                </div>\
                <div class="endpoint-card-body">\
                    <div class="endpoint-stat"><span class="endpoint-stat-label">总请求</span><span class="endpoint-stat-value">{{ endpoint.total_requests.toLocaleString() }}</span></div>\
                    <div class="endpoint-stat"><span class="endpoint-stat-label">错误数</span><span class="endpoint-stat-value">{{ endpoint.total_errors }}</span></div>\
                    <div class="endpoint-stat"><span class="endpoint-stat-label">响应时间</span><span class="endpoint-stat-value">{{ formatResponseTime(endpoint.avg_response_time_ms) }}</span></div>\
                    <div class="endpoint-stat"><span class="endpoint-stat-label">当前连接</span><span class="endpoint-stat-value">{{ endpoint.current_connections }}</span></div>\
                    <div class="endpoint-stat full-width"><span class="endpoint-stat-label">最后检查</span><span class="endpoint-stat-value">{{ formatLastCheck(endpoint.last_check_time) }}</span></div>\
                </div>\
            </div>\
        </div>\
    </div>\
</div>',
  };
})();
