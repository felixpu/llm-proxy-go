/**
 * Vue 3 应用入口
 * Hash 路由 + Store 注册 + 组件挂载
 */
(function () {
  "use strict";

  var createApp = Vue.createApp;
  var ref = Vue.ref;
  var computed = Vue.computed;
  var onMounted = Vue.onMounted;
  var onUnmounted = Vue.onUnmounted;

  // 路由表：hash → 页面组件名
  var routes = {
    "": "DashboardPage",
    login: "LoginPage",
    "model-provider": "ModelProviderPage",
    routing: "RoutingPage",
    settings: "SettingsPage",
    "cache-monitor": "CacheMonitorPage",
    logs: "LogsPage",
    "system-logs": "SystemLogsPage",
    users: "UsersPage",
    "api-keys": "ApiKeysPage",
    help: "HelpPage",
    "api-docs": "ApiDocsPage",
  };

  // 解析 hash 路由
  function parseRoute() {
    var hash = window.location.hash.replace(/^#\/?/, "");
    return hash || "";
  }

  // 根组件
  var App = {
    name: "App",
    setup: function () {
      var currentRoute = ref(parseRoute());
      var authReady = ref(false);

      var isLoginPage = computed(function () {
        return currentRoute.value === "login";
      });

      var currentComponent = computed(function () {
        return routes[currentRoute.value] || "DashboardPage";
      });

      // 监听 hash 变化
      function onHashChange() {
        currentRoute.value = parseRoute();
        // 移动端切换页面时关闭侧边栏
        window.VueStores.sidebar.closeMobile();
      }

      onMounted(async function () {
        window.addEventListener("hashchange", onHashChange);
        // 非登录页：先获取用户信息，未认证则跳转登录
        if (!isLoginPage.value) {
          await window.VueStores.auth.fetchUser();
          if (!window.VueStores.auth.user) {
            window.location.hash = "#/login";
          }
        }
        authReady.value = true;
      });

      onUnmounted(function () {
        window.removeEventListener("hashchange", onHashChange);
      });

      return {
        currentRoute: currentRoute,
        currentComponent: currentComponent,
        isLoginPage: isLoginPage,
        authReady: authReady,
      };
    },
    template:
      '\
      <LoginPage v-if="isLoginPage" />\
      <AppLayout v-else-if="authReady" :current-route="currentRoute">\
          <component :is="currentComponent" :key="currentRoute" />\
      </AppLayout>\
    ',
  };

  // 创建 Vue 应用
  var app = createApp(App);

  // 注册全局组件
  var components = window.VueComponents || {};
  Object.keys(components).forEach(function (name) {
    app.component(name, components[name]);
  });

  // 注册页面组件（带 fallback）
  var pages = window.VuePages || {};
  Object.keys(pages).forEach(function (name) {
    app.component(name, pages[name]);
  });

  // 注入 stores（provide/inject）
  var stores = window.VueStores || {};
  app.provide("authStore", stores.auth);
  app.provide("themeStore", stores.theme);
  app.provide("sidebarStore", stores.sidebar);
  app.provide("toastStore", stores.toast);
  app.provide("modalStore", stores.modal);
  app.provide("confirmStore", stores.confirm);
  app.provide("headerActionsStore", stores.headerActions);

  // 注入工具函数
  app.provide("api", window.VueApi);
  app.provide("utils", window.VueUtils);

  // 挂载应用
  app.mount("#app");
})();
