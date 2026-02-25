/**
 * headerActions store
 * 页面通过 shallowRef 注册 header 操作按钮组件，AppHeader 渲染。
 * 替代 Teleport to="#header-actions" 方案，避免 DOM 生命周期冲突。
 */
window.VueStores = window.VueStores || {};

(function () {
  "use strict";
  window.VueStores.headerActions = Vue.shallowRef(null);
})();
