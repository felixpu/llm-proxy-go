/**
 * AppSidebar - 侧边栏导航组件
 * 从 Alpine.js base.html 中的 sidebar 区域移植
 * 支持分组导航、折叠/展开、移动端适配、主题切换
 */
window.VueComponents = window.VueComponents || {};

(function () {
  const { inject, ref, computed } = Vue;

  window.VueComponents.AppSidebar = {
    name: "AppSidebar",
    props: {
      currentRoute: { type: String, default: "" },
    },
    setup(props) {
      const sidebarStore = inject("sidebarStore");
      const themeStore = inject("themeStore");
      const authStore = inject("authStore");

      // 各分组的展开状态
      const groupOpen = ref({
        overview: true,
        config: true,
        monitor: true,
        user: true,
      });

      const toggleGroup = (group) => {
        groupOpen.value[group] = !groupOpen.value[group];
      };

      // 判断导航项是否激活
      const isActive = (route) => {
        return props.currentRoute === route;
      };

      const isAdmin = computed(() => authStore.isAdmin);
      const username = computed(() => authStore.username);
      const role = computed(() => authStore.role);
      const collapsed = computed(() => sidebarStore.collapsed);
      const mobileOpen = computed(() => sidebarStore.mobileOpen);

      const handleLogout = () => {
        authStore.logout();
      };

      return {
        sidebarStore,
        themeStore,
        authStore,
        groupOpen,
        toggleGroup,
        isActive,
        isAdmin,
        username,
        role,
        collapsed,
        mobileOpen,
        handleLogout,
      };
    },
    template: `
      <nav class="sidebar" :class="{ 'collapsed': collapsed, 'open': mobileOpen }">
        <!-- Sidebar header: two-row layout -->
        <div class="sidebar-header">
          <div class="sidebar-header-row1">
            <img src="/logo.png" alt="LLM Proxy Logo">
            <div class="logo-text">
              <h1>LLM Proxy</h1>
              <span class="version">v0.1.0</span>
            </div>
          </div>
          <div class="sidebar-header-row2">
            <div class="header-user" v-if="username">
              <span class="username">{{ username }}</span>
              <span class="role-badge" :class="'role-' + role">{{ role }}</span>
            </div>
            <div class="header-actions-bar">
              <button class="header-icon-btn theme-toggle-sidebar" @click="themeStore.toggle()" :title="themeStore.isDark() ? '切换到明亮模式' : '切换到暗黑模式'" aria-label="切换主题">
                <svg class="icon-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
                <svg class="icon-sun" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
              </button>
              <a class="header-icon-btn" href="#/api-docs" :class="{ 'active': isActive('api-docs') }" title="API 文档">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>
              </a>
              <a class="header-icon-btn logout-btn" href="javascript:void(0)" title="退出登录" @click.prevent="handleLogout">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
              </a>
            </div>
          </div>
        </div>

        <!-- Navigation menu -->
        <ul class="nav-menu">
          <!-- 概览分组 -->
          <li class="nav-group">
            <button class="nav-group-header" @click="toggleGroup('overview')" type="button">
              <span class="nav-group-title">概览</span>
              <svg class="nav-group-arrow" :class="{ 'rotated': groupOpen.overview }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
            </button>
            <ul class="nav-group-items" v-show="groupOpen.overview">
              <li><a href="#/" :class="{ 'active': isActive('') }" title="仪表盘">
                <svg class="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>
                <span class="nav-text">仪表盘</span>
              </a></li>
            </ul>
          </li>

          <!-- 配置分组 (仅管理员) -->
          <li class="nav-group" v-if="isAdmin">
            <button class="nav-group-header" @click="toggleGroup('config')" type="button">
              <span class="nav-group-title">配置</span>
              <svg class="nav-group-arrow" :class="{ 'rotated': groupOpen.config }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
            </button>
            <ul class="nav-group-items" v-show="groupOpen.config">
              <li><a href="#/model-provider" :class="{ 'active': isActive('model-provider') }" title="模型管理">
                <svg class="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/></svg>
                <span class="nav-text">模型管理</span>
              </a></li>
              <li><a href="#/routing" :class="{ 'active': isActive('routing') }" title="路由规则">
                <svg class="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M6 21V9a9 9 0 0 0 9 9"/></svg>
                <span class="nav-text">路由规则</span>
              </a></li>
              <li><a href="#/settings" :class="{ 'active': isActive('settings') }" title="系统设置">
                <svg class="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>
                <span class="nav-text">系统设置</span>
              </a></li>
            </ul>
          </li>
          <!-- 监控分组 (仅管理员) -->
          <li class="nav-group" v-if="isAdmin">
            <button class="nav-group-header" @click="toggleGroup('monitor')" type="button">
              <span class="nav-group-title">监控</span>
              <svg class="nav-group-arrow" :class="{ 'rotated': groupOpen.monitor }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
            </button>
            <ul class="nav-group-items" v-show="groupOpen.monitor">
              <li><a href="#/cache-monitor" :class="{ 'active': isActive('cache-monitor') }" title="缓存监控">
                <svg class="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/></svg>
                <span class="nav-text">缓存监控</span>
              </a></li>
              <li><a href="#/logs" :class="{ 'active': isActive('logs') }" title="请求日志">
                <svg class="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
                <span class="nav-text">请求日志</span>
              </a></li>
              <li><a href="#/system-logs" :class="{ 'active': isActive('system-logs') }" title="系统日志">
                <svg class="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="12" y1="18" x2="12" y2="12"/><line x1="9" y1="15" x2="15" y2="15"/></svg>
                <span class="nav-text">系统日志</span>
              </a></li>
            </ul>
          </li>

          <!-- 用户分组 -->
          <li class="nav-group">
            <button class="nav-group-header" @click="toggleGroup('user')" type="button">
              <span class="nav-group-title">用户</span>
              <svg class="nav-group-arrow" :class="{ 'rotated': groupOpen.user }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
            </button>
            <ul class="nav-group-items" v-show="groupOpen.user">
              <li v-if="isAdmin"><a href="#/users" :class="{ 'active': isActive('users') }" title="用户管理">
                <svg class="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
                <span class="nav-text">用户管理</span>
              </a></li>
              <li><a href="#/api-keys" :class="{ 'active': isActive('api-keys') }" title="API Keys">
                <svg class="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>
                <span class="nav-text">API Keys</span>
              </a></li>
              <li><a href="#/help" :class="{ 'active': isActive('help') }" title="使用帮助">
                <svg class="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
                <span class="nav-text">使用帮助</span>
              </a></li>
            </ul>
          </li>
        </ul>

        <!-- 收缩/展开按钮 -->
        <button class="sidebar-toggle" @click="sidebarStore.toggle()" :title="collapsed ? '展开侧边栏' : '收缩侧边栏'">
          <svg class="toggle-icon" :class="{ 'rotated': collapsed }" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="15 18 9 12 15 6"/>
          </svg>
        </button>

      </nav>
    `,
  };
})();
