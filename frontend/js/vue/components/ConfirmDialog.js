/**
 * ConfirmDialog - 确认对话框组件
 * 从 Alpine.js base.html 中的 confirm-dialog 区域移植
 */
window.VueComponents = window.VueComponents || {};

(function () {
  const { inject, computed, onMounted, onUnmounted } = Vue;

  window.VueComponents.ConfirmDialog = {
    name: "ConfirmDialog",
    setup() {
      const confirmStore = inject("confirmStore");

      const open = computed(() => confirmStore.open);
      const type = computed(() => confirmStore.type);
      const title = computed(() => confirmStore.title);
      const message = computed(() => confirmStore.message);
      const detail = computed(() => confirmStore.detail);
      const cancelText = computed(() => confirmStore.cancelText);
      const confirmText = computed(() => confirmStore.confirmText);

      // 确认按钮样式：danger 用红色，其他用主色
      const confirmBtnClass = computed(() =>
        type.value === "danger" ? "btn btn-danger" : "btn btn-primary",
      );

      // Escape 键取消
      const handleKeydown = (e) => {
        if (e.key === "Escape" && open.value) {
          confirmStore.cancel();
        }
      };

      onMounted(() => {
        document.addEventListener("keydown", handleKeydown);
      });

      onUnmounted(() => {
        document.removeEventListener("keydown", handleKeydown);
      });

      // 点击遮罩取消
      const handleOverlayClick = (e) => {
        if (e.target === e.currentTarget) {
          confirmStore.cancel();
        }
      };

      return {
        open,
        type,
        title,
        message,
        detail,
        cancelText,
        confirmText,
        confirmBtnClass,
        confirmStore,
        handleOverlayClick,
      };
    },
    template: `
      <Transition
        enter-active-class="transition ease-out duration-200"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition ease-in duration-150"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <div v-if="open" class="confirm-overlay" @click="handleOverlayClick">
          <div class="confirm-dialog" :class="'confirm-' + type">
            <div class="confirm-icon">
              <!-- danger 图标 -->
              <svg v-if="type === 'danger'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/>
              </svg>
              <!-- warning 图标 -->
              <svg v-else-if="type === 'warning'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
                <line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>
              </svg>
              <!-- info 图标 -->
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/>
              </svg>
            </div>
            <h3 class="confirm-title">{{ title }}</h3>
            <p class="confirm-message">{{ message }}</p>
            <p v-if="detail" class="confirm-detail">{{ detail }}</p>
            <div class="confirm-actions">
              <button type="button" class="btn btn-secondary" @click="confirmStore.cancel()">
                {{ cancelText }}
              </button>
              <button type="button" :class="confirmBtnClass" @click="confirmStore.confirm()">
                {{ confirmText }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    `,
  };
})();
