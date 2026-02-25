/**
 * 模型与服务商管理页面
 * 支持模型 CRUD、服务商 CRUD、模型检测、关联管理
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

  window.VuePages.ModelProviderPage = {
    name: "ModelProviderPage",
    setup: function () {
      var toastStore = inject("toastStore");
      var confirmStore = inject("confirmStore");

      // 状态
      var loading = ref(true);
      var saving = ref(false);
      var activeTab = ref("overview");
      var providers = ref([]);
      var models = ref([]);

      // 模态框状态
      var showModelModal = ref(false);
      var showProviderModal = ref(false);
      var showApiKey = ref(false);
      var editingModel = ref(null);
      var editingProvider = ref(null);

      // 模型检测状态
      var detecting = ref(false);
      var detectedModels = ref([]);
      var detectedApiFormat = ref("");
      var detectError = ref("");
      var modelSearchQuery = ref("");

      // 下拉菜单
      var openDropdown = ref(null);
      var roleSelectOpen = ref(false);

      // 表单数据
      var modelForm = reactive({
        name: "",
        role: "default",
        cost_per_mtok_input: 0,
        cost_per_mtok_output: 0,
        billing_multiplier: 1.0,
        weight: 100,
        supports_thinking: false,
        enabled: true,
      });
      var providerForm = reactive({
        name: "",
        base_url: "",
        api_key: "",
        weight: 1,
        max_concurrent: 10,
        enabled: true,
        description: "",
        model_ids: [],
      });

      // 角色选项
      var roleOptions = [
        { value: "simple", label: "simple - 简单任务" },
        { value: "default", label: "default - 默认任务" },
        { value: "complex", label: "complex - 复杂任务" },
      ];

      // tier 轮换
      var tierCycle = ["default", "complex", "simple"];
      function getGroupTier(index) {
        return tierCycle[index % tierCycle.length];
      }

      // 计算属性
      var relationCount = computed(function () {
        var count = 0;
        providers.value.forEach(function (p) {
          count += (p.models || []).length;
        });
        return count;
      });

      var unassociatedModels = computed(function () {
        var associatedIds = {};
        providers.value.forEach(function (p) {
          (p.models || []).forEach(function (m) {
            associatedIds[m.id] = true;
          });
        });
        return models.value.filter(function (m) {
          return m.enabled && !associatedIds[m.id];
        });
      });

      var enabledModels = computed(function () {
        return models.value.filter(function (m) {
          return m.enabled;
        });
      });

      var groupedDetectedModels = computed(function () {
        if (!detectedModels.value || detectedModels.value.length === 0) {
          return [];
        }
        var query = (modelSearchQuery.value || "").toLowerCase().trim();
        var filtered = query
          ? detectedModels.value.filter(function (dm) {
              if (!dm || !dm.id) return false;
              return (
                dm.id.toLowerCase().indexOf(query) !== -1 ||
                (dm.display_name || "").toLowerCase().indexOf(query) !== -1
              );
            })
          : detectedModels.value;

        var groups = {};
        for (var i = 0; i < filtered.length; i++) {
          var dm = filtered[i];
          if (!dm || !dm.id) continue;
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
            return {
              key: g.key,
              label: g.label,
              models: g.models,
              tier: getGroupTier(idx),
            };
          });
      });

      var filteredModelCount = computed(function () {
        if (!detectedModels.value || detectedModels.value.length === 0)
          return 0;
        var query = (modelSearchQuery.value || "").toLowerCase().trim();
        if (!query) return detectedModels.value.length;
        return detectedModels.value.filter(function (dm) {
          if (!dm || !dm.id) return false;
          return (
            dm.id.toLowerCase().indexOf(query) !== -1 ||
            (dm.display_name || "").toLowerCase().indexOf(query) !== -1
          );
        }).length;
      });

      var roleLabel = computed(function () {
        var found = roleOptions.find(function (o) {
          return o.value === modelForm.role;
        });
        return found ? found.label : "请选择";
      });

      // 缩写模型名称
      function shortenModelName(name) {
        var match = name.match(/claude-(\w+)-(\d+)-(\d+)/);
        return match ? match[1] + "-" + match[2] + "." + match[3] : name;
      }

      // 初始化 tab（不再读取 hash，避免与全局路由冲突）
      function initTab() {
        // tab 状态仅在组件内管理
      }

      // 切换 Tab
      function switchTab(tabName) {
        activeTab.value = tabName;
      }

      // 加载数据
      async function loadData() {
        loading.value = true;
        try {
          var results = await Promise.all([
            VueApi.get("/api/config/providers"),
            VueApi.get("/api/config/models"),
          ]);
          var providersData = await results[0].json();
          var modelsData = await results[1].json();
          providers.value = providersData.providers || [];
          models.value = modelsData.models || [];
        } catch (error) {
          toastStore.error("加载失败: " + error.message);
        } finally {
          loading.value = false;
        }
      }

      // ========== 模型操作 ==========

      function showAddModelModal() {
        editingModel.value = null;
        modelForm.name = "";
        modelForm.role = "default";
        modelForm.cost_per_mtok_input = 0;
        modelForm.cost_per_mtok_output = 0;
        modelForm.billing_multiplier = 1.0;
        modelForm.weight = 100;
        modelForm.supports_thinking = false;
        modelForm.enabled = true;
        roleSelectOpen.value = false;
        showModelModal.value = true;
      }

      function showEditModelModal(model) {
        editingModel.value = model;
        modelForm.name = model.name;
        modelForm.role = model.role;
        modelForm.cost_per_mtok_input = model.cost_per_mtok_input;
        modelForm.cost_per_mtok_output = model.cost_per_mtok_output;
        modelForm.billing_multiplier =
          model.billing_multiplier != null ? model.billing_multiplier : 1.0;
        modelForm.weight = model.weight != null ? model.weight : 100;
        modelForm.supports_thinking = !!model.supports_thinking;
        modelForm.enabled = !!model.enabled;
        roleSelectOpen.value = false;
        showModelModal.value = true;
      }

      async function saveModel() {
        saving.value = true;
        try {
          var url = editingModel.value
            ? "/api/config/models/" + editingModel.value.id
            : "/api/config/models";
          var method = editingModel.value ? "PUT" : "POST";
          var response = await VueApi.request(url, {
            method: method,
            body: JSON.stringify({
              name: modelForm.name,
              role: modelForm.role,
              cost_per_mtok_input: modelForm.cost_per_mtok_input,
              cost_per_mtok_output: modelForm.cost_per_mtok_output,
              billing_multiplier: modelForm.billing_multiplier,
              weight: modelForm.weight,
              supports_thinking: modelForm.supports_thinking,
              enabled: modelForm.enabled,
            }),
          });
          if (!response.ok) {
            var err = await response.json();
            throw new Error(err.detail || "保存失败");
          }
          showModelModal.value = false;
          toastStore.success(editingModel.value ? "模型已更新" : "模型已创建");
          await loadData();
        } catch (error) {
          toastStore.error(error.message);
        } finally {
          saving.value = false;
        }
      }

      async function deleteModel(model) {
        var confirmed = await confirmStore.delete(model.name, "模型");
        if (!confirmed) return;
        try {
          var response = await VueApi.delete("/api/config/models/" + model.id);
          if (!response.ok) throw new Error("删除失败");
          toastStore.success("模型已删除");
          await loadData();
        } catch (error) {
          toastStore.error(error.message);
        }
      }

      // ========== 服务商操作 ==========

      function showAddProviderModal() {
        editingProvider.value = null;
        showApiKey.value = false;
        detectedModels.value = [];
        detectedApiFormat.value = "";
        detectError.value = "";
        modelSearchQuery.value = "";
        providerForm.name = "";
        providerForm.base_url = "";
        providerForm.api_key = "";
        providerForm.weight = 1;
        providerForm.max_concurrent = 10;
        providerForm.enabled = true;
        providerForm.description = "";
        providerForm.model_ids = [];
        showProviderModal.value = true;
      }

      function showEditProviderModal(provider) {
        editingProvider.value = provider;
        showApiKey.value = false;
        detectedModels.value = [];
        detectedApiFormat.value = "";
        detectError.value = "";
        modelSearchQuery.value = "";
        providerForm.name = provider.name;
        providerForm.base_url = provider.base_url;
        providerForm.api_key = "";
        providerForm.weight = provider.weight;
        providerForm.max_concurrent = provider.max_concurrent;
        providerForm.enabled = provider.enabled;
        providerForm.description = provider.description || "";
        providerForm.model_ids = (provider.models || []).map(function (m) {
          return m.id;
        });
        showProviderModal.value = true;
      }

      function toggleModelId(modelId) {
        var index = providerForm.model_ids.indexOf(modelId);
        if (index === -1) {
          providerForm.model_ids.push(modelId);
        } else {
          providerForm.model_ids.splice(index, 1);
        }
      }

      async function saveProvider() {
        saving.value = true;
        try {
          var data = {
            name: providerForm.name,
            base_url: providerForm.base_url,
            weight: providerForm.weight,
            max_concurrent: providerForm.max_concurrent,
            enabled: providerForm.enabled,
            description: providerForm.description || null,
            model_ids: providerForm.model_ids,
          };
          if (providerForm.api_key) {
            data.api_key = providerForm.api_key;
          } else if (!editingProvider.value) {
            toastStore.error("请填写 API Key");
            saving.value = false;
            return;
          }
          var url = editingProvider.value
            ? "/api/config/providers/" + editingProvider.value.id
            : "/api/config/providers";
          var method = editingProvider.value ? "PUT" : "POST";
          var response = await VueApi.request(url, {
            method: method,
            body: JSON.stringify(data),
          });
          if (!response.ok) {
            var err = await response.json();
            throw new Error(err.detail || "保存失败");
          }
          showProviderModal.value = false;
          toastStore.success(
            editingProvider.value ? "服务商已更新" : "服务商已创建",
          );
          await loadData();
        } catch (error) {
          toastStore.error(error.message);
        } finally {
          saving.value = false;
        }
      }

      async function deleteProvider(provider) {
        var confirmed = await confirmStore.delete(provider.name, "服务商");
        if (!confirmed) return;
        try {
          var response = await VueApi.delete(
            "/api/config/providers/" + provider.id,
          );
          if (!response.ok) throw new Error("删除失败");
          toastStore.success("服务商已删除");
          await loadData();
        } catch (error) {
          toastStore.error(error.message);
        }
      }

      // ========== 模型检测 ==========

      async function detectModels() {
        if (!providerForm.base_url) {
          toastStore.error("请先填写 API 地址");
          return;
        }
        if (!providerForm.api_key && !editingProvider.value) {
          toastStore.error("请先填写 API Key");
          return;
        }
        detecting.value = true;
        detectError.value = "";
        detectedModels.value = [];
        try {
          var payload = {
            base_url: providerForm.base_url,
            provider_type: "provider",
          };
          if (providerForm.api_key) {
            payload.api_key = providerForm.api_key;
          }
          if (editingProvider.value) {
            payload.provider_id = editingProvider.value.id;
          }
          var response = await VueApi.post(
            "/api/config/detect-models",
            payload,
          );
          var data = await response.json();
          if (data.success) {
            detectedModels.value = data.models || [];
            detectedApiFormat.value = data.api_format || "";
            if (detectedModels.value.length === 0) {
              detectError.value = "未检测到任何模型";
            }
          } else {
            detectError.value = data.error || "检测失败";
          }
        } catch (error) {
          detectError.value = "请求失败: " + error.message;
        } finally {
          detecting.value = false;
        }
      }

      function isModelSelected(modelId) {
        return models.value.some(function (m) {
          return (
            m.name === modelId && providerForm.model_ids.indexOf(m.id) !== -1
          );
        });
      }

      async function toggleDetectedModel(detectedModelId) {
        var existingModel = models.value.find(function (m) {
          return m.name === detectedModelId;
        });
        if (existingModel) {
          toggleModelId(existingModel.id);
        } else {
          if (!editingProvider.value) {
            toastStore.info("请先保存服务商，然后再添加检测到的模型");
            return;
          }
          try {
            var response = await VueApi.post("/api/config/models", {
              name: detectedModelId,
              role: "default",
            });
            if (!response.ok) {
              var err = await response.json();
              throw new Error(err.detail || "创建模型失败");
            }
            var data = await response.json();
            toastStore.success('模型 "' + detectedModelId + '" 已创建并关联');
            await loadData();
            providerForm.model_ids.push(data.id);
          } catch (error) {
            toastStore.error(error.message);
          }
        }
      }

      // ========== 下拉菜单 ==========

      function toggleDropdown(id) {
        openDropdown.value = openDropdown.value === id ? null : id;
      }

      function closeDropdowns() {
        openDropdown.value = null;
      }

      // ========== 生命周期 ==========

      onMounted(function () {
        initTab();
        loadData();
        document.addEventListener("click", closeDropdowns);
      });

      onUnmounted(function () {
        document.removeEventListener("click", closeDropdowns);
      });

      return {
        loading: loading,
        saving: saving,
        activeTab: activeTab,
        providers: providers,
        models: models,
        showModelModal: showModelModal,
        showProviderModal: showProviderModal,
        showApiKey: showApiKey,
        editingModel: editingModel,
        editingProvider: editingProvider,
        detecting: detecting,
        detectedModels: detectedModels,
        detectedApiFormat: detectedApiFormat,
        detectError: detectError,
        modelSearchQuery: modelSearchQuery,
        openDropdown: openDropdown,
        roleSelectOpen: roleSelectOpen,
        modelForm: modelForm,
        providerForm: providerForm,
        roleOptions: roleOptions,
        relationCount: relationCount,
        unassociatedModels: unassociatedModels,
        enabledModels: enabledModels,
        groupedDetectedModels: groupedDetectedModels,
        filteredModelCount: filteredModelCount,
        roleLabel: roleLabel,
        shortenModelName: shortenModelName,
        switchTab: switchTab,
        showAddModelModal: showAddModelModal,
        showEditModelModal: showEditModelModal,
        saveModel: saveModel,
        deleteModel: deleteModel,
        showAddProviderModal: showAddProviderModal,
        showEditProviderModal: showEditProviderModal,
        toggleModelId: toggleModelId,
        saveProvider: saveProvider,
        deleteProvider: deleteProvider,
        detectModels: detectModels,
        isModelSelected: isModelSelected,
        toggleDetectedModel: toggleDetectedModel,
        toggleDropdown: toggleDropdown,
      };
    },
    template:
      '\
<div>\
    <div class="tabs">\
        <button class="tab-btn" :class="{\'active\': activeTab === \'overview\'}" @click="switchTab(\'overview\')">概览</button>\
        <button class="tab-btn" :class="{\'active\': activeTab === \'providers\'}" @click="switchTab(\'providers\')">服务商</button>\
        <button class="tab-btn" :class="{\'active\': activeTab === \'models\'}" @click="switchTab(\'models\')">模型</button>\
    </div>\
    <div>\
        <div class="tab-pane" :class="{\'active\': activeTab === \'overview\'}">\
            <div class="overview-stats">\
                <div class="stat-card clickable" @click="switchTab(\'providers\')">\
                    <div class="stat-icon">\
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>\
                    </div>\
                    <div class="stat-info">\
                        <span class="stat-value">{{ providers.length }}</span>\
                        <span class="stat-label">服务商</span>\
                    </div>\
                </div>\
                <div class="stat-card clickable" @click="switchTab(\'models\')">\
                    <div class="stat-icon">\
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/></svg>\
                    </div>\
                    <div class="stat-info">\
                        <span class="stat-value">{{ models.length }}</span>\
                        <span class="stat-label">模型</span>\
                    </div>\
                </div>\
                <div class="stat-card">\
                    <div class="stat-icon">\
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>\
                    </div>\
                    <div class="stat-info">\
                        <span class="stat-value">{{ relationCount }}</span>\
                        <span class="stat-label">关联数</span>\
                    </div>\
                </div>\
            </div>\
            <div class="section">\
                <h3>服务商与模型关联</h3>\
                <p class="section-desc">展示服务商和模型之间的关联关系。每个服务商可以支持多个模型，一个模型也可以被多个服务商提供。</p>\
                <div v-show="loading" class="loading">加载中...</div>\
                <div v-show="!loading && providers.length === 0" class="empty">暂无服务商，请先添加服务商</div>\
                <div v-show="!loading && providers.length > 0" class="relation-grid">\
                    <div v-for="provider in providers" :key="provider.id" class="relation-card">\
                        <div class="relation-card-header">\
                            <div class="provider-info">\
                                <span class="provider-name">{{ provider.name }}</span>\
                                <span :class="\'status-badge status-\' + (provider.enabled ? \'healthy\' : \'unhealthy\')">{{ provider.enabled ? \'启用\' : \'禁用\' }}</span>\
                            </div>\
                            <div class="provider-meta">\
                                <span class="url-cell" :title="provider.base_url">{{ provider.base_url }}</span>\
                            </div>\
                        </div>\
                        <div class="relation-card-body">\
                            <div v-if="provider.models && provider.models.length > 0" class="model-tags">\
                                <span v-for="m in provider.models" :key="m.id" :class="\'model-tag role-\' + m.role + (m.enabled ? \'\' : \' disabled\')" :title="m.name + (m.enabled ? \'\' : \' (已禁用)\')">\
                                    <span class="model-tag-name">{{ shortenModelName(m.name) }}</span>\
                                    <span class="model-tag-role">{{ m.role }}</span>\
                                </span>\
                            </div>\
                            <div v-else class="no-models">未关联模型</div>\
                        </div>\
                        <div class="relation-card-footer">\
                            <span class="text-muted">权重: {{ provider.weight }} | 并发: {{ provider.max_concurrent }}</span>\
                            <button class="btn btn-sm" @click="showEditProviderModal(provider)">编辑</button>\
                        </div>\
                    </div>\
                </div>\
                <div v-if="unassociatedModels.length > 0" class="unassociated-models">\
                    <h4>未关联服务商的模型</h4>\
                    <div class="model-tags">\
                        <span v-for="m in unassociatedModels" :key="m.id" :class="\'model-tag role-\' + m.role + \' unassociated\'">\
                            <span class="model-tag-name">{{ m.name }}</span>\
                            <span class="model-tag-role">{{ m.role }}</span>\
                        </span>\
                    </div>\
                </div>\
            </div>\
        </div>\
        <div class="tab-pane" :class="{\'active\': activeTab === \'providers\'}">\
            <div class="section">\
                <div class="section-header">\
                    <div class="section-header-text">\
                        <h4>服务商</h4>\
                        <p class="section-desc">管理 API 服务商。每个服务商可以支持多个模型。</p>\
                    </div>\
                    <div class="section-header-action">\
                        <button class="btn btn-primary" @click="showAddProviderModal()">+ 添加服务商</button>\
                    </div>\
                </div>\
                <div v-show="loading" class="loading">加载中...</div>\
                <div v-show="!loading && providers.length === 0" class="empty">暂无服务商，点击"添加服务商"开始配置</div>\
                <div class="providers-table-container">\
                    <table v-show="!loading && providers.length > 0" class="table">\
                        <thead>\
                            <tr>\
                                <th>名称</th>\
                                <th>API 地址</th>\
                                <th>支持的模型</th>\
                                <th>权重</th>\
                                <th>最大并发</th>\
                                <th>状态</th>\
                                <th>操作</th>\
                            </tr>\
                        </thead>\
                        <tbody>\
                            <tr v-for="p in providers" :key="p.id">\
                                <td><strong>{{ p.name }}</strong></td>\
                                <td class="url-cell" :title="p.base_url">{{ p.base_url }}</td>\
                                <td>\
                                    <span v-if="p.models && p.models.length > 0">\
                                        <span v-for="m in p.models" :key="m.id" class="tag" :class="{\'disabled\': !m.enabled}" :title="m.name + (m.enabled ? \'\' : \' (已禁用)\')">{{ shortenModelName(m.name) }}</span>\
                                    </span>\
                                    <span v-else class="text-muted">未配置</span>\
                                </td>\
                                <td>{{ p.weight }}</td>\
                                <td>{{ p.max_concurrent }}</td>\
                                <td>\
                                    <span :class="\'status-badge status-\' + (p.enabled ? \'healthy\' : \'unhealthy\')">{{ p.enabled ? \'启用\' : \'禁用\' }}</span>\
                                </td>\
                                <td>\
                                    <div class="dropdown">\
                                        <button class="dropdown-trigger" @click.stop="toggleDropdown(\'p-\' + p.id)">\
                                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="1"/><circle cx="12" cy="5" r="1"/><circle cx="12" cy="19" r="1"/></svg>\
                                        </button>\
                                        <div class="dropdown-menu" v-show="openDropdown === \'p-\' + p.id">\
                                            <button class="dropdown-item" @click="showEditProviderModal(p); openDropdown = null">\
                                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>\
                                                编辑\
                                            </button>\
                                            <div class="dropdown-divider"></div>\
                                            <button class="dropdown-item danger" @click="deleteProvider(p); openDropdown = null">\
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
                <div class="providers-mobile">\
                    <div v-for="p in providers" :key="p.id" class="provider-card">\
                        <div class="provider-card-header">\
                            <div>\
                                <div class="provider-card-title">{{ p.name }}</div>\
                                <div class="provider-card-url" :title="p.base_url">{{ p.base_url }}</div>\
                            </div>\
                            <span :class="\'status-badge status-\' + (p.enabled ? \'healthy\' : \'unhealthy\')">{{ p.enabled ? \'启用\' : \'禁用\' }}</span>\
                        </div>\
                        <div class="provider-card-body">\
                            <div class="provider-stat">\
                                <span class="provider-stat-label">权重</span>\
                                <span class="provider-stat-value">{{ p.weight }}</span>\
                            </div>\
                            <div class="provider-stat">\
                                <span class="provider-stat-label">最大并发</span>\
                                <span class="provider-stat-value">{{ p.max_concurrent }}</span>\
                            </div>\
                            <div class="provider-stat full-width">\
                                <span class="provider-stat-label">支持的模型</span>\
                                <div class="provider-models">\
                                    <div v-if="p.models && p.models.length > 0">\
                                        <span v-for="m in p.models" :key="m.id" class="tag" :class="{\'disabled\': !m.enabled}" :title="m.name + (m.enabled ? \'\' : \' (已禁用)\')">{{ shortenModelName(m.name) }}</span>\
                                    </div>\
                                    <span v-else class="text-muted">未配置</span>\
                                </div>\
                            </div>\
                        </div>\
                        <div class="provider-card-footer">\
                            <button class="btn btn-sm" @click="showEditProviderModal(p)">编辑</button>\
                            <button class="btn btn-sm btn-danger" @click="deleteProvider(p)">删除</button>\
                        </div>\
                    </div>\
                </div>\
            </div>\
        </div>\
        <div class="tab-pane" :class="{\'active\': activeTab === \'models\'}">\
            <div class="section">\
                <div class="section-header">\
                    <div class="section-header-text">\
                        <h4>模型</h4>\
                        <p class="section-desc">管理可用的 AI 模型。模型需要关联到服务商才能使用。</p>\
                    </div>\
                    <div class="section-header-action">\
                        <button class="btn btn-primary" @click="showAddModelModal()">+ 添加模型</button>\
                    </div>\
                </div>\
                <div v-show="loading" class="loading">加载中...</div>\
                <div v-show="!loading && models.length === 0" class="empty">暂无模型，点击"添加模型"开始配置</div>\
                <div v-show="!loading && models.length > 0" class="models-table-container">\
                    <table class="table">\
                        <thead>\
                            <tr>\
                                <th>模型名称</th>\
                                <th>角色</th>\
                                <th>状态</th>\
                                <th>输入成本 ($/1m)</th>\
                                <th>输出成本 ($/1m)</th>\
                                <th>计费倍率</th>\
                                <th>权重</th>\
                                <th>思考</th>\
                                <th>操作</th>\
                            </tr>\
                        </thead>\
                        <tbody>\
                            <tr v-for="m in models" :key="m.id">\
                                <td><strong>{{ m.name }}</strong></td>\
                                <td><span :class="\'model-role role-\' + m.role">{{ m.role }}</span></td>\
                                <td><span :class="\'status-badge status-\' + (m.enabled ? \'healthy\' : \'unhealthy\')">{{ m.enabled ? \'启用\' : \'禁用\' }}</span></td>\
                                <td>${{ m.cost_per_mtok_input }}</td>\
                                <td>${{ m.cost_per_mtok_output }}</td>\
                                <td>{{ (m.billing_multiplier != null ? m.billing_multiplier : 1.0) }}x</td>\
                                <td>{{ m.weight != null ? m.weight : 100 }}</td>\
                                <td><span :style="m.supports_thinking ? \'color: var(--success)\' : \'color: var(--text-secondary)\'">{{ m.supports_thinking ? \'\\u2713\' : \'\\u2014\' }}</span></td>\
                                <td>\
                                    <div class="dropdown">\
                                        <button class="dropdown-trigger" @click.stop="toggleDropdown(\'m-\' + m.id)">\
                                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="1"/><circle cx="12" cy="5" r="1"/><circle cx="12" cy="19" r="1"/></svg>\
                                        </button>\
                                        <div class="dropdown-menu" v-show="openDropdown === \'m-\' + m.id">\
                                            <button class="dropdown-item" @click="showEditModelModal(m); openDropdown = null">\
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
                <div v-show="!loading && models.length > 0" class="models-mobile">\
                    <div v-for="m in models" :key="m.id" class="model-mobile-card">\
                        <div class="model-mobile-header">\
                            <div>\
                                <div class="model-mobile-name">{{ m.name }}</div>\
                                <span :class="\'model-role role-\' + m.role">{{ m.role }}</span>\
                                <span :class="\'status-badge status-\' + (m.enabled ? \'healthy\' : \'unhealthy\')" style="margin-left: 6px">{{ m.enabled ? \'启用\' : \'禁用\' }}</span>\
                            </div>\
                        </div>\
                        <div class="model-mobile-body">\
                            <div class="model-mobile-stat">\
                                <span class="model-mobile-label">输入成本</span>\
                                <span class="model-mobile-value">${{ m.cost_per_mtok_input }}/1m</span>\
                            </div>\
                            <div class="model-mobile-stat">\
                                <span class="model-mobile-label">输出成本</span>\
                                <span class="model-mobile-value">${{ m.cost_per_mtok_output }}/1m</span>\
                            </div>\
                            <div class="model-mobile-stat">\
                                <span class="model-mobile-label">计费倍率</span>\
                                <span class="model-mobile-value">{{ (m.billing_multiplier != null ? m.billing_multiplier : 1.0) }}x</span>\
                            </div>\
                            <div class="model-mobile-stat">\
                                <span class="model-mobile-label">思考</span>\
                                <span class="model-mobile-value" :style="m.supports_thinking ? \'color: var(--success)\' : \'\'">{{ m.supports_thinking ? \'支持\' : \'不支持\' }}</span>\
                            </div>\
                        </div>\
                        <div class="model-mobile-footer">\
                            <button class="btn btn-sm" @click="showEditModelModal(m)">编辑</button>\
                            <button class="btn btn-sm btn-danger" @click="deleteModel(m)">删除</button>\
                        </div>\
                    </div>\
                </div>\
            </div>\
        </div>\
    </div>\
    <div class="modal" v-show="showModelModal" @keydown.escape="showModelModal = false">\
        <div class="modal-content" @click.stop>\
            <div class="modal-header">\
                <h3>{{ editingModel ? \'编辑模型\' : \'添加模型\' }}</h3>\
                <button class="modal-close" @click="showModelModal = false">&times;</button>\
            </div>\
            <div class="modal-body">\
                <form @submit.prevent="saveModel">\
                    <div class="form-group">\
                        <label>模型名称</label>\
                        <input type="text" v-model="modelForm.name" required placeholder="如: claude-sonnet-4-20250514">\
                    </div>\
                    <div class="form-group">\
                        <label>角色</label>\
                        <div class="custom-select">\
                            <button type="button" class="custom-select-trigger" :class="{ \'open\': roleSelectOpen }" @click.stop="roleSelectOpen = !roleSelectOpen">\
                                <span>{{ roleLabel }}</span>\
                                <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"></polyline></svg>\
                            </button>\
                            <div class="custom-select-dropdown" v-show="roleSelectOpen">\
                                <button type="button" v-for="option in roleOptions" :key="option.value" class="custom-select-option" :class="{ \'selected\': modelForm.role === option.value }" @click="modelForm.role = option.value; roleSelectOpen = false">{{ option.label }}</button>\
                            </div>\
                        </div>\
                    </div>\
                    <div class="form-row">\
                        <div class="form-group">\
                            <label>输入成本 ($/1m tokens)</label>\
                            <input type="number" v-model.number="modelForm.cost_per_mtok_input" step="0.0001">\
                        </div>\
                        <div class="form-group">\
                            <label>输出成本 ($/1m tokens)</label>\
                            <input type="number" v-model.number="modelForm.cost_per_mtok_output" step="0.0001">\
                        </div>\
                    </div>\
                    <div class="form-row">\
                        <div class="form-group">\
                            <label>计费倍率</label>\
                            <input type="number" v-model.number="modelForm.billing_multiplier" step="0.1" min="0.1">\
                            <small style="color: var(--text-secondary)">最终成本 = 基础成本 \\u00d7 倍率</small>\
                        </div>\
                        <div class="form-group">\
                            <label class="label-with-help">\
                                权重\
                                <span class="help-icon" data-tooltip="权重用于同角色多模型的流量分配。&#10;&#10;\\u2022 权重越高，被选中的概率越大&#10;\\u2022 权重为 0 表示禁用该模型&#10;\\u2022 默认值 100，建议范围 1-1000&#10;&#10;示例：模型A(100) + 模型B(200)&#10;\\u2192 A 被选中概率 33%，B 为 67%">\
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>\
                                </span>\
                            </label>\
                            <input type="number" v-model.number="modelForm.weight" step="1" min="0" max="1000">\
                        </div>\
                    </div>\
                    <div class="form-row">\
                        <div class="form-group">\
                            <label>支持思考 (Extended Thinking)</label>\
                            <label class="checkbox-label">\
                                <input type="checkbox" v-model="modelForm.supports_thinking">\
                                启用\
                            </label>\
                        </div>\
                        <div class="form-group">\
                            <label>模型状态</label>\
                            <label class="checkbox-label">\
                                <input type="checkbox" v-model="modelForm.enabled">\
                                启用\
                            </label>\
                        </div>\
                    </div>\
                </form>\
            </div>\
            <div class="modal-footer">\
                <button type="button" class="btn" @click="showModelModal = false">取消</button>\
                <button type="button" class="btn btn-primary" :disabled="saving" @click="saveModel">\
                    <span v-show="!saving">保存</span>\
                    <span v-show="saving">保存中...</span>\
                </button>\
            </div>\
        </div>\
    </div>\
    <div class="modal" v-show="showProviderModal" @keydown.escape="showProviderModal = false">\
        <div class="modal-content" @click.stop>\
            <div class="modal-header">\
                <h3>{{ editingProvider ? \'编辑服务商\' : \'添加服务商\' }}</h3>\
                <button class="modal-close" @click="showProviderModal = false">&times;</button>\
            </div>\
            <div class="modal-body">\
                <form @submit.prevent="saveProvider">\
                    <div class="form-group">\
                        <label>名称</label>\
                        <input type="text" v-model="providerForm.name" required placeholder="如: Anthropic 官方">\
                    </div>\
                    <div class="form-group">\
                        <label>API 地址</label>\
                        <input type="url" v-model="providerForm.base_url" required placeholder="https://api.anthropic.com">\
                    </div>\
                    <div class="form-group">\
                        <label>API Key</label>\
                        <div class="password-input-wrapper">\
                            <input :type="showApiKey ? \'text\' : \'password\'" v-model="providerForm.api_key" :required="!editingProvider" :placeholder="editingProvider ? \'留空保持不变\' : \'sk-...\'">\
                            <button type="button" class="password-toggle" @click="showApiKey = !showApiKey">\
                                <span class="eye-icon">\
                                    <svg v-show="!showApiKey" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>\
                                    <svg v-show="showApiKey" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/></svg>\
                                </span>\
                            </button>\
                        </div>\
                    </div>\
                    <div class="form-group">\
                        <button type="button" class="btn" @click="detectModels()" :disabled="detecting" :style="(!providerForm.base_url || (!providerForm.api_key && !editingProvider)) ? \'opacity:0.5;pointer-events:none\' : \'\'">\
                            <span v-show="detecting" class="detect-btn-spinner"></span>\
                            <span v-show="!detecting">\\ud83d\\udd0d 检测可用模型</span>\
                            <span v-show="detecting">检测中...</span>\
                        </button>\
                        <p class="help-text">根据 Base URL 和 API Key 自动检测服务商支持的模型</p>\
                        <div class="detected-models-panel" v-show="detecting">\
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
                                        <div class="skeleton-tag"></div>\
                                        <div class="skeleton-tag"></div>\
                                    </div>\
                                </div>\
                            </div>\
                        </div>\
                        <div class="detected-models-panel" v-show="!detecting && detectedModels.length > 0">\
                            <div class="detected-models-header">\
                                <div class="detected-models-header-left">\
                                    <span>检测到 {{ detectedModels.length }} 个模型</span>\
                                    <span v-show="detectedApiFormat" class="detected-models-format">{{ detectedApiFormat }}</span>\
                                </div>\
                            </div>\
                            <div class="detected-models-search" v-show="detectedModels.length > 5">\
                                <svg class="detected-models-search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/></svg>\
                                <input type="text" v-model="modelSearchQuery" placeholder="搜索模型...">\
                            </div>\
                            <div class="detected-models-body">\
                                <div class="detected-models-empty" v-show="filteredModelCount === 0">\
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/></svg>\
                                    <div>未找到匹配的模型</div>\
                                </div>\
                                <div v-for="group in groupedDetectedModels" :key="group.key" class="detected-group">\
                                    <div class="detected-group-header">\
                                        <span class="detected-group-indicator" :class="\'tier-\' + group.tier"></span>\
                                        <span>{{ group.label }}</span>\
                                        <span class="detected-group-count">{{ group.models.length }}</span>\
                                    </div>\
                                    <div class="detected-group-models">\
                                        <span v-for="dm in group.models" :key="dm.id" class="detected-model-tag" :class="[\'tier-\' + group.tier, isModelSelected(dm.id) ? \'is-selected\' : \'\']" :title="dm.display_name || dm.id" @click="toggleDetectedModel(dm.id)">\
                                            <svg v-show="isModelSelected(dm.id)" class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>\
                                            <span>{{ dm.id }}</span>\
                                        </span>\
                                    </div>\
                                </div>\
                            </div>\
                        </div>\
                        <div class="detected-models-error" v-show="detectError">{{ detectError }}</div>\
                    </div>\
                    <div class="form-row">\
                        <div class="form-group">\
                            <label>权重</label>\
                            <input type="number" v-model.number="providerForm.weight" min="1">\
                        </div>\
                        <div class="form-group">\
                            <label>最大并发</label>\
                            <input type="number" v-model.number="providerForm.max_concurrent" min="1">\
                        </div>\
                    </div>\
                    <div class="form-group">\
                        <label>描述 <span class="text-muted">(可选)</span></label>\
                        <input type="text" v-model="providerForm.description" placeholder="服务商描述信息">\
                    </div>\
                    <div class="form-group">\
                        <label class="checkbox-label">\
                            <input type="checkbox" v-model="providerForm.enabled">\
                            启用此服务商\
                        </label>\
                    </div>\
                    <div class="form-group">\
                        <label>支持的模型</label>\
                        <div class="checkbox-group">\
                            <template v-if="enabledModels.length > 0">\
                                <label v-for="m in enabledModels" :key="m.id" class="checkbox-label">\
                                    <input type="checkbox" :value="m.id" :checked="providerForm.model_ids.indexOf(m.id) !== -1" @change="toggleModelId(m.id)">\
                                    <span>{{ m.name }}</span>\
                                    <span class="text-muted">({{ m.role }})</span>\
                                </label>\
                            </template>\
                            <span v-else class="text-muted">请先添加模型</span>\
                        </div>\
                    </div>\
                </form>\
            </div>\
            <div class="modal-footer">\
                <button type="button" class="btn" @click="showProviderModal = false">取消</button>\
                <button type="button" class="btn btn-primary" :disabled="saving" @click="saveProvider">\
                    <span v-show="!saving">保存</span>\
                    <span v-show="saving">保存中...</span>\
                </button>\
            </div>\
        </div>\
    </div>\
</div>',
  };
})();
