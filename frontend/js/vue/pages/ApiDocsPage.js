/**
 * ApiDocsPage - API 文档页面
 * 嵌入 Swagger UI 展示 OpenAPI 规范
 */
window.VuePages = window.VuePages || {};

(function () {
  const { inject, ref, onMounted, onUnmounted, watch, nextTick } = Vue;

  window.VuePages.ApiDocsPage = {
    name: "ApiDocsPage",
    setup() {
      const themeStore = inject("themeStore");
      const containerRef = ref(null);
      let swaggerUI = null;

      const initSwaggerUI = () => {
        if (!containerRef.value || !window.SwaggerUIBundle) return;

        // 清理旧实例
        if (swaggerUI) {
          containerRef.value.innerHTML = "";
          swaggerUI = null;
        }

        swaggerUI = window.SwaggerUIBundle({
          url: "/api/docs/openapi.yaml",
          domNode: containerRef.value,
          deepLinking: true,
          presets: [
            window.SwaggerUIBundle.presets.apis,
            window.SwaggerUIStandalonePreset,
          ],
          plugins: [window.SwaggerUIBundle.plugins.DownloadUrl],
          layout: "StandaloneLayout",
          defaultModelsExpandDepth: 1,
          defaultModelExpandDepth: 1,
          docExpansion: "list",
          filter: true,
          showExtensions: true,
          tryItOutEnabled: true,
        });
      };

      const applyTheme = () => {
        const isDark = themeStore.isDark();
        const el = containerRef.value;
        if (!el) return;
        if (isDark) {
          el.classList.add("swagger-ui-dark");
        } else {
          el.classList.remove("swagger-ui-dark");
        }
      };

      onMounted(() => {
        nextTick(() => {
          applyTheme();
          initSwaggerUI();
        });

        // 监听主题变化
        window.addEventListener("theme-changed", applyTheme);
      });

      onUnmounted(() => {
        window.removeEventListener("theme-changed", applyTheme);
        swaggerUI = null;
      });

      return { containerRef, themeStore };
    },
    template: `
      <div class="api-docs-page">
        <div class="page-header">
          <h2>API 文档</h2>
          <p class="page-desc">交互式 API 文档，支持在线调试</p>
        </div>
        <div ref="containerRef" class="swagger-container" :class="{ 'swagger-ui-dark': themeStore.isDark() }"></div>
      </div>
    `,
  };
})();
