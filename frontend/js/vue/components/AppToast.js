/**
 * AppToast - 全局提示消息组件
 * 从 Alpine.js base.html 中的 toast 区域移植
 */
window.VueComponents = window.VueComponents || {};

(function () {
  const { inject, computed } = Vue;

  window.VueComponents.AppToast = {
    name: "AppToast",
    setup() {
      const toastStore = inject("toastStore");

      const visible = computed(() => toastStore.visible);
      const message = computed(() => toastStore.message);
      const type = computed(() => toastStore.type);

      return { visible, message, type };
    },
    template: `
      <Transition
        enter-active-class="transition ease-out duration-300"
        enter-from-class="opacity-0 translate-y-2"
        enter-to-class="opacity-100 translate-y-0"
        leave-active-class="transition ease-in duration-200"
        leave-from-class="opacity-100 translate-y-0"
        leave-to-class="opacity-0 translate-y-2"
      >
        <div v-if="visible" class="toast" :class="type">
          {{ message }}
        </div>
      </Transition>
    `,
  };
})();
