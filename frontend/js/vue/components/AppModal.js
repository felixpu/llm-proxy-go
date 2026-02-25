/**
 * AppModal - 通用模态框组件
 * 从 Alpine.js base.html 中的 modal 区域移植
 */
window.VueComponents = window.VueComponents || {};

(function () {
  const { inject, computed, onMounted, onUnmounted } = Vue;

  window.VueComponents.AppModal = {
    name: "AppModal",
    setup() {
      const modalStore = inject("modalStore");

      const open = computed(() => modalStore.open);
      const title = computed(() => modalStore.title);
      const size = computed(() => modalStore.size);
      const showFooter = computed(() => modalStore.showFooter);
      const cancelText = computed(() => modalStore.cancelText || "取消");
      const confirmText = computed(() => modalStore.confirmText || "确定");

      // Escape 键关闭
      const handleKeydown = (e) => {
        if (e.key === "Escape" && open.value) {
          modalStore.close();
        }
      };

      onMounted(() => {
        document.addEventListener("keydown", handleKeydown);
      });

      onUnmounted(() => {
        document.removeEventListener("keydown", handleKeydown);
      });

      const handleOverlayClick = (e) => {
        // 点击遮罩层关闭（不是内容区域）
        if (e.target === e.currentTarget) {
          modalStore.close();
        }
      };

      const handleConfirm = () => {
        if (modalStore.onConfirm) {
          modalStore.onConfirm();
        }
      };

      return {
        open,
        title,
        size,
        showFooter,
        cancelText,
        confirmText,
        modalStore,
        handleOverlayClick,
        handleConfirm,
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
        <div v-if="open" class="modal" @click="handleOverlayClick">
          <div class="modal-content" :class="'modal-' + size">
            <div class="modal-header">
              <h3>{{ title }}</h3>
              <button class="modal-close" @click="modalStore.close()">&times;</button>
            </div>
            <div class="modal-body">
              <slot></slot>
            </div>
            <div v-if="showFooter" class="modal-footer">
              <button type="button" class="btn btn-secondary" @click="modalStore.close()">
                {{ cancelText }}
              </button>
              <button type="button" class="btn btn-primary" @click="handleConfirm">
                {{ confirmText }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    `,
  };
})();
