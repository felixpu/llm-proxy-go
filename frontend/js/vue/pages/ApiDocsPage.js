/**
 * ApiDocsPage - API 文档页面
 * Swagger UI resources are loaded on demand (~1.6MB saved on other pages).
 */
window.VuePages = window.VuePages || {};

(function () {
  const { inject, ref, onMounted, onUnmounted, nextTick } = Vue;

  // Track loading state across navigations so we only inject tags once.
  let _assetsLoaded = false;
  let _assetsPromise = null;

  /**
   * Dynamically inject Swagger UI CSS + JS and resolve when ready.
   */
  function loadSwaggerAssets() {
    if (_assetsLoaded && window.SwaggerUIBundle) {
      return Promise.resolve();
    }
    if (_assetsPromise) return _assetsPromise;

    _assetsPromise = new Promise(function (resolve, reject) {
      // CSS
      if (!document.querySelector('link[href*="swagger-ui.css"]')) {
        var link = document.createElement("link");
        link.rel = "stylesheet";
        link.href = "/vendor/swagger-ui/swagger-ui.css";
        document.head.appendChild(link);
      }

      // JS — bundle first, then preset
      var scripts = [
        "/vendor/swagger-ui/swagger-ui-bundle.js",
        "/vendor/swagger-ui/swagger-ui-standalone-preset.js",
      ];
      var idx = 0;

      function next() {
        if (idx >= scripts.length) {
          _assetsLoaded = true;
          resolve();
          return;
        }
        var src = scripts[idx++];
        if (document.querySelector('script[src="' + src + '"]')) {
          next();
          return;
        }
        var s = document.createElement("script");
        s.src = src;
        s.onload = next;
        s.onerror = function () {
          reject(new Error("Failed to load " + src));
        };
        document.body.appendChild(s);
      }

      next();
    });
    return _assetsPromise;
  }

  window.VuePages.ApiDocsPage = {
    name: "ApiDocsPage",
    setup: function () {
      var themeStore = inject("themeStore");
      var containerRef = ref(null);
      var assetsReady = ref(false);
      var loadError = ref("");
      var swaggerUI = null;

      function initSwaggerUI() {
        if (!containerRef.value || !window.SwaggerUIBundle) return;
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
      }

      function applyTheme() {
        var isDark = themeStore.isDark();
        var el = containerRef.value;
        if (!el) return;
        if (isDark) {
          el.classList.add("swagger-ui-dark");
        } else {
          el.classList.remove("swagger-ui-dark");
        }
      }

      onMounted(function () {
        loadSwaggerAssets()
          .then(function () {
            assetsReady.value = true;
            nextTick(function () {
              applyTheme();
              initSwaggerUI();
            });
          })
          .catch(function (err) {
            loadError.value = "加载 Swagger UI 失败: " + err.message;
          });
        window.addEventListener("theme-changed", applyTheme);
      });

      onUnmounted(function () {
        window.removeEventListener("theme-changed", applyTheme);
        swaggerUI = null;
      });

      return { containerRef, themeStore, assetsReady, loadError };
    },
    template: [
      '<div class="api-docs-page">',
      '  <div class="page-header">',
      "    <h2>API 文档</h2>",
      '    <p class="page-desc">交互式 API 文档，支持在线调试</p>',
      "  </div>",
      '  <div v-if="loadError" class="alert alert-error">{{ loadError }}</div>',
      '  <div v-else-if="!assetsReady" class="loading-placeholder" style="text-align:center;padding:3rem;">',
      '    <span class="spinner"></span> 加载 API 文档组件…',
      "  </div>",
      '  <div v-show="assetsReady" ref="containerRef" class="swagger-container"',
      "       :class=\"{ 'swagger-ui-dark': themeStore.isDark() }\"></div>",
      "</div>",
    ].join("\n"),
  };
})();
