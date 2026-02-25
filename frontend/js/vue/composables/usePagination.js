/**
 * usePagination 组合式函数
 * 提供分页数据加载、页码导航、自动刷新等功能
 */
;(function () {
    'use strict';

    window.VueComposables = window.VueComposables || {};

    var ref = Vue.ref;
    var computed = Vue.computed;
    var watch = Vue.watch;

    /**
     * 创建分页管理器
     * @param {Function} fetchFn - 数据获取函数，签名: async (page, pageSize) => { items, total }
     * @param {Object} options - 配置选项
     * @param {number} options.pageSize - 每页条数，默认 20
     * @param {boolean} options.immediate - 是否立即加载第一页，默认 true
     * @returns {Object} 分页状态和方法
     */
    function usePagination(fetchFn, options) {
        if (options === undefined) options = {};

        var pageSize = ref(options.pageSize || 20);
        var page = ref(1);
        var total = ref(0);
        var items = ref([]);
        var loading = ref(false);

        // 总页数
        var totalPages = computed(function () {
            return Math.max(1, Math.ceil(total.value / pageSize.value));
        });

        // 页码范围（最多显示 5 个页码按钮）
        var pageRange = computed(function () {
            var tp = totalPages.value;
            var cp = page.value;
            var range = [];

            if (tp <= 7) {
                // 总页数较少，全部显示
                for (var i = 1; i <= tp; i++) range.push(i);
            } else {
                // 始终显示第一页
                range.push(1);

                var start = Math.max(2, cp - 1);
                var end = Math.min(tp - 1, cp + 1);

                // 调整范围确保至少显示 3 个中间页码
                if (cp <= 3) {
                    start = 2;
                    end = 4;
                } else if (cp >= tp - 2) {
                    start = tp - 3;
                    end = tp - 1;
                }

                // 左侧省略号
                if (start > 2) range.push('...');

                for (var j = start; j <= end; j++) range.push(j);

                // 右侧省略号
                if (end < tp - 1) range.push('...');

                // 始终显示最后一页
                range.push(tp);
            }

            return range;
        });

        /**
         * 加载指定页数据
         */
        async function loadPage() {
            loading.value = true;
            try {
                var result = await fetchFn(page.value, pageSize.value);
                if (result) {
                    items.value = result.items || [];
                    total.value = result.total || 0;
                }
            } catch (e) {
                console.error('[usePagination] 加载数据失败:', e);
                items.value = [];
            } finally {
                loading.value = false;
            }
        }

        /**
         * 跳转到指定页
         * @param {number} p - 目标页码
         */
        function goToPage(p) {
            if (p < 1 || p > totalPages.value || p === page.value) return;
            page.value = p;
        }

        /** 下一页 */
        function nextPage() {
            if (page.value < totalPages.value) {
                page.value++;
            }
        }

        /** 上一页 */
        function prevPage() {
            if (page.value > 1) {
                page.value--;
            }
        }

        /** 刷新当前页 */
        function refresh() {
            return loadPage();
        }

        // 监听页码变化自动加载
        watch(page, function () {
            loadPage();
        });

        // 是否立即加载
        if (options.immediate !== false) {
            loadPage();
        }

        return {
            page: page,
            pageSize: pageSize,
            total: total,
            items: items,
            loading: loading,
            totalPages: totalPages,
            pageRange: pageRange,
            goToPage: goToPage,
            nextPage: nextPage,
            prevPage: prevPage,
            refresh: refresh
        };
    }

    window.VueComposables.usePagination = usePagination;
})();
