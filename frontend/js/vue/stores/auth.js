/**
 * Auth Store - 用户认证状态管理
 * 管理当前登录用户信息、角色判断、登出
 */
(function () {
  "use strict";

  window.VueStores = window.VueStores || {};

  var reactive = Vue.reactive;

  var auth = reactive({
    // 当前用户对象
    user: null,
    // 是否为管理员
    isAdmin: false,
    // 用户名
    username: "",
    // 用户角色
    role: "",

    /**
     * 获取当前用户信息
     * 调用 GET /api/auth/me 接口
     */
    fetchUser: async function () {
      try {
        var response = await window.VueApi.get("/api/auth/me");
        if (!response.ok) {
          throw new Error("获取用户信息失败");
        }
        var data = await response.json();
        this.user = data;
        this.username = data.username || "";
        this.role = data.role || "";
        this.isAdmin = data.role === "admin";
      } catch (e) {
        // 401 已在 VueApi 中处理跳转，这里清空状态
        this.user = null;
        this.username = "";
        this.role = "";
        this.isAdmin = false;
      }
    },

    /**
     * 登出当前用户
     * 调用 POST /api/auth/logout 后跳转登录页
     */
    logout: async function () {
      try {
        await window.VueApi.post("/api/auth/logout");
      } catch (e) {
        // 忽略登出请求错误，仍然跳转
      }
      this.user = null;
      this.username = "";
      this.role = "";
      this.isAdmin = false;
      window.location.hash = "#/login";
    },
  });

  window.VueStores.auth = auth;
})();
