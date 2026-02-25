/**
 * Vue 工具函数集合
 * 提供格式化、剪贴板、防抖等通用工具
 */
;(function () {
    'use strict';

    /**
     * 格式化日期时间
     * @param {string} isoString - ISO 8601 日期字符串
     * @returns {string} 格式化后的字符串，如 "2024-01-15 14:30:25"
     */
    function formatDateTime(isoString) {
        if (!isoString) return '';
        try {
            var d = new Date(isoString);
            if (isNaN(d.getTime())) return isoString;
            var year = d.getFullYear();
            var month = String(d.getMonth() + 1).padStart(2, '0');
            var day = String(d.getDate()).padStart(2, '0');
            var hours = String(d.getHours()).padStart(2, '0');
            var minutes = String(d.getMinutes()).padStart(2, '0');
            var seconds = String(d.getSeconds()).padStart(2, '0');
            return year + '-' + month + '-' + day + ' ' + hours + ':' + minutes + ':' + seconds;
        } catch (e) {
            return isoString;
        }
    }

    /**
     * 格式化运行时间
     * @param {number} seconds - 秒数
     * @returns {string} 如 "2d 5h 30m 10s"
     */
    function formatUptime(seconds) {
        if (seconds === null || seconds === undefined || isNaN(seconds)) return '';
        seconds = Math.floor(Number(seconds));
        if (seconds < 0) return '0s';

        var d = Math.floor(seconds / 86400);
        var h = Math.floor((seconds % 86400) / 3600);
        var m = Math.floor((seconds % 3600) / 60);
        var s = seconds % 60;

        var parts = [];
        if (d > 0) parts.push(d + 'd');
        if (h > 0) parts.push(h + 'h');
        if (m > 0) parts.push(m + 'm');
        parts.push(s + 's');
        return parts.join(' ');
    }

    /**
     * 格式化字节数
     * @param {number} bytes - 字节数
     * @returns {string} 如 "1.5 KB", "2.3 MB"
     */
    function formatBytes(bytes) {
        if (bytes === null || bytes === undefined || isNaN(bytes)) return '0 B';
        bytes = Number(bytes);
        if (bytes === 0) return '0 B';

        var units = ['B', 'KB', 'MB', 'GB', 'TB'];
        var i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(1024));
        if (i >= units.length) i = units.length - 1;
        if (i === 0) return bytes + ' B';
        return (bytes / Math.pow(1024, i)).toFixed(2) + ' ' + units[i];
    }

    /**
     * 复制文本到剪贴板
     * @param {string} text - 要复制的文本
     * @returns {Promise<void>}
     */
    function copyToClipboard(text) {
        // 优先使用现代 API
        if (navigator.clipboard && navigator.clipboard.writeText) {
            return navigator.clipboard.writeText(text);
        }
        // 降级方案：使用 textarea
        return new Promise(function (resolve, reject) {
            try {
                var textarea = document.createElement('textarea');
                textarea.value = text;
                textarea.style.position = 'fixed';
                textarea.style.opacity = '0';
                document.body.appendChild(textarea);
                textarea.select();
                document.execCommand('copy');
                document.body.removeChild(textarea);
                resolve();
            } catch (e) {
                reject(e);
            }
        });
    }

    /**
     * 基础 HTML 清理，防止 XSS
     * 允许安全标签：b, i, em, strong, a, br, p, ul, ol, li, code, pre, span
     * @param {string} html - 原始 HTML
     * @returns {string} 清理后的 HTML
     */
    function sanitizeHtml(html) {
        if (!html) return '';
        var allowedTags = ['b', 'i', 'em', 'strong', 'a', 'br', 'p', 'ul', 'ol', 'li', 'code', 'pre', 'span'];
        // 移除 script/style 标签及其内容
        var cleaned = html.replace(/<script[\s\S]*?<\/script>/gi, '');
        cleaned = cleaned.replace(/<style[\s\S]*?<\/style>/gi, '');
        // 移除事件属性
        cleaned = cleaned.replace(/\s+on\w+\s*=\s*["'][^"']*["']/gi, '');
        cleaned = cleaned.replace(/\s+on\w+\s*=\s*\S+/gi, '');
        // 移除 javascript: 协议
        cleaned = cleaned.replace(/href\s*=\s*["']javascript:[^"']*["']/gi, 'href="#"');
        // 移除不允许的标签，保留内容
        var tagPattern = /<\/?([a-zA-Z][a-zA-Z0-9]*)\b[^>]*>/g;
        cleaned = cleaned.replace(tagPattern, function (match, tagName) {
            if (allowedTags.indexOf(tagName.toLowerCase()) !== -1) {
                return match;
            }
            return '';
        });
        return cleaned;
    }

    /**
     * 防抖函数
     * @param {Function} fn - 要防抖的函数
     * @param {number} delay - 延迟毫秒数
     * @returns {Function}
     */
    function debounce(fn, delay) {
        var timer = null;
        return function () {
            var context = this;
            var args = arguments;
            if (timer) clearTimeout(timer);
            timer = setTimeout(function () {
                fn.apply(context, args);
            }, delay);
        };
    }

    /**
     * 格式化数字，添加千分位分隔符
     * @param {number} num - 数字
     * @returns {string} 如 "1,234,567"
     */
    function formatNumber(num) {
        if (num === null || num === undefined || isNaN(num)) return '0';
        return Number(num).toLocaleString();
    }

    // 导出工具函数
    window.VueUtils = {
        formatDateTime: formatDateTime,
        formatUptime: formatUptime,
        formatBytes: formatBytes,
        copyToClipboard: copyToClipboard,
        sanitizeHtml: sanitizeHtml,
        debounce: debounce,
        formatNumber: formatNumber
    };
})();
