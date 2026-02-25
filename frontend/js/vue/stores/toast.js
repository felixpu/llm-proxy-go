/**
 * Toast Store - 全局 Toast 通知管理
 * 支持 success/error/warning/info 四种类型，自动消失
 */
;(function () {
    'use strict';

    window.VueStores = window.VueStores || {};

    var reactive = Vue.reactive;

    var toast = reactive({
        // 消息内容
        message: '',
        // 消息类型: info | success | warning | error
        type: 'info',
        // 是否可见
        visible: false,
        // 定时器 ID
        timeout: null,

        /**
         * 显示 Toast 消息
         * @param {string} message - 消息内容
         * @param {string} type - 消息类型
         * @param {number} duration - 显示时长（毫秒）
         */
        show: function (message, type, duration) {
            if (type === undefined) type = 'info';
            if (duration === undefined) duration = 3000;

            this.message = message;
            this.type = type;
            this.visible = true;

            // 清除之前的定时器
            if (this.timeout) {
                clearTimeout(this.timeout);
            }

            var self = this;
            this.timeout = setTimeout(function () {
                self.visible = false;
            }, duration);
        },

        /** 成功提示 */
        success: function (message, duration) {
            this.show(message, 'success', duration || 3000);
        },

        /** 错误提示 */
        error: function (message, duration) {
            this.show(message, 'error', duration || 4000);
        },

        /** 警告提示 */
        warning: function (message, duration) {
            this.show(message, 'warning', duration || 3500);
        },

        /** 信息提示 */
        info: function (message, duration) {
            this.show(message, 'info', duration || 3000);
        },

        /** 手动隐藏 */
        hide: function () {
            this.visible = false;
            if (this.timeout) {
                clearTimeout(this.timeout);
                this.timeout = null;
            }
        }
    });

    window.VueStores.toast = toast;
})();
