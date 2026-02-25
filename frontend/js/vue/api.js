/**
 * Vue API 客户端封装
 * 统一的 HTTP 请求处理，支持 CSRF、401/403 自动处理
 */
(function () {
  "use strict";

  /**
   * 从 cookie 中读取 CSRF token
   * @returns {string|null}
   */
  function getCSRFToken() {
    var cookies = document.cookie.split(";");
    for (var i = 0; i < cookies.length; i++) {
      var parts = cookies[i].trim().split("=");
      var name = parts.shift();
      if (name === "csrf_token") {
        return parts.join("=");
      }
    }
    return null;
  }

  /**
   * 发送 HTTP 请求
   * @param {string} url - 请求地址
   * @param {object} options - fetch 配置
   * @returns {Promise<Response>}
   */
  async function request(url, options) {
    if (options === undefined) options = {};

    var defaultHeaders = { "Content-Type": "application/json" };
    var merged = Object.assign({}, { credentials: "same-origin" }, options, {
      headers: Object.assign({}, defaultHeaders, options.headers || {}),
    });

    // 对变更类请求自动附加 CSRF token
    var method = (merged.method || "GET").toUpperCase();
    if (["POST", "PUT", "DELETE", "PATCH"].indexOf(method) !== -1) {
      var token = getCSRFToken();
      if (token) {
        merged.headers["X-CSRF-Token"] = token;
      }
    }

    var response = await fetch(url, merged);

    // 401 → 跳转登录页
    if (response.status === 401) {
      window.location.hash = "#/login";
      throw new Error("未授权，正在跳转登录页");
    }

    // 403 → 抛出权限错误
    if (response.status === 403) {
      // 尝试通知 toast store
      if (window.VueStores && window.VueStores.toast) {
        window.VueStores.toast.error("没有权限执行此操作");
      }
      throw new Error("没有权限");
    }

    return response;
  }

  // 导出 API 客户端
  window.VueApi = {
    request: request,
    getCSRFToken: getCSRFToken,

    /** GET 请求 */
    get: function (url) {
      return request(url, { method: "GET" });
    },

    /** POST 请求 */
    post: function (url, data) {
      return request(url, {
        method: "POST",
        body: JSON.stringify(data),
      });
    },

    /** PUT 请求 */
    put: function (url, data) {
      return request(url, {
        method: "PUT",
        body: JSON.stringify(data),
      });
    },

    /** PATCH 请求 */
    patch: function (url, data) {
      return request(url, {
        method: "PATCH",
        body: JSON.stringify(data),
      });
    },

    /** DELETE 请求 */
    delete: function (url) {
      return request(url, { method: "DELETE" });
    },
  };
})();
