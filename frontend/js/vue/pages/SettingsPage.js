/**
 * SettingsPage - 系统设置页面
 * 从 Alpine.js settings.html 移植到 Vue 3
 */
window.VuePages = window.VuePages || {};

(function () {
  "use strict";

  var ref = Vue.ref;
  var reactive = Vue.reactive;
  var inject = Vue.inject;
  var onMounted = Vue.onMounted;

  window.VuePages.SettingsPage = {
    name: "SettingsPage",
    setup: function () {
      var toastStore = inject("toastStore");
      var confirmStore = inject("confirmStore");

      var loadBalance = reactive({ strategy: "round_robin" });
      var healthCheck = reactive({
        enabled: false,
        interval_seconds: 60,
        timeout_seconds: 10,
      });
      var uiConfig = reactive({
        dashboard_refresh_seconds: 30,
        logs_refresh_seconds: 30,
      });
      var configSource = ref("-");
      var savingHealthCheck = ref(false);
      var savingUIConfig = ref(false);
      var exporting = ref(false);
      var importing = ref(false);
      var strategyOpen = ref(false);

      var strategyOptions = [
        { value: "round_robin", label: "轮询 (Round Robin)" },
        { value: "weighted", label: "加权 (Weighted)" },
        { value: "least_connections", label: "最少连接 (Least Connections)" },
        { value: "conversation_hash", label: "会话哈希 (Conversation Hash)" },
      ];

      function strategyLabel() {
        var found = strategyOptions.find(function (o) {
          return o.value === loadBalance.strategy;
        });
        return found ? found.label : "请选择";
      }

      async function loadSettings() {
        try {
          var results = await Promise.all([
            VueApi.get("/api/config/load-balance"),
            VueApi.get("/api/config/health-check"),
            VueApi.get("/api/config/ui"),
          ]);
          var lb = await results[0].json();
          var hc = await results[1].json();
          var ui = await results[2].json();
          loadBalance.strategy = lb.strategy;
          healthCheck.enabled = !!hc.enabled;
          healthCheck.interval_seconds = hc.interval_seconds;
          healthCheck.timeout_seconds = hc.timeout_seconds;
          uiConfig.dashboard_refresh_seconds = ui.dashboard_refresh_seconds;
          uiConfig.logs_refresh_seconds = ui.logs_refresh_seconds;
        } catch (error) {
          toastStore.error("加载设置失败: " + error.message);
        }
      }

      async function updateLoadBalance() {
        try {
          var response = await VueApi.put("/api/config/load-balance", {
            strategy: loadBalance.strategy,
          });
          if (!response.ok) throw new Error("更新失败");
          toastStore.success("负载均衡策略已更新");
        } catch (error) {
          toastStore.error(error.message);
        }
      }

      async function updateHealthCheck() {
        savingHealthCheck.value = true;
        try {
          var response = await VueApi.put("/api/config/health-check", {
            enabled: !!healthCheck.enabled,
            interval_seconds: healthCheck.interval_seconds,
            timeout_seconds: healthCheck.timeout_seconds,
          });
          if (!response.ok) throw new Error("更新失败");
          toastStore.success("健康检查设置已更新");
          await loadSettings();
        } catch (error) {
          toastStore.error(error.message);
        } finally {
          savingHealthCheck.value = false;
        }
      }

      async function updateUIConfig() {
        savingUIConfig.value = true;
        try {
          var response = await VueApi.put("/api/config/ui", {
            dashboard_refresh_seconds: uiConfig.dashboard_refresh_seconds,
            logs_refresh_seconds: uiConfig.logs_refresh_seconds,
          });
          if (!response.ok) {
            var result = await response.json();
            throw new Error(result.detail || "更新失败");
          }
          toastStore.success("界面设置已更新");
        } catch (error) {
          toastStore.error(error.message);
        } finally {
          savingUIConfig.value = false;
        }
      }

      function exportConfig() {
        exporting.value = true;
        try {
          window.open("/api/config/backup/export", "_blank");
        } finally {
          setTimeout(function () {
            exporting.value = false;
          }, 1000);
        }
      }

      function triggerImport() {
        var input = document.getElementById("backup-file-input");
        if (input) input.click();
      }

      async function handleImportFile(event) {
        var file = event.target.files && event.target.files[0];
        if (!file) return;
        // Reset input so same file can be re-selected
        event.target.value = "";

        var confirmed = await confirmStore.show({
          title: "导入配置",
          message: "确定要导入此配置文件吗？",
          detail:
            "导入将覆盖所有现有配置数据（模型、服务商、用户、路由规则等），并自动退出登录。此操作不可撤销。",
          confirmText: "确认导入",
          type: "warning",
        });
        if (!confirmed) return;

        importing.value = true;
        try {
          var text = await file.text();
          var json = JSON.parse(text);
          var response = await VueApi.post("/api/config/backup/import", json);
          var result = await response.json();
          if (!response.ok) throw new Error(result.error || "导入失败");
          toastStore.success("配置导入成功，即将跳转登录页...");
          setTimeout(function () {
            window.location.hash = "#/login";
          }, 1500);
        } catch (error) {
          toastStore.error("导入失败: " + error.message);
        } finally {
          importing.value = false;
        }
      }

      function selectStrategy(value) {
        loadBalance.strategy = value;
        updateLoadBalance();
        strategyOpen.value = false;
      }

      onMounted(function () {
        loadSettings();
      });

      return {
        loadBalance: loadBalance,
        healthCheck: healthCheck,
        uiConfig: uiConfig,
        configSource: configSource,
        savingHealthCheck: savingHealthCheck,
        savingUIConfig: savingUIConfig,
        exporting: exporting,
        importing: importing,
        strategyOpen: strategyOpen,
        strategyOptions: strategyOptions,
        strategyLabel: strategyLabel,
        updateHealthCheck: updateHealthCheck,
        updateUIConfig: updateUIConfig,
        exportConfig: exportConfig,
        triggerImport: triggerImport,
        handleImportFile: handleImportFile,
        selectStrategy: selectStrategy,
      };
    },
    template:
      '\
<div class="settings-page">\
    <!-- 负载均衡设置 -->\
    <div class="section">\
        <h3>负载均衡</h3>\
        <div class="form-group">\
            <label>负载均衡策略</label>\
            <div class="custom-select">\
                <button type="button" class="custom-select-trigger" :class="{ open: strategyOpen }" @click="strategyOpen = !strategyOpen">\
                    <span>{{ strategyLabel() }}</span>\
                    <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                        <polyline points="6 9 12 15 18 9"></polyline>\
                    </svg>\
                </button>\
                <div class="custom-select-dropdown" v-show="strategyOpen">\
                    <button type="button" class="custom-select-option" v-for="option in strategyOptions" :key="option.value" :class="{ selected: loadBalance.strategy === option.value }" @click="selectStrategy(option.value)">{{ option.label }}</button>\
                </div>\
            </div>\
            <span class="help-text">\
                <strong>轮询:</strong> 依次选择端点<br>\
                <strong>加权:</strong> 按权重比例分配请求<br>\
                <strong>最少连接:</strong> 选择当前连接数最少的端点<br>\
                <strong>会话哈希:</strong> 相同会话的请求路由到同一端点（优化 Prompt Caching）\
            </span>\
        </div>\
    </div>\
    <!-- 健康检查设置 -->\
    <div class="section">\
        <h3>健康检查</h3>\
        <form @submit.prevent="updateHealthCheck">\
            <div class="form-group">\
                <label class="checkbox-label">\
                    <input type="checkbox" v-model="healthCheck.enabled">\
                    启用健康检查\
                </label>\
            </div>\
            <div class="form-row">\
                <div class="form-group">\
                    <label>检查间隔（秒）</label>\
                    <input type="number" v-model.number="healthCheck.interval_seconds" min="10" max="3600">\
                </div>\
                <div class="form-group">\
                    <label>超时时间（秒）</label>\
                    <input type="number" v-model.number="healthCheck.timeout_seconds" min="1" max="60">\
                </div>\
            </div>\
            <button type="button" class="btn btn-primary" :disabled="savingHealthCheck" @click="updateHealthCheck()">\
                <span v-show="!savingHealthCheck">保存健康检查设置</span>\
                <span v-show="savingHealthCheck">保存中...</span>\
            </button>\
        </form>\
    </div>\
    <!-- 界面设置 -->\
    <div class="section">\
        <h3>界面设置</h3>\
        <form @submit.prevent="updateUIConfig">\
            <div class="form-row">\
                <div class="form-group">\
                    <label>仪表盘刷新间隔（秒）</label>\
                    <input type="number" v-model.number="uiConfig.dashboard_refresh_seconds" min="5" max="300">\
                    <span class="help-text">首页状态自动刷新间隔，范围 5-300 秒</span>\
                </div>\
                <div class="form-group">\
                    <label>日志页刷新间隔（秒）</label>\
                    <input type="number" v-model.number="uiConfig.logs_refresh_seconds" min="5" max="300">\
                    <span class="help-text">日志页面自动刷新间隔，范围 5-300 秒</span>\
                </div>\
            </div>\
            <button type="submit" class="btn btn-primary" :disabled="savingUIConfig">\
                <span v-show="!savingUIConfig">保存界面设置</span>\
                <span v-show="savingUIConfig">保存中...</span>\
            </button>\
        </form>\
    </div>\
    <!-- 数据管理 -->\
    <div class="section">\
        <h3>数据管理</h3>\
        <p class="help-text">导出或导入系统配置，用于备份和还原。导入会覆盖所有现有配置并退出登录。</p>\
        <div class="actions-row">\
            <button class="btn" @click="exportConfig()" :disabled="exporting">\
                <span v-show="!exporting">导出配置</span>\
                <span v-show="exporting">导出中...</span>\
            </button>\
            <button class="btn btn-warning" @click="triggerImport()" :disabled="importing">\
                <span v-show="!importing">导入配置</span>\
                <span v-show="importing">导入中...</span>\
            </button>\
            <input type="file" id="backup-file-input" accept=".json" style="display:none" @change="handleImportFile">\
        </div>\
    </div>\
</div>\
',
  };
})();
