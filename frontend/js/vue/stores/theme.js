/**
 * Theme Store - 主题状态管理
 * 支持亮色/暗色/跟随系统三种模式，持久化到 localStorage
 */
;(function () {
    'use strict';

    window.VueStores = window.VueStores || {};

    var reactive = Vue.reactive;

    var theme = reactive({
        // 当前主题模式: 'light' | 'dark' | 'system'
        mode: localStorage.getItem('theme') || 'system',

        /**
         * 切换主题（暗 → 亮，亮/系统 → 暗）
         */
        toggle: function () {
            if (this.isDark()) {
                this.mode = 'light';
            } else {
                this.mode = 'dark';
            }
            localStorage.setItem('theme', this.mode);
            this.apply();
        },

        /**
         * 设置指定主题模式
         * @param {string} mode - 'light' | 'dark' | 'system'
         */
        setMode: function (mode) {
            this.mode = mode;
            localStorage.setItem('theme', mode);
            this.apply();
        },

        /**
         * 将当前主题应用到 DOM
         */
        apply: function () {
            var isDark = this.isDark();
            document.documentElement.classList.toggle('dark', isDark);
        },

        /**
         * 判断当前是否为暗色模式
         * @returns {boolean}
         */
        isDark: function () {
            if (this.mode === 'dark') return true;
            if (this.mode === 'light') return false;
            // system 模式：跟随系统偏好
            return window.matchMedia('(prefers-color-scheme: dark)').matches;
        },

        /**
         * 获取当前主题对应的图标名称
         * @returns {string} 'sun' 或 'moon'
         */
        getIcon: function () {
            return this.isDark() ? 'sun' : 'moon';
        }
    });

    // 初始应用主题
    theme.apply();

    // 监听系统主题变化，system 模式下自动切换
    window.matchMedia('(prefers-color-scheme: dark)')
        .addEventListener('change', function () {
            if (theme.mode === 'system') {
                theme.apply();
            }
        });

    window.VueStores.theme = theme;
})();
