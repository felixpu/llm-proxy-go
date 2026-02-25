/**
 * 用户管理页面
 * 支持创建、编辑、修改密码、删除用户
 */
window.VuePages = window.VuePages || {};

(function () {
  "use strict";

  var ref = Vue.ref;
  var reactive = Vue.reactive;
  var onMounted = Vue.onMounted;
  var onUnmounted = Vue.onUnmounted;
  var inject = Vue.inject;

  var createFormValidator =
    window.VueComposables.useValidation.createFormValidator;

  window.VuePages.UsersPage = {
    name: "UsersPage",
    setup: function () {
      var toastStore = inject("toastStore");
      var confirmStore = inject("confirmStore");
      var headerActionsStore = inject("headerActionsStore");

      // 状态
      var users = ref([]);
      var loading = ref(true);
      var saving = ref(false);
      var showCreate = ref(false);
      var showEdit = ref(false);
      var showPassword = ref(false);
      var editingUser = ref(null);
      var openDropdown = ref(null);
      var createRoleOpen = ref(false);
      var editRoleOpen = ref(false);

      // 表单
      var createForm = reactive({
        username: "",
        password: "",
        role: "user",
      });
      var editForm = reactive({
        role: "user",
        is_active: true,
      });
      var passwordForm = reactive({
        password: "",
      });

      // 角色选项
      var roleOptions = [
        { value: "user", label: "普通用户" },
        { value: "admin", label: "管理员" },
      ];

      // 验证器
      var createValidator = createFormValidator({
        username: [
          "required",
          { type: "minLength", value: 3, message: "用户名至少需要 3 个字符" },
          { type: "username", message: "用户名只能包含字母、数字和下划线" },
        ],
        password: [
          "required",
          { type: "minLength", value: 6, message: "密码至少需要 6 个字符" },
        ],
      });

      var passwordValidator = createFormValidator({
        password: [
          "required",
          { type: "minLength", value: 6, message: "密码至少需要 6 个字符" },
        ],
      });

      // 格式化日期
      function formatDateTime(isoString) {
        if (!isoString) return "-";
        return VueUtils.formatDateTime(isoString) || "-";
      }

      // 获取角色标签
      function getRoleLabel(value) {
        var found = roleOptions.find(function (o) {
          return o.value === value;
        });
        return found ? found.label : "请选择";
      }

      // 加载用户列表
      async function loadUsers() {
        loading.value = true;
        try {
          var response = await VueApi.get("/api/users");
          users.value = await response.json();
        } catch (error) {
          toastStore.error("加载用户列表失败");
        } finally {
          loading.value = false;
        }
      }

      // 显示创建模态框
      function showCreateModal() {
        createForm.username = "";
        createForm.password = "";
        createForm.role = "user";
        createValidator.reset();
        createRoleOpen.value = false;
        showCreate.value = true;
      }

      // 显示编辑模态框
      function showEditModal(user) {
        editingUser.value = user;
        editForm.role = user.role;
        editForm.is_active = user.is_active;
        editRoleOpen.value = false;
        showEdit.value = true;
      }

      // 显示修改密码模态框
      function showPasswordModal(user) {
        editingUser.value = user;
        passwordForm.password = "";
        passwordValidator.reset();
        showPassword.value = true;
      }

      // 创建用户
      async function createUser() {
        if (!createValidator.validateAll(createForm)) return;
        saving.value = true;
        try {
          var response = await VueApi.post("/api/users", {
            username: createForm.username,
            password: createForm.password,
            role: createForm.role,
          });
          if (response.ok) {
            showCreate.value = false;
            toastStore.success("用户创建成功");
            await loadUsers();
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

      // 更新用户
      async function updateUser() {
        saving.value = true;
        try {
          var response = await VueApi.patch(
            "/api/users/" + editingUser.value.id,
            {
              role: editForm.role,
              is_active: editForm.is_active,
            },
          );
          if (response.ok) {
            showEdit.value = false;
            toastStore.success("用户更新成功");
            await loadUsers();
          } else {
            var error = await response.json();
            toastStore.error(error.detail || "更新失败");
          }
        } catch (error) {
          toastStore.error("更新失败: " + error.message);
        } finally {
          saving.value = false;
        }
      }

      // 修改密码
      async function changePassword() {
        if (!passwordValidator.validateAll(passwordForm)) return;
        saving.value = true;
        try {
          var response = await VueApi.post(
            "/api/users/" + editingUser.value.id + "/password",
            {
              password: passwordForm.password,
            },
          );
          if (response.ok) {
            showPassword.value = false;
            toastStore.success("密码修改成功");
          } else {
            var error = await response.json();
            toastStore.error(error.detail || "修改失败");
          }
        } catch (error) {
          toastStore.error("修改失败: " + error.message);
        } finally {
          saving.value = false;
        }
      }

      // 删除用户
      async function deleteUser(user) {
        var confirmed = await confirmStore.show({
          title: "删除用户",
          message: '确定要删除用户 "' + user.username + '" 吗？',
          detail: "此操作不可恢复，该用户的所有 API Key 和会话也将被删除。",
          confirmText: "确认删除",
          type: "danger",
        });
        if (!confirmed) return;
        try {
          var response = await VueApi.delete("/api/users/" + user.id);
          if (response.ok) {
            toastStore.success("用户已删除");
            await loadUsers();
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
      var UsersHeaderActions = {
        name: "UsersHeaderActions",
        setup: function () {
          return { showCreateModal: showCreateModal };
        },
        template:
          '<button class="btn btn-primary" @click="showCreateModal()">\
              <svg style="width:14px;height:14px;margin-right:4px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>\
              <span>新建用户</span>\
          </button>',
      };

      // 初始化
      onMounted(function () {
        headerActionsStore.value = UsersHeaderActions;
        loadUsers();
        document.addEventListener("click", closeDropdowns);
      });

      onUnmounted(function () {
        headerActionsStore.value = null;
        document.removeEventListener("click", closeDropdowns);
      });

      return {
        users: users,
        loading: loading,
        saving: saving,
        showCreate: showCreate,
        showEdit: showEdit,
        showPassword: showPassword,
        editingUser: editingUser,
        openDropdown: openDropdown,
        createRoleOpen: createRoleOpen,
        editRoleOpen: editRoleOpen,
        createForm: createForm,
        editForm: editForm,
        passwordForm: passwordForm,
        createValidator: createValidator,
        passwordValidator: passwordValidator,
        roleOptions: roleOptions,
        formatDateTime: formatDateTime,
        getRoleLabel: getRoleLabel,
        showCreateModal: showCreateModal,
        showEditModal: showEditModal,
        showPasswordModal: showPasswordModal,
        createUser: createUser,
        updateUser: updateUser,
        changePassword: changePassword,
        deleteUser: deleteUser,
        toggleDropdown: toggleDropdown,
      };
    },
    template:
      '\
<div class="section">\
    <h3>用户列表</h3>\
    <div v-show="loading" class="loading">加载中...</div>\
    <div v-show="!loading && users.length === 0" v-cloak class="empty">暂无用户</div>\
    <div class="users-table-container">\
        <table v-show="!loading && users.length > 0" v-cloak class="table" id="users-table">\
            <thead>\
                <tr>\
                    <th>ID</th>\
                    <th>用户名</th>\
                    <th>角色</th>\
                    <th>状态</th>\
                    <th>创建时间</th>\
                    <th>操作</th>\
                </tr>\
            </thead>\
            <tbody>\
                <tr v-for="user in users" :key="user.id">\
                    <td>{{ user.id }}</td>\
                    <td>{{ user.username }}</td>\
                    <td>\
                        <span :class="\'role-badge role-\' + user.role">{{ user.role }}</span>\
                    </td>\
                    <td>\
                        <span :class="\'status-badge \' + (user.is_active ? \'status-healthy\' : \'status-unhealthy\')">\
                            {{ user.is_active ? "正常" : "已禁用" }}\
                        </span>\
                    </td>\
                    <td>{{ formatDateTime(user.created_at) }}</td>\
                    <td>\
                        <div class="dropdown">\
                            <button class="dropdown-trigger" @click.stop="toggleDropdown(user.id)">\
                                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                    <circle cx="12" cy="12" r="1"/><circle cx="12" cy="5" r="1"/><circle cx="12" cy="19" r="1"/>\
                                </svg>\
                            </button>\
                            <div class="dropdown-menu" v-show="openDropdown === user.id" v-cloak>\
                                <button class="dropdown-item" @click="showEditModal(user); openDropdown = null">\
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>\
                                    编辑\
                                </button>\
                                <button class="dropdown-item" @click="showPasswordModal(user); openDropdown = null">\
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>\
                                    修改密码\
                                </button>\
                                <div class="dropdown-divider"></div>\
                                <button class="dropdown-item danger" @click="deleteUser(user); openDropdown = null">\
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
    <div class="users-mobile" v-show="!loading && users.length > 0" v-cloak>\
        <div v-for="user in users" :key="user.id" class="user-card">\
            <div class="user-card-header">\
                <div>\
                    <div class="user-card-title">{{ user.username }}</div>\
                    <div class="user-card-badges">\
                        <span :class="\'role-badge role-\' + user.role">{{ user.role }}</span>\
                        <span :class="\'status-badge \' + (user.is_active ? \'status-healthy\' : \'status-unhealthy\')">\
                            {{ user.is_active ? "正常" : "已禁用" }}\
                        </span>\
                    </div>\
                </div>\
            </div>\
            <div class="user-card-body">\
                <div class="user-stat">\
                    <span class="user-stat-label">创建时间</span>\
                    <span class="user-stat-value">{{ formatDateTime(user.created_at) }}</span>\
                </div>\
            </div>\
            <div class="user-card-footer">\
                <button class="btn btn-sm" @click="showEditModal(user)">编辑</button>\
                <button class="btn btn-sm" @click="showPasswordModal(user)">修改密码</button>\
                <button class="btn btn-sm btn-danger" @click="deleteUser(user)">删除</button>\
            </div>\
        </div>\
    </div>\
    <div class="modal" v-show="showCreate" v-cloak @keydown.escape="showCreate = false">\
        <div class="modal-content" @click.stop>\
            <div class="modal-header">\
                <h3>创建用户</h3>\
                <button class="modal-close" @click="showCreate = false">&times;</button>\
            </div>\
            <div class="modal-body">\
                <form @submit.prevent="createUser">\
                    <div class="form-group" :class="{\'has-error\': createValidator.hasError(\'username\')}">\
                        <label>用户名 <span class="required-mark">*</span></label>\
                        <input type="text" v-model="createForm.username"\
                               @blur="createValidator.touch(\'username\'); createValidator.validateField(\'username\', createForm.username)"\
                               @input="createValidator.touched.username && createValidator.validateField(\'username\', createForm.username)"\
                               placeholder="请输入用户名">\
                        <div class="form-error" v-show="createValidator.hasError(\'username\')" v-cloak>\
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>\
                            <span>{{ createValidator.getError(\'username\') }}</span>\
                        </div>\
                    </div>\
                    <div class="form-group" :class="{\'has-error\': createValidator.hasError(\'password\')}">\
                        <label>密码 <span class="required-mark">*</span></label>\
                        <input type="password" v-model="createForm.password"\
                               @blur="createValidator.touch(\'password\'); createValidator.validateField(\'password\', createForm.password)"\
                               @input="createValidator.touched.password && createValidator.validateField(\'password\', createForm.password)"\
                               placeholder="请输入密码">\
                        <div class="form-error" v-show="createValidator.hasError(\'password\')" v-cloak>\
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>\
                            <span>{{ createValidator.getError(\'password\') }}</span>\
                        </div>\
                        <div class="form-hint">密码至少需要 6 个字符</div>\
                    </div>\
                    <div class="form-group">\
                        <label>角色</label>\
                        <div class="custom-select">\
                            <button type="button" class="custom-select-trigger" :class="{ \'open\': createRoleOpen }" @click.stop="createRoleOpen = !createRoleOpen">\
                                <span>{{ getRoleLabel(createForm.role) }}</span>\
                                <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                    <polyline points="6 9 12 15 18 9"></polyline>\
                                </svg>\
                            </button>\
                            <div class="custom-select-dropdown" v-show="createRoleOpen" v-cloak>\
                                <button type="button" v-for="option in roleOptions" :key="option.value" class="custom-select-option" :class="{ \'selected\': createForm.role === option.value }" @click="createForm.role = option.value; createRoleOpen = false">{{ option.label }}</button>\
                            </div>\
                        </div>\
                    </div>\
                </form>\
            </div>\
            <div class="modal-footer">\
                <button type="button" class="btn btn-secondary" @click="showCreate = false">取消</button>\
                <button type="button" class="btn btn-primary" :disabled="saving || !createValidator.isValid()" @click="createUser">\
                    <span v-show="!saving">创建</span>\
                    <span v-show="saving">创建中...</span>\
                </button>\
            </div>\
        </div>\
    </div>\
    <div class="modal" v-show="showEdit" v-cloak @keydown.escape="showEdit = false">\
        <div class="modal-content" @click.stop>\
            <div class="modal-header">\
                <h3>编辑用户</h3>\
                <button class="modal-close" @click="showEdit = false">&times;</button>\
            </div>\
            <div class="modal-body">\
                <form @submit.prevent="updateUser">\
                    <div class="form-group">\
                        <label>用户名</label>\
                        <input type="text" :value="editingUser?.username" disabled>\
                    </div>\
                    <div class="form-group">\
                        <label>角色</label>\
                        <div class="custom-select">\
                            <button type="button" class="custom-select-trigger" :class="{ \'open\': editRoleOpen }" @click.stop="editRoleOpen = !editRoleOpen">\
                                <span>{{ getRoleLabel(editForm.role) }}</span>\
                                <svg class="custom-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                                    <polyline points="6 9 12 15 18 9"></polyline>\
                                </svg>\
                            </button>\
                            <div class="custom-select-dropdown" v-show="editRoleOpen" v-cloak>\
                                <button type="button" v-for="option in roleOptions" :key="option.value" class="custom-select-option" :class="{ \'selected\': editForm.role === option.value }" @click="editForm.role = option.value; editRoleOpen = false">{{ option.label }}</button>\
                            </div>\
                        </div>\
                    </div>\
                    <div class="form-group">\
                        <label class="checkbox-label">\
                            <input type="checkbox" v-model="editForm.is_active">\
                            账号启用\
                        </label>\
                    </div>\
                </form>\
            </div>\
            <div class="modal-footer">\
                <button type="button" class="btn btn-secondary" @click="showEdit = false">取消</button>\
                <button type="button" class="btn btn-primary" :disabled="saving" @click="updateUser">\
                    <span v-show="!saving">保存</span>\
                    <span v-show="saving">保存中...</span>\
                </button>\
            </div>\
        </div>\
    </div>\
    <div class="modal" v-show="showPassword" v-cloak @keydown.escape="showPassword = false">\
        <div class="modal-content" @click.stop>\
            <div class="modal-header">\
                <h3>修改密码</h3>\
                <button class="modal-close" @click="showPassword = false">&times;</button>\
            </div>\
            <div class="modal-body">\
                <form @submit.prevent="changePassword">\
                    <div class="form-group">\
                        <label>用户名</label>\
                        <input type="text" :value="editingUser?.username" disabled>\
                    </div>\
                    <div class="form-group" :class="{\'has-error\': passwordValidator.hasError(\'password\')}">\
                        <label>新密码 <span class="required-mark">*</span></label>\
                        <input type="password" v-model="passwordForm.password"\
                               @blur="passwordValidator.touch(\'password\'); passwordValidator.validateField(\'password\', passwordForm.password)"\
                               @input="passwordValidator.touched.password && passwordValidator.validateField(\'password\', passwordForm.password)"\
                               placeholder="请输入新密码">\
                        <div class="form-error" v-show="passwordValidator.hasError(\'password\')" v-cloak>\
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>\
                            <span>{{ passwordValidator.getError(\'password\') }}</span>\
                        </div>\
                        <div class="form-hint">密码至少需要 6 个字符</div>\
                    </div>\
                </form>\
            </div>\
            <div class="modal-footer">\
                <button type="button" class="btn btn-secondary" @click="showPassword = false">取消</button>\
                <button type="button" class="btn btn-primary" :disabled="saving || !passwordValidator.isValid()" @click="changePassword">\
                    <span v-show="!saving">修改密码</span>\
                    <span v-show="saving">修改中...</span>\
                </button>\
            </div>\
        </div>\
    </div>\
</div>',
  };
})();
