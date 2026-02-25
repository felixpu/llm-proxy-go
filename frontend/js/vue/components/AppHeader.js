/**
 * AppHeader - 页面头部组件
 * 从 Alpine.js base.html 中的 header 区域移植
 */
window.VueComponents = window.VueComponents || {};

(function () {
  const { inject, computed } = Vue;

  // 路由 → 页面标题映射
  const ROUTE_TITLES = {
    "": "仪表盘",
    "model-provider": "模型管理",
    routing: "路由规则",
    settings: "系统设置",
    "cache-monitor": "缓存监控",
    logs: "请求日志",
    "system-logs": "系统日志",
    users: "用户管理",
    "api-keys": "API Keys",
    help: "使用帮助",
  };

  window.VueComponents.AppHeader = {
    name: "AppHeader",
    props: {
      title: { type: String, default: "" },
      currentRoute: { type: String, default: "" },
    },
    setup(props) {
      const sidebarStore = inject("sidebarStore");
      const headerActions = inject("headerActionsStore");

      // 页面标题：优先使用 prop，否则从路由映射获取
      const pageTitle = computed(() => {
        if (props.title) return props.title;
        return ROUTE_TITLES[props.currentRoute] || "仪表盘";
      });

      // 面包屑：非首页时显示当前页面名称
      const showBreadcrumbCurrent = computed(() => {
        return props.currentRoute !== "";
      });

      const breadcrumbCurrent = computed(() => {
        return ROUTE_TITLES[props.currentRoute] || "";
      });

      return {
        sidebarStore,
        pageTitle,
        showBreadcrumbCurrent,
        breadcrumbCurrent,
        headerActions,
      };
    },
    template: `
      <header class="header">
        <div class="header-left">
          <button class="mobile-menu-btn" @click="sidebarStore.toggleMobile()" aria-label="打开菜单">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="3" y1="6" x2="21" y2="6"/>
              <line x1="3" y1="12" x2="21" y2="12"/>
              <line x1="3" y1="18" x2="21" y2="18"/>
            </svg>
          </button>
          <nav class="breadcrumb" aria-label="面包屑导航">
            <a href="#/">首页</a>
            <template v-if="showBreadcrumbCurrent">
              <span class="breadcrumb-separator">/</span>
              <span class="breadcrumb-current">{{ breadcrumbCurrent }}</span>
            </template>
          </nav>
        </div>
        <div class="header-row">
          <h2>{{ pageTitle }}</h2>
          <div class="header-actions" style="display: flex; align-items: center; gap: 12px;">
            <component v-if="headerActions" :is="headerActions" />
          </div>
        </div>
      </header>
    `,
  };
})();
