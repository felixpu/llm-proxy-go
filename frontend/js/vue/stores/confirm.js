/**
 * Confirm Store - 全局确认对话框管理
 * 替代原生 confirm()，支持 Promise 异步等待用户选择
 */
;(function () {
    'use strict';

    window.VueStores = window.VueStores || {};

    var reactive = Vue.reactive;

    var confirm = reactive({
        // 是否打开
        open: false,
        // 标题
        title: '',
        // 主要消息
        message: '',
        // 详细说明
        detail: '',
        // 确认按钮文本
        confirmText: '确认',
        // 取消按钮文本
        cancelText: '取消',
        // 类型: info | warning | danger
        type: 'warning',
        // Promise resolve 回调（内部使用）
        _resolve: null,

        /**
         * 显示确认对话框
         * @param {Object} options - 配置选项
         * @returns {Promise<boolean>} 用户选择结果
         */
        show: function (options) {
            if (options === undefined) options = {};

            this.title = options.title || '确认操作';
            this.message = options.message || '确定要执行此操作吗？';
            this.detail = options.detail || '';
            this.confirmText = options.confirmText || '确认';
            this.cancelText = options.cancelText || '取消';
            this.type = options.type || 'warning';
            this.open = true;

            var self = this;
            return new Promise(function (resolve) {
                self._resolve = resolve;
            });
        },

        /**
         * 删除确认快捷方法
         * @param {string} itemName - 要删除的项目名称
         * @param {string} itemType - 项目类型（如：模型、用户）
         * @returns {Promise<boolean>}
         */
        delete: function (itemName, itemType) {
            if (itemType === undefined) itemType = '项目';
            return this.show({
                title: '删除' + itemType,
                message: '确定要删除 "' + itemName + '" 吗？',
                detail: '此操作不可撤销',
                confirmText: '确认删除',
                cancelText: '取消',
                type: 'danger'
            });
        },

        /**
         * 危险操作确认快捷方法
         * @param {string} action - 操作描述
         * @param {string} detail - 详细说明
         * @returns {Promise<boolean>}
         */
        danger: function (action, detail) {
            return this.show({
                title: '危险操作',
                message: action,
                detail: detail || '',
                confirmText: '确认执行',
                cancelText: '取消',
                type: 'danger'
            });
        },

        /**
         * 用户点击确认
         */
        confirm: function () {
            if (this._resolve) {
                this._resolve(true);
                this._resolve = null;
            }
            this.open = false;
        },

        /**
         * 用户点击取消
         */
        cancel: function () {
            if (this._resolve) {
                this._resolve(false);
                this._resolve = null;
            }
            this.open = false;
        }
    });

    window.VueStores.confirm = confirm;
})();
