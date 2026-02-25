/**
 * DataTable - 通用数据表格组件
 * 支持列配置、加载状态、空状态、自定义单元格渲染
 */
window.VueComponents = window.VueComponents || {};

(function () {
  const { computed } = Vue;

  window.VueComponents.DataTable = {
    name: 'DataTable',
    props: {
      columns: { type: Array, required: true },
      data: { type: Array, default: () => [] },
      loading: { type: Boolean, default: false },
      emptyText: { type: String, default: '暂无数据' },
      rowKey: { type: String, default: 'id' }
    },
    setup(props) {
      const isEmpty = computed(() => !props.loading && props.data.length === 0);

      // 获取单元格值
      const getCellValue = (row, col) => {
        if (col.render) return col.render(row);
        return row[col.key];
      };

      return { isEmpty, getCellValue };
    },
    template: `
      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th v-for="col in columns"
                  :key="col.key"
                  :style="col.width ? { width: col.width } : {}">
                {{ col.label }}
              </th>
              <th v-if="$slots.actions" style="width: 120px;">操作</th>
            </tr>
          </thead>
          <tbody>
            <!-- 加载状态 -->
            <tr v-if="loading">
              <td :colspan="columns.length + ($slots.actions ? 1 : 0)" style="text-align: center; padding: 40px;">
                <div class="loading-spinner">加载中...</div>
              </td>
            </tr>
            <!-- 空状态 -->
            <tr v-else-if="isEmpty">
              <td :colspan="columns.length + ($slots.actions ? 1 : 0)" style="text-align: center; padding: 40px; color: var(--text-secondary);">
                {{ emptyText }}
              </td>
            </tr>
            <!-- 数据行 -->
            <tr v-else v-for="row in data" :key="row[rowKey]">
              <td v-for="col in columns" :key="col.key">
                <slot :name="'cell-' + col.key" :row="row" :value="getCellValue(row, col)">
                  {{ getCellValue(row, col) }}
                </slot>
              </td>
              <td v-if="$slots.actions">
                <slot name="actions" :row="row"></slot>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    `
  };
})();
