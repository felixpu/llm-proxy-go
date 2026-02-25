/**
 * LoginPage - 登录页面组件
 * 独立布局（不使用 AppLayout），全屏居中登录表单
 */
window.VuePages = window.VuePages || {};

(function () {
  "use strict";

  var ref = Vue.ref;

  window.VuePages.LoginPage = {
    name: "LoginPage",
    setup: function () {
      var username = ref("admin");
      var password = ref("");
      var error = ref("");
      var loading = ref(false);

      async function handleLogin() {
        if (!username.value || !password.value) {
          error.value = "请输入用户名和密码";
          return;
        }

        error.value = "";
        loading.value = true;

        try {
          var csrfToken = window.VueApi.getCSRFToken();
          var headers = { "Content-Type": "application/json" };
          if (csrfToken) {
            headers["X-CSRF-Token"] = csrfToken;
          }

          var response = await fetch("/api/auth/login", {
            method: "POST",
            headers: headers,
            credentials: "same-origin",
            body: JSON.stringify({
              username: username.value,
              password: password.value,
            }),
          });

          var data = await response.json();

          if (response.ok && data.success) {
            // 登录成功，跳转首页并刷新用户信息
            window.location.hash = "#/";
            await window.VueStores.auth.fetchUser();
          } else if (response.status === 403) {
            // CSRF 验证失败，刷新页面重新获取 cookie
            error.value = "安全验证失败，正在刷新...";
            setTimeout(function () {
              window.location.reload();
            }, 1000);
          } else {
            error.value =
              data.detail || data.message || "登录失败，请检查用户名和密码";
          }
        } catch (e) {
          error.value = "连接失败: " + e.message;
        } finally {
          loading.value = false;
        }
      }

      return {
        username: username,
        password: password,
        error: error,
        loading: loading,
        handleLogin: handleLogin,
      };
    },
    template:
      '\
      <div class="login-page">\
        <div class="login-container">\
          <div class="login-header">\
            <h1>LLM Proxy</h1>\
            <p>请登录以继续</p>\
          </div>\
          <div class="login-error" v-if="error">{{ error }}</div>\
          <form class="login-form" @submit.prevent="handleLogin">\
            <div class="form-group">\
              <label>用户名</label>\
              <div class="input-wrapper">\
                <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                  <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"></path>\
                  <circle cx="12" cy="7" r="4"></circle>\
                </svg>\
                <input type="text" v-model="username" placeholder="请输入用户名"\
                       autofocus autocomplete="username" :disabled="loading">\
              </div>\
            </div>\
            <div class="form-group">\
              <label>密码</label>\
              <div class="input-wrapper">\
                <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">\
                  <rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect>\
                  <path d="M7 11V7a5 5 0 0 1 10 0v4"></path>\
                </svg>\
                <input type="password" v-model="password" placeholder="请输入密码"\
                       autocomplete="current-password" :disabled="loading">\
              </div>\
            </div>\
            <button type="submit" class="btn btn-primary" :disabled="loading">\
              <span v-if="!loading">登录</span>\
              <span v-else>登录中...</span>\
            </button>\
          </form>\
        </div>\
      </div>\
    ',
  };
})();
