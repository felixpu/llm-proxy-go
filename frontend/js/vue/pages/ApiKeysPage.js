/**
 * API Keys 管理页面
 * 支持创建、复制、禁用、删除 API Key
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

  var createFormValidator =
    window.VueComposables.useValidation.createFormValidator;

  window.VuePages.ApiKeysPage = {
    name: "ApiKeysPage",
    setup: function () {
      var toastStore = inject("toastStore");
      var confirmStore = inject("confirmStore");
      var headerActionsStore = inject("headerActionsStore");

      // 状态
      var apiKeys = ref([]);
      var loading = ref(true);
      var saving = ref(false);
      var showCreate = ref(false);
      var showKeyDisplay = ref(false);
      var createdKey = ref("");
      var openDropdown = ref(null);
      var expirySelectOpen = ref(false);

      // 表单
      var createForm = reactive({
        name: "",
        expiryType: "",
        startsAt: "",
        expiresAt: "",
      });

      // 验证器
      var createValidator = createFormValidator({
        name: [
          "required",
          { type: "minLength", value: 2, message: "名称至少需要 2 个字符" },
        ],
      });

      // 有效期选项
      var expiryOptions = [
        { value: "", label: "永不过期" },
        { value: "30", label: "30 天" },
        { value: "90", label: "90 天" },
        { value: "180", label: "180 天" },
        { value: "365", label: "1 年" },
        { value: "custom", label: "自定义时间范围" },
      ];

      // 计算属性：最小日期时间
      var minDateTime = computed(function () {
        var now = new Date();
        var year = now.getFullYear();
        var month = String(now.getMonth() + 1).padStart(2, "0");
        var day = String(now.getDate()).padStart(2, "0");
        var hours = String(now.getHours()).padStart(2, "0");
        var minutes = String(now.getMinutes()).padStart(2, "0");
        return year + "-" + month + "-" + day + "T" + hours + ":" + minutes;
      });

      var expiryLabel = computed(function () {
        var found = expiryOptions.find(function (o) {
          return o.value === createForm.expiryType;
        });
        return found ? found.label : "请选择";
      });

      // 格式化日期
      function formatDateTime(isoString) {
        if (!isoString) return "-";
        return VueUtils.formatDateTime(isoString) || "-";
      }

      // 加载 API Keys
      async function loadApiKeys() {
        loading.value = true;
        try {
          var response = await VueApi.get("/api/keys");
          apiKeys.value = await response.json();
        } catch (error) {
          toastStore.error("加载 API Keys 失败");
        } finally {
          loading.value = false;
        }
      }

      // 显示创建模态框
      function showCreateModal() {
        createForm.name = "";
        createForm.expiryType = "";
        createForm.startsAt = "";
        createForm.expiresAt = "";
        createValidator.reset();
        expirySelectOpen.value = false;
        showCreate.value = true;
      }

      // 验证自定义时间范围
      function validateCustomTimeRange() {
        if (createForm.expiryType !== "custom") return true;
        var now = new Date();
        if (!createForm.expiresAt) {
          toastStore.error("请选择结束时间");
          return false;
        }
        var expiresAt = new Date(createForm.expiresAt);
        if (expiresAt <= now) {
          toastStore.error("结束时间不能早于当前时间");
          return false;
        }
        if (createForm.startsAt) {
          var startsAt = new Date(createForm.startsAt);
          if (startsAt < now) {
            toastStore.error("起始时间不能早于当前时间");
            return false;
          }
          if (startsAt >= expiresAt) {
            toastStore.error("起始时间必须早于结束时间");
            return false;
          }
        }
        return true;
      }

      // 创建 API Key
      async function createKey() {
        if (!createValidator.validateAll(createForm)) return;
        if (!validateCustomTimeRange()) return;
        saving.value = true;
        try {
          var requestBody = { name: createForm.name };
          if (createForm.expiryType === "custom") {
            if (createForm.startsAt) {
              requestBody.starts_at = new Date(
                createForm.startsAt,
              ).toISOString();
            }
            if (createForm.expiresAt) {
              requestBody.expires_at = new Date(
                createForm.expiresAt,
              ).toISOString();
            }
          } else if (createForm.expiryType) {
            requestBody.expires_days = parseInt(createForm.expiryType);
          }
          var response = await VueApi.post("/api/keys", requestBody);
          if (response.ok) {
            var data = await response.json();
            showCreate.value = false;
            createdKey.value = data.key;
            showKeyDisplay.value = true;
            await loadApiKeys();
          } else {
            var error = await response.json();
            toastStore.error(error.detail || "创建失败");
          }
        } catch (error) {
          toastStore.error("创建失败: " + error.message);
        } finally {
          saving.value = false;
        }
      }

      // 复制 Key
      function copyKey(key) {
        if (!key || key.trim() === "") {
          toastStore.error("此 Key 无法复制，请重新创建");
          return;
        }
        VueUtils.copyToClipboard(key)
          .then(function () {
            toastStore.success("已复制到剪贴板");
          })
          .catch(function () {
            toastStore.error("复制失败");
          });
      }

      // 禁用 API Key
      async function revokeKey(key) {
        var confirmed = await confirmStore.show({
          title: "禁用 API Key",
          message: "确定要禁用此 API Key 吗？",
          detail: "禁用后使用此 Key 的请求将被拒绝。",
          confirmText: "确认禁用",
          type: "warning",
        });
        if (!confirmed) return;
        try {
          var response = await VueApi.post("/api/keys/" + key.id + "/revoke");
          if (response.ok) {
            toastStore.success("API Key 已禁用");
            await loadApiKeys();
          } else {
            var error = await response.json();
            toastStore.error(error.detail || "禁用失败");
          }
        } catch (error) {
          toastStore.error("禁用失败: " + error.message);
        }
      }

      // 删除 API Key
      async function deleteKey(key) {
        var confirmed = await confirmStore.delete(key.name, "API Key");
        if (!confirmed) return;
        try {
          var response = await VueApi.delete("/api/keys/" + key.id);
          if (response.ok) {
            toastStore.success("API Key 已删除");
            await loadApiKeys();
          } else {
            var error = await response.json();
            toastStore.error(error.detail || "删除失败");
          }
        } catch (error) {
          toastStore.error("删除失败: " + error.message);
        }
      }

      // 切换下拉菜单
      function toggleDropdown(id) {
        openDropdown.value = openDropdown.value === id ? null : id;
      }

      function closeDropdowns() {
        openDropdown.value = null;
      }

      // header 操作按钮组件
      var ApiKeysHeaderActions = {
        name: "ApiKeysHeaderActions",
        setup: function () {
          return { showCreateModal: showCreateModal };
        },
        template:
          '<button class="btn btn-primary" @click="showCreateModal()">\
              <svg style="width:14px;height:14px;margin-right:4px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>\
              <span>新建 API Key</span>\
          </button>',
      };

      // 初始化
      onMounted(function () {
        headerActionsStore.value = ApiKeysHeaderActions;
        loadApiKeys();
        document.addEventListener("click", closeDropdowns);
      });

      onUnmounted(function () {
        headerActionsStore.value = null;
        document.removeEventListener("click", closeDropdowns);
      });

      return {
        apiKeys: apiKeys,
        loading: loading,
        saving: saving,
        showCreate: showCreate,
        showKeyDisplay: showKeyDisplay,
        createdKey: createdKey,
        openDropdown: openDropdown,
        expirySelectOpen: expirySelectOpen,
        createForm: createForm,
        createValidator: createValidator,
        expiryOptions: expiryOptions,
        minDateTime: minDateTime,
        expiryLabel: expiryLabel,
        formatDateTime: formatDateTime,
        loadApiKeys: loadApiKeys,
        showCreateModal: showCreateModal,
        createKey: createKey,
        copyKey: copyKey,
        revokeKey: revokeKey,
        deleteKey: deleteKey,
        toggleDropdown: toggleDropdown,
      };
    },
    template:
      '\
<div class="section">\
    <h3>我的 API Keys</h3>\
    <div v-show="loading" class="loading">加载中...</div>\
    <div v-show="!loading && apiKeys.length === 0" v-cloak class="empty">\
        暂无 API Key，点击上方按钮创建\
    </div>\
    <div class="keys-table-container">\
        <table v-show="!loading && apiKeys.length > 0" v-cloak class="table">\
            <thead>\
                <tr>\
                    <th>名称</th>\
                    <th>Key 前缀</th>\
                    <th>状态</th>\
                    <th>创建时间</th>\
                    <th>最后使用</th>\
                    <th>过期时间</th>\
                    <th>操作</th>\
                </tr>\
            </thead>\
            <tbody>\
                <tr v-for="key in apiKeys" :key="key.id">\
                    <td>{{ key.name }}</td>\
                    <td><code>{{ key.key_prefix }}...</code></td>\
                    <td>\
                        <span :class="\'status-badge \' + (key.is_active ? \'status-healthy\' : \'status-unhealthy\')">\
                            {{ key.is_active ? "有效" : "已禁用" }}\
                        </span>\
                    </td>\
                    <td>{{ formatDateTime(key.created_at) }}</td>\
                    <td>{{ formatDateTime(key.last_used_at) }}</td>\
                    <td>{{ key.expires_at ? formatDateTime(key.expires_at) : "永不过期" }}</td>\
                    <td>\
                        <div class="dropdown">\
                            <button class="dropdown-trigger" @click.stop="toggleDropdown(key.id)">\
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                    <circle cx="12" cy="12" r="1"/><circle cx="12" cy="5" r="1"/><circle cx="12" cy="19" r="1"/>\
                                </svg>\
                            </button>\
                            <div class="dropdown-menu" v-show="openDropdown === key.id" v-cloak>\
                                <button class="dropdown-item" @click="copyKey(key.key_full); openDropdown = null">\
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>\
                                    复制 Key\
                                </button>\
                                <button v-show="key.is_active" class="dropdown-item" @click="revokeKey(key); openDropdown = null">\
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg>\
                                    禁用\
                                </button>\
                                <div class="dropdown-divider"></div>\
                                <button class="dropdown-item danger" @click="deleteKey(key); openDropdown = null">\
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
    <div class="keys-mobile">\
        <div v-for="key in apiKeys" :key="key.id" class="key-card">\
            <div class="key-card-header">\
                <div>\
                    <div class="key-card-title">{{ key.name }}</div>\
                    <div class="key-card-prefix">\
                        <code>{{ key.key_prefix }}...</code>\
                    </div>\
                </div>\
                <span :class="\'status-badge \' + (key.is_active ? \'status-healthy\' : \'status-unhealthy\')">\
                    {{ key.is_active ? "有效" : "已禁用" }}\
                </span>\
            </div>\
            <div class="key-card-body">\
                <div class="key-stat">\
                    <span class="key-stat-label">创建时间</span>\
                    <span class="key-stat-value">{{ formatDateTime(key.created_at) }}</span>\
                </div>\
                <div class="key-stat">\
                    <span class="key-stat-label">最后使用</span>\
                    <span class="key-stat-value">{{ formatDateTime(key.last_used_at) }}</span>\
                </div>\
                <div class="key-stat full-width">\
                    <span class="key-stat-label">过期时间</span>\
                    <span class="key-stat-value">{{ key.expires_at ? formatDateTime(key.expires_at) : "永不过期" }}</span>\
                </div>\
            </div>\
            <div class="key-card-actions">\
                <button class="btn btn-sm" @click="copyKey(key.key_full)">\
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>\
                    复制\
                </button>\
                <button v-show="key.is_active" class="btn btn-sm" @click="revokeKey(key)">\
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg>\
                    禁用\
                </button>\
                <button class="btn btn-sm btn-danger" @click="deleteKey(key)">\
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>\
                    删除\
                </button>\
            </div>\
        </div>\
    </div>\
    <div class="modal" v-show="showCreate" v-cloak @keydown.escape="showCreate = false">\
        <div class="modal-content" @click.stop>\
            <div class="modal-header">\
                <h3>创建 API Key</h3>\
                <button class="modal-close" @click="showCreate = false">&times;</button>\
            </div>\
            <div class="modal-body">\
                <form @submit.prevent="createKey">\
                    <div class="form-group" :class="{\'has-error\': createValidator.hasError(\'name\')}">\
                        <label>名称 <span class="required-mark">*</span></label>\
                        <input type="text" v-model="createForm.name"\
                               @blur="createValidator.touch(\'name\'); createValidator.validateField(\'name\', createForm.name)"\
                               @input="createValidator.touched.name && createValidator.validateField(\'name\', createForm.name)"\
                               placeholder="例如：开发环境">\
                        <div class="form-error" v-show="createValidator.hasError(\'name\')" v-cloak>\
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>\
                            <span>{{ createValidator.getError(\'name\') }}</span>\
                        </div>\
                    </div>\
                    <div class="form-group">\
                        <label>有效期</label>\
                        <div class="custom-select">\
                            <button type="button" class="custom-select-trigger" :class="{ \'open\': expirySelectOpen }" @click.stop="expirySelectOpen = !expirySelectOpen">\
                                <span>{{ expiryLabel }}</span>\
                                <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                    <polyline points="6 9 12 15 18 9"></polyline>\
                                </svg>\
                            </button>\
                            <div class="custom-select-dropdown" v-show="expirySelectOpen" v-cloak>\
                                <button type="button" v-for="option in expiryOptions" :key="option.value" class="custom-select-option" :class="{ \'selected\': createForm.expiryType === option.value }" @click="createForm.expiryType = option.value; expirySelectOpen = false">{{ option.label }}</button>\
                            </div>\
                        </div>\
                    </div>\
                    <div v-show="createForm.expiryType === \'custom\'" v-cloak>\
                        <div class="form-group">\
                            <label>起始时间（可选，默认立即生效）</label>\
                            <input type="datetime-local" v-model="createForm.startsAt" :min="minDateTime">\
                        </div>\
                        <div class="form-group">\
                            <label>结束时间</label>\
                            <input type="datetime-local" v-model="createForm.expiresAt" :min="minDateTime"\
                                   :required="createForm.expiryType === \'custom\'">\
                        </div>\
                    </div>\
                </form>\
            </div>\
            <div class="modal-footer">\
                <button type="button" class="btn btn-secondary" @click="showCreate = false">取消</button>\
                <button type="button" class="btn btn-primary" :disabled="saving || !createValidator.isValid()" @click="createKey">\
                    <span v-show="!saving">创建</span>\
                    <span v-show="saving">创建中...</span>\
                </button>\
            </div>\
        </div>\
    </div>\
    <div class="modal" v-show="showKeyDisplay" v-cloak @keydown.escape="showKeyDisplay = false">\
        <div class="modal-content" @click.stop>\
            <div class="modal-header">\
                <h3>API Key 创建成功</h3>\
                <button class="modal-close" @click="showKeyDisplay = false">&times;</button>\
            </div>\
            <div class="modal-body">\
                <p>请复制并安全保存以下 API Key：</p>\
                <div class="api-key-display">{{ createdKey }}</div>\
            </div>\
            <div class="modal-footer">\
                <button class="btn btn-primary" @click="copyKey(createdKey)">复制 Key</button>\
                <button class="btn btn-secondary" @click="showKeyDisplay = false">关闭</button>\
            </div>\
        </div>\
    </div>\
</div>',
  };
})();
