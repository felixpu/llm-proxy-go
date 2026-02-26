/**
 * LogsPage - 请求日志页面
 * 从 Alpine.js logs.html 移植到 Vue 3
 * 统计卡片 + 筛选工具栏 + 日志表格 + 分页 + 详情弹窗 + 自动刷新
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

  // 路由方式映射
  var ROUTING_METHOD_MAP = {
    rule: "规则",
    cache_l1: "L1缓存",
    cache_l2: "L2缓存",
    llm: "LLM",
    fallback: "回退",
    unknown: "未知",
  };

  // 状态选项
  var STATUS_OPTIONS = [
    { value: "", label: "状态" },
    { value: "true", label: "成功" },
    { value: "false", label: "失败" },
  ];

  // 刷新间隔选项
  var REFRESH_OPTIONS = [
    { value: 0, label: "不刷新" },
    { value: 5, label: "5 秒" },
    { value: 15, label: "15 秒" },
    { value: 30, label: "30 秒" },
    { value: 60, label: "60 秒" },
  ];

  // HTML 转义
  function escapeHtml(text) {
    var map = {
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#x27;",
    };
    return String(text).replace(/[&<>"']/g, function (c) {
      return map[c];
    });
  }

  // RFC3339 格式转换（不含毫秒，兼容 Go time.RFC3339）
  function toLocalDateTimeString(date) {
    return date.toISOString().replace(/\.\d{3}Z$/, "Z");
  }

  window.VuePages.LogsPage = {
    name: "LogsPage",
    setup: function () {
      var toastStore = inject("toastStore");
      var confirmStore = inject("confirmStore");
      var headerActionsStore = inject("headerActionsStore");

      // === 状态 ===
      var loading = ref(true);
      var logs = ref([]);
      var total = ref(0);
      var currentPage = ref(0);
      var pageSize = ref(50);
      var timeRange = ref("day");
      var timeError = ref("");

      // 详情弹窗
      var showDetailModal = ref(false);
      var logDetail = ref(null);
      var detailLoading = ref(false);
      var markingInaccurate = ref(false);

      // 请求内容展开/收起
      var expandedRequest = ref(false);
      var highlightedRequest = ref("");
      var expandLoading = ref(false);

      // 自动刷新
      var autoRefreshInterval = ref(30);
      var refreshIntervalId = null;
      var debounceTimer = null;

      // 下拉菜单开关
      var modelOpen = ref(false);
      var endpointOpen = ref(false);
      var statusOpen = ref(false);
      var deleteOpen = ref(false);
      var refreshOpen = ref(false);
      var popoverOpen = ref(false);

      // 统计数据
      var stats = reactive({
        totalRequests: 0,
        totalCost: 0,
        avgLatency: 0,
        successRate: 0,
      });

      // 筛选器
      var filters = reactive({
        model: "",
        endpoint: "",
        startTime: "",
        endTime: "",
        success: "",
        customStartDate: "",
        customEndDate: "",
      });

      // 筛选选项（从 stats 接口获取）
      var filterOptions = reactive({
        models: [],
        endpoints: [],
      });

      // === 计算属性 ===
      var totalPages = computed(function () {
        return Math.ceil(total.value / pageSize.value);
      });

      var refreshLabel = computed(function () {
        return autoRefreshInterval.value === 0
          ? "不刷新"
          : autoRefreshInterval.value + "s";
      });

      var statusLabel = computed(function () {
        var found = STATUS_OPTIONS.find(function (o) {
          return o.value === filters.success;
        });
        return found ? found.label : "状态";
      });

      var hasActiveFilters = computed(function () {
        return filters.model || filters.endpoint || filters.success;
      });

      // === 方法 ===

      // 格式化日期
      function formatDateTime(isoString) {
        if (!isoString) return "-";
        return VueUtils.formatDateTime(isoString) || "-";
      }

      // 格式化路由方式
      function formatRoutingMethod(method) {
        return ROUTING_METHOD_MAP[method] || method || "-";
      }

      // 构建查询参数
      function buildFilterParams() {
        var params = new URLSearchParams();
        if (filters.model) params.append("model", filters.model);
        if (filters.endpoint) params.append("endpoint", filters.endpoint);
        if (filters.startTime) params.append("start_time", filters.startTime);
        if (filters.endTime) params.append("end_time", filters.endTime);
        if (filters.success) params.append("success", filters.success);
        return params;
      }

      // 加载日志列表
      async function loadLogs() {
        loading.value = true;
        try {
          var params = buildFilterParams();
          params.set("limit", pageSize.value);
          params.set("offset", currentPage.value * pageSize.value);
          var response = await VueApi.get("/api/logs?" + params);
          if (!response.ok) throw new Error("加载日志失败");
          var data = await response.json();
          logs.value = data.logs || [];
          total.value = data.total || 0;
        } catch (error) {
          toastStore.error("加载日志失败: " + error.message);
        } finally {
          loading.value = false;
        }
      }

      // 加载统计数据 + 筛选选项（合并为单次请求）
      async function loadStats() {
        try {
          var params = buildFilterParams();
          var response = await VueApi.get("/api/logs/stats?" + params);
          if (!response.ok) throw new Error("加载统计失败");
          var data = await response.json();
          stats.totalRequests = data.total_requests || 0;
          stats.totalCost = data.total_cost || 0;
          stats.avgLatency = data.avg_latency || 0;
          stats.successRate = data.success_rate || 0;
          // Extract filter options from the same response
          filterOptions.models = (data.by_model || []).map(function (item) {
            return item.model_name;
          });
          filterOptions.endpoints = (data.by_endpoint || []).map(
            function (item) {
              return item.endpoint_name;
            },
          );
        } catch (error) {
          console.error("加载统计失败:", error);
        }
      }

      // 加载 UI 配置（刷新间隔）
      async function loadUIConfig() {
        try {
          var response = await VueApi.get("/api/config/ui");
          if (response.ok) {
            var config = await response.json();
            autoRefreshInterval.value = config.logs_refresh_seconds || 30;
          }
        } catch (error) {
          // 忽略
        }
      }

      // 应用筛选
      function applyFilters() {
        currentPage.value = 0;
        loadLogs();
        loadStats();
      }

      // 防抖应用筛选
      function debouncedApply() {
        if (debounceTimer) clearTimeout(debounceTimer);
        debounceTimer = setTimeout(function () {
          applyFilters();
        }, 300);
      }

      // 刷新日志（手动 + 自动刷新共用）
      function refreshLogs() {
        loadLogs();
        loadStats();
      }

      // 启动自动刷新
      function startAutoRefresh() {
        stopAutoRefresh();
        if (autoRefreshInterval.value > 0) {
          refreshIntervalId = setInterval(function () {
            refreshLogs();
          }, autoRefreshInterval.value * 1000);
        }
      }

      // 停止自动刷新
      function stopAutoRefresh() {
        if (refreshIntervalId) {
          clearInterval(refreshIntervalId);
          refreshIntervalId = null;
        }
      }

      // 设置自动刷新间隔
      function setAutoRefresh(seconds) {
        autoRefreshInterval.value = seconds;
        startAutoRefresh();
        // 持久化到后端
        VueApi.put("/api/config/ui", { logs_refresh_seconds: seconds }).catch(
          function (e) {
            console.error("保存刷新间隔失败:", e);
          },
        );
      }

      // 时间范围切换
      function setTimeRange(range) {
        timeRange.value = range;
        timeError.value = "";
        var now = new Date();
        var todayEnd = new Date(
          now.getFullYear(),
          now.getMonth(),
          now.getDate(),
          23,
          59,
          59,
        );
        var endStr = toLocalDateTimeString(todayEnd);

        if (range === "day") {
          var today = new Date(
            now.getFullYear(),
            now.getMonth(),
            now.getDate(),
          );
          filters.startTime = toLocalDateTimeString(today);
          filters.endTime = endStr;
        } else if (range === "week") {
          var dayOfWeek = now.getDay() || 7;
          var monday = new Date(
            now.getFullYear(),
            now.getMonth(),
            now.getDate() - dayOfWeek + 1,
          );
          filters.startTime = toLocalDateTimeString(monday);
          filters.endTime = endStr;
        } else if (range === "month") {
          var firstDay = new Date(now.getFullYear(), now.getMonth(), 1);
          filters.startTime = toLocalDateTimeString(firstDay);
          filters.endTime = endStr;
        } else if (range === "year") {
          var janFirst = new Date(now.getFullYear(), 0, 1);
          filters.startTime = toLocalDateTimeString(janFirst);
          filters.endTime = endStr;
        } else {
          // custom: 保留当前值，不自动触发查询
          return;
        }
        applyFilters();
      }

      // 自定义时间确认（含边界校验）
      function applyCustomTime() {
        timeError.value = "";
        if (!filters.customEndDate) {
          timeError.value = "请设置结束日期";
          return;
        }
        if (filters.customStartDate && filters.customEndDate) {
          if (filters.customStartDate > filters.customEndDate) {
            timeError.value = "开始日期不能晚于结束日期";
            return;
          }
        }
        if (filters.customStartDate) {
          filters.startTime = new Date(filters.customStartDate + "T00:00:00")
            .toISOString()
            .replace(/\.\d{3}Z$/, "Z");
        } else {
          filters.startTime = "";
        }
        filters.endTime = new Date(filters.customEndDate + "T23:59:59")
          .toISOString()
          .replace(/\.\d{3}Z$/, "Z");
        applyFilters();
      }

      // 重置筛选器
      function resetFilters() {
        filters.model = "";
        filters.endpoint = "";
        filters.startTime = "";
        filters.endTime = "";
        filters.success = "";
        filters.customStartDate = "";
        filters.customEndDate = "";
        timeError.value = "";
        currentPage.value = 0;
        setTimeRange("day");
      }

      // 跳转页面
      function goToPage(page) {
        currentPage.value = page;
        loadLogs();
      }

      // 显示日志详情
      async function showLogDetailFn(logId) {
        showDetailModal.value = true;
        detailLoading.value = true;
        logDetail.value = null;
        // Reset expand state and cache
        expandedRequest.value = false;
        highlightedRequest.value = "";
        expandLoading.value = false;
        try {
          var response = await VueApi.get("/api/logs/" + logId);
          if (!response.ok) throw new Error("加载详情失败");
          logDetail.value = await response.json();
        } catch (error) {
          toastStore.error("加载详情失败: " + error.message);
          showDetailModal.value = false;
        } finally {
          detailLoading.value = false;
        }
      }

      // 切换不准确标记
      async function toggleInaccurate() {
        if (!logDetail.value) return;
        markingInaccurate.value = true;
        try {
          var newValue = !logDetail.value.is_inaccurate;
          var response = await VueApi.post(
            "/api/logs/" + logDetail.value.id + "/mark-inaccurate",
            { inaccurate: newValue },
          );
          if (!response.ok) throw new Error("操作失败");
          logDetail.value.is_inaccurate = newValue;
          // 更新列表中的对应记录
          var found = logs.value.find(function (l) {
            return l.id === logDetail.value.id;
          });
          if (found) found.is_inaccurate = newValue;
          toastStore.success(newValue ? "已标记为不准确" : "已取消标记");
        } catch (error) {
          toastStore.error("操作失败: " + error.message);
        } finally {
          markingInaccurate.value = false;
        }
      }

      // 确认删除日志
      async function confirmDeleteLogs(deleteAll) {
        var params = new URLSearchParams();
        var description = "";

        if (deleteAll) {
          description = "全部日志";
        } else {
          if (filters.model) {
            params.append("model", filters.model);
            description += "模型: " + escapeHtml(filters.model) + "; ";
          }
          if (filters.endpoint) {
            params.append("endpoint", filters.endpoint);
            description += "端点: " + escapeHtml(filters.endpoint) + "; ";
          }
          if (filters.startTime) {
            params.append("start_time", filters.startTime);
            description +=
              "开始时间: " +
              escapeHtml(filters.startTime.replace("T", " ")) +
              "; ";
          }
          if (filters.endTime) {
            params.append("end_time", filters.endTime);
            description +=
              "结束时间: " +
              escapeHtml(filters.endTime.replace("T", " ")) +
              "; ";
          }
          if (!description) {
            description = "全部日志（未设置筛选条件）";
          }
        }

        try {
          // 获取将要删除的日志数量
          var countParams = new URLSearchParams();
          if (!deleteAll) {
            if (filters.model) countParams.append("model", filters.model);
            if (filters.endpoint)
              countParams.append("endpoint", filters.endpoint);
            if (filters.startTime)
              countParams.append("start_time", filters.startTime);
            if (filters.endTime)
              countParams.append("end_time", filters.endTime);
          }

          var countResponse = await VueApi.get(
            "/api/logs?limit=1&offset=0&" + countParams,
          );
          if (!countResponse.ok) throw new Error("获取日志数量失败");
          var countData = await countResponse.json();

          var logCount;
          if (deleteAll) {
            var allResponse = await VueApi.get("/api/logs?limit=1&offset=0");
            var allData = await allResponse.json();
            logCount = allData.total;
          } else {
            logCount = countData.total;
          }

          if (logCount === 0) {
            toastStore.warning("没有符合条件的日志可删除");
            return;
          }

          var confirmed = await confirmStore.show({
            title: "删除日志",
            message: "确定要删除 " + logCount + " 条日志吗？",
            detail: "条件: " + description,
            confirmText: "确认删除",
            type: "danger",
          });
          if (!confirmed) return;

          var deleteResponse = await VueApi.delete(
            "/api/logs?" + (deleteAll ? "" : params.toString()),
          );
          if (!deleteResponse.ok) {
            var errData = await deleteResponse.json();
            throw new Error(errData.detail || "删除失败");
          }

          var result = await deleteResponse.json();
          toastStore.success("成功删除 " + result.deleted + " 条日志记录");

          currentPage.value = 0;
          await loadLogs();
          await loadStats();
        } catch (error) {
          toastStore.error("删除日志失败: " + error.message);
        }
      }

      // 关闭所有下拉菜单
      function closeAllDropdowns() {
        modelOpen.value = false;
        endpointOpen.value = false;
        statusOpen.value = false;
        deleteOpen.value = false;
        refreshOpen.value = false;
      }

      // header 操作按钮组件
      var LogsHeaderActions = {
        name: "LogsHeaderActions",
        setup: function () {
          return {
            refreshOpen: refreshOpen,
            refreshLabel: refreshLabel,
            autoRefreshInterval: autoRefreshInterval,
            REFRESH_OPTIONS: REFRESH_OPTIONS,
            setAutoRefresh: setAutoRefresh,
            refreshLogs: refreshLogs,
          };
        },
        template:
          '<div class="custom-select custom-select-compact" style="min-width:100px;">\
              <button type="button" class="custom-select-trigger" :class="{ open: refreshOpen }" @click.stop="refreshOpen = !refreshOpen">\
                  <svg class="select-prefix-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/></svg>\
                  <span>{{ refreshLabel }}</span>\
                  <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>\
              </button>\
              <div class="custom-select-dropdown" v-show="refreshOpen" v-cloak>\
                  <button type="button" v-for="opt in REFRESH_OPTIONS" :key="opt.value" class="custom-select-option" :class="{ selected: autoRefreshInterval === opt.value }" @click="setAutoRefresh(opt.value); refreshOpen = false">{{ opt.label }}</button>\
              </div>\
          </div>\
          <button class="icon-btn" @click="refreshLogs()" title="立即刷新">\
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>\
          </button>',
      };

      // 生命周期
      onMounted(async function () {
        headerActionsStore.value = LogsHeaderActions;
        document.addEventListener("click", closeAllDropdowns);
        await loadUIConfig();
        setTimeRange("day");
        startAutoRefresh();
      });

      onUnmounted(function () {
        headerActionsStore.value = null;
        document.removeEventListener("click", closeAllDropdowns);
        stopAutoRefresh();
        if (debounceTimer) {
          clearTimeout(debounceTimer);
        }
      });

      // Format JSON with syntax highlighting for v-html display
      function formatJsonHighlight(str) {
        return VueUtils.highlightJson(VueUtils.formatJsonPretty(str));
      }

      // Preview HTML with inline "...more" link
      var previewHtmlWithMore = computed(function () {
        if (!logDetail.value) return "";
        var preview = logDetail.value.message_preview;
        var html = preview ? formatJsonHighlight(preview) : "";
        if (logDetail.value.request_content) {
          if (!preview) {
            html =
              '<span style="color: var(--text-secondary);">(无预览)</span> ';
          }
          html +=
            '<span class="expand-inline-link" data-action="expand">...更多</span>';
        }
        return html;
      });

      // Toggle request content expand/collapse
      function toggleRequestExpand() {
        if (
          !expandedRequest.value &&
          !highlightedRequest.value &&
          logDetail.value
        ) {
          // Lazy highlight on first expand
          expandLoading.value = true;
          requestAnimationFrame(function () {
            highlightedRequest.value = formatJsonHighlight(
              logDetail.value.request_content,
            );
            expandedRequest.value = true;
            expandLoading.value = false;
          });
          return;
        }
        expandedRequest.value = !expandedRequest.value;
      }

      // Event delegation for "...more" click inside v-html
      function handlePreviewClick(event) {
        var target = event.target;
        if (
          target.classList.contains("expand-inline-link") &&
          target.dataset.action === "expand"
        ) {
          toggleRequestExpand();
        }
      }

      // 返回所有需要暴露的变量和方法
      return {
        // 状态
        loading: loading,
        logs: logs,
        total: total,
        currentPage: currentPage,
        pageSize: pageSize,
        timeRange: timeRange,
        timeError: timeError,
        showDetailModal: showDetailModal,
        logDetail: logDetail,
        detailLoading: detailLoading,
        markingInaccurate: markingInaccurate,
        autoRefreshInterval: autoRefreshInterval,
        // 下拉菜单
        modelOpen: modelOpen,
        endpointOpen: endpointOpen,
        statusOpen: statusOpen,
        deleteOpen: deleteOpen,
        refreshOpen: refreshOpen,
        popoverOpen: popoverOpen,
        // 数据
        stats: stats,
        filters: filters,
        filterOptions: filterOptions,
        // 计算属性
        totalPages: totalPages,
        refreshLabel: refreshLabel,
        statusLabel: statusLabel,
        hasActiveFilters: hasActiveFilters,
        // 常量
        STATUS_OPTIONS: STATUS_OPTIONS,
        REFRESH_OPTIONS: REFRESH_OPTIONS,
        // 方法
        formatDateTime: formatDateTime,
        formatRoutingMethod: formatRoutingMethod,
        loadLogs: loadLogs,
        loadStats: loadStats,
        applyFilters: applyFilters,
        debouncedApply: debouncedApply,
        refreshLogs: refreshLogs,
        setAutoRefresh: setAutoRefresh,
        setTimeRange: setTimeRange,
        applyCustomTime: applyCustomTime,
        resetFilters: resetFilters,
        goToPage: goToPage,
        showLogDetail: showLogDetailFn,
        toggleInaccurate: toggleInaccurate,
        confirmDeleteLogs: confirmDeleteLogs,
        formatJsonHighlight: formatJsonHighlight,
        expandedRequest: expandedRequest,
        highlightedRequest: highlightedRequest,
        expandLoading: expandLoading,
        previewHtmlWithMore: previewHtmlWithMore,
        toggleRequestExpand: toggleRequestExpand,
        handlePreviewClick: handlePreviewClick,
      };

      // __CONTINUE_TEMPLATE__
    },
    template:
      '\
<div>\
    <!-- 统计卡片 -->\
    <div class="stats-grid">\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">{{ (stats.totalRequests || 0).toLocaleString() }}</span>\
                <span class="stat-label">总请求数</span>\
            </div>\
        </div>\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="1" x2="12" y2="23"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">${{ (stats.totalCost || 0).toFixed(6) }}</span>\
                <span class="stat-label">总成本</span>\
            </div>\
        </div>\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">{{ (stats.avgLatency || 0).toFixed(0) + " ms" }}</span>\
                <span class="stat-label">平均延迟</span>\
            </div>\
        </div>\
        <div class="stat-card">\
            <div class="stat-icon">\
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>\
            </div>\
            <div class="stat-info">\
                <span class="stat-value">{{ (stats.successRate || 0).toFixed(1) + "%" }}</span>\
                <span class="stat-label">成功率</span>\
            </div>\
        </div>\
    </div>\
    <!-- 日志表格区 -->\
    <div class="section">\
        <div class="section-header">\
            <h3>请求记录</h3>\
            <span class="text-muted text-sm">共 {{ total }} 条记录</span>\
        </div>\
        <!-- 筛选工具栏 -->\
        <div class="filter-toolbar">\
            <div class="filter-toolbar-group">\
                <!-- 模型下拉 -->\
                <div class="custom-select custom-select-compact">\
                    <button type="button" class="custom-select-trigger" :class="{ open: modelOpen }" @click.stop="modelOpen = !modelOpen">\
                        <svg class="select-prefix-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>\
                        <span>{{ filters.model || "模型" }}</span>\
                        <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>\
                    </button>\
                    <div class="custom-select-dropdown" v-show="modelOpen" v-cloak>\
                        <button type="button" class="custom-select-option" :class="{ selected: filters.model === \'\'}" @click="filters.model = \'\'; modelOpen = false; debouncedApply()">所有模型</button>\
                        <button type="button" v-for="m in filterOptions.models" :key="m" class="custom-select-option" :class="{ selected: filters.model === m }" @click="filters.model = m; modelOpen = false; debouncedApply()">{{ m }}</button>\
                    </div>\
                </div>\
                <!-- 端点下拉 -->\
                <div class="custom-select custom-select-compact">\
                    <button type="button" class="custom-select-trigger" :class="{ open: endpointOpen }" @click.stop="endpointOpen = !endpointOpen">\
                        <svg class="select-prefix-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>\
                        <span>{{ filters.endpoint || "端点" }}</span>\
                        <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>\
                    </button>\
                    <div class="custom-select-dropdown" v-show="endpointOpen" v-cloak>\
                        <button type="button" class="custom-select-option" :class="{ selected: filters.endpoint === \'\' }" @click="filters.endpoint = \'\'; endpointOpen = false; debouncedApply()">所有端点</button>\
                        <button type="button" v-for="ep in filterOptions.endpoints" :key="ep" class="custom-select-option" :class="{ selected: filters.endpoint === ep }" @click="filters.endpoint = ep; endpointOpen = false; debouncedApply()">{{ ep }}</button>\
                    </div>\
                </div>\
                <!-- 状态下拉 -->\
                <div class="custom-select custom-select-compact">\
                    <button type="button" class="custom-select-trigger" :class="{ open: statusOpen }" @click.stop="statusOpen = !statusOpen">\
                        <svg class="select-prefix-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>\
                        <span>{{ statusLabel }}</span>\
                        <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>\
                    </button>\
                    <div class="custom-select-dropdown" v-show="statusOpen" v-cloak>\
                        <button type="button" v-for="opt in STATUS_OPTIONS" :key="opt.value" class="custom-select-option" :class="{ selected: filters.success === opt.value }" @click="filters.success = opt.value; statusOpen = false; debouncedApply()">{{ opt.label }}</button>\
                    </div>\
                </div>\
            </div>\
            <div class="filter-toolbar-divider"></div>\
            <!-- 时间分段控件 -->\
            <div class="segment-control">\
                <button class="segment-btn" :class="{ active: timeRange === \'day\' }" @click="setTimeRange(\'day\')">今天</button>\
                <button class="segment-btn" :class="{ active: timeRange === \'week\' }" @click="setTimeRange(\'week\')">本周</button>\
                <button class="segment-btn" :class="{ active: timeRange === \'month\' }" @click="setTimeRange(\'month\')">本月</button>\
                <button class="segment-btn" :class="{ active: timeRange === \'year\' }" @click="setTimeRange(\'year\')">今年</button>\
                <div class="segment-btn-wrapper">\
                    <button class="segment-btn" :class="{ active: timeRange === \'custom\' }" @click="setTimeRange(\'custom\'); popoverOpen = true">\
                        <svg style="width:14px;height:14px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="18" height="18" rx="2" ry="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg>\
                    </button>\
                    <div class="custom-time-popover" v-show="popoverOpen && timeRange === \'custom\'" v-cloak @click.stop>\
                        <div class="popover-row">\
                            <label>开始</label>\
                            <input type="date" v-model="filters.customStartDate">\
                        </div>\
                        <div class="popover-row">\
                            <label>结束</label>\
                            <input type="date" v-model="filters.customEndDate">\
                        </div>\
                        <span class="text-danger text-sm" v-show="timeError" style="display:block;margin-bottom:6px;">{{ timeError }}</span>\
                        <button class="btn btn-sm btn-primary" style="width:100%;" @click="applyCustomTime(); if(!timeError) popoverOpen = false">确定</button>\
                    </div>\
                </div>\
            </div>\
            <div class="filter-toolbar-divider"></div>\
            <!-- 操作按钮组 -->\
            <div class="filter-toolbar-actions">\
                <button class="icon-btn" @click="resetFilters()" title="重置筛选">\
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>\
                </button>\
                <div class="dropdown">\
                    <button class="icon-btn icon-btn-danger" @click.stop="deleteOpen = !deleteOpen" title="清除日志">\
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>\
                    </button>\
                    <div class="dropdown-menu" v-show="deleteOpen" v-cloak>\
                        <button class="dropdown-item danger" @click="deleteOpen = false; confirmDeleteLogs(false)">\
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>\
                            清除筛选结果\
                        </button>\
                        <div class="dropdown-divider"></div>\
                        <button class="dropdown-item danger" @click="deleteOpen = false; confirmDeleteLogs(true)">\
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>\
                            清除全部日志\
                        </button>\
                    </div>\
                </div>\
            </div>\
        </div>\
        <!-- 活动筛选标签 -->\
        <div class="filter-tags" v-show="hasActiveFilters" v-cloak>\
            <span class="filter-tag" v-if="filters.model">\
                模型: <span>{{ filters.model }}</span>\
                <button @click="filters.model = \'\'; debouncedApply()">&times;</button>\
            </span>\
            <span class="filter-tag" v-if="filters.endpoint">\
                端点: <span>{{ filters.endpoint }}</span>\
                <button @click="filters.endpoint = \'\'; debouncedApply()">&times;</button>\
            </span>\
            <span class="filter-tag" v-if="filters.success">\
                状态: <span>{{ filters.success === \'true\' ? \'成功\' : \'失败\' }}</span>\
                <button @click="filters.success = \'\'; debouncedApply()">&times;</button>\
            </span>\
        </div>\
        <!-- 日志表格 -->\
        <div class="table-container">\
            <table class="table">\
                <thead>\
                    <tr>\
                        <th>时间</th>\
                        <th>用户</th>\
                        <th>模型</th>\
                        <th>端点</th>\
                        <th>任务类型</th>\
                        <th>路由方式</th>\
                        <th>匹配规则</th>\
                        <th>延迟</th>\
                        <th>成本</th>\
                        <th>状态</th>\
                        <th>操作</th>\
                    </tr>\
                </thead>\
                <tbody>\
                    <tr v-show="loading">\
                        <td colspan="11" class="text-center">加载中...</td>\
                    </tr>\
                    <tr v-show="!loading && logs.length === 0" v-cloak>\
                        <td colspan="11" class="text-center text-muted" style="padding:40px;">暂无日志记录</td>\
                    </tr>\
                    <tr v-for="log in logs" :key="log.id" :class="{ \'row-inaccurate\': log.is_inaccurate }">\
                        <td>{{ formatDateTime(log.created_at) }}</td>\
                        <td>{{ log.username }}</td>\
                        <td><span class="model-tag">{{ log.model_name }}</span></td>\
                        <td>{{ log.endpoint_name }}</td>\
                        <td><span class="task-type-badge" :class="\'type-\' + (log.task_type || \'default\')">{{ log.task_type || "-" }}</span></td>\
                        <td><span class="routing-method-badge" :class="\'method-\' + (log.routing_method || \'unknown\')">{{ formatRoutingMethod(log.routing_method) }}</span></td>\
                        <td>{{ log.matched_rule_name || "-" }}</td>\
                        <td>{{ (log.latency_ms || 0).toFixed(0) + " ms" }}</td>\
                        <td>{{ "$" + (log.cost || 0).toFixed(6) }}</td>\
                        <td>\
                            <span class="log-status-icons">\
                                <span v-show="log.success" class="log-icon log-icon-success" title="成功">\
                                    <svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd"/></svg>\
                                </span>\
                                <span v-show="!log.success" class="log-icon log-icon-error" title="失败">\
                                    <svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd"/></svg>\
                                </span>\
                                <span v-show="log.stream" class="log-icon log-icon-stream" title="流式">\
                                    <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M3 7c2-2 4-2 6 0s4 2 6 0"/><path d="M3 13c2-2 4-2 6 0s4 2 6 0"/></svg>\
                                </span>\
                                <span v-show="log.is_inaccurate" class="log-icon log-icon-warning" title="标记为不准确">\
                                    <svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clip-rule="evenodd"/></svg>\
                                </span>\
                            </span>\
                        </td>\
                        <td>\
                            <button class="btn btn-sm btn-ghost" @click="showLogDetail(log.id)" title="查看详情">\
                                <svg viewBox="0 0 20 20" fill="currentColor" style="width:16px;height:16px;"><path d="M10 12a2 2 0 100-4 2 2 0 000 4z"/><path fill-rule="evenodd" d="M.458 10C1.732 5.943 5.522 3 10 3s8.268 2.943 9.542 7c-1.274 4.057-5.064 7-9.542 7S1.732 14.057.458 10zM14 10a4 4 0 11-8 0 4 4 0 018 0z" clip-rule="evenodd"/></svg>\
                            </button>\
                        </td>\
                    </tr>\
                </tbody>\
            </table>\
        </div>\
        <!-- 分页 -->\
        <div class="pagination" v-show="totalPages > 1" v-cloak>\
            <div class="pagination-controls">\
                <button class="btn btn-sm" :disabled="currentPage === 0" @click="goToPage(currentPage - 1)">上一页</button>\
                <span class="page-info">第 {{ currentPage + 1 }} / {{ totalPages }} 页</span>\
                <button class="btn btn-sm" :disabled="currentPage >= totalPages - 1" @click="goToPage(currentPage + 1)">下一页</button>\
                <span class="text-muted text-sm">共 {{ total }} 条记录</span>\
            </div>\
        </div>\
    </div>\
    <!-- 日志详情弹窗 -->\
    <div class="modal" v-show="showDetailModal" v-cloak @keydown.escape="showDetailModal = false">\
        <div class="modal-content modal-lg" @click.stop>\
            <div class="modal-header">\
                <h3>请求详情</h3>\
                <button class="modal-close" @click="showDetailModal = false">&times;</button>\
            </div>\
            <div class="modal-body" v-show="!detailLoading">\
                <div v-if="logDetail" class="log-detail-content">\
                    <!-- 基本信息 -->\
                    <div class="detail-section">\
                        <h4>基本信息</h4>\
                        <div class="detail-grid">\
                            <div class="detail-item">\
                                <span class="detail-label">请求 ID</span>\
                                <span class="detail-value">{{ logDetail.request_id }}</span>\
                            </div>\
                            <div class="detail-item">\
                                <span class="detail-label">时间</span>\
                                <span class="detail-value">{{ formatDateTime(logDetail.created_at) }}</span>\
                            </div>\
                            <div class="detail-item">\
                                <span class="detail-label">用户</span>\
                                <span class="detail-value">{{ logDetail.username }}</span>\
                            </div>\
                            <div class="detail-item">\
                                <span class="detail-label">模型</span>\
                                <span class="detail-value">{{ logDetail.model_name }}</span>\
                            </div>\
                            <div class="detail-item">\
                                <span class="detail-label">端点</span>\
                                <span class="detail-value">{{ logDetail.endpoint_name }}</span>\
                            </div>\
                            <div class="detail-item">\
                                <span class="detail-label">状态</span>\
                                <span class="detail-value" :class="logDetail.success ? \'text-success\' : \'text-danger\'">{{ logDetail.success ? "成功" : "失败" }}</span>\
                            </div>\
                        </div>\
                    </div>\
                    <!-- 路由决策 -->\
                    <div class="detail-section">\
                        <h4>路由决策</h4>\
                        <div class="detail-grid">\
                            <div class="detail-item">\
                                <span class="detail-label">任务类型</span>\
                                <span class="task-type-badge" :class="\'type-\' + (logDetail.task_type || \'default\')">{{ logDetail.task_type || "-" }}</span>\
                            </div>\
                            <div class="detail-item">\
                                <span class="detail-label">路由方式</span>\
                                <span class="routing-method-badge" :class="\'method-\' + (logDetail.routing_method || \'unknown\')">{{ formatRoutingMethod(logDetail.routing_method) }}</span>\
                            </div>\
                            <div class="detail-item">\
                                <span class="detail-label">匹配规则</span>\
                                <span class="detail-value">{{ logDetail.matched_rule_name || "-" }}</span>\
                            </div>\
                            <div class="detail-item detail-item-full">\
                                <span class="detail-label">路由原因</span>\
                                <span class="detail-value">{{ logDetail.routing_reason || "-" }}</span>\
                            </div>\
                        </div>\
                        <div class="detail-item detail-item-full" v-show="logDetail.all_matches && logDetail.all_matches.length > 0">\
                            <span class="detail-label">所有匹配规则</span>\
                            <div class="all-matches-list">\
                                <div class="match-item" v-for="match in (logDetail.all_matches || [])" :key="match.rule_id">\
                                    <span class="match-name">{{ match.name }}</span>\
                                    <span class="task-type-badge" :class="\'type-\' + match.task_type">{{ match.task_type }}</span>\
                                    <span class="match-priority">优先级: {{ match.priority }}</span>\
                                </div>\
                            </div>\
                        </div>\
                    </div>\
                    <!-- Token 和成本 -->\
                    <div class="detail-section">\
                        <h4>Token 和成本</h4>\
                        <div class="detail-grid">\
                            <div class="detail-item">\
                                <span class="detail-label">输入 Tokens</span>\
                                <span class="detail-value">{{ (logDetail.input_tokens || 0).toLocaleString() }}</span>\
                            </div>\
                            <div class="detail-item">\
                                <span class="detail-label">输出 Tokens</span>\
                                <span class="detail-value">{{ (logDetail.output_tokens || 0).toLocaleString() }}</span>\
                            </div>\
                            <div class="detail-item">\
                                <span class="detail-label">延迟</span>\
                                <span class="detail-value">{{ (logDetail.latency_ms || 0).toFixed(0) + " ms" }}</span>\
                            </div>\
                            <div class="detail-item">\
                                <span class="detail-label">成本</span>\
                                <span class="detail-value">{{ "$" + (logDetail.cost || 0).toFixed(6) }}</span>\
                            </div>\
                        </div>\
                    </div>\
                    <!-- 消息预览 / 完整请求内容（单一 div，内容切换） -->\
                    <div class="detail-section" v-show="logDetail.message_preview || logDetail.request_content">\
                        <h4>{{ expandedRequest ? \'完整请求内容\' : \'消息预览\' }}</h4>\
                        <pre class="message-preview json-viewer"\
                             :key="\'req-\' + expandedRequest"\
                             :class="{ expanded: expandedRequest }"\
                             v-html="expandedRequest ? highlightedRequest : previewHtmlWithMore"\
                             @click="handlePreviewClick($event)"></pre>\
                        <div v-if="expandLoading" class="expand-loading"><span>加载中...</span></div>\
                        <button v-if="expandedRequest && !expandLoading" class="btn-expand-toggle" @click="toggleRequestExpand()">\
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="18 15 12 9 6 15"/></svg>\
                            收起\
                        </button>\
                    </div>\
                    <!-- 响应内容 / 错误原因 -->\
                    <div class="detail-section" v-show="logDetail.response_content">\
                        <h4>{{ logDetail.success ? \'响应内容\' : \'错误原因\' }}</h4>\
                        <pre class="request-content json-viewer" v-html="formatJsonHighlight(logDetail.response_content)"></pre>\
                    </div>\
                </div>\
            </div>\
            <div class="modal-body" v-show="detailLoading">\
                <div class="text-center" style="padding:40px;">加载中...</div>\
            </div>\
            <div class="modal-footer">\
                <button class="btn" :class="logDetail && logDetail.is_inaccurate ? \'btn-warning\' : \'btn-outline\'" @click="toggleInaccurate()" :disabled="markingInaccurate">\
                    <span v-show="!markingInaccurate">{{ logDetail && logDetail.is_inaccurate ? \'取消不准确标记\' : \'标记为不准确\' }}</span>\
                    <span v-show="markingInaccurate">处理中...</span>\
                </button>\
                <button class="btn" @click="showDetailModal = false">关闭</button>\
            </div>\
        </div>\
    </div>\
</div>',
  };
})();
