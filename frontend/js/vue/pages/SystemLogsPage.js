/**
 * SystemLogsPage - 系统日志页面
 * 从 Alpine.js system_logs.html 移植到 Vue 3
 * SSE 实时日志流 + 过滤 + 搜索 + 高亮
 */
window.VuePages = window.VuePages || {};

(function () {
  const { ref, reactive, computed, onMounted, onUnmounted, nextTick, inject } =
    Vue;

  const LEVELS = [
    { value: "ALL", label: "全部" },
    { value: "DEBUG", label: "DEBUG" },
    { value: "INFO", label: "INFO" },
    { value: "WARNING", label: "WARN" },
    { value: "ERROR", label: "ERROR" },
  ];

  window.VuePages.SystemLogsPage = {
    name: "SystemLogsPage",
    setup() {
      const toastStore = inject("toastStore");
      const confirmStore = inject("confirmStore");
      const headerActionsStore = inject("headerActionsStore");

      const logs = ref([]);
      const autoScroll = ref(true);
      const levelFilter = ref("INFO");
      const searchKeyword = ref("");
      const streaming = ref(true);
      const userScrolling = ref(false);
      const logContainer = ref(null);
      const expandedSet = reactive(new Set());
      let eventSource = null;

      const stats = reactive({
        totalLines: 0,
        errorCount: 0,
        warningCount: 0,
        lastUpdate: "-",
      });

      // --- computed ---
      const filteredLogs = computed(() => {
        return logs.value.filter((log) => {
          if (levelFilter.value !== "ALL" && log.level !== levelFilter.value)
            return false;
          if (
            searchKeyword.value &&
            !log.message
              .toLowerCase()
              .includes(searchKeyword.value.toLowerCase())
          )
            return false;
          return true;
        });
      });

      // --- methods ---
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

      function escapeRegex(str) {
        return str.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      }

      function highlightSearch(text) {
        var escaped = escapeHtml(text);
        if (!searchKeyword.value) return escaped;
        var safeKeyword = escapeRegex(searchKeyword.value);
        var regex = new RegExp("(" + safeKeyword + ")", "gi");
        return escaped.replace(regex, "<mark>$1</mark>");
      }

      function updateStats() {
        stats.totalLines = logs.value.length;
        stats.errorCount = logs.value.filter(function (l) {
          return l.level === "ERROR";
        }).length;
        stats.warningCount = logs.value.filter(function (l) {
          return l.level === "WARNING";
        }).length;
        stats.lastUpdate = new Date().toLocaleTimeString("zh-CN");
      }

      function scrollToBottom() {
        if (!logContainer.value) return;
        userScrolling.value = true;
        logContainer.value.scrollTop = logContainer.value.scrollHeight;
        setTimeout(function () {
          userScrolling.value = false;
        }, 100);
      }

      function handleScroll() {
        if (userScrolling.value) return;
        var container = logContainer.value;
        if (!container) return;
        var isAtBottom =
          Math.abs(
            container.scrollHeight -
              container.scrollTop -
              container.clientHeight,
          ) < 10;
        if (!isAtBottom && autoScroll.value) {
          autoScroll.value = false;
        }
      }

      function connectStream() {
        if (eventSource) {
          eventSource.close();
        }
        eventSource = new EventSource("/api/system-logs/stream");
        eventSource.onmessage = function (event) {
          try {
            var log = JSON.parse(event.data);
            logs.value.push(log);
            if (logs.value.length > 5000) {
              logs.value.shift();
            }
            updateStats();
            if (autoScroll.value) {
              nextTick(function () {
                scrollToBottom();
              });
            }
          } catch (e) {
            console.error("解析日志失败:", e);
          }
        };
        eventSource.onerror = function () {
          console.error("SSE 连接错误");
          streaming.value = false;
        };
      }

      function toggleStream() {
        if (streaming.value) {
          if (eventSource) eventSource.close();
          streaming.value = false;
        } else {
          logs.value = [];
          expandedSet.clear();
          updateStats();
          connectStream();
          streaming.value = true;
        }
      }

      function toggleExpanded(index) {
        if (expandedSet.has(index)) {
          expandedSet.delete(index);
        } else {
          expandedSet.add(index);
        }
      }

      function isExpanded(index) {
        return expandedSet.has(index);
      }

      function onAutoScrollChange() {
        if (autoScroll.value) scrollToBottom();
      }

      async function clearLogs() {
        var confirmed = await confirmStore.show({
          title: "清除日志",
          message: "确定要清除所有日志吗？",
          detail: "此操作不可恢复。",
          confirmText: "确认清除",
          type: "danger",
        });
        if (!confirmed) return;
        try {
          var response = await window.VueApi.post("/api/system-logs/clear");
          var result = await response.json();
          if (result.success) {
            logs.value = [];
            expandedSet.clear();
            updateStats();
            toastStore.success("日志已清除");
          } else {
            toastStore.error("清除日志失败");
          }
        } catch (e) {
          toastStore.error("清除日志失败: " + e.message);
        }
      }

      // header 操作按钮组件
      var SystemLogsHeaderActions = {
        name: "SystemLogsHeaderActions",
        setup: function () {
          return {
            streaming: streaming,
            toggleStream: toggleStream,
            clearLogs: clearLogs,
          };
        },
        template:
          '<button class="btn" @click="toggleStream">\
              <svg v-if="streaming" style="width: 14px; height: 14px; margin-right: 4px;" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>\
              <svg v-else style="width: 14px; height: 14px; margin-right: 4px;" viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"/></svg>\
              <span>{{ streaming ? "暂停" : "恢复" }}</span>\
          </button>\
          <button class="btn btn-danger" @click="clearLogs">清除日志</button>',
      };

      // --- lifecycle ---
      onMounted(function () {
        headerActionsStore.value = SystemLogsHeaderActions;
        connectStream();
      });

      onUnmounted(function () {
        headerActionsStore.value = null;
        if (eventSource) {
          eventSource.close();
          eventSource = null;
        }
      });

      return {
        logs,
        autoScroll,
        levelFilter,
        searchKeyword,
        streaming,
        logContainer,
        stats,
        filteredLogs,
        LEVELS: LEVELS,
        highlightSearch,
        handleScroll,
        scrollToBottom,
        toggleStream,
        clearLogs,
        toggleExpanded,
        isExpanded,
        onAutoScrollChange,
      };
    },
    template: `
<div class="system-logs-container">
  <!-- 统计卡片区 -->
  <div class="stats-grid">
    <div class="stat-card">
      <div class="stat-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
          <polyline points="14 2 14 8 20 8"/>
          <line x1="16" y1="13" x2="8" y2="13"/>
          <line x1="16" y1="17" x2="8" y2="17"/>
          <polyline points="10 9 9 9 8 9"/>
        </svg>
      </div>
      <div class="stat-info">
        <span class="stat-value">{{ stats.totalLines.toLocaleString() }}</span>
        <span class="stat-label">日志总数</span>
      </div>
    </div>
    <div class="stat-card">
      <div class="stat-icon stat-icon-error">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10"/>
          <line x1="12" y1="8" x2="12" y2="12"/>
          <line x1="12" y1="16" x2="12.01" y2="16"/>
        </svg>
      </div>
      <div class="stat-info">
        <span class="stat-value">{{ stats.errorCount.toLocaleString() }}</span>
        <span class="stat-label">错误数量</span>
      </div>
    </div>
    <div class="stat-card">
      <div class="stat-icon stat-icon-warning">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
          <line x1="12" y1="9" x2="12" y2="13"/>
          <line x1="12" y1="17" x2="12.01" y2="17"/>
        </svg>
      </div>
      <div class="stat-info">
        <span class="stat-value">{{ stats.warningCount.toLocaleString() }}</span>
        <span class="stat-label">警告数量</span>
      </div>
    </div>
    <div class="stat-card">
      <div class="stat-icon stat-icon-success">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10"/>
          <polyline points="12 6 12 12 16 14"/>
        </svg>
      </div>
      <div class="stat-info">
        <span class="stat-value">{{ stats.lastUpdate }}</span>
        <span class="stat-label">最近更新</span>
      </div>
    </div>
  </div>
  <!-- 操作栏 -->
  <div class="filter-toolbar">
    <div class="status-indicator" :class="streaming ? 'active' : 'paused'">
      <span class="pulse"></span>
      <span>{{ streaming ? '实时监控中' : '已暂停' }}</span>
    </div>

    <div class="filter-toolbar-divider"></div>

    <label class="checkbox-inline">
      <input type="checkbox" v-model="autoScroll" @change="onAutoScrollChange">
      <span>自动滚动</span>
    </label>

    <div class="filter-toolbar-divider"></div>

    <div class="segment-control">
      <button v-for="lv in LEVELS" :key="lv.value"
        class="segment-btn"
        :class="{
          'active': levelFilter === lv.value,
          'segment-btn-warn': lv.value === 'WARNING' && levelFilter === lv.value,
          'segment-btn-error': lv.value === 'ERROR' && levelFilter === lv.value
        }"
        @click="levelFilter = lv.value">{{ lv.label }}</button>
    </div>

    <div class="filter-toolbar-divider"></div>

    <div class="toolbar-search">
      <svg class="toolbar-search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
      <input type="text" v-model="searchKeyword" placeholder="搜索..." class="toolbar-search-input">
    </div>

    <div class="filter-toolbar-actions" style="margin-left: auto; display: flex; align-items: center; gap: 8px;">
      <span class="log-count">{{ filteredLogs.length }} / {{ logs.length }}</span>
    </div>
  </div>

  <!-- 日志显示区 -->
  <div class="log-terminal" ref="logContainer" @scroll="handleScroll">
    <div v-for="(log, index) in filteredLogs" :key="index"
      class="log-line" :class="{ expanded: isExpanded(index) }"
      @click="toggleExpanded(index)">
      <span class="log-col log-line-number">{{ index + 1 }}</span>
      <span class="log-col log-level" :class="'log-level-' + log.level.toLowerCase()">{{ log.level }}</span>
      <span class="log-col log-worker">{{ log.worker_id || '-' }}</span>
      <span class="log-col log-timestamp">{{ log.timestamp }}</span>
      <span class="log-col log-caller">{{ log.caller || '' }}</span>
      <span class="log-col log-message" v-html="highlightSearch(log.message)"></span>
    </div>
    <div v-if="filteredLogs.length === 0" class="log-empty">暂无日志</div>
  </div>
</div>`,
  };
})();
