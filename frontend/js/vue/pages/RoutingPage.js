/**
 * 路由配置页面
 * 支持规则路由配置、LLM 路由配置、路由模型管理、规则管理、路由测试
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
  var watch = Vue.watch;

  window.VuePages.RoutingPage = {
    name: "RoutingPage",
    setup: function () {
      var toastStore = inject("toastStore");
      var confirmStore = inject("confirmStore");

      // 基础状态
      var loading = ref(true);
      var saving = ref(false);
      var activeTab = ref("routing-config");
      var providers = ref([]);
      var models = ref([]);
      var llmConfigExpanded = ref(false);

      // 配置
      var config = reactive({
        enabled: false,
        force_smart_routing: false,
        primary_model_id: "",
        fallback_model_id: "",
        timeout_seconds: 5,
        retry_count: 2,
        cache_enabled: true,
        cache_ttl_seconds: 300,
        max_tokens: 1024,
        temperature: 0,
        rule_based_routing_enabled: true,
        rule_fallback_strategy: "default",
        rule_fallback_task_type: "default",
      });

      // 模型模态框
      var showModelForm = ref(false);
      var editingModel = ref(null);
      var modelForm = reactive({
        model_name: "",
        provider_id: "",
        priority: 1,
        billing_multiplier: 1.0,
      });
      var modelFormErrors = reactive({
        model_name: "",
        provider_id: "",
      });

      // 模型检测
      var detectingModels = ref(false);
      var providerModels = ref([]);
      var providerModelSearch = ref("");
      var detectError = ref("");

      // 路由测试
      var testContent = ref("");
      var testResult = ref(null);
      var testing = ref(false);

      // 规则状态
      var builtinRules = ref([]);
      var customRules = ref([]);
      var ruleStats = ref(null);
      var rulesLoading = ref(false);

      // 规则模态框
      var showRuleForm = ref(false);
      var editingRule = ref(null);
      var savingRule = ref(false);
      var ruleForm = reactive({
        name: "",
        description: "",
        keywords: "",
        pattern: "",
        condition: "",
        task_type: "default",
        priority: 50,
        enabled: true,
      });
      var ruleFormErrors = reactive({
        name: "",
        task_type: "",
        pattern: "",
        condition: "",
      });

      // 规则测试
      var ruleTestMessage = ref("");
      var ruleTestResult = ref(null);
      var ruleTestLoading = ref(false);

      // 分析功能状态
      var showAnalysisModal = ref(false);
      var analysisTimeRange = ref("7d");
      var analysisCustomStart = ref("");
      var analysisCustomEnd = ref("");
      var analysisModelId = ref("");
      var analysisModels = ref([]);
      var analysisStarting = ref(false);
      var analysisTask = ref(null);
      var analysisPolling = ref(false);
      var analysisReports = ref([]);
      var analysisReportsTotal = ref(0);
      var showAnalysisReport = ref(false);
      var currentReport = ref(null);
      var analysisPollingTimer = ref(null);
      var smoothProgress = ref(0);
      var smoothProgressTimer = ref(null);

      // 规则视图状态（统一管理内置和自定义规则）
      var ruleSearchQuery = ref("");
      var debouncedSearchQuery = ref("");
      var ruleFilterTaskType = ref("");
      var ruleFilterSource = ref("");
      var ruleSortBy = ref("priority-desc");
      // 规则卡片展开状态（支持多选）
      var expandedRuleIds = ref(new Set());

      // 下拉菜单
      // 统一的下拉菜单状态管理（只允许一个菜单打开）
      var openDropdown = ref(null);

      // ========== 计算属性 ==========

      var tierCycle = ["default", "complex", "simple"];

      function getGroupTier(index) {
        return tierCycle[index % tierCycle.length];
      }

      var filteredAllRules = computed(function () {
        // 合并内置和自定义规则，附加来源标记
        var allRules = [];
        for (var i = 0; i < builtinRules.value.length; i++) {
          allRules.push(Object.assign({}, builtinRules.value[i], { _source: "builtin" }));
        }
        for (var j = 0; j < customRules.value.length; j++) {
          allRules.push(Object.assign({}, customRules.value[j], { _source: "custom" }));
        }

        var rules = allRules;

        // 来源过滤
        if (ruleFilterSource.value) {
          rules = rules.filter(function (r) {
            return r._source === ruleFilterSource.value;
          });
        }

        // 搜索（使用防抖后的查询）
        if (debouncedSearchQuery.value) {
          var q = debouncedSearchQuery.value.toLowerCase();
          rules = rules.filter(function (r) {
            return (
              r.name.toLowerCase().indexOf(q) !== -1 ||
              (r.description || "").toLowerCase().indexOf(q) !== -1 ||
              (r.keywords || []).some(function (kw) {
                return kw.toLowerCase().indexOf(q) !== -1;
              })
            );
          });
        }

        // 过滤任务类型
        if (ruleFilterTaskType.value) {
          rules = rules.filter(function (r) {
            return r.task_type === ruleFilterTaskType.value;
          });
        }

        // 排序
        rules.sort(function (a, b) {
          // 先按来源分组（内置在前）
          if (a._source !== b._source) {
            return a._source === "builtin" ? -1 : 1;
          }

          // 同组内按选择的排序方式
          switch (ruleSortBy.value) {
            case "priority-desc":
              return b.priority - a.priority;
            case "priority-asc":
              return a.priority - b.priority;
            case "hits-desc":
              var hitA = getRuleHit(a.id)?.count || 0;
              var hitB = getRuleHit(b.id)?.count || 0;
              return hitB - hitA;
            case "hits-asc":
              var hitA = getRuleHit(a.id)?.count || 0;
              var hitB = getRuleHit(b.id)?.count || 0;
              return hitA - hitB;
            case "name-asc":
              return a.name.localeCompare(b.name);
            case "name-desc":
              return b.name.localeCompare(a.name);
            default:
              // 默认按优先级降序
              return b.priority - a.priority;
          }
        });
        return rules;
      });

      var groupedProviderModels = computed(function () {
        var query = (providerModelSearch.value || "").toLowerCase().trim();
        var filtered = query
          ? providerModels.value.filter(function (dm) {
              return (
                dm.id.toLowerCase().indexOf(query) !== -1 ||
                (dm.display_name || "").toLowerCase().indexOf(query) !== -1
              );
            })
          : providerModels.value;

        var groups = Object.create(null);
        for (var i = 0; i < filtered.length; i++) {
          var dm = filtered[i];
          var key = (dm.owned_by || "other").toLowerCase();
          if (!groups[key]) {
            groups[key] = {
              key: key,
              label: dm.owned_by || "其他",
              models: [],
            };
          }
          groups[key].models.push(dm);
        }

        return Object.values(groups)
          .sort(function (a, b) {
            return b.models.length - a.models.length;
          })
          .map(function (g, idx) {
            return Object.assign({}, g, { tier: getGroupTier(idx) });
          });
      });

      var filteredProviderModelCount = computed(function () {
        var query = (providerModelSearch.value || "").toLowerCase().trim();
        if (!query) return providerModels.value.length;
        return providerModels.value.filter(function (dm) {
          return (
            dm.id.toLowerCase().indexOf(query) !== -1 ||
            (dm.display_name || "").toLowerCase().indexOf(query) !== -1
          );
        }).length;
      });

      var fallbackStrategyLabel = computed(function () {
        var map = {
          default: "使用默认任务类型",
          llm: "调用 LLM 路由",
          user: "使用指定任务类型",
        };
        return map[config.rule_fallback_strategy] || "使用默认任务类型";
      });

      var fallbackTaskTypeLabel = computed(function () {
        var map = {
          simple: "simple (轻量)",
          default: "default (平衡)",
          complex: "complex (高能)",
        };
        return map[config.rule_fallback_task_type] || "default (平衡)";
      });

      var primaryModelLabel = computed(function () {
        var found = models.value.find(function (m) {
          return m.id === config.primary_model_id;
        });
        return found ? found.model_name : "-- 请先添加路由模型 --";
      });

      var fallbackModelLabel = computed(function () {
        var found = models.value.find(function (m) {
          return m.id === config.fallback_model_id;
        });
        return found ? found.model_name : "-- 无 --";
      });

      var modalProviderLabel = computed(function () {
        var found = providers.value.find(function (p) {
          return p.id === modelForm.provider_id;
        });
        return found ? found.name : "-- 请选择服务商 --";
      });

      // 统一规则统计
      var allRulesCount = computed(function () {
        return builtinRules.value.length + customRules.value.length;
      });
      var allEnabledCount = computed(function () {
        return builtinRules.value.filter(function (r) { return r.enabled; }).length +
               customRules.value.filter(function (r) { return r.enabled; }).length;
      });
      var allBuiltinCount = computed(function () {
        return builtinRules.value.length;
      });
      var allCustomCount = computed(function () {
        return customRules.value.length;
      });
      var allSimpleCount = computed(function () {
        return builtinRules.value.filter(function (r) { return r.task_type === "simple"; }).length +
               customRules.value.filter(function (r) { return r.task_type === "simple"; }).length;
      });
      var allDefaultCount = computed(function () {
        return builtinRules.value.filter(function (r) { return r.task_type === "default"; }).length +
               customRules.value.filter(function (r) { return r.task_type === "default"; }).length;
      });
      var allComplexCount = computed(function () {
        return builtinRules.value.filter(function (r) { return r.task_type === "complex"; }).length +
               customRules.value.filter(function (r) { return r.task_type === "complex"; }).length;
      });

      // ========== 方法 ==========

      function getProviderName(providerId) {
        var provider = providers.value.find(function (p) {
          return p.id === providerId;
        });
        return provider ? provider.name : "-";
      }

      function fillConfig(cfg) {
        if (!cfg) return;
        config.enabled = cfg.enabled !== false;
        config.force_smart_routing = cfg.force_smart_routing || false;
        config.primary_model_id = cfg.primary_model_id || "";
        config.fallback_model_id = cfg.fallback_model_id || "";
        config.timeout_seconds =
          cfg.timeout_seconds != null ? cfg.timeout_seconds : 5;
        config.retry_count = cfg.retry_count != null ? cfg.retry_count : 2;
        config.cache_enabled = cfg.cache_enabled !== false;
        config.cache_ttl_seconds =
          cfg.cache_ttl_seconds != null ? cfg.cache_ttl_seconds : 300;
        config.max_tokens = cfg.max_tokens != null ? cfg.max_tokens : 1024;
        config.temperature = cfg.temperature != null ? cfg.temperature : 0;
        config.rule_based_routing_enabled =
          cfg.rule_based_routing_enabled !== false;
        config.rule_fallback_strategy = cfg.rule_fallback_strategy || "default";
        config.rule_fallback_task_type =
          cfg.rule_fallback_task_type || "default";
      }

      function loadData() {
        loading.value = true;
        return Promise.all([
          VueApi.get("/api/config/providers"),
          VueApi.get("/api/config/routing/models"),
          VueApi.get("/api/config/routing/llm-config"),
        ])
          .then(function (results) {
            return Promise.all([
              results[0].json(),
              results[1].json(),
              results[2].json(),
            ]);
          })
          .then(function (parsed) {
            providers.value = parsed[0].providers || [];
            models.value = parsed[1].models || [];
            fillConfig(parsed[2]);
          })
          .catch(function (error) {
            toastStore.error("加载数据失败: " + error.message);
          })
          .finally(function () {
            loading.value = false;
          });
      }

      function loadRules() {
        rulesLoading.value = true;
        return Promise.all([
          VueApi.get("/api/config/routing/rules/builtin"),
          VueApi.get("/api/config/routing/rules/custom"),
          VueApi.get("/api/config/routing/rules/stats"),
        ])
          .then(function (results) {
            return Promise.all([
              results[0].json(),
              results[1].json(),
              results[2].json(),
            ]);
          })
          .then(function (parsed) {
            builtinRules.value = parsed[0].rules || [];
            customRules.value = parsed[1].rules || [];
            ruleStats.value = parsed[2];
          })
          .catch(function (error) {
            toastStore.error("加载规则失败: " + error.message);
          })
          .finally(function () {
            rulesLoading.value = false;
          });
      }

      function switchTab(tabName) {
        activeTab.value = tabName;
        if (
          tabName === "routing-config" &&
          builtinRules.value.length === 0 &&
          customRules.value.length === 0
        ) {
          loadRules();
        }
      }

      function saveConfig() {
        var needsLLM =
          config.force_smart_routing ||
          (config.rule_based_routing_enabled &&
            config.rule_fallback_strategy === "llm");
        config.enabled = needsLLM;

        if (needsLLM && !config.primary_model_id) {
          toastStore.error("启用 LLM 路由时请选择主路由模型");
          return;
        }
        saving.value = true;
        var payload = Object.assign({}, config, {
          primary_model_id: config.primary_model_id
            ? parseInt(config.primary_model_id)
            : 0,
          fallback_model_id: config.fallback_model_id
            ? parseInt(config.fallback_model_id)
            : 0,
        });
        VueApi.put("/api/config/routing/llm-config", payload)
          .then(function () {
            toastStore.success("配置已保存");
          })
          .catch(function (error) {
            toastStore.error(error.message);
          })
          .finally(function () {
            saving.value = false;
          });
      }

      // ========== 模型检测 ==========

      function selectProvider(providerId) {
        modelForm.provider_id = providerId;
        providerModels.value = [];
        providerModelSearch.value = "";
        detectError.value = "";
        if (!providerId) return;
        var provider = providers.value.find(function (p) {
          return p.id === providerId;
        });
        if (!provider) return;
        detectProviderModels(provider);
      }

      function detectProviderModels(provider) {
        detectingModels.value = true;
        detectError.value = "";
        VueApi.post("/api/config/detect-models", {
          base_url: provider.base_url,
          provider_id: provider.id,
          provider_type: "provider",
        })
          .then(function (response) {
            return response.json();
          })
          .then(function (data) {
            if (data.success) {
              providerModels.value = data.models || [];
              if (providerModels.value.length === 0) {
                detectError.value = "未检测到任何模型，可手动输入模型 ID";
              }
            } else {
              detectError.value = data.error || "检测失败，可手动输入模型 ID";
            }
          })
          .catch(function (error) {
            detectError.value =
              "请求失败: " + error.message + "，可手动输入模型 ID";
          })
          .finally(function () {
            detectingModels.value = false;
          });
      }

      function selectModel(modelId) {
        modelForm.model_name = modelId;
      }

      // ========== 模型操作 ==========

      function showModelModal(model) {
        editingModel.value = model || null;
        providerModels.value = [];
        providerModelSearch.value = "";
        detectError.value = "";
        if (model) {
          modelForm.model_name = model.model_name;
          modelForm.provider_id = model.provider_id;
          modelForm.priority = model.priority != null ? model.priority : 1;
          modelForm.billing_multiplier =
            model.billing_multiplier != null ? model.billing_multiplier : 1.0;
        } else {
          modelForm.model_name = "";
          modelForm.provider_id = "";
          modelForm.priority = 1;
          modelForm.billing_multiplier = 1.0;
        }
        // 清空错误信息
        modelFormErrors.model_name = "";
        modelFormErrors.provider_id = "";
        showModelForm.value = true;
        if (model && model.provider_id) {
          var provider = providers.value.find(function (p) {
            return p.id === model.provider_id;
          });
          if (provider) detectProviderModels(provider);
        }
      }

      function validateModelForm() {
        var isValid = true;

        // 验证模型名称
        if (!modelForm.model_name.trim()) {
          modelFormErrors.model_name = "请输入或选择模型名称";
          isValid = false;
        } else {
          modelFormErrors.model_name = "";
        }

        // 验证提供商
        if (!modelForm.provider_id) {
          modelFormErrors.provider_id = "请选择提供商";
          isValid = false;
        } else {
          modelFormErrors.provider_id = "";
        }

        return isValid;
      }

      function saveModel() {
        if (!validateModelForm()) {
          return;
        }
        saving.value = true;
        var data = {
          model_name: modelForm.model_name,
          provider_id: parseInt(modelForm.provider_id),
          priority: modelForm.priority,
          billing_multiplier: modelForm.billing_multiplier,
        };
        var url = editingModel.value
          ? "/api/config/routing/models/" + editingModel.value.id
          : "/api/config/routing/models";
        var method = editingModel.value ? "PUT" : "POST";
        VueApi.request(url, { method: method, body: JSON.stringify(data) })
          .then(function () {
            toastStore.success("模型已保存");
            showModelForm.value = false;
            return loadData();
          })
          .catch(function (error) {
            toastStore.error(error.message);
          })
          .finally(function () {
            saving.value = false;
          });
      }

      function deleteModel(model) {
        confirmStore
          .delete(model.model_name, "模型")
          .then(function (confirmed) {
            if (!confirmed) return;
            VueApi.delete("/api/config/routing/models/" + model.id)
              .then(function () {
                toastStore.success("模型已删除");
                return loadData();
              })
              .catch(function (error) {
                toastStore.error(error.message);
              });
          });
      }

      // ========== 规则操作 ==========

      function showRuleModal(rule) {
        editingRule.value = rule || null;
        if (rule) {
          ruleForm.name = rule.name || "";
          ruleForm.description = rule.description || "";
          ruleForm.keywords = (rule.keywords || []).join(", ");
          ruleForm.pattern = rule.pattern || "";
          ruleForm.condition = rule.condition || "";
          ruleForm.task_type = rule.task_type || "default";
          ruleForm.priority = rule.priority != null ? rule.priority : 50;
          ruleForm.enabled = rule.enabled !== false;
        } else {
          ruleForm.name = "";
          ruleForm.description = "";
          ruleForm.keywords = "";
          ruleForm.pattern = "";
          ruleForm.condition = "";
          ruleForm.task_type = "default";
          ruleForm.priority = 50;
          ruleForm.enabled = true;
        }
        // 清空错误信息
        ruleFormErrors.name = "";
        ruleFormErrors.task_type = "";
        ruleFormErrors.pattern = "";
        ruleFormErrors.condition = "";
        showRuleForm.value = true;
      }

      function validateRuleForm() {
        var isValid = true;

        // 验证规则名称
        if (!ruleForm.name.trim()) {
          ruleFormErrors.name = "请输入规则名称";
          isValid = false;
        } else {
          ruleFormErrors.name = "";
        }

        // 验证任务类型
        if (!ruleForm.task_type) {
          ruleFormErrors.task_type = "请选择任务类型";
          isValid = false;
        } else {
          ruleFormErrors.task_type = "";
        }

        // 验证 pattern（如果填写了，检查是否为有效正则）
        if (ruleForm.pattern.trim()) {
          try {
            // Strip Go-style inline flags (?i), (?s), (?m), (?U) etc. before JS validation
            var patternForTest = ruleForm.pattern.replace(/^\(\?[ismUu]+\)/, '');
            new RegExp(patternForTest);
            ruleFormErrors.pattern = "";
          } catch (e) {
            ruleFormErrors.pattern = "正则表达式格式无效";
            isValid = false;
          }
        } else {
          ruleFormErrors.pattern = "";
        }

        // 验证 condition（如果填写了，检查基本语法）
        if (ruleForm.condition.trim()) {
          var cond = ruleForm.condition.trim();
          // 简单检查：必须包含操作符
          if (!/[=<>!]/.test(cond)) {
            ruleFormErrors.condition = "条件表达式需要包含比较操作符（=, <, >, !=）";
            isValid = false;
          } else {
            ruleFormErrors.condition = "";
          }
        } else {
          ruleFormErrors.condition = "";
        }

        return isValid;
      }

      function saveRule() {
        if (!validateRuleForm()) {
          return;
        }
        savingRule.value = true;
        var keywords = ruleForm.keywords
          .split(/[,，]/)
          .map(function (k) {
            return k.trim();
          })
          .filter(function (k) {
            return k.length > 0;
          });
        var payload = {
          name: ruleForm.name,
          description: ruleForm.description || "",
          keywords: keywords,
          pattern: ruleForm.pattern || "",
          condition: ruleForm.condition || "",
          task_type: ruleForm.task_type,
          priority: ruleForm.priority,
          enabled: ruleForm.enabled,
        };
        var url = editingRule.value
          ? "/api/config/routing/rules/" + editingRule.value.id
          : "/api/config/routing/rules";
        var method = editingRule.value ? "PUT" : "POST";
        VueApi.request(url, { method: method, body: JSON.stringify(payload) })
          .then(function () {
            toastStore.success(editingRule.value ? "规则已更新" : "规则已创建");
            showRuleForm.value = false;
            return loadRules();
          })
          .catch(function (error) {
            toastStore.error(error.message);
          })
          .finally(function () {
            savingRule.value = false;
          });
      }

      function deleteRule(rule) {
        if (rule.is_builtin) {
          toastStore.error("内置规则不可删除");
          return;
        }
        confirmStore
          .show({
            title: "删除路由规则",
            message: '确定要删除规则 "' + rule.name + '" 吗？',
            detail: "删除后无法恢复。",
            confirmText: "确认删除",
            type: "danger",
          })
          .then(function (confirmed) {
            if (!confirmed) return;
            VueApi.delete("/api/config/routing/rules/" + rule.id)
              .then(function () {
                toastStore.success("规则已删除");
                return loadRules();
              })
              .catch(function (error) {
                toastStore.error(error.message);
              });
          });
      }

      // ========== 测试 ==========

      function testRouting() {
        if (!testContent.value.trim()) {
          toastStore.error("请输入测试内容");
          return;
        }
        testing.value = true;
        testResult.value = null;
        VueApi.post("/api/routing/test", {
          model: "auto",
          messages: [{ role: "user", content: testContent.value }],
          max_tokens: 100,
        })
          .then(function (response) {
            return response.json();
          })
          .then(function (data) {
            testResult.value = data;
          })
          .catch(function (error) {
            toastStore.error("测试失败: " + error.message);
          })
          .finally(function () {
            testing.value = false;
          });
      }

      function testRuleMessage() {
        if (!ruleTestMessage.value.trim()) {
          toastStore.error("请输入测试消息");
          return;
        }
        ruleTestLoading.value = true;
        ruleTestResult.value = null;
        VueApi.post("/api/config/routing/rules/test", {
          message: ruleTestMessage.value,
        })
          .then(function (response) {
            return response.json();
          })
          .then(function (data) {
            ruleTestResult.value = data;
          })
          .catch(function (error) {
            toastStore.error("测试失败: " + error.message);
          })
          .finally(function () {
            ruleTestLoading.value = false;
          });
      }

      function getRuleHit(ruleId) {
        if (!ruleStats.value || !ruleStats.value.rule_hits) return null;
        return ruleStats.value.rule_hits[ruleId] || null;
      }

      function copyRuleConfig(rule) {
        var copy = Object.assign({}, rule);
        delete copy._source;
        var text = JSON.stringify(copy, null, 2);
        navigator.clipboard
          .writeText(text)
          .then(function () {
            toastStore.success("规则配置已复制");
          })
          .catch(function () {
            toastStore.error("复制失败");
          });
      }

      // ========== 分析功能 ==========

      function openAnalysisModal() {
        analysisTask.value = null;
        showAnalysisReport.value = false;
        currentReport.value = null;
        analysisTimeRange.value = "7d";
        analysisCustomStart.value = "";
        analysisCustomEnd.value = "";
        analysisModelId.value = "";
        showAnalysisModal.value = true;
        loadAnalysisModels();
        loadAnalysisReports();
      }

      function loadAnalysisModels() {
        VueApi.get("/api/config/routing/models")
          .then(function (r) { return r.json(); })
          .then(function (data) {
            analysisModels.value = data.models || [];
            if (analysisModels.value.length > 0 && !analysisModelId.value) {
              analysisModelId.value = analysisModels.value[0].id;
            }
          })
          .catch(function () {});
      }

      function loadAnalysisReports() {
        VueApi.get("/api/routing/analysis/reports?limit=10&offset=0")
          .then(function (r) { return r.json(); })
          .then(function (data) {
            analysisReports.value = data.reports || [];
            analysisReportsTotal.value = data.total || 0;
          })
          .catch(function () {});
      }

      function getAnalysisTimeRange() {
        var now = new Date();
        var start = null;
        var end = now.toISOString();
        switch (analysisTimeRange.value) {
          case "1d":
            start = new Date(now.getTime() - 86400000).toISOString();
            break;
          case "7d":
            start = new Date(now.getTime() - 7 * 86400000).toISOString();
            break;
          case "30d":
            start = new Date(now.getTime() - 30 * 86400000).toISOString();
            break;
          case "custom":
            start = analysisCustomStart.value || null;
            end = analysisCustomEnd.value || end;
            break;
          default:
            start = new Date(now.getTime() - 7 * 86400000).toISOString();
        }
        return { start: start, end: end };
      }

      function startAnalysis() {
        if (!analysisModelId.value) {
          toastStore.error("请选择分析模型");
          return;
        }
        analysisStarting.value = true;
        analysisTask.value = null;
        var timeRange = getAnalysisTimeRange();
        var payload = {
          model_id: parseInt(analysisModelId.value),
          start_time: timeRange.start,
          end_time: timeRange.end,
        };
        VueApi.post("/api/routing/analysis/analyze", payload)
          .then(function (r) { return r.json(); })
          .then(function (data) {
            if (data.task_id) {
              analysisTask.value = { id: data.task_id, status: "pending", progress: 0, stage: "initializing" };
              startPollingTask(data.task_id);
            }
          })
          .catch(function (error) {
            toastStore.error("启动分析失败: " + error.message);
          })
          .finally(function () {
            analysisStarting.value = false;
          });
      }

      function startPollingTask(taskId) {
        stopPollingTask();
        analysisPolling.value = true;
        startSmoothProgress();
        analysisPollingTimer.value = setInterval(function () {
          VueApi.get("/api/routing/analysis/task/" + taskId)
            .then(function (r) { return r.json(); })
            .then(function (task) {
              analysisTask.value = task;
              if (task.status === "completed" || task.status === "failed") {
                stopPollingTask();
                smoothProgress.value = 100;
                if (task.status === "completed" && task.report) {
                  currentReport.value = task.report;
                  showAnalysisReport.value = true;
                  loadAnalysisReports();
                } else if (task.status === "failed") {
                  toastStore.error("分析失败: " + (task.error || "未知错误"));
                }
              }
            })
            .catch(function () {
              stopPollingTask();
            });
        }, 2000);
      }

      function stopPollingTask() {
        analysisPolling.value = false;
        stopSmoothProgress();
        if (analysisPollingTimer.value) {
          clearInterval(analysisPollingTimer.value);
          analysisPollingTimer.value = null;
        }
      }

      function startSmoothProgress() {
        stopSmoothProgress();
        smoothProgress.value = 0;
        smoothProgressTimer.value = setInterval(function () {
          var task = analysisTask.value;
          if (!task) return;
          var target = task.progress || 0;
          var current = smoothProgress.value;
          if (current < target) {
            // Ease toward target: close gap by 20% each tick
            var step = Math.max(0.5, (target - current) * 0.2);
            smoothProgress.value = Math.min(target, current + step);
          } else if (task.stage === "calling_llm" && current < 84) {
            // During LLM call, slowly creep forward (+0.3%/tick, cap at 84)
            smoothProgress.value = Math.min(84, current + 0.3);
          }
        }, 100);
      }

      function stopSmoothProgress() {
        if (smoothProgressTimer.value) {
          clearInterval(smoothProgressTimer.value);
          smoothProgressTimer.value = null;
        }
      }

      function viewReport(report) {
        currentReport.value = report;
        showAnalysisReport.value = true;
      }

      function loadReportDetail(id) {
        VueApi.get("/api/routing/analysis/reports/" + id)
          .then(function (r) { return r.json(); })
          .then(function (report) {
            currentReport.value = report;
            showAnalysisReport.value = true;
          })
          .catch(function (error) {
            toastStore.error("加载报告失败: " + error.message);
          });
      }

      function closeAnalysisReport() {
        showAnalysisReport.value = false;
        currentReport.value = null;
      }

      function getAnalysisStageLabel(stage) {
        var map = {
          initializing: "初始化",
          collecting_logs: "收集日志",
          extracting_messages: "提取消息",
          loading_rules: "加载规则",
          building_prompt: "构建提示词",
          calling_llm: "调用 LLM 分析",
          parsing_result: "解析结果",
          done: "完成",
        };
        return map[stage] || stage;
      }

      var stageOrder = ["collecting_logs", "extracting_messages", "loading_rules", "building_prompt", "calling_llm", "parsing_result", "done"];
      function isStageCompleted(currentStage, checkStage) {
        var ci = stageOrder.indexOf(currentStage);
        var si = stageOrder.indexOf(checkStage);
        if (ci === -1 || si === -1) return false;
        return ci > si;
      }

      function getSeverityClass(severity) {
        var map = { high: "severity-high", medium: "severity-medium", low: "severity-low" };
        return map[severity] || "";
      }

      function getSeverityLabel(severity) {
        var map = { high: "高", medium: "中", low: "低" };
        return map[severity] || severity;
      }

      function formatReportDate(dateStr) {
        if (!dateStr) return "-";
        var d = new Date(dateStr);
        var month = String(d.getMonth() + 1).padStart(2, "0");
        var day = String(d.getDate()).padStart(2, "0");
        var hours = String(d.getHours()).padStart(2, "0");
        var minutes = String(d.getMinutes()).padStart(2, "0");
        return d.getFullYear() + "-" + month + "-" + day + " " + hours + ":" + minutes;
      }

      // ========== 下拉菜单 ==========

      function toggleDropdown(id) {
        openDropdown.value = openDropdown.value === id ? null : id;
      }

      function closeDropdowns() {
        openDropdown.value = null;
      }

      // 键盘导航处理（简化版：只处理打开/关闭）
      function handleSelectKeydown(event, dropdownId) {
        switch (event.key) {
          case "Enter":
          case " ": // Space
            event.preventDefault();
            openDropdown.value = openDropdown.value === dropdownId ? null : dropdownId;
            break;
          case "Escape":
            event.preventDefault();
            closeDropdowns();
            break;
        }
      }

      // 切换规则卡片展开状态
      function toggleRuleExpand(ruleId) {
        var newSet = new Set(expandedRuleIds.value);
        if (newSet.has(ruleId)) {
          newSet.delete(ruleId);
        } else {
          newSet.add(ruleId);
        }
        expandedRuleIds.value = newSet;
      }

      // 快速切换规则启用/禁用（仅自定义规则）
      async function toggleRuleEnabled(rule) {
        if (rule._source === "builtin") return;
        var newEnabled = !rule.enabled;
        rule.enabled = newEnabled;
        try {
          var response = await VueApi.request("/api/config/routing/rules/" + rule.id, {
            method: "PUT",
            body: JSON.stringify({ enabled: newEnabled }),
          });
          if (!response.ok) {
            rule.enabled = !newEnabled;
            var err = await response.json();
            toastStore.error(err.detail || "切换失败");
          }
        } catch (error) {
          rule.enabled = !newEnabled;
          toastStore.error("切换失败: " + error.message);
        }
      }

      // ========== 一键应用建议 ==========

      function findRuleByName(name) {
        var custom = customRules.value.find(function (r) { return r.name === name; });
        if (custom) return { rule: custom, source: "custom" };
        var builtin = builtinRules.value.find(function (r) { return r.name === name; });
        if (builtin) return { rule: builtin, source: "builtin" };
        return null;
      }

      function isRecActionable(rec) {
        if (rec.action === "add") return !!rec.rule_spec;
        if (rec.action === "delete") {
          var found = findRuleByName(rec.rule_name);
          return found && found.source === "custom";
        }
        return !!rec.rule_spec && !!findRuleByName(rec.rule_name);
      }

      function applyRuleSpec(spec) {
        if (spec.keywords && spec.keywords.length > 0) {
          ruleForm.keywords = spec.keywords.join(", ");
        }
        if (spec.pattern) ruleForm.pattern = spec.pattern;
        if (spec.condition) ruleForm.condition = spec.condition;
        if (spec.task_type) ruleForm.task_type = spec.task_type;
        if (spec.priority != null) ruleForm.priority = spec.priority;
        if (spec.enabled != null) ruleForm.enabled = spec.enabled;
      }

      function applyRecommendation(rec) {
        if (!rec.rule_spec && rec.action !== "delete") {
          toastStore.error("该建议无结构化参数，请手动操作");
          return;
        }

        var found;
        switch (rec.action) {
          case "add":
            showRuleModal(null);
            ruleForm.name = rec.rule_name || "";
            ruleForm.description = rec.description || "";
            applyRuleSpec(rec.rule_spec);
            showAnalysisReport.value = false;
            activeTab.value = "routing-config";
            break;

          case "modify":
            found = findRuleByName(rec.rule_name);
            if (!found) { toastStore.error("未找到规则: " + rec.rule_name); return; }
            if (found.source === "custom") {
              showRuleModal(found.rule);
              applyRuleSpec(rec.rule_spec);
            } else {
              showRuleModal(null);
              ruleForm.name = found.rule.name;
              ruleForm.description = found.rule.description || "";
              ruleForm.keywords = (found.rule.keywords || []).join(", ");
              ruleForm.pattern = found.rule.pattern || "";
              ruleForm.condition = found.rule.condition || "";
              ruleForm.task_type = found.rule.task_type || "default";
              ruleForm.priority = found.rule.priority != null ? found.rule.priority : 50;
              ruleForm.enabled = found.rule.enabled !== false;
              applyRuleSpec(rec.rule_spec);
              toastStore.info("将创建自定义规则覆盖内置规则");
            }
            showAnalysisReport.value = false;
            activeTab.value = "routing-config";
            break;

          case "delete":
            found = findRuleByName(rec.rule_name);
            if (!found) { toastStore.error("未找到规则: " + rec.rule_name); return; }
            if (found.source === "builtin") { toastStore.error("内置规则不可删除"); return; }
            deleteRule(found.rule);
            break;

          case "reorder":
            found = findRuleByName(rec.rule_name);
            if (!found) { toastStore.error("未找到规则: " + rec.rule_name); return; }
            if (found.source === "custom") {
              var newPriority = rec.rule_spec.priority;
              if (newPriority == null) { toastStore.error("建议未指定优先级"); return; }
              VueApi.request("/api/config/routing/rules/" + found.rule.id, {
                method: "PUT",
                body: JSON.stringify(Object.assign({}, found.rule, { priority: newPriority })),
              })
                .then(function () {
                  toastStore.success("优先级已更新");
                  return loadRules();
                })
                .catch(function (error) { toastStore.error(error.message); });
            } else {
              showRuleModal(null);
              ruleForm.name = found.rule.name;
              ruleForm.description = found.rule.description || "";
              ruleForm.keywords = (found.rule.keywords || []).join(", ");
              ruleForm.pattern = found.rule.pattern || "";
              ruleForm.condition = found.rule.condition || "";
              ruleForm.task_type = found.rule.task_type || "default";
              ruleForm.priority = rec.rule_spec.priority != null ? rec.rule_spec.priority : 50;
              ruleForm.enabled = found.rule.enabled !== false;
              toastStore.info("将创建自定义规则覆盖内置规则");
              showAnalysisReport.value = false;
              activeTab.value = "routing-config";
            }
            break;
        }
      }

      // ========== 搜索防抖 ==========

      var searchDebounceTimer = null;
      watch(ruleSearchQuery, function (newVal) {
        if (searchDebounceTimer) {
          clearTimeout(searchDebounceTimer);
        }
        searchDebounceTimer = setTimeout(function () {
          debouncedSearchQuery.value = newVal;
        }, 300);
      });

      // ========== 生命周期 ==========

      onMounted(function () {
        document.addEventListener("click", closeDropdowns);
        // 全局 ESC 键监听
        document.addEventListener("keydown", function (e) {
          if (e.key === "Escape" && openDropdown.value) {
            closeDropdowns();
          }
        });
        loadData().then(function () {
          if (activeTab.value === "routing-config") {
            loadRules();
          }
        });
      });

      onUnmounted(function () {
        document.removeEventListener("click", closeDropdowns);
        stopPollingTask();
      });

      // ========== 返回 ==========

      return {
        loading: loading,
        saving: saving,
        activeTab: activeTab,
        providers: providers,
        models: models,
        llmConfigExpanded: llmConfigExpanded,
        config: config,
        showModelForm: showModelForm,
        editingModel: editingModel,
        modelForm: modelForm,
        modelFormErrors: modelFormErrors,
        detectingModels: detectingModels,
        providerModels: providerModels,
        providerModelSearch: providerModelSearch,
        detectError: detectError,
        testContent: testContent,
        testResult: testResult,
        testing: testing,
        builtinRules: builtinRules,
        customRules: customRules,
        ruleStats: ruleStats,
        rulesLoading: rulesLoading,
        showRuleForm: showRuleForm,
        editingRule: editingRule,
        savingRule: savingRule,
        ruleForm: ruleForm,
        ruleTestMessage: ruleTestMessage,
        ruleTestResult: ruleTestResult,
        ruleTestLoading: ruleTestLoading,
        ruleSearchQuery: ruleSearchQuery,
        ruleFilterTaskType: ruleFilterTaskType,
        ruleFilterSource: ruleFilterSource,
        ruleSortBy: ruleSortBy,
        expandedRuleIds: expandedRuleIds,
        openDropdown: openDropdown,
        filteredAllRules: filteredAllRules,
        groupedProviderModels: groupedProviderModels,
        filteredProviderModelCount: filteredProviderModelCount,
        fallbackStrategyLabel: fallbackStrategyLabel,
        fallbackTaskTypeLabel: fallbackTaskTypeLabel,
        primaryModelLabel: primaryModelLabel,
        fallbackModelLabel: fallbackModelLabel,
        modalProviderLabel: modalProviderLabel,
        allRulesCount: allRulesCount,
        allEnabledCount: allEnabledCount,
        allBuiltinCount: allBuiltinCount,
        allCustomCount: allCustomCount,
        allSimpleCount: allSimpleCount,
        allDefaultCount: allDefaultCount,
        allComplexCount: allComplexCount,
        getProviderName: getProviderName,
        switchTab: switchTab,
        saveConfig: saveConfig,
        selectProvider: selectProvider,
        selectModel: selectModel,
        showModelModal: showModelModal,
        saveModel: saveModel,
        deleteModel: deleteModel,
        showRuleModal: showRuleModal,
        saveRule: saveRule,
        deleteRule: deleteRule,
        ruleFormErrors: ruleFormErrors,
        testRouting: testRouting,
        testRuleMessage: testRuleMessage,
        copyRuleConfig: copyRuleConfig,
        getRuleHit: getRuleHit,
        toggleDropdown: toggleDropdown,
        handleSelectKeydown: handleSelectKeydown,
        toggleRuleExpand: toggleRuleExpand,
        toggleRuleEnabled: toggleRuleEnabled,
        showAnalysisModal: showAnalysisModal,
        analysisTimeRange: analysisTimeRange,
        analysisCustomStart: analysisCustomStart,
        analysisCustomEnd: analysisCustomEnd,
        analysisModelId: analysisModelId,
        analysisModels: analysisModels,
        analysisStarting: analysisStarting,
        analysisTask: analysisTask,
        analysisPolling: analysisPolling,
        smoothProgress: smoothProgress,
        analysisReports: analysisReports,
        analysisReportsTotal: analysisReportsTotal,
        showAnalysisReport: showAnalysisReport,
        currentReport: currentReport,
        openAnalysisModal: openAnalysisModal,
        startAnalysis: startAnalysis,
        viewReport: viewReport,
        loadReportDetail: loadReportDetail,
        closeAnalysisReport: closeAnalysisReport,
        getAnalysisStageLabel: getAnalysisStageLabel,
        isStageCompleted: isStageCompleted,
        getSeverityClass: getSeverityClass,
        getSeverityLabel: getSeverityLabel,
        formatReportDate: formatReportDate,
        findRuleByName: findRuleByName,
        isRecActionable: isRecActionable,
        applyRecommendation: applyRecommendation,
      };
    },
    template:
      '\
<div class="routing-page">\
    <div class="tabs">\
        <button class="tab-btn" :class="{\'active\': activeTab === \'routing-config\'}" @click="switchTab(\'routing-config\')">路由配置</button>\
        <button class="tab-btn" :class="{\'active\': activeTab === \'cache\'}" @click="switchTab(\'cache\')">缓存</button>\
    </div>\
    <div class="tab-content">\
        <div class="tab-pane" :class="{\'active\': activeTab === \'routing-config\'}">\
            <div class="config-single-column">\
                <div class="config-card">\
                    <h4>规则路由配置</h4>\
                    <div class="form-group">\
                        <label class="checkbox-label">\
                            <input type="checkbox" v-model="config.rule_based_routing_enabled">\
                            启用规则路由\
                        </label>\
                        <p class="help-text">开启后优先使用规则匹配，无匹配时按 Fallback 策略处理</p>\
                    </div>\
                    <div class="form-group">\
                        <label class="checkbox-label">\
                            <input type="checkbox" v-model="config.force_smart_routing">\
                            强制智能路由，忽略用户指定的模型\
                        </label>\
                        <p class="help-text">开启后所有请求都走智能路由，忽略用户指定的模型</p>\
                    </div>\
                    <div class="nested-config" v-show="config.rule_based_routing_enabled" v-cloak>\
                        <div class="form-group">\
                            <label>Fallback 策略</label>\
                            <div class="custom-select">\
                                <button type="button" class="custom-select-trigger" :class="{ \'open\': openDropdown === \'fallback-strategy\' }" @click.stop="openDropdown = openDropdown === \'fallback-strategy\' ? null : \'fallback-strategy\'" @keydown="handleSelectKeydown($event, \'fallback-strategy\')" tabindex="0">\
                                    <span>{{ fallbackStrategyLabel }}</span>\
                                    <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                        <polyline points="6 9 12 15 18 9"></polyline>\
                                    </svg>\
                                </button>\
                                <div class="custom-select-dropdown" v-show="openDropdown === \'fallback-strategy\'" v-cloak>\
                                    <button type="button" class="custom-select-option" :class="{ \'selected\': config.rule_fallback_strategy === \'default\' }" @click="config.rule_fallback_strategy = \'default\'; openDropdown = null">使用默认任务类型</button>\
                                    <button type="button" class="custom-select-option" :class="{ \'selected\': config.rule_fallback_strategy === \'llm\' }" @click="config.rule_fallback_strategy = \'llm\'; openDropdown = null">调用 LLM 路由</button>\
                                    <button type="button" class="custom-select-option" :class="{ \'selected\': config.rule_fallback_strategy === \'user\' }" @click="config.rule_fallback_strategy = \'user\'; openDropdown = null">使用指定任务类型</button>\
                                </div>\
                            </div>\
                            <p class="help-text">规则无匹配时的处理策略</p>\
                        </div>\
                        <div class="form-group" v-show="config.rule_fallback_strategy === \'user\'" v-cloak>\
                            <label>指定任务类型</label>\
                            <div class="custom-select">\
                                <button type="button" class="custom-select-trigger" :class="{ \'open\': openDropdown === \'fallback-task-type\' }" @click.stop="openDropdown = openDropdown === \'fallback-task-type\' ? null : \'fallback-task-type\'" @keydown="handleSelectKeydown($event, \'fallback-task-type\')" tabindex="0">\
                                    <span>{{ fallbackTaskTypeLabel }}</span>\
                                    <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                        <polyline points="6 9 12 15 18 9"></polyline>\
                                    </svg>\
                                </button>\
                                <div class="custom-select-dropdown" v-show="openDropdown === \'fallback-task-type\'" v-cloak>\
                                    <button type="button" class="custom-select-option" :class="{ \'selected\': config.rule_fallback_task_type === \'simple\' }" @click="config.rule_fallback_task_type = \'simple\'; openDropdown = null">simple (轻量)</button>\
                                    <button type="button" class="custom-select-option" :class="{ \'selected\': config.rule_fallback_task_type === \'default\' }" @click="config.rule_fallback_task_type = \'default\'; openDropdown = null">default (平衡)</button>\
                                    <button type="button" class="custom-select-option" :class="{ \'selected\': config.rule_fallback_task_type === \'complex\' }" @click="config.rule_fallback_task_type = \'complex\'; openDropdown = null">complex (高能)</button>\
                                </div>\
                            </div>\
                        </div>\
                    </div>\
            <div class="collapsible-section" v-show="config.rule_based_routing_enabled && config.rule_fallback_strategy === \'llm\'" v-cloak>\
                <button class="collapsible-header" @click="llmConfigExpanded = !llmConfigExpanded">\
                    <div class="collapsible-header-left">\
                        <h4>LLM 路由配置</h4>\
                    </div>\
                    <svg class="collapsible-icon" :class="{\'expanded\': llmConfigExpanded}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                        <polyline points="6 9 12 15 18 9"></polyline>\
                    </svg>\
                </button>\
                <div class="collapsible-content" v-show="llmConfigExpanded" v-cloak>\
                    <div class="config-card">\
                        <h4>基础配置</h4>\
                        <div class="form-row">\
                            <div class="form-group">\
                                <label>主路由模型</label>\
                                <div class="custom-select">\
                                    <button type="button" class="custom-select-trigger" :class="{ \'open\': openDropdown === \'primary-model\' }" @click.stop="openDropdown = openDropdown === \'primary-model\' ? null : \'primary-model\'" @keydown="handleSelectKeydown($event, \'primary-model\')" tabindex="0">\
                                        <span>{{ primaryModelLabel }}</span>\
                                        <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                            <polyline points="6 9 12 15 18 9"></polyline>\
                                        </svg>\
                                    </button>\
                                    <div class="custom-select-dropdown" v-show="openDropdown === \'primary-model\'" v-cloak>\
                                        <button type="button" class="custom-select-option" :class="{ \'selected\': config.primary_model_id === \'\' }" @click="config.primary_model_id = \'\'; openDropdown = null">-- 请先添加路由模型 --</button>\
                                        <button type="button" v-for="m in models" :key="m.id" class="custom-select-option" :class="{ \'selected\': config.primary_model_id === m.id }" @click="config.primary_model_id = m.id; openDropdown = null">{{ m.model_name }}</button>\
                                    </div>\
                                </div>\
                            </div>\
                            <div class="form-group">\
                                <label>备用模型</label>\
                                <div class="custom-select">\
                                    <button type="button" class="custom-select-trigger" :class="{ \'open\': openDropdown === \'fallback-model\' }" @click.stop="openDropdown = openDropdown === \'fallback-model\' ? null : \'fallback-model\'" @keydown="handleSelectKeydown($event, \'fallback-model\')" tabindex="0">\
                                        <span>{{ fallbackModelLabel }}</span>\
                                        <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                            <polyline points="6 9 12 15 18 9"></polyline>\
                                        </svg>\
                                    </button>\
                                    <div class="custom-select-dropdown" v-show="openDropdown === \'fallback-model\'" v-cloak>\
                                        <button type="button" class="custom-select-option" :class="{ \'selected\': config.fallback_model_id === \'\' }" @click="config.fallback_model_id = \'\'; openDropdown = null">-- 无 --</button>\
                                        <button type="button" v-for="m in models" :key="m.id" class="custom-select-option" :class="{ \'selected\': config.fallback_model_id === m.id }" @click="config.fallback_model_id = m.id; openDropdown = null">{{ m.model_name }}</button>\
                                    </div>\
                                </div>\
                            </div>\
                        </div>\
                        <div class="form-row">\
                            <div class="form-group">\
                                <label>超时时间（秒）</label>\
                                <input type="number" v-model.number="config.timeout_seconds" min="1" max="30">\
                            </div>\
                            <div class="form-group">\
                                <label>重试次数</label>\
                                <input type="number" v-model.number="config.retry_count" min="0" max="5">\
                            </div>\
                            <div class="form-group">\
                                <label>Max Tokens</label>\
                                <input type="number" v-model.number="config.max_tokens" min="100" max="4096">\
                            </div>\
                            <div class="form-group">\
                                <label>Temperature</label>\
                                <input type="number" v-model.number="config.temperature" min="0" max="1" step="0.1">\
                            </div>\
                        </div>\
                        <p class="help-text">超时、重试、Max Tokens、Temperature 用于调用路由 LLM 进行任务分类</p>\
                    </div>\
\
                    <div class="resource-section">\
                        <div class="resource-header">\
                            <h4>路由模型</h4>\
                            <button class="btn btn-primary" @click="showModelModal()">+ 添加模型</button>\
                        </div>\
                        <div class="routing-models-table-container">\
                            <table class="table">\
                                <thead>\
                                    <tr>\
                                        <th>模型 ID</th>\
                                        <th>服务商</th>\
                                        <th>优先级</th>\
                                        <th>计费倍率</th>\
                                        <th>操作</th>\
                                    </tr>\
                                </thead>\
                                <tbody>\
                                    <tr v-show="loading"><td colspan="5" class="loading">加载中...</td></tr>\
                                    <tr v-show="!loading && models.length === 0" v-cloak><td colspan="5" class="empty">暂无模型，请先添加</td></tr>\
                                    <tr v-for="m in models" :key="m.id">\
                                        <td><code>{{ m.model_name }}</code></td>\
                                        <td>{{ getProviderName(m.provider_id) }}</td>\
                                        <td>{{ m.priority != null ? m.priority : 1 }}</td>\
                                        <td>{{ (m.billing_multiplier != null ? m.billing_multiplier : 1.0) + \'x\' }}</td>\
                                        <td>\
                                            <div class="dropdown">\
                                                <button class="dropdown-trigger" @click.stop="toggleDropdown(\'model-\' + m.id)">\
                                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                                        <circle cx="12" cy="12" r="1"/><circle cx="12" cy="5" r="1"/><circle cx="12" cy="19" r="1"/>\
                                                    </svg>\
                                                </button>\
                                                <div class="dropdown-menu" v-show="openDropdown === \'model-\' + m.id" v-cloak>\
                                                    <button class="dropdown-item" @click="showModelModal(m); openDropdown = null">\
                                                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>\
                                                        编辑\
                                                    </button>\
                                                    <div class="dropdown-divider"></div>\
                                                    <button class="dropdown-item danger" @click="deleteModel(m); openDropdown = null">\
                                                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>\
                                                        删除\
                                                    </button>\
                                                </div>\
                                            </div>\
                                        </td>\
                                    </tr>\
                                </tbody>\
                            </table>\
                        </div>\
                        <div class="models-mobile">\
                            <div v-show="loading" class="loading">加载中...</div>\
                            <div v-show="!loading && models.length === 0" v-cloak class="empty">暂无模型，请先添加</div>\
                            <div v-for="m in models" :key="m.id" class="model-mobile-card">\
                                <div class="model-mobile-header">\
                                    <div class="model-mobile-name">{{ m.model_name }}</div>\
                                </div>\
                                <div class="model-mobile-body">\
                                    <div class="model-mobile-stat">\
                                        <span class="model-mobile-label">服务商</span>\
                                        <span class="model-mobile-value">{{ getProviderName(m.provider_id) }}</span>\
                                    </div>\
                                    <div class="model-mobile-stat">\
                                        <span class="model-mobile-label">优先级</span>\
                                        <span class="model-mobile-value">{{ m.priority != null ? m.priority : 1 }}</span>\
                                    </div>\
                                    <div class="model-mobile-stat">\
                                        <span class="model-mobile-label">计费倍率</span>\
                                        <span class="model-mobile-value">{{ (m.billing_multiplier != null ? m.billing_multiplier : 1.0) + \'x\' }}</span>\
                                    </div>\
                                </div>\
                                <div class="model-mobile-footer">\
                                    <button class="btn btn-sm" @click="showModelModal(m)">编辑</button>\
                                    <button class="btn btn-sm btn-danger" @click="deleteModel(m)">删除</button>\
                                </div>\
                            </div>\
                        </div>\
                    </div>\
                </div>\
            </div>\
            <div class="form-actions">\
                <button class="btn btn-primary" @click="saveConfig()" :disabled="saving">\
                    <span v-show="!saving">保存配置</span>\
                    <span v-show="saving">保存中...</span>\
                </button>\
            </div>\
                </div>\
            </div>\
\
            <div class="resource-section">\
                <div class="resource-header">\
                    <h4>路由规则</h4>\
                    <div class="resource-header-actions">\
                        <button class="btn btn-outline" @click="openAnalysisModal()">\
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width:14px;height:14px;margin-right:4px;vertical-align:-2px;"><path d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.66 0 3-4.03 3-9s-1.34-9-3-9m0 18c-1.66 0-3-4.03-3-9s1.34-9 3-9m-9 9a9 9 0 019-9"/></svg>\
                            分析规则\
                        </button>\
                        <button class="btn btn-primary" @click="showRuleModal()">+ 添加规则</button>\
                    </div>\
                </div>\
                <div class="rules-stats-mini" v-show="allRulesCount > 0" v-cloak>\
                    <div class="rules-stat-chip"><span class="stat-dot dot-total"></span> 总计 <span class="stat-num">{{ allRulesCount }}</span></div>\
                    <div class="rules-stat-chip"><span class="stat-dot dot-enabled"></span> 启用 <span class="stat-num">{{ allEnabledCount }}</span></div>\
                    <div class="rules-stat-chip"><span class="stat-dot dot-builtin"></span> 内置 <span class="stat-num">{{ allBuiltinCount }}</span></div>\
                    <div class="rules-stat-chip"><span class="stat-dot dot-custom"></span> 自定义 <span class="stat-num">{{ allCustomCount }}</span></div>\
                    <div class="rules-stat-chip" v-show="allSimpleCount"><span class="stat-dot dot-simple"></span> simple <span class="stat-num">{{ allSimpleCount }}</span></div>\
                    <div class="rules-stat-chip" v-show="allDefaultCount"><span class="stat-dot dot-default"></span> default <span class="stat-num">{{ allDefaultCount }}</span></div>\
                    <div class="rules-stat-chip" v-show="allComplexCount"><span class="stat-dot dot-complex"></span> complex <span class="stat-num">{{ allComplexCount }}</span></div>\
                    <div class="rules-stat-chip" v-show="ruleStats && ruleStats.total_requests" style="margin-left:auto;"><span class="stat-dot dot-hits"></span> 总命中 <span class="stat-num">{{ ruleStats ? ruleStats.total_requests || 0 : 0 }}</span></div>\
                </div>\
                <div class="rules-toolbar" v-show="allRulesCount > 0" v-cloak>\
                    <div class="rules-search-box">\
                        <svg class="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/></svg>\
                        <input type="text" v-model="ruleSearchQuery" placeholder="搜索名称、描述、关键词...">\
                    </div>\
                    <div class="rules-filter-group">\
                        <button class="rules-filter-btn" :class="{\'active\': ruleFilterSource === \'\'}" @click="ruleFilterSource = \'\'">全部</button>\
                        <button class="rules-filter-btn" :class="{\'active\': ruleFilterSource === \'builtin\'}" @click="ruleFilterSource = ruleFilterSource === \'builtin\' ? \'\' : \'builtin\'">内置</button>\
                        <button class="rules-filter-btn" :class="{\'active\': ruleFilterSource === \'custom\'}" @click="ruleFilterSource = ruleFilterSource === \'custom\' ? \'\' : \'custom\'">自定义</button>\
                    </div>\
                    <div class="rules-toolbar-sep"></div>\
                    <div class="rules-filter-group">\
                        <button class="rules-filter-btn" :class="{\'active\': ruleFilterTaskType === \'\'}" @click="ruleFilterTaskType = \'\'">全部</button>\
                        <button class="rules-filter-btn" :class="{\'active\': ruleFilterTaskType === \'simple\'}" @click="ruleFilterTaskType = ruleFilterTaskType === \'simple\' ? \'\' : \'simple\'">simple</button>\
                        <button class="rules-filter-btn" :class="{\'active\': ruleFilterTaskType === \'default\'}" @click="ruleFilterTaskType = ruleFilterTaskType === \'default\' ? \'\' : \'default\'">default</button>\
                        <button class="rules-filter-btn" :class="{\'active\': ruleFilterTaskType === \'complex\'}" @click="ruleFilterTaskType = ruleFilterTaskType === \'complex\' ? \'\' : \'complex\'">complex</button>\
                    </div>\
                    <select class="rules-sort-select" v-model="ruleSortBy">\
                        <option value="priority-desc">优先级 高→低</option>\
                        <option value="priority-asc">优先级 低→高</option>\
                        <option value="hits-desc">命中数 高→低</option>\
                        <option value="hits-asc">命中数 低→高</option>\
                        <option value="name-asc">名称 A→Z</option>\
                        <option value="name-desc">名称 Z→A</option>\
                    </select>\
                </div>\
                <div v-show="rulesLoading" class="rules-empty-state" style="padding: var(--spacing-xl);">加载中...</div>\
\
                <div class="rules-cards-grid" v-show="!rulesLoading" v-cloak>\
                    <div class="rules-empty-state" v-show="filteredAllRules.length === 0">\
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>\
                        <div v-show="allRulesCount === 0">暂无规则，点击上方按钮添加</div>\
                        <div v-show="allRulesCount > 0">无匹配结果</div>\
                        <button class="rules-clear-filters" v-show="ruleSearchQuery || ruleFilterTaskType || ruleFilterSource" @click="ruleSearchQuery=\'\'; ruleFilterTaskType=\'\'; ruleFilterSource=\'\'">清除过滤条件</button>\
                    </div>\
                    <div v-for="r in filteredAllRules" :key="r._source + \'-\' + r.id" class="rule-card" :class="{\'card-disabled\': !r.enabled}">\
                        <div class="rule-card-header">\
                            <div class="rule-card-title">\
                                <div class="rule-card-name">{{ r.name }}</div>\
                                <div class="rule-card-desc" v-show="r.description">{{ r.description }}</div>\
                            </div>\
                            <div class="rule-card-badges">\
                                <span class="rule-source-badge" :class="\'source-\' + r._source">{{ r._source === \'builtin\' ? \'内置\' : \'自定义\' }}</span>\
                                <span class="task-type-badge" :class="\'type-\' + r.task_type">{{ r.task_type }}</span>\
                            </div>\
                        </div>\
                        <div class="rule-card-body">\
                            <div class="rule-card-meta">\
                                <span class="rule-card-meta-item">优先级: <span class="meta-value">{{ r.priority }}</span></span>\
                                <span class="rule-card-meta-item meta-sep">|</span>\
                                <span class="rule-card-meta-item" v-show="getRuleHit(r.id)">命中: <span class="meta-value">{{ getRuleHit(r.id) ? getRuleHit(r.id).count : 0 }}</span> <span class="text-muted" style="font-size:10px">{{ getRuleHit(r.id) && getRuleHit(r.id).percentage ? \'(\' + getRuleHit(r.id).percentage.toFixed(1) + \'%)\' : \'\' }}</span></span>\
                                <span class="rule-card-meta-item meta-sep" v-show="getRuleHit(r.id)">|</span>\
                                <span class="rule-card-meta-item" v-show="r._source === \'builtin\'"><span class="status-pill" :class="r.enabled ? \'status-enabled\' : \'status-disabled\'" style="padding:2px 6px;font-size:10px;">{{ r.enabled ? \'启用\' : \'禁用\' }}</span></span>\
                                <span class="rule-card-meta-item" v-show="r._source !== \'builtin\'"><label class="toggle-switch" @click.stop><input type="checkbox" :checked="r.enabled" @change="toggleRuleEnabled(r)"><span class="toggle-slider"></span></label></span>\
                            </div>\
                            <div class="rule-card-keywords" v-show="r.keywords && r.keywords.length > 0">\
                                <span v-for="kw in (r.keywords || []).slice(0, 8)" :key="kw" class="rule-keyword-tag">{{ kw }}</span>\
                                <span class="rule-keyword-tag" v-show="(r.keywords || []).length > 8">{{ \'+\' + ((r.keywords || []).length - 8) }}</span>\
                            </div>\
                            <div v-show="!r.keywords || r.keywords.length === 0" class="text-muted" style="font-size:var(--font-size-xs);">无关键词匹配</div>\
                        </div>\
                        <div class="rule-card-expand" v-show="expandedRuleIds.has(r._source + \'-\' + r.id)" v-cloak>\
                            <div class="rule-card-detail">\
                                <div class="rule-card-detail-row" v-show="r.pattern">\
                                    <span class="rule-card-detail-label">正则:</span>\
                                    <span class="rule-card-detail-value">{{ r.pattern }}</span>\
                                </div>\
                                <div class="rule-card-detail-row" v-show="r.condition">\
                                    <span class="rule-card-detail-label">条件:</span>\
                                    <span class="rule-card-detail-value">{{ r.condition }}</span>\
                                </div>\
                                <div class="rule-card-detail-row" v-show="r.keywords && r.keywords.length > 8">\
                                    <span class="rule-card-detail-label">全部关键词:</span>\
                                    <span class="rule-card-detail-value">{{ (r.keywords || []).join(\', \') }}</span>\
                                </div>\
                            </div>\
                        </div>\
                        <div class="rule-card-footer">\
                            <button class="rule-card-action" :class="{\'expanded\': expandedRuleIds.has(r._source + \'-\' + r.id)}" @click="toggleRuleExpand(r._source + \'-\' + r.id)" v-show="r.pattern || r.condition || (r.keywords && r.keywords.length > 8)">\
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>\
                                <span>{{ expandedRuleIds.has(r._source + \'-\' + r.id) ? \'收起\' : \'详情\' }}</span>\
                            </button>\
                            <button class="rule-card-action" @click="showRuleModal(r)">\
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>\
                                编辑\
                            </button>\
                            <button class="rule-card-action" @click="copyRuleConfig(r)">\
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>\
                                复制\
                            </button>\
                            <button class="rule-card-action rule-card-action--danger" v-show="r._source === \'custom\'" @click="deleteRule(r)">\
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>\
                                删除\
                            </button>\
                        </div>\
                    </div>\
                </div>\
            </div>\
\
            <div class="resource-section">\
                <div class="resource-header">\
                    <h4>规则测试</h4>\
                </div>\
                <div class="config-card">\
                    <p class="help-text">输入消息内容，测试规则匹配结果</p>\
                    <div class="form-group">\
                        <textarea v-model="ruleTestMessage" placeholder="输入测试消息内容..." rows="3"></textarea>\
                    </div>\
                    <button class="btn btn-primary" @click="testRuleMessage()" :disabled="ruleTestLoading">\
                        <span v-show="!ruleTestLoading">测试规则</span>\
                        <span v-show="ruleTestLoading">测试中...</span>\
                    </button>\
                    <div v-show="ruleTestResult" v-cloak class="test-result" style="margin-top: var(--spacing-md);">\
                        <div class="result-item"><strong>最终任务类型:</strong> <span class="task-type-badge" :class="\'type-\' + (ruleTestResult ? ruleTestResult.final_task_type : \'\')">{{ ruleTestResult ? ruleTestResult.final_task_type || \'-\' : \'-\' }}</span></div>\
                        <div class="result-item"><strong>匹配规则:</strong> <span>{{ ruleTestResult && ruleTestResult.matched_rule ? ruleTestResult.matched_rule.name : \'无匹配\' }}</span></div>\
                        <div class="result-item"><strong>匹配原因:</strong> <span>{{ ruleTestResult ? ruleTestResult.reason || \'-\' : \'-\' }}</span></div>\
                        <div class="result-item" v-show="ruleTestResult && ruleTestResult.all_matches && ruleTestResult.all_matches.length > 0">\
                            <strong>所有匹配:</strong>\
                            <ul style="margin: var(--spacing-xs) 0 0 var(--spacing-md); padding: 0;">\
                                <li v-for="mt in (ruleTestResult ? ruleTestResult.all_matches || [] : [])" :key="mt.rule_name">{{ mt.rule_name + \' → \' + mt.task_type + \' (优先级: \' + mt.priority + \')\' }}</li>\
                            </ul>\
                        </div>\
                    </div>\
                </div>\
            </div>\
        </div>\
        <div class="tab-pane" :class="{\'active\': activeTab === \'cache\'}">\
            <div class="config-single-column">\
                <div class="config-card">\
                    <h4>缓存配置</h4>\
                    <div class="form-group">\
                        <label class="checkbox-label">\
                            <input type="checkbox" v-model="config.cache_enabled">\
                            启用缓存\
                        </label>\
                        <p class="help-text">缓存路由决策结果，减少 LLM 调用</p>\
                    </div>\
                    <div class="nested-config" v-show="config.cache_enabled" v-cloak>\
                        <div class="form-group">\
                            <label>L1/L2 缓存时间（秒）</label>\
                            <input type="number" v-model.number="config.cache_ttl_seconds" min="60" max="86400">\
                            <p class="help-text">L1 内存缓存和 L2 精确匹配缓存的过期时间</p>\
                        </div>\
                    </div>\
                </div>\
            </div>\
            <div class="form-actions">\
                <button class="btn btn-primary" @click="saveConfig()" :disabled="saving">\
                    <span v-show="!saving">保存配置</span>\
                    <span v-show="saving">保存中...</span>\
                </button>\
            </div>\
        </div>\
    </div>\
    <div class="section">\
        <h3>路由测试</h3>\
        <p class="help-text">输入测试内容，查看路由决策结果</p>\
        <div class="routing-test">\
            <textarea v-model="testContent" placeholder="输入测试消息内容..." rows="4"></textarea>\
            <button class="btn btn-primary" @click="testRouting()" :disabled="testing">\
                <span v-show="!testing">测试路由</span>\
                <span v-show="testing">测试中...</span>\
            </button>\
            <div v-show="testResult" v-cloak class="test-result">\
                <div class="result-item"><strong>推断任务类型:</strong> <span>{{ testResult ? testResult.inferred_task_type || \'-\' : \'-\' }}</span></div>\
                <div class="result-item"><strong>选择角色:</strong> <span :class="\'role-\' + (testResult ? testResult.selected_role : \'\')">{{ testResult ? testResult.selected_role || \'-\' : \'-\' }}</span></div>\
                <div class="result-item"><strong>选择模型:</strong> <span>{{ testResult ? testResult.selected_model || \'无可用模型\' : \'-\' }}</span></div>\
                <div class="result-item"><strong>路由方式:</strong> <span>{{ testResult ? testResult.routing_method || \'llm\' : \'-\' }}</span></div>\
                <div class="result-item"><strong>决策理由:</strong> <span>{{ testResult ? testResult.reasoning || \'-\' : \'-\' }}</span></div>\
                <div class="result-item"><strong>缓存命中:</strong> <span>{{ testResult ? (testResult.cache_hit ? \'是\' : \'否\') : \'-\' }}</span></div>\
            </div>\
        </div>\
    </div>\
\
    <div class="modal" v-show="showModelForm" v-cloak @keydown.escape="showModelForm = false">\
        <div class="modal-content modal-lg" @click.stop>\
            <div class="modal-header">\
                <h3>{{ editingModel ? \'编辑路由模型\' : \'添加路由模型\' }}</h3>\
                <button class="modal-close" @click="showModelForm = false">&times;</button>\
            </div>\
            <div class="modal-body">\
                <form @submit.prevent="saveModel">\
                    <div class="form-group">\
                        <label>所属服务商 *</label>\
                        <div v-if="providers.length === 0" class="empty-provider-hint">\
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" style="width:20px;height:20px;opacity:0.5;">\
                                <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>\
                            </svg>\
                            <span>暂无服务商，请先在 <a href="/providers">模型管理</a> 页面添加服务商</span>\
                        </div>\
                        <div v-if="providers.length > 0" class="custom-select">\
                            <button type="button" class="custom-select-trigger" :class="{ \'open\': openDropdown === \'modal-provider\', \'error\': modelFormErrors.provider_id }" @click.stop="openDropdown = openDropdown === \'modal-provider\' ? null : \'modal-provider\'" @keydown="handleSelectKeydown($event, \'modal-provider\')" tabindex="0">\
                                <span>{{ modalProviderLabel }}</span>\
                                <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                    <polyline points="6 9 12 15 18 9"></polyline>\
                                </svg>\
                            </button>\
                            <div class="custom-select-dropdown" v-show="openDropdown === \'modal-provider\'" v-cloak>\
                                <button type="button" class="custom-select-option" :class="{ \'selected\': !modelForm.provider_id }" @click="modelForm.provider_id = \'\'; openDropdown = null">-- 请选择服务商 --</button>\
                                <button type="button" v-for="p in providers" :key="p.id" class="custom-select-option" :class="{ \'selected\': modelForm.provider_id === p.id }" @click="selectProvider(p.id); openDropdown = null">{{ p.name }}</button>\
                            </div>\
                        </div>\
                        <p class="error-text" v-if="modelFormErrors.provider_id">{{ modelFormErrors.provider_id }}</p>\
                    </div>\
                    <div class="form-group" v-show="modelForm.provider_id" v-cloak>\
                        <label>模型 ID *</label>\
                        <input type="text" v-model="modelForm.model_name" placeholder="选择下方检测到的模型，或手动输入模型 ID" required :class="{ \'error\': modelFormErrors.model_name }">\
                        <p class="error-text" v-if="modelFormErrors.model_name">{{ modelFormErrors.model_name }}</p>\
                        <div class="detected-models-panel" v-show="detectingModels" v-cloak>\
                            <div class="detected-models-skeleton">\
                                <div class="skeleton-group">\
                                    <div class="skeleton-header"></div>\
                                    <div class="skeleton-tags">\
                                        <div class="skeleton-tag"></div>\
                                        <div class="skeleton-tag"></div>\
                                        <div class="skeleton-tag"></div>\
                                    </div>\
                                </div>\
                                <div class="skeleton-group">\
                                    <div class="skeleton-header"></div>\
                                    <div class="skeleton-tags">\
                                        <div class="skeleton-tag"></div>\
                                        <div class="skeleton-tag"></div>\
                                    </div>\
                                </div>\
                            </div>\
                        </div>\
\
                        <div class="detected-models-panel" v-show="!detectingModels && providerModels.length > 0" v-cloak>\
                            <div class="detected-models-header">\
                                <div class="detected-models-header-left">\
                                    <span>{{ \'检测到 \' + providerModels.length + \' 个模型\' }}</span>\
                                </div>\
                            </div>\
                            <div class="detected-models-search" v-show="providerModels.length > 5">\
                                <svg class="detected-models-search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                    <circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>\
                                </svg>\
                                <input type="text" v-model="providerModelSearch" placeholder="搜索模型...">\
                            </div>\
                            <div class="detected-models-body">\
                                <div class="detected-models-empty" v-show="filteredProviderModelCount === 0">\
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">\
                                        <circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>\
                                    </svg>\
                                    <div>未找到匹配的模型</div>\
                                </div>\
                                <div v-for="group in groupedProviderModels" :key="group.key" class="detected-group">\
                                    <div class="detected-group-header">\
                                        <span class="detected-group-indicator" :class="\'tier-\' + group.tier"></span>\
                                        <span>{{ group.label }}</span>\
                                        <span class="detected-group-count">{{ group.models.length }}</span>\
                                    </div>\
                                    <div class="detected-group-models">\
                                        <span v-for="dm in group.models" :key="dm.id" class="detected-model-tag"\
                                              :class="[\'tier-\' + group.tier, modelForm.model_name === dm.id ? \'is-selected\' : \'\']"\
                                              :title="dm.display_name || dm.id"\
                                              @click="selectModel(dm.id)">\
                                            <svg v-show="modelForm.model_name === dm.id" class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">\
                                                <polyline points="20 6 9 17 4 12"/>\
                                            </svg>\
                                            <span>{{ dm.id }}</span>\
                                        </span>\
                                    </div>\
                                </div>\
                            </div>\
                        </div>\
                        <div class="detected-models-error" v-show="detectError" v-cloak>{{ detectError }}</div>\
                    </div>\
                    <div class="form-group" v-show="modelForm.provider_id" v-cloak>\
                        <label>优先级</label>\
                        <input type="number" v-model.number="modelForm.priority" min="1" max="100">\
                        <p class="help-text">数字越小优先级越高</p>\
                    </div>\
                    <div class="form-group" v-show="modelForm.provider_id" v-cloak>\
                        <label>计费倍率</label>\
                        <input type="number" v-model.number="modelForm.billing_multiplier" step="0.1" min="0.1">\
                        <p class="help-text">最终成本 = 基础成本 × 倍率</p>\
                    </div>\
                </form>\
            </div>\
            <div class="modal-footer">\
                <button type="button" class="btn" @click="showModelForm = false">取消</button>\
                <button type="button" class="btn btn-primary" :disabled="saving || !modelForm.provider_id" @click="saveModel">保存</button>\
            </div>\
        </div>\
    </div>\
\
    <div class="modal modal-top" v-show="showRuleForm" v-cloak @keydown.escape="showRuleForm = false">\
        <div class="modal-content" @click.stop>\
            <div class="modal-header">\
                <h3>{{ editingRule ? \'编辑路由规则\' : \'添加路由规则\' }}</h3>\
                <button class="modal-close" @click="showRuleForm = false">&times;</button>\
            </div>\
            <div class="modal-body">\
                <form @submit.prevent="saveRule">\
                    <div class="form-group">\
                        <label>规则名称 *</label>\
                        <input type="text" v-model="ruleForm.name" placeholder="如: my_custom_rule" required :class="{ \'error\': ruleFormErrors.name }">\
                        <p class="error-text" v-if="ruleFormErrors.name">{{ ruleFormErrors.name }}</p>\
                    </div>\
                    <div class="form-group">\
                        <label>描述</label>\
                        <input type="text" v-model="ruleForm.description" placeholder="规则用途说明">\
                    </div>\
                    <div class="form-group">\
                        <label>关键词</label>\
                        <input type="text" v-model="ruleForm.keywords" placeholder="多个关键词用逗号分隔，如: 架构, 设计, 重构">\
                        <p class="help-text">消息中包含任一关键词即匹配（OR 关系）</p>\
                    </div>\
                    <div class="form-group">\
                        <label>正则表达式</label>\
                        <input type="text" v-model="ruleForm.pattern" placeholder="如: (?i)(设计|规划).*(系统|架构)" :class="{ \'error\': ruleFormErrors.pattern }">\
                        <p class="error-text" v-if="ruleFormErrors.pattern">{{ ruleFormErrors.pattern }}</p>\
                        <p class="help-text" v-else>Go 正则语法，留空则不使用正则匹配</p>\
                    </div>\
                    <div class="form-group">\
                        <label>条件表达式</label>\
                        <input type="text" v-model="ruleForm.condition" placeholder="如: len(message) > 2000 AND contains(message, \'分析\')" :class="{ \'error\': ruleFormErrors.condition }">\
                        <p class="error-text" v-if="ruleFormErrors.condition">{{ ruleFormErrors.condition }}</p>\
                        <p class="help-text" v-else>支持: len(), contains(), has_code_block(), count(), matches(), AND/OR/NOT</p>\
                    </div>\
                    <div class="form-row">\
                        <div class="form-group">\
                            <label>任务类型 *</label>\
                            <select v-model="ruleForm.task_type" :class="{ \'error\': ruleFormErrors.task_type }">\
                                <option value="simple">simple</option>\
                                <option value="default">default</option>\
                                <option value="complex">complex</option>\
                            </select>\
                            <p class="error-text" v-if="ruleFormErrors.task_type">{{ ruleFormErrors.task_type }}</p>\
                        </div>\
                        <div class="form-group">\
                            <label>优先级</label>\
                            <input type="number" v-model.number="ruleForm.priority" min="1" max="1000">\
                            <p class="help-text">数字越大优先级越高</p>\
                        </div>\
                    </div>\
                    <div class="form-group">\
                        <label class="checkbox-label">\
                            <input type="checkbox" v-model="ruleForm.enabled">\
                            启用规则\
                        </label>\
                    </div>\
                </form>\
            </div>\
            <div class="modal-footer">\
                <button type="button" class="btn" @click="showRuleForm = false">取消</button>\
                <button type="button" class="btn btn-primary" :disabled="savingRule" @click="saveRule">\
                    <span v-show="!savingRule">保存</span>\
                    <span v-show="savingRule">保存中...</span>\
                </button>\
            </div>\
        </div>\
    </div>\
\
    <div class="modal" v-show="showAnalysisModal" v-cloak @keydown.escape="showAnalysisModal = false">\
        <div class="modal-content modal-lg" @click.stop>\
            <div class="modal-header">\
                <h3>路由规则分析</h3>\
                <button class="modal-close" @click="showAnalysisModal = false">&times;</button>\
            </div>\
            <div class="modal-body">\
                <div v-show="!showAnalysisReport && (!analysisTask || analysisTask.status === \'failed\')">\
                    <div class="form-group">\
                        <label>分析时间范围</label>\
                        <div class="rules-filter-group">\
                            <button class="rules-filter-btn" :class="{\'active\': analysisTimeRange === \'1d\'}" @click="analysisTimeRange = \'1d\'">1 天</button>\
                            <button class="rules-filter-btn" :class="{\'active\': analysisTimeRange === \'7d\'}" @click="analysisTimeRange = \'7d\'">7 天</button>\
                            <button class="rules-filter-btn" :class="{\'active\': analysisTimeRange === \'30d\'}" @click="analysisTimeRange = \'30d\'">30 天</button>\
                            <button class="rules-filter-btn" :class="{\'active\': analysisTimeRange === \'custom\'}" @click="analysisTimeRange = \'custom\'">自定义</button>\
                        </div>\
                    </div>\
                    <div class="form-row" v-show="analysisTimeRange === \'custom\'" v-cloak>\
                        <div class="form-group">\
                            <label>开始时间</label>\
                            <input type="datetime-local" v-model="analysisCustomStart">\
                        </div>\
                        <div class="form-group">\
                            <label>结束时间</label>\
                            <input type="datetime-local" v-model="analysisCustomEnd">\
                        </div>\
                    </div>\
                    <div class="form-group">\
                        <label>分析模型</label>\
                        <select v-model="analysisModelId">\
                            <option value="" disabled>-- 请选择模型 --</option>\
                            <option v-for="m in analysisModels" :key="m.id" :value="m.id">{{ m.model_name }}</option>\
                        </select>\
                        <p class="help-text">选择用于分析的 LLM 模型，建议使用高能模型获得更准确的分析结果</p>\
                    </div>\
                    <div class="form-group" v-show="analysisModels.length === 0" v-cloak>\
                        <p class="help-text" style="color: var(--danger-color)">暂无可用模型，请先在 LLM 路由配置中添加路由模型</p>\
                    </div>\
                    <button class="btn btn-primary" @click="startAnalysis()" :disabled="analysisStarting || analysisModels.length === 0" style="margin-top: var(--spacing-sm)">\
                        <span v-show="!analysisStarting">开始分析</span>\
                        <span v-show="analysisStarting">启动中...</span>\
                    </button>\
                </div>\
\
                <div v-show="analysisTask && (analysisTask.status === \'pending\' || analysisTask.status === \'running\')" v-cloak class="analysis-progress">\
                    <div class="analysis-progress-header">\
                        <h4>分析进行中</h4>\
                        <span class="analysis-progress-pct">{{ Math.round(smoothProgress) }}%</span>\
                    </div>\
                    <div class="progress-bar-container">\
                        <div class="progress-bar-fill" :style="\'width:\' + Math.round(smoothProgress) + \'%\'"></div>\
                    </div>\
                    <div class="analysis-steps">\
                        <div v-for="s in [{key:\'collecting_logs\',label:\'收集日志\'},{key:\'extracting_messages\',label:\'提取消息\'},{key:\'loading_rules\',label:\'加载规则\'},{key:\'building_prompt\',label:\'构建提示词\'},{key:\'calling_llm\',label:\'LLM 分析\'},{key:\'parsing_result\',label:\'解析结果\'}]" :key="s.key" class="analysis-step" :class="{\'step-active\': analysisTask && analysisTask.stage === s.key, \'step-done\': isStageCompleted(analysisTask ? analysisTask.stage : \'\', s.key)}">\
                            <span class="step-dot"></span>\
                            <span class="step-label">{{ s.label }}</span>\
                        </div>\
                    </div>\
                </div>\
\
                <div v-show="showAnalysisReport && currentReport" v-cloak class="analysis-report">\
                    <div class="analysis-report-header">\
                        <button class="btn btn-sm" @click="closeAnalysisReport()" style="margin-bottom:var(--spacing-sm)">&larr; 返回</button>\
                        <div class="analysis-report-meta">\
                            <span v-show="currentReport && currentReport.model_used">模型: <code>{{ currentReport ? currentReport.model_used : \'\' }}</code></span>\
                            <span v-show="currentReport && currentReport.total_logs">日志: {{ (currentReport ? currentReport.analyzed_logs : 0) + \'/\' + (currentReport ? currentReport.total_logs : 0) }}</span>\
                        </div>\
                    </div>\
\
                    <div class="analysis-summary-cards" v-show="currentReport && currentReport.summary" v-cloak>\
                        <div class="analysis-summary-card">\
                            <div class="analysis-summary-label">规则匹配率</div>\
                            <div class="analysis-summary-value">{{ currentReport && currentReport.summary ? ((currentReport.summary.rule_match_rate * 100).toFixed(1) + \'%\') : \'-\' }}</div>\
                        </div>\
                        <div class="analysis-summary-card">\
                            <div class="analysis-summary-label">LLM 回退率</div>\
                            <div class="analysis-summary-value">{{ currentReport && currentReport.summary ? ((currentReport.summary.llm_fallback_rate * 100).toFixed(1) + \'%\') : \'-\' }}</div>\
                        </div>\
                        <div class="analysis-summary-card">\
                            <div class="analysis-summary-label">不准确率</div>\
                            <div class="analysis-summary-value">{{ currentReport && currentReport.summary ? ((currentReport.summary.inaccurate_rate * 100).toFixed(1) + \'%\') : \'-\' }}</div>\
                        </div>\
                    </div>\
\
                    <div class="analysis-section" v-show="currentReport && currentReport.issues && currentReport.issues.length > 0" v-cloak>\
                        <h4>发现问题 ({{ currentReport ? currentReport.issues.length : 0 }})</h4>\
                        <div v-for="(issue, idx) in (currentReport ? currentReport.issues : [])" :key="idx" class="analysis-issue-card" :class="getSeverityClass(issue.severity)">\
                            <div class="analysis-issue-header">\
                                <span class="analysis-issue-type">{{ issue.type }}</span>\
                                <span class="analysis-severity-badge" :class="getSeverityClass(issue.severity)">{{ getSeverityLabel(issue.severity) }}</span>\
                            </div>\
                            <div class="analysis-issue-rule" v-show="issue.rule_name">规则: <code>{{ issue.rule_name }}</code></div>\
                            <div class="analysis-issue-desc">{{ issue.description }}</div>\
                            <div class="analysis-issue-examples" v-show="issue.examples && issue.examples.length > 0">\
                                <div class="analysis-example" v-for="(ex, eidx) in (issue.examples || []).slice(0, 3)" :key="eidx">{{ ex }}</div>\
                            </div>\
                        </div>\
                    </div>\
\
                    <div class="analysis-section" v-show="currentReport && currentReport.recommendations && currentReport.recommendations.length > 0" v-cloak>\
                        <h4>优化建议 ({{ currentReport ? currentReport.recommendations.length : 0 }})</h4>\
                        <div v-for="(rec, idx) in (currentReport ? currentReport.recommendations : [])" :key="idx" class="analysis-rec-card">\
                            <div class="analysis-rec-header">\
                                <span class="analysis-rec-action" :class="\'action-\' + rec.action">{{ rec.action }}</span>\
                                <span class="analysis-rec-rule" v-show="rec.rule_name"><code>{{ rec.rule_name }}</code></span>\
                            </div>\
                            <div class="analysis-rec-desc">{{ rec.description }}</div>\
                            <div class="analysis-rec-details" v-show="rec.details">{{ rec.details }}</div>\
                            <button class="rec-apply-btn" v-show="isRecActionable(rec)" @click="applyRecommendation(rec)">应用建议</button>\
                        </div>\
                    </div>\
\
                    <div class="analysis-section" v-show="currentReport && currentReport.conclusion" v-cloak>\
                        <h4>总结</h4>\
                        <div class="analysis-conclusion">{{ currentReport ? currentReport.conclusion : \'\' }}</div>\
                    </div>\
                </div>\
\
                <div class="analysis-section" v-show="!showAnalysisReport && analysisReports.length > 0" v-cloak style="margin-top: var(--spacing-lg);">\
                    <h4>历史报告 ({{ analysisReportsTotal }})</h4>\
                    <div class="analysis-reports-list">\
                        <div v-for="r in analysisReports" :key="r.id" class="analysis-report-item" @click="loadReportDetail(r.id)">\
                            <div class="analysis-report-item-info">\
                                <span class="analysis-report-item-date">{{ formatReportDate(r.created_at) }}</span>\
                                <span class="analysis-report-item-model"><code>{{ r.model_used }}</code></span>\
                            </div>\
                            <div class="analysis-report-item-stats">\
                                <span>{{ r.analyzed_logs + \'/\' + r.total_logs + \' 条日志\' }}</span>\
                            </div>\
                        </div>\
                    </div>\
                </div>\
            </div>\
        </div>\
    </div>\
</div>',
  };
})();
