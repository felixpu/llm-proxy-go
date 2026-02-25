/**
 * Modal Store - 全局模态框状态管理
 * 支持自定义标题、大小、底部按钮、内容组件
 */
;(function () {
    'use strict';

    window.VueStores = window.VueStores || {};

    var reactive = Vue.reactive;

    var modal = reactive({
        // 是否打开
        open: false,
        // 标题
        title: '',
        // 大小: small | medium | large
        size: 'medium',
        // 是否显示底部按钮
        showFooter: false,
        // 取消按钮文本
        cancelText: '取消',
        // 确认按钮文本
        confirmText: '确定',
        // 确认回调
        onConfirm: null,
        // 内容组件名称（用于动态组件渲染）
        contentComponent: null,
        // 传递给内容组件的 props
        contentProps: null,

        /**
         * 显示模态框
         * @param {Object|string} options - 配置选项或标题字符串
         */
        show: function (options) {
            if (options === undefined) options = {};

            // 兼容旧调用方式: show('标题', 'large')
            if (typeof options === 'string') {
                this.title = options;
                this.size = arguments[1] || 'medium';
                this.showFooter = false;
                this.onConfirm = null;
                this.contentComponent = null;
                this.contentProps = null;
            } else {
                this.title = options.title || '';
                this.size = options.size || 'medium';
                this.showFooter = options.showFooter || false;
                this.cancelText = options.cancelText || '取消';
                this.confirmText = options.confirmText || '确定';
                this.onConfirm = options.onConfirm || null;
                this.contentComponent = options.contentComponent || null;
                this.contentProps = options.contentProps || null;
            }
            this.open = true;
        },

        /**
         * 关闭模态框
         */
        close: function () {
            this.open = false;
            this.onConfirm = null;
            this.contentComponent = null;
            this.contentProps = null;
        }
    });

    window.VueStores.modal = modal;
})();
