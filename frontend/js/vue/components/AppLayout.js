/**
 * AppLayout - 主布局组件
 * 组合 AppSidebar + AppHeader + 内容区域 + 全局组件（Toast/Modal/Confirm）
 */
window.VueComponents = window.VueComponents || {};

(function () {
  const { inject, computed } = Vue;

  window.VueComponents.AppLayout = {
    name: "AppLayout",
    components: {
      AppSidebar: window.VueComponents.AppSidebar,
      AppHeader: window.VueComponents.AppHeader,
      AppToast: window.VueComponents.AppToast,
      AppModal: window.VueComponents.AppModal,
      ConfirmDialog: window.VueComponents.ConfirmDialog,
    },
    props: {
      currentRoute: { type: String, default: "" },
      title: { type: String, default: "" },
    },
    setup(props) {
      const sidebarStore = inject("sidebarStore");

      const collapsed = computed(() => sidebarStore.collapsed);
      const mobileOpen = computed(() => sidebarStore.mobileOpen);

      const handleOverlayClick = () => {
        sidebarStore.closeMobile();
      };

      return { collapsed, mobileOpen, handleOverlayClick };
    },
    template: `
      <div>
        <!-- 移动端侧边栏遮罩 -->
        <div class="sidebar-overlay"
             :class="{ 'active': mobileOpen }"
             @click="handleOverlayClick"></div>

        <div class="layout">
          <!-- 侧边栏 -->
          <AppSidebar :current-route="currentRoute" />

          <!-- 主内容区 -->
          <main class="content" :class="{ 'sidebar-collapsed': collapsed }">
            <AppHeader :current-route="currentRoute" :title="title" />
            <div class="main-content">
              <slot></slot>
            </div>
          </main>
        </div>

        <!-- 全局组件 -->
        <AppToast />
        <AppModal>
          <slot name="modal"></slot>
        </AppModal>
        <ConfirmDialog />
      </div>
    `,
  };
})();
