/**
 * Sidebar Store - 侧边栏状态管理
 * 支持收缩/展开、移动端菜单、状态持久化到 localStorage
 */
;(function () {
    'use strict';

    window.VueStores = window.VueStores || {};

    var reactive = Vue.reactive;

    var sidebar = reactive({
        // 是否收缩
        collapsed: localStorage.getItem('sidebarCollapsed') === 'true',
        // 移动端菜单是否打开
        mobileOpen: false,

        /**
         * 切换收缩/展开状态
         */
        toggle: function () {
            this.collapsed = !this.collapsed;
            localStorage.setItem('sidebarCollapsed', this.collapsed);
        },

        /**
         * 展开侧边栏
         */
        expand: function () {
            this.collapsed = false;
            localStorage.setItem('sidebarCollapsed', 'false');
        },

        /**
         * 收缩侧边栏
         */
        collapse: function () {
            this.collapsed = true;
            localStorage.setItem('sidebarCollapsed', 'true');
        },

        /**
         * 切换移动端菜单
         */
        toggleMobile: function () {
            this.mobileOpen = !this.mobileOpen;
        },

        /**
         * 关闭移动端菜单
         */
        closeMobile: function () {
            this.mobileOpen = false;
        }
    });

    window.VueStores.sidebar = sidebar;
})();
